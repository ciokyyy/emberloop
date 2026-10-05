package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchFile is the complete launch wick types into a pane: the binary, its
// args including the full prompt, and the env. It lives in wick's state dir
// because herdr pane run types one shell line: newlines and quoting in the
// prompt would break, and the pane's shell may be fish, bash or zsh. Instead
// the pane gets one shell-agnostic line, `wick-pig exec <launch-file>`, which
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

// launchPath is where the launch of s is written: under wick's state dir,
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

// writeLaunch stores cmd for `wick-pig exec` to run.
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

// readLaunch loads the launch `wick-pig exec` runs.
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
func execLine(path string) string { return "wick-pig exec " + shellArg(path) }

// execFileFromLine parses the launch file out of the typed line. It reverses
// shellArg's quoting, so a state dir with spaces still resolves.
func execFileFromLine(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "wick-pig exec ")
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
// behind even when its caller never cleans up.
func RunExecFile(path string) error {
	cmd, err := readLaunch(path)
	if err != nil {
		return err
	}
	_ = os.Remove(path)
	if cmd.Bin == "" {
		return fmt.Errorf("launch file %s names no binary", path)
	}
	c := exec.Command(cmd.Bin, cmd.Args...)
	c.Env = append(os.Environ(), cmd.Env...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
