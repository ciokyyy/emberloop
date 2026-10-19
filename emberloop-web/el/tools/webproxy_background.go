package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The Daemon-hosted background processes: the command runs under the Daemon, in
// the Session's worktree, outside its turn. It survives the turn that started
// it, the browser shows it in the Session's Processes panel, and it dies with
// the Session. bash starts one (background: true); the background tool watches
// and stops it.

// backgroundProxyDescription is served to a Daemon-bound role. Start is not
// an action: bash(background: true) starts a process, so there is one way in.
const backgroundProxyDescription = `Watch or stop a process started with bash(background: true). A process outlives the turn that started it.

Actions:
- list: every process of this Session with its id and state.
- read: pane, optional lines (default 40). Returns recent output.
- wait: pane, optional timeout in seconds (default 60, maximum 600). Returns when the process exits or the timeout ends, with recent output. Use this, never sleep.
- stop: pane. Ends the process and everything it spawned.

The User sees these in the Session's Processes panel and can stop one there. Started processes die with the Session.`

// backgroundProxySchema is the background schema.
func backgroundProxySchema() map[string]any {
	return obj(map[string]any{
		"action":  map[string]any{"type": "string", "enum": []any{"list", "read", "wait", "stop"}},
		"pane":    map[string]any{"type": "string", "description": "read, wait, stop: the process id bash returned."},
		"lines":   map[string]any{"type": "integer", "description": "read, wait: how many recent lines. Default 40."},
		"timeout": map[string]any{"type": "integer", "description": "wait: seconds to wait. Default 60, maximum 600."},
	}, "action")
}

// backgroundProxyView is one process as the Daemon's background route reports
// it.
type backgroundProxyView struct {
	ID      string `json:"id"`
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Running bool   `json:"running"`
	Exit    string `json:"exit"`
	Output  string `json:"output"`
}

// StartBackground starts command as a Daemon background process of the bound
// Session and reports its id and first output. It is what bash does for
// background: true.
func StartBackground(ctx context.Context, command string) (string, error) {
	daemonURL, sessionID, ok := WebDaemonBinding()
	if !ok {
		return "", fmt.Errorf("background: not bound to a Daemon session (%s and %s are required)", SudoGateSessionEnv, SudoGateDaemonEnv)
	}
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("command is required")
	}
	base := daemonURL + "/api/sessions/" + sessionID + "/background"
	body, _ := json.Marshal(map[string]string{"command": command})
	resp, err := webProxyPost(ctx, base, "application/json", body)
	if err != nil {
		return "", fmt.Errorf("background: %s", err.Error())
	}
	defer resp.Body.Close()
	var out struct {
		Process backgroundProxyView `json:"process"`
		Output  string              `json:"output"`
		Error   string              `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		if out.Error == "" {
			out.Error = fmt.Sprintf("refused (%d)", resp.StatusCode)
		}
		return "", fmt.Errorf("background: %s", out.Error)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "started %s (pid %d)", out.Process.ID, out.Process.PID)
	if !out.Process.Running {
		fmt.Fprintf(&b, ", already %s", out.Process.Exit)
	}
	if text := strings.TrimRight(out.Output, "\n"); text != "" {
		b.WriteString("\n" + text)
	}
	if out.Process.Running {
		fmt.Fprintf(&b, "\nNext: background(action=\"wait\", pane=\"%s\") for it to exit, background(action=\"read\", pane=\"%s\") to look, background(action=\"stop\", pane=\"%s\") when done.",
			out.Process.ID, out.Process.ID, out.Process.ID)
	}
	return sanitize(b.String()), nil
}

// backgroundFetch reads one process with its last n output lines.
func backgroundFetch(ctx context.Context, base, pane string, n int) (backgroundProxyView, error) {
	resp, err := webProxyGet(ctx, base+"/"+url.PathEscape(pane)+"?lines="+strconv.Itoa(n))
	if err != nil {
		return backgroundProxyView{}, fmt.Errorf("background: %s", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return backgroundProxyView{}, fmt.Errorf("background: no process %s in this session", pane)
	}
	var out struct {
		Process backgroundProxyView `json:"process"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return backgroundProxyView{}, fmt.Errorf("background: %s", err.Error())
	}
	return out.Process, nil
}

// backgroundText renders a process's output, with its exit when it has ended.
func backgroundText(p backgroundProxyView) string {
	text := strings.TrimRight(p.Output, "\n")
	if !p.Running {
		text = strings.TrimRight(text+"\n["+p.Exit+"]", "\n")
	}
	if strings.TrimSpace(text) == "" {
		text = "(no output yet)"
	}
	return sanitize(text)
}

const (
	defaultBackgroundWait = 60 * time.Second
	maxBackgroundWait     = 600 * time.Second
	backgroundWaitPoll    = time.Second
)

// webProxyBackground runs one background action through the Session's
// background route.
func webProxyBackground(ctx context.Context, daemonURL, sessionID string, args json.RawMessage) (string, error) {
	var p struct {
		Action, Pane string
		Lines        int
		Timeout      int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid background parameters: %w", err)
	}
	base := daemonURL + "/api/sessions/" + sessionID + "/background"
	n := p.Lines
	if n <= 0 {
		n = 40
	}
	switch p.Action {
	case "list":
		resp, err := webProxyGet(ctx, base)
		if err != nil {
			return "", fmt.Errorf("background: %s", err.Error())
		}
		defer resp.Body.Close()
		var out struct {
			Processes []backgroundProxyView `json:"processes"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("background: %s", err.Error())
		}
		if len(out.Processes) == 0 {
			return "no background processes", nil
		}
		var b strings.Builder
		for _, v := range out.Processes {
			state := "running"
			if !v.Running {
				state = v.Exit
			}
			fmt.Fprintf(&b, "%s  %s  %s\n", v.ID, state, v.Command)
		}
		return sanitize(strings.TrimRight(b.String(), "\n")), nil
	case "read":
		if p.Pane == "" {
			return "", fmt.Errorf("pane is required")
		}
		v, err := backgroundFetch(ctx, base, p.Pane, n)
		if err != nil {
			return "", err
		}
		return backgroundText(v), nil
	case "wait":
		if p.Pane == "" {
			return "", fmt.Errorf("pane is required")
		}
		wait := defaultBackgroundWait
		if p.Timeout > 0 {
			wait = min(time.Duration(p.Timeout)*time.Second, maxBackgroundWait)
		}
		deadline := time.Now().Add(wait)
		for {
			v, err := backgroundFetch(ctx, base, p.Pane, n)
			if err != nil {
				return "", err
			}
			if !v.Running {
				return backgroundText(v), nil
			}
			if !time.Now().Before(deadline) {
				return backgroundText(v) + fmt.Sprintf("\n[still running after %v]", wait), nil
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backgroundWaitPoll):
			}
		}
	case "stop":
		if p.Pane == "" {
			return "", fmt.Errorf("pane is required")
		}
		resp, err := webProxyDelete(ctx, base+"/"+url.PathEscape(p.Pane))
		if err != nil {
			return "", fmt.Errorf("background: %s", err.Error())
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("background: no process %s in this session", p.Pane)
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("background: refused (%d)", resp.StatusCode)
		}
		return "closed " + p.Pane + " and everything it spawned", nil
	}
	return "", fmt.Errorf("unknown action %q: use list, read, wait or stop", p.Action)
}
