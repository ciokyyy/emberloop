package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/emberloop/tools"
)

// Sudo requests: a bash command that invokes sudo never runs on its own. The
// bash tool (claude through el-mcp, pig in process) posts it here, the
// model is told the request went to the user, and the UI shows a card with
// Approve and Deny. An approved command runs in the background; its output
// reaches the Session as a TOON "sudo_result" message. The password, when the
// user types one, lives in memory for the run only: it is never stored in an
// Event, the request or a log.

const (
	sudoRunTimeout = 10 * time.Minute
	sudoOutputCap  = 30 * 1024
)

type sudoRequest struct {
	ID      string
	Session string
	Command string
	Cwd     string
	// State: pending, running, denied, done, failed.
	State string
}

type sudoStore struct {
	mu   sync.Mutex
	seq  int
	reqs map[string]*sudoRequest
}

func (d *Daemon) sudoStore() *sudoStore {
	d.sudoOnce.Do(func() { d.sudo = &sudoStore{reqs: map[string]*sudoRequest{}} })
	return d.sudo
}

// sudoEventText is the JSON Text of the sudo_request and sudo_resolved Events.
func sudoEventText(r *sudoRequest, extra map[string]any) string {
	m := map[string]any{"id": r.ID, "command": r.Command, "cwd": r.Cwd, "state": r.State}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// handleSudoCreate queues one request for a live Session.
func (d *Daemon) handleSudoCreate(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || strings.TrimSpace(req.Command) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "command is required"})
		return
	}
	d.mu.Lock()
	s, ok := d.sessions[sessionID]
	var workDir, status string
	var meta SessionMeta
	if ok {
		workDir, status = s.meta.WorkDir, s.meta.Status
		meta = s.meta
	}
	d.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no such session"})
		return
	}
	if status == "ended" {
		writeJSON(w, http.StatusGone, map[string]any{"error": "session ended"})
		return
	}
	if req.Cwd == "" {
		req.Cwd = workDir
	}
	st := d.sudoStore()
	st.mu.Lock()
	st.seq++
	rq := &sudoRequest{ID: fmt.Sprintf("sudo-%d-%d", time.Now().Unix(), st.seq), Session: sessionID, Command: req.Command, Cwd: req.Cwd, State: "pending"}
	st.reqs[rq.ID] = rq
	st.mu.Unlock()
	d.appendEvent(sessionID, "sudo_request", sudoEventText(rq, nil), "pending")
	d.pushSudo(meta, rq.ID, req.Command)
	writeJSON(w, http.StatusCreated, map[string]any{"id": rq.ID})
}

// handleSudoDecide takes the user's Approve or Deny.
func (d *Daemon) handleSudoDecide(w http.ResponseWriter, r *http.Request, sessionID, reqID string) {
	var req struct {
		Action   string `json:"action"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || (req.Action != "approve" && req.Action != "deny") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "action must be approve or deny"})
		return
	}
	st := d.sudoStore()
	st.mu.Lock()
	rq, ok := st.reqs[reqID]
	if !ok || rq.Session != sessionID {
		st.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "request expired or unknown"})
		return
	}
	if rq.State != "pending" {
		state := rq.State
		st.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "request already " + state})
		return
	}
	if req.Action == "deny" {
		rq.State = "denied"
	} else {
		rq.State = "running"
	}
	snap := *rq
	st.mu.Unlock()

	d.appendEvent(sessionID, "sudo_resolved", sudoEventText(&snap, nil), snap.State)
	if req.Action == "deny" {
		go d.deliverSudoResult(&snap, "denied", -1, "The user denied this request.")
	} else {
		go d.runSudo(&snap, req.Password)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": reqID, "state": snap.State})
}

// sudoShell prefixes the command so every sudo inside it reads the password
// from stdin (-S) or never prompts (-n).
func sudoShell(command string, withPassword bool) string {
	flags := "-n"
	if withPassword {
		flags = "-S -p ''"
	}
	return "sudo() { command sudo " + flags + " \"$@\"; }\n" + command
}

func (d *Daemon) runSudo(rq *sudoRequest, password string) {
	ctx, cancel := context.WithTimeout(context.Background(), sudoRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", sudoShell(rq.Command, password != ""))
	cmd.Dir = rq.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	if password != "" {
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := capSudoOutput(tools.SanitizeOutput(out.String()))
	code := 0
	status := "approved"
	if err != nil {
		status = "failed"
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			text += "\n[" + err.Error() + "]"
		}
		if ctx.Err() == context.DeadlineExceeded {
			text += fmt.Sprintf("\n[timed out after %v]", sudoRunTimeout)
		}
	}
	d.deliverSudoResult(rq, status, code, text)
}

func capSudoOutput(s string) string {
	if len(s) <= sudoOutputCap {
		return s
	}
	half := sudoOutputCap / 2
	return s[:half] + "\n[... output cut ...]\n" + s[len(s)-half:]
}

// deliverSudoResult records the end state and sends the TOON message to the
// Session as a Follow-up.
func (d *Daemon) deliverSudoResult(rq *sudoRequest, status string, exit int, output string) {
	state := "done"
	switch status {
	case "denied":
		state = "denied"
	case "failed":
		state = "failed"
	}
	st := d.sudoStore()
	st.mu.Lock()
	if cur, ok := st.reqs[rq.ID]; ok {
		cur.State = state
	}
	st.mu.Unlock()
	snap := *rq
	snap.State = state
	d.appendEvent(rq.Session, "sudo_resolved", sudoEventText(&snap, map[string]any{"exit": exit}), state)

	text := sudoResultTOON(rq, status, exit, output)
	if code, _ := d.resumeReleased(rq.Session); code != 0 {
		return
	}
	d.routeLive(rq.Session, "followup", text)
}

// sudoResultTOON renders the result as TOON: indented key: value lines, text
// values as quoted strings so one line holds one field. The UI parses the
// same shape back.
func sudoResultTOON(rq *sudoRequest, status string, exit int, output string) string {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	var b strings.Builder
	b.WriteString("sudo_result:\n")
	fmt.Fprintf(&b, "  request: %s\n", rq.ID)
	fmt.Fprintf(&b, "  status: %s\n", status)
	fmt.Fprintf(&b, "  exit: %d\n", exit)
	fmt.Fprintf(&b, "  command: %s\n", q(rq.Command))
	fmt.Fprintf(&b, "  output: %s", q(output))
	return b.String()
}
