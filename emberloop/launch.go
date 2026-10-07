package emberloop

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// launchFile is the complete launch emberloop types into a pane: the binary, its
// args including the full prompt, and the env. It lives in emberloop's state dir
// because herdr pane run types one shell line: newlines and quoting in the
// prompt would break, and the pane's shell may be fish, bash or zsh. Instead
// the pane gets one shell-agnostic line, `emberloop exec <launch-file>`, which
// reads the file and execs the harness with the prompt as a real argument.
// The brief and prompt files are still written, for the archive and
// debugging, but the agent is not told to read them.
type launchFile struct {
	Bin  string   `json:"bin"`
	Args []string `json:"args"`
	Env  []string `json:"env"`
}

// launchDir holds one launch file per session, beside the worker store.
func launchDir() string { return filepath.Join(filepath.Dir(statePath), "launches") }

// launchPath is where the launch of s is written: under emberloop's state dir,
// named for the session, so whoever owns the session's lifetime can delete
// it. The key follows the MCP config's (session id, then label, then role),
// so CleanupSession finds the same file the launch wrote.
func launchPath(s Session) string {
	key := s.ID
	if key == "" {
		key = s.Label
	}
	if key == "" {
		key = s.Role
	}
	if key == "" {
		key = "session"
	}
	key = strings.ReplaceAll(key, "/", "-")
	key = strings.ReplaceAll(key, "\\", "-")
	return filepath.Join(launchDir(), key+".json")
}

// writeLaunch stores cmd for `emberloop exec` to run.
func writeLaunch(s Session, cmd Command) (string, error) {
	path := launchPath(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	raw, err := json.Marshal(launchFile{Bin: cmd.Bin, Args: cmd.Args, Env: cmd.Env})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// readLaunch loads the launch `emberloop exec` runs.
func readLaunch(path string) (Command, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Command{}, err
	}
	var lf launchFile
	if err := json.Unmarshal(raw, &lf); err != nil {
		return Command{}, fmt.Errorf("launch file %s is not valid JSON: %w", path, err)
	}
	return Command{Bin: lf.Bin, Args: lf.Args, Env: lf.Env}, nil
}

// execLine is the one shell-agnostic line typed into the pane. It carries no
// env assignments (fish has no VAR=val prefix form) and no prompt text, so it
// is one line in fish, bash and zsh alike.
func execLine(path string) string { return "emberloop exec " + shellArg(path) }

// execFileFromLine parses the launch file out of the typed line. It reverses
// shellArg's quoting, so a state dir with spaces still resolves.
func execFileFromLine(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "emberloop exec ")
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && strings.HasPrefix(rest, `"`) && strings.HasSuffix(rest, `"`) {
		inner := rest[1 : len(rest)-1]
		var b strings.Builder
		escaped := false
		for _, r := range inner {
			if escaped {
				b.WriteRune(r)
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			b.WriteRune(r)
		}
		if escaped {
			b.WriteRune('\\')
		}
		return b.String(), true
	}
	if rest == "" {
		return "", false
	}
	return rest, true
}

// RunExecFile runs the launch at path: it reads the file and execs the binary
// with the exact argv and env, preserving newlines and quotes. The file is
// consumed: it is removed once read, so a started session leaves nothing
// behind even when its caller never cleans up. This process is the agent's
// parent: when the child exits badly its end is recorded in the agent's
// state, so a crash leaves a trace instead of a bare shell.
func RunExecFile(path string) error {
	cmd, err := readLaunch(path)
	if err != nil {
		return err
	}
	_ = os.Remove(path)
	if cmd.Bin == "" {
		return fmt.Errorf("launch file %s names no binary", path)
	}
	if log, name := openToolsLog(cmd); log != nil {
		// A pig session keeps both the tools log and the strip: stdin
		// and stdout stay on the PTY under the strip while the child's
		// stderr keeps its own pipe to the log, one stamped line at a
		// time. With the strip off the child runs unwrapped, exactly as
		// before. Either way this process stays the parent, so a bad
		// exit still lands in the agent's state with its tail.
		defer log.Close()
		stderr, tail := TeeStderr(&stampWriter{w: log, label: name})
		record := func(err error) error {
			if exit, ok := ChildExitOf(err); ok {
				exit.Stderr = tail()
				_ = RecordChildExit(agentIDFromEnv(cmd.Env), exit)
			}
			return err
		}
		if HeaderStripDisabled() {
			c := exec.Command(cmd.Bin, cmd.Args...)
			c.Env = append(append(os.Environ(), cmd.Env...), "PIG_DEBUG_TOOLS=1")
			c.Stdin, c.Stdout = os.Stdin, os.Stdout
			c.Stderr = stderr
			return record(c.Run())
		}
		cmd.Env = append(cmd.Env, "PIG_DEBUG_TOOLS=1")
		return record(WrapLaunchWithChildStderr(cmd, stderr))
	}
	// The strip wrapper runs the child: a bad exit still lands in the
	// agent's state, without a stderr tail the PTY owns.
	err = WrapLaunch(cmd)
	if exit, ok := ChildExitOf(err); ok {
		_ = RecordChildExit(agentIDFromEnv(cmd.Env), exit)
	}
	return err
}

// TeeStderr wraps dst so the child's stderr still streams there while its
// last lines are kept for the exit trace. The top-level launcher shares it,
// so both launchers record the same trace.
func TeeStderr(dst io.Writer) (w io.Writer, tail func() string) {
	var kept stderrTail
	return io.MultiWriter(dst, &kept), kept.String
}

// agentIDFromEnv is the emberloop agent id a launch carries, or "".
func agentIDFromEnv(env []string) string {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "EMBERLOOP_AGENT_ID="); ok {
			return v
		}
	}
	return ""
}

// stderrTail keeps the last lines a child wrote to stderr, so the exit
// trace says what it was doing when it died. It never grows past its cap.
type stderrTail struct {
	lines []string
	part  []byte
}

// tailLines is how many stderr lines the trace keeps; tailBytes caps the
// total, so one huge line cannot blow up the agent's state.
const (
	tailLines = 20
	tailBytes = 8192
)

func (t *stderrTail) Write(p []byte) (int, error) {
	t.part = append(t.part, p...)
	for {
		i := strings.IndexByte(string(t.part), '\n')
		if i < 0 {
			break
		}
		t.lines = append(t.lines, string(t.part[:i]))
		if len(t.lines) > tailLines {
			t.lines = t.lines[len(t.lines)-tailLines:]
		}
		t.part = t.part[i+1:]
	}
	if len(t.part) > tailBytes {
		t.part = t.part[len(t.part)-tailBytes:]
	}
	return len(p), nil
}

// String is the tail as one block: whole lines first, then any partial
// last line, trimmed and capped.
func (t *stderrTail) String() string {
	out := strings.Join(t.lines, "\n")
	if len(t.part) > 0 {
		if out != "" {
			out += "\n"
		}
		out += string(t.part)
	}
	out = strings.TrimSpace(out)
	if len(out) > tailBytes {
		out = out[len(out)-tailBytes:]
	}
	return out
}

// openToolsLog opens the per-session log of how many tools pig sent with each
// provider request, so a session that says a tool is missing can be checked
// against what its provider was actually sent. It applies to pig sessions
// only, and EMBERLOOP_PIG_TOOLS_LOG=0 turns it off. A log that cannot be opened is
// no reason to stop the session: it returns nil and the session runs as before.
func openToolsLog(cmd Command) (*os.File, string) {
	if filepath.Base(cmd.Bin) != "pig" || os.Getenv("EMBERLOOP_PIG_TOOLS_LOG") == "0" {
		return nil, ""
	}
	name := ""
	for _, e := range cmd.Env {
		if v, ok := strings.CutPrefix(e, "EMBERLOOP_SESSION_ID="); ok && v != "" {
			name = v
		}
	}
	if name == "" {
		name = fmt.Sprintf("pid-%d", os.Getpid())
	}
	dir := filepath.Join(filepath.Dir(statePath), "debug")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, ""
	}
	f, err := os.OpenFile(filepath.Join(dir, name+".tools.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, ""
	}
	return f, name
}

// stampWriter writes pig's stderr to the tools log one line at a time, each
// line led by the time it arrived, since pig's own lines carry none. It
// takes any sink, so the wrapper's stderr seam tests it without a file.
type stampWriter struct {
	w     io.Writer
	label string
	part  []byte
}

func (s *stampWriter) Write(p []byte) (int, error) {
	s.part = append(s.part, p...)
	for {
		i := strings.IndexByte(string(s.part), '\n')
		if i < 0 {
			break
		}
		line := string(s.part[:i])
		s.part = s.part[i+1:]
		fmt.Fprintf(s.w, "%s %s\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), line)
	}
	return len(p), nil
}
