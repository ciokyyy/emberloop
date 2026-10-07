package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ErrNoChecks is the missing-file answer both the tool and the spawn-time
// run give: the repo has no .emberloop/check to run.
var ErrNoChecks = errors.New("no checks configured (the repo has no .emberloop/check)")

// CheckTimeout bounds one repo check run, in the tool budget. A var so tests
// can shorten it; the spawn-time run in the harness shares this value.
var CheckTimeout = 3 * time.Minute

// CheckTailLines is how many trailing output lines a check result keeps: a
// check's verdict is at its end.
const CheckTailLines = 60

// checkTool re-runs one of the repo's named checks from .emberloop/check, so a
// reviewer judges real results rather than the worker's word.
type checkTool struct{}

func (checkTool) Name() string   { return "check" }
func (checkTool) ReadOnly() bool { return true }
func (checkTool) Description() string {
	return "Re-run one of the repo's named checks from .emberloop/check: name (default \"default\"), args substituted into the command's {args} slot. Shell metacharacters in args are refused, and a check without a slot rejects non-empty args."
}
func (checkTool) Schema() map[string]any {
	return obj(map[string]any{
		"name": map[string]any{"type": "string", "description": "Which named check to run (default \"default\")."},
		"args": map[string]any{"type": "string", "description": "A string substituted into the command's {args} slot only."},
	})
}

func (checkTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	var a struct {
		Name string `json:"name"`
		Args string `json:"args"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid check arguments: %w", err)
	}
	cmd, display, err := resolveCheck(root, a.Name, a.Args)
	if err != nil {
		return "", err
	}
	return runCheckCommand(ctx, root, cmd, display), nil
}

// resolveCheck loads the check file under root and returns the shell command
// for name with args substituted, plus the display string the status line
// names. An empty name means "default". The file is either a plain command
// (the one check, named "default") or a JSON map of names to commands, where
// a command may hold one {args} slot.
func resolveCheck(root, name, args string) (cmd, display string, err error) {
	raw, err := os.ReadFile(filepath.Join(root, ".emberloop", "check"))
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return "", "", ErrNoChecks
	}
	if name == "" {
		name = "default"
	}
	if checks, ok := parseCheckMap(strings.TrimSpace(string(raw))); ok {
		available := sortedCheckNames(checks)
		command, ok := checks[name]
		if !ok {
			return "", "", fmt.Errorf("unknown check %q; available: %s", name, strings.Join(available, ", "))
		}
		if strings.TrimSpace(command) == "" {
			return "", "", fmt.Errorf("check %q has an empty command", name)
		}
		final, err := substituteCheckArgs(name, command, args)
		if err != nil {
			return "", "", err
		}
		if name == "default" {
			return final, final, nil
		}
		return final, name + ": " + final, nil
	}
	command := strings.TrimSpace(string(raw))
	if name != "default" {
		return "", "", fmt.Errorf("unknown check %q; available: default", name)
	}
	if args != "" {
		return "", "", fmt.Errorf("check %q takes no arguments (its command has no {args} slot)", name)
	}
	return command, command, nil
}

// SpawnCheck resolves the check the spawn-time run executes for root: the
// map's "default", or its first name in order when no default exists. The
// display names the check when it is not the default, so the review packet
// says which check ran. A plain file resolves to its one command, exactly as
// before.
func SpawnCheck(root string) (cmd, display string, err error) {
	raw, err := os.ReadFile(filepath.Join(root, ".emberloop", "check"))
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return "", "", ErrNoChecks
	}
	if checks, ok := parseCheckMap(strings.TrimSpace(string(raw))); ok {
		if _, ok := checks["default"]; !ok {
			return resolveCheck(root, sortedCheckNames(checks)[0], "")
		}
	}
	return resolveCheck(root, "default", "")
}

// parseCheckMap reads the file body as a JSON map of check names to commands.
// Anything that is not such a map is a plain command instead: a repo's
// single-command file keeps working unchanged.
func parseCheckMap(body string) (map[string]string, bool) {
	if !strings.HasPrefix(body, "{") {
		return nil, false
	}
	var checks map[string]string
	if err := json.Unmarshal([]byte(body), &checks); err != nil || len(checks) == 0 {
		return nil, false
	}
	return checks, true
}

// sortedCheckNames is the map's keys in order, so errors and fallbacks are
// deterministic.
func sortedCheckNames(checks map[string]string) []string {
	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// substituteCheckArgs fills the command's {args} slot. args runs inside `sh
// -c`, so anything that could start a new command, redirect, expand or glob
// is refused; a command without a slot rejects non-empty args.
func substituteCheckArgs(name, command, args string) (string, error) {
	if !strings.Contains(command, "{args}") {
		if args != "" {
			return "", fmt.Errorf("check %q takes no arguments (its command has no {args} slot)", name)
		}
		return command, nil
	}
	if bad := refusedArgsChar(args); bad != "" {
		return "", fmt.Errorf("args must not contain shell metacharacters (found %q)", bad)
	}
	return strings.ReplaceAll(command, "{args}", args), nil
}

// refusedArgsChar is the first shell metacharacter in args, or "" when it
// carries none: command separators and backgrounding, expansions, quoting
// and escapes, redirection, subshells, globs, and newlines.
func refusedArgsChar(args string) string {
	for _, c := range []string{";", "|", "&", "$", "`", "\n", "\r", "<", ">", "(", ")", "\\", "'", `"`, "*", "?", "~", "#", "!"} {
		if strings.Contains(args, c) {
			return c
		}
	}
	return ""
}

// runCheckCommand runs cmd in root through sh and formats the result the way
// the spawn-time run does: a status line plus the 60-line tail.
func runCheckCommand(ctx context.Context, root, cmd, display string) string {
	ctx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = root
	// A timeout must reach the children too, or CombinedOutput waits on a pipe
	// they still hold. Own process group, killed as a group.
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 2 * time.Second
	out, err := c.CombinedOutput()
	text := CheckTail(strings.TrimSpace(string(out)))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("`%s` TIMED OUT after %s, which is not a pass. Output so far:\n%s", display, CheckTimeout, text)
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return fmt.Sprintf("`%s` FAILED (exit %d):\n%s", display, exit.ExitCode(), text)
		}
		return fmt.Sprintf("`%s` could not run: %v", display, err)
	}
	return fmt.Sprintf("`%s` passed (exit 0):\n%s", display, text)
}

// CheckTail keeps the last CheckTailLines lines of text.
func CheckTail(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= CheckTailLines {
		return text
	}
	return "…\n" + strings.Join(lines[len(lines)-CheckTailLines:], "\n")
}
