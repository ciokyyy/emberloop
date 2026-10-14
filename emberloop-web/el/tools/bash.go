package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// bash is ported from el's apps/backend/tools/terminal.go, rewriter.go and
// sanitizer.go. It runs through os/exec directly: ctx.ExecuteTool("bash")
// would resolve back to this tool, and a direct exec keeps it host-agnostic.
//
// ponytail: no bubblewrap sandbox and no read-only shell policy. Read-only
// roles are denied bash by their tool allowlist instead. Port Sandbox when a
// writable role must run untrusted code.
const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
	rewriteTimeout     = 500 * time.Millisecond
)

var (
	ansiRegex     = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07`)
	multiNewlines = regexp.MustCompile(`\n{3,}`)
)

// sanitize strips terminal escapes, normalises line endings and collapses
// blank runs. Oversize output is not truncated here: Execute stores it in
// the overflow store instead.
func sanitize(raw string) string {
	s := ansiRegex.ReplaceAllString(raw, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return multiNewlines.ReplaceAllString(s, "\n\n")
}

// bashSource labels a stored bash output with the command that produced it,
// truncated so the label stays one line.
func bashSource(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if len(cmd) > 80 {
		cmd = cmd[:80] + "..."
	}
	return "bash " + cmd
}

// RTKRewrite asks `rtk rewrite` for the token-compressed form of a command.
// rtk exits 0 or 3 when it has one and 1 when it has none, so the rewritten
// text is the signal. Anything else (rtk missing, a timeout, an empty answer)
// leaves the command as written.
func RTKRewrite(ctx context.Context, cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" || trimmed == "rtk" || strings.HasPrefix(trimmed, "rtk ") {
		return cmd
	}
	ctx, cancel := context.WithTimeout(ctx, rewriteTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "rtk", "rewrite", trimmed).Output()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || (exit.ExitCode() != 3) {
			return cmd
		}
	}
	if rewritten := strings.TrimSpace(string(out)); rewritten != "" {
		return rewritten
	}
	return cmd
}

type bashTool struct{}

func (bashTool) Name() string { return "bash" }
func (bashTool) Description() string {
	return "Execute a bash command in the workspace. Returns combined stdout and stderr, and the exit status when it is not 0. Commands are token-compressed via rtk. A server, watcher or anything that does not exit goes to the background tool, never here. Python never runs here: use the execute tool, not python or python3 through bash. Prefer find for locating code and execute for multi-step exploration (loops over grep/read, filtering output) before repeated bash grep."
}
func (bashTool) Schema() map[string]any {
	return obj(map[string]any{
		"command": map[string]any{"type": "string", "description": "The shell command to run in bash."},
		"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 120, maximum 600)."},
	}, "command")
}

func (bashTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	var p struct {
		Command string
		Timeout int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid bash parameters: %w", err)
	}
	if strings.TrimSpace(p.Command) == "" {
		return "", errors.New("command is required")
	}
	if gate := SudoGate; gate != nil && UsesSudo(p.Command) {
		if msg, taken := gate(ctx, root, p.Command); taken {
			return msg, nil
		}
	}
	timeout := defaultBashTimeout
	if p.Timeout > 0 {
		timeout = min(time.Duration(p.Timeout)*time.Second, maxBashTimeout)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", RTKRewrite(ctx, p.Command))
	cmd.Dir = root
	// A timeout must reach the children too, or CombinedOutput waits on a pipe
	// they still hold. Own process group, killed as a group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second

	out, err := cmd.CombinedOutput()
	text := sanitize(string(out))
	if summary, stored, merr := MaybeOverflow(root, overflowSession(), bashSource(p.Command), "bash", text, ""); merr != nil {
		// The store is unavailable: cap the output where the old 30KB
		// truncation did, and say so, rather than failing the call or
		// flooding context without bound.
		text = truncateHeadTail(text, 15*1024, 15*1024) +
			fmt.Sprintf("\n[overflow store unavailable: %v]", merr)
	} else if stored {
		text = summary
	}
	if err == nil {
		return text, nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return text + fmt.Sprintf("\n[timed out after %v]", timeout), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// Exit 1 with nothing printed is the "no match" answer of grep and
		// diff-like probes; say so instead of returning an empty string.
		if exit.ExitCode() == 1 && strings.TrimSpace(text) == "" {
			return "(no output, exit status 1)", nil
		}
		return text + fmt.Sprintf("\n[exit status %d]", exit.ExitCode()), nil
	}
	return "", err
}
