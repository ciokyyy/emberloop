package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Grid places the pane a background command runs in through the shared agent
// grid and returns its pane id. emberloop installs its grid here at startup, so a
// server or watcher sits in the caller's tab beside the agents instead of
// splitting the caller's own pane. A host with no grid leaves it nil, and start
// refuses rather than stacking panes.
var Grid func(cwd string) (string, error)

// HerdrRun runs one herdr command for start's pane run and for read and stop.
// It is a var so a emberloop test can watch the pane through the same fake grid.
var HerdrRun = herdr

// backgroundTool runs a long-lived command (a dev server, a watcher) in a
// herdr pane placed by the grid, so the model can read its output and close it,
// and the human can watch it. bash would block until its timeout and then kill
// the process.
//
// ponytail: panes are not tracked. start returns the pane id and the model
// carries it; a pane the model forgets is closed by the human with the pane.
type backgroundTool struct{}

func (backgroundTool) Name() string { return "background" }
func (backgroundTool) Description() string {
	return `Run a command that does not exit (dev server, watcher, tail) in its own pane beside this session. Never start one with bash.

Actions:
- start: command. Returns the pane id.
- read: pane, optional lines (default 40). Returns recent pane output.
- stop: pane. Closes the pane and ends the process.`
}
func (backgroundTool) Schema() map[string]any {
	return obj(map[string]any{
		"action":  map[string]any{"type": "string", "enum": []any{"start", "read", "stop"}},
		"command": map[string]any{"type": "string", "description": "start: the command to run."},
		"pane":    map[string]any{"type": "string", "description": "read, stop: the pane id start returned."},
		"lines":   map[string]any{"type": "integer", "description": "read: how many recent lines. Default 40."},
	}, "action")
}

func herdr(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "herdr", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("herdr %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (t backgroundTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	var p struct {
		Action, Command, Pane string
		Lines                 int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid background parameters: %w", err)
	}
	switch p.Action {
	case "start":
		if strings.TrimSpace(p.Command) == "" {
			return "", errors.New("command is required")
		}
		if os.Getenv("HERDR_PANE_ID") == "" && os.Getenv("HERDR_TAB_ID") == "" {
			return "", errors.New("not inside herdr, so there is nowhere to place a pane; run it with bash and a trailing & instead")
		}
		if Grid == nil {
			return "", errors.New("no pane grid is installed; run the command with bash and a trailing & instead")
		}
		pane, err := Grid(root)
		if err != nil {
			return "", err
		}
		if _, err := HerdrRun(ctx, "pane", "run", pane, p.Command); err != nil {
			_, _ = HerdrRun(ctx, "pane", "close", pane)
			return "", err
		}
		return fmt.Sprintf("started in pane %s. Next: background(action=\"read\", pane=\"%s\") to check it, background(action=\"stop\", pane=\"%s\") when done.", pane, pane, pane), nil
	case "read":
		if p.Pane == "" {
			return "", errors.New("pane is required")
		}
		n := p.Lines
		if n <= 0 {
			n = 40
		}
		out, err := HerdrRun(ctx, "pane", "read", p.Pane, "--lines", fmt.Sprint(n))
		if err != nil {
			return "", err
		}
		return sanitize(out), nil
	case "stop":
		if p.Pane == "" {
			return "", errors.New("pane is required")
		}
		if _, err := HerdrRun(ctx, "pane", "close", p.Pane); err != nil {
			return "", err
		}
		return "closed " + p.Pane, nil
	}
	return "", fmt.Errorf("unknown action %q: use start, read or stop", p.Action)
}
