package wick

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// herdrAgentSource names the wrapper as the reporter of a claude pane. It
// must not start with "herdr:"; the pig extension reports under its own
// source, and the two are never active for one pane.
const herdrAgentSource = "wick"

// agentReporter tells herdr that a wrapped child is an agent and what it is
// doing. A wrapped claude is no longer the pane's foreground process, so
// herdr's process detection never marks the pane an agent and the spawn's
// readiness wait fails. Pig reports itself through its extension, so only
// the claude harness is reported here. Every call is best effort: a herdr
// that is gone or refuses a report never disturbs the child.
type agentReporter struct {
	run       func(args []string)
	pane      string
	label     string
	sessionID string
	mu        sync.Mutex
	seq       int64
	last      string
	released  bool
}

// newAgentReporter builds the reporter for a wrapped child, or nil when
// none applies: not claude, or no herdr pane around the process.
func newAgentReporter(cfg WrapConfig) *agentReporter {
	if cfg.Source.Harness != "claude" {
		return nil
	}
	pane := cfg.PaneID
	if pane == "" && os.Getenv("HERDR_ENV") == "1" {
		pane = os.Getenv("HERDR_PANE_ID")
	}
	if pane == "" {
		return nil
	}
	run := cfg.Report
	if run == nil {
		run = runHerdr
	}
	label := cfg.Source.AgentName
	if label == "" {
		label = "claude"
	}
	return &agentReporter{
		run: run, pane: pane, label: label, sessionID: cfg.Source.SessionID,
		seq: time.Now().UnixMilli() * 1000,
	}
}

// report sends a state to herdr when it changed. The strip's working and
// compressing states both mean busy; anything else is idle.
func (r *agentReporter) report(state HeaderState) {
	if r == nil {
		return
	}
	herdrState := "idle"
	if state == HeaderWorking || state == HeaderCompressing {
		herdrState = "working"
	}
	r.mu.Lock()
	if r.released || r.last == herdrState {
		r.mu.Unlock()
		return
	}
	r.last = herdrState
	r.seq++
	args := []string{"pane", "report-agent", r.pane,
		"--source", herdrAgentSource, "--agent", r.label,
		"--state", herdrState, "--seq", strconv.FormatInt(r.seq, 10)}
	if r.sessionID != "" {
		args = append(args, "--agent-session-id", r.sessionID)
	}
	r.mu.Unlock()
	r.run(args)
}

// release hands authority back so herdr stops treating the pane as an
// agent after the child ends.
func (r *agentReporter) release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return
	}
	r.released = true
	r.seq++
	args := []string{"pane", "release-agent", r.pane,
		"--source", herdrAgentSource, "--agent", r.label,
		"--seq", strconv.FormatInt(r.seq, 10)}
	r.mu.Unlock()
	r.run(args)
}

// runHerdr runs one herdr command with a short limit and ignores the
// result.
func runHerdr(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, herdrBin(), args...).Run()
}
