package wick

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// restartBoundaryTimeout bounds one boundary wait: a compress signal waits
// for the running tool call, but a tool that never answers must not hold the
// supervisor forever. A var so tests run it fast.
var restartBoundaryTimeout = 30 * time.Second

// PaneRestart is one supervisor handoff: the agent to restart, the pane it
// runs in, and the harness session it runs now. Handoff is the text the new
// child receives as its first message; launchFor must deliver that same text
// (pig as the launch prompt, claude as Late through DeliverLate), and an
// empty handoff is refused. SessionDir overrides the transcript root (temp
// transcripts in tests); empty resolves through the harness. Force skips the
// boundary wait and restarts past a running tool call: the compress key's
// second press.
type PaneRestart struct {
	Harness      string
	Pane         string
	AgentID      string
	OldSessionID string
	SessionDir   string
	Handoff      string
	Force        bool
}

// WaitForToolBoundary waits until the transcript at path shows no
// outstanding tool call: every call has its result. A missing transcript is
// the wanted quiet state, not a failure: the harness simply wrote nothing
// yet. It polls on pollEvery, the shared test-shrinkable tick.
func WaitForToolBoundary(harness, transcriptPath string) error {
	deadline := time.Now().Add(restartBoundaryTimeout)
	for {
		snap, err := summarizePath(harness, transcriptPath)
		if err != nil {
			return err
		}
		if snap.Outstanding == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("compress waited for %d outstanding tool calls past %s", snap.Outstanding, restartBoundaryTimeout)
		}
		time.Sleep(pollEvery)
	}
}

// summarizePath folds the transcript at path into a snapshot. A missing file
// is zeros: no model, no context, nothing outstanding.
func summarizePath(harness, transcriptPath string) (ContextSnapshot, error) {
	f, err := os.Open(transcriptPath)
	if os.IsNotExist(err) {
		return ContextSnapshot{ToolBytes: map[string]int64{}}, nil
	}
	if err != nil {
		return ContextSnapshot{}, err
	}
	defer f.Close()
	return SummarizeContext(harness, f)
}

// RestartPane restarts one agent in its own pane with the handoff as the new
// child's first message: it waits for a tool-call boundary, stops the child,
// starts the new child in the same pane, records the new harness session
// with the compressed count, then delivers a late handoff. The pane never
// closes: neither the happy path nor any failure closes or drops it.
// launchFor builds the new child's launch for the fresh session id carrying
// req.Handoff, so the top-level session and every spawned pane share this
// core with their own session template. A launchFor error leaves the pane
// with the child stopped and the state untouched: the pane stays open at its
// shell for a retry. Once pane run succeeds the new child is live, so a later
// error (late delivery, state record) still returns the new session id the
// pane runs on.
func RestartPane(run Exec, req PaneRestart, launchFor func(newSessionID string) (Session, Launch, error)) (string, error) {
	if req.AgentID == "" {
		return "", fmt.Errorf("agent is required")
	}
	if req.Pane == "" {
		return "", fmt.Errorf("pane is required")
	}
	if strings.TrimSpace(req.Handoff) == "" {
		return "", fmt.Errorf("handoff is required")
	}
	if Registered(req.Harness) == nil {
		return "", fmt.Errorf("unknown harness %q", req.Harness)
	}
	if !req.Force && req.OldSessionID != "" {
		if path := restartTranscriptPath(req.Harness, req.SessionDir, req.OldSessionID); path != "" {
			if err := WaitForToolBoundary(req.Harness, path); err != nil {
				return "", err
			}
		}
	}
	if err := stopPane(run, req.Pane); err != nil {
		return "", err
	}
	next := newSessionID()
	sess, launch, err := launchFor(next)
	if err != nil {
		return "", err
	}
	if res, err := run(herdrBin(), []string{"pane", "run", req.Pane, launch.Line}, ExecOpts{Timeout: 15 * time.Second}); err != nil {
		CleanupSession(sess)
		return "", err
	} else if res.Code != 0 {
		CleanupSession(sess)
		return "", fmt.Errorf("herdr pane run %s failed: %s", req.Pane, firstLine(res.Stderr, res.Stdout))
	}
	// The pane keeps its agent across the handoff, so it keeps its name too:
	// re-assert it for the agent's type. Silent by contract, never blocking.
	reassertPaneName(run, req.AgentID, req.Pane)
	// The agent reads its MCP config at startup; the launch file stays for
	// the pane's `wick-pig exec` to consume.
	cleanupMCP(sess)
	// The child is live on next from here: record first, so a late-delivery
	// error still leaves the state on the session the pane runs.
	if err := recordPaneRestart(req.AgentID, req.Pane, next); err != nil {
		return next, err
	}
	if err := DeliverLate(run, req.Pane, launch.Late); err != nil {
		return next, err
	}
	return next, nil
}

// restartTranscriptPath resolves the old session's transcript: SessionDir
// wins when set (temp transcripts in tests), else the harness session root.
// Empty means the harness wrote nothing yet, and the boundary is trivially
// met.
func restartTranscriptPath(harness, sessionDir, sessionID string) string {
	h := Registered(harness)
	if h == nil {
		return ""
	}
	root := sessionDir
	if root == "" {
		root = h.SessionRoot(Session{Dir: sessionDir})
	}
	return findSession(root, sessionID)
}

// recordPaneRestart points the agent at its new harness session and counts
// the handoff. A missing state (the top-level session's first compress)
// starts one: the pane and the fresh session with compressed 1. A worker
// record follows the rotation the way RotateAgentSession moves it.
func recordPaneRestart(agentID, pane, next string) error {
	err := withFileLock("state-"+agentID, func() error {
		s, ok, err := ReadAgentState(agentID)
		if err != nil {
			return err
		}
		if !ok {
			s = AgentState{ID: agentID}
		}
		s.ID = agentID
		if s.Pane == "" {
			s.Pane = pane
		}
		s.SessionID = next
		s.Compressed++
		// A restart starts a new child: stamp its launch and drop the old
		// child's trace, so a later lost notice never carries a stale exit.
		s.LaunchedAt = time.Now().UTC().Format(time.RFC3339)
		s.ExitCode, s.ExitSignal, s.ExitStderr = nil, "", ""
		return WriteAgentState(s)
	})
	if err != nil {
		return err
	}
	return pointWorkerAtSession(agentID, next)
}

// reassertPaneName names pane for the agent's type after a restart. A pane
// keeps its agent across a handoff, so the name never goes stale. No state
// or no type (the top-level session names its own pane at startup) renames
// nothing; a failed rename is silent and never blocks.
func reassertPaneName(run Exec, agentID, pane string) {
	s, ok, err := ReadAgentState(agentID)
	if err != nil || !ok || s.Type == "" {
		return
	}
	renamePane(run, pane, PaneNameForType(s.Type))
}
