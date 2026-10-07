package emberloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"example.com/emberloop/tools"
)

// Compress request queue (ticket 08): the herdr plugin's signal path. One key
// press calls RequestCompress with the focused pane. A press during a running
// tool call queues to the next boundary: the mark below is what the header
// strip reads as `compress queued` while the call waits. A second press while
// the mark stands forces the compress now, past the running call.

// compressQueuedPath is the queue mark for one agent: a live waiter's pid.
// A dead owner reads as no queue, so a crashed wait neither pins the header
// nor forces the next press.
func compressQueuedPath(agentID string) string {
	return filepath.Join(agentStateDir(), agentID+".compress-queued")
}

// markCompressQueued records this process as the waiter for agentID.
func markCompressQueued(agentID string) {
	_ = os.MkdirAll(agentStateDir(), 0o755)
	_ = os.WriteFile(compressQueuedPath(agentID), []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// clearCompressQueued drops the queue mark for agentID, whoever wrote it: a
// forced press takes the queue over.
func clearCompressQueued(agentID string) {
	_ = os.Remove(compressQueuedPath(agentID))
}

// clearCompressQueueOwned drops the queue mark only when this process wrote
// it, so a waiter finishing never clears a mark a later press wrote.
func clearCompressQueueOwned(agentID string) {
	if pid, ok := readCompressQueuePid(agentID); ok && pid == os.Getpid() {
		_ = os.Remove(compressQueuedPath(agentID))
	}
}

// readCompressQueuePid returns the waiter pid in the queue mark. Missing,
// unreadable and unparseable marks read as no queue; unparseable ones are
// removed.
func readCompressQueuePid(agentID string) (int, bool) {
	raw, err := os.ReadFile(compressQueuedPath(agentID))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		_ = os.Remove(compressQueuedPath(agentID))
		return 0, false
	}
	return pid, true
}

// CompressQueued reports whether a compress request for agentID is waiting at
// a tool-call boundary. Empty and dead-owner marks read as no queue.
func CompressQueued(agentID string) bool {
	if agentID == "" {
		return false
	}
	pid, ok := readCompressQueuePid(agentID)
	if !ok {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		_ = os.Remove(compressQueuedPath(agentID))
		return false
	}
	return true
}

// waitCompressBoundary waits for the real tool-call boundary: every call has
// its result, however long the tool runs. No timeout: the queue stands until
// the boundary lands or a forced press takes the mark over, which stops the
// waiter as superseded instead of compressing twice.
func waitCompressBoundary(harness, transcriptPath, agentID, pane string) error {
	for {
		if pid, ok := readCompressQueuePid(agentID); !ok || pid != os.Getpid() {
			return fmt.Errorf("pane %q compressed by a forced press while this request waited: stopping", pane)
		}
		snap, err := summarizePath(harness, transcriptPath)
		if err != nil {
			return err
		}
		if snap.Outstanding == 0 {
			return nil
		}
		time.Sleep(pollEvery)
	}
}

// RequestCompress is the herdr compress key's signal: the focused pane,
// whatever the harness. At a tool boundary it compresses at once. During a
// running tool call it queues to the boundary (the header shows the queue);
// a second press while the queue stands forces the compress now.
func RequestCompress(run Exec, pane string, s tools.Settings) (string, error) {
	agentID, err := resolvePaneToAgent(pane, run)
	if err != nil {
		return "", err
	}
	if CompressQueued(agentID) {
		clearCompressQueued(agentID)
		return CompressPaneWithForce(run, pane, s, true)
	}
	return CompressPaneWithForce(run, pane, s, false)
}

// compressTarget is everything a compress restarts: the harness and session
// under compression, its transcript, and the relaunch builder for the
// handoff. Workers restart as workers; the top-level session restarts from
// the record it wrote at startup.
type compressTarget struct {
	harness   string
	sessionID string
	path      string
	launchFor func(handoff string) func(newSessionID string) (Session, Launch, error)
}

// resolveCompressTarget turns the agent in pane into its compress target. A
// worker restarts from its worker record; without one the agent must be the
// top-level session's own record, which carries the harness, session and
// launch the restart rebuilds. Anything else is a clear error that starts
// nothing.
func resolveCompressTarget(agentID, pane string) (compressTarget, error) {
	store, err := load()
	if err != nil {
		return compressTarget{}, err
	}
	if w, ok := store[agentID]; ok {
		h := Registered(w.Harness)
		if h == nil {
			return compressTarget{}, fmt.Errorf("unknown harness %q (have %s)", w.Harness, strings.Join(HarnessNames(), ", "))
		}
		sessionID := w.SessionID
		if st, ok, err := ReadAgentState(agentID); err != nil {
			return compressTarget{}, err
		} else if ok && st.SessionID != "" {
			sessionID = st.SessionID
		}
		if sessionID == "" {
			return compressTarget{}, fmt.Errorf("pane %q is agent %q with no session: nothing to compress", pane, agentID)
		}
		path := findSession(h.SessionRoot(Session{}), sessionID)
		if path == "" {
			return compressTarget{}, fmt.Errorf("no transcript for session %q", sessionID)
		}
		return compressTarget{
			harness:   w.Harness,
			sessionID: sessionID,
			path:      path,
			launchFor: func(handoff string) func(string) (Session, Launch, error) {
				return workerLaunchFor(w, handoff)
			},
		}, nil
	}
	st, ok, err := ReadAgentState(agentID)
	if err != nil {
		return compressTarget{}, err
	}
	if !ok || st.Harness == "" || st.SessionID == "" || (st.Type != "" && st.Type != "orchestrator") {
		return compressTarget{}, fmt.Errorf("pane %q is agent %q, not a worker: manual compress needs a worker", pane, agentID)
	}
	h := Registered(st.Harness)
	if h == nil {
		return compressTarget{}, fmt.Errorf("unknown harness %q (have %s)", st.Harness, strings.Join(HarnessNames(), ", "))
	}
	path := findSession(h.SessionRoot(Session{}), st.SessionID)
	if path == "" {
		return compressTarget{}, fmt.Errorf("no transcript for session %q", st.SessionID)
	}
	return compressTarget{
		harness:   st.Harness,
		sessionID: st.SessionID,
		path:      path,
		launchFor: func(handoff string) func(string) (Session, Launch, error) {
			return topLevelLaunchFor(st, handoff)
		},
	}, nil
}

// topLevelLaunchFor rebuilds the top-level session's launch for the fresh
// session id carrying the handoff, the way emberloop started it: its role,
// directory, model and effort.
func topLevelLaunchFor(st AgentState, handoff string) func(newSessionID string) (Session, Launch, error) {
	return func(next string) (Session, Launch, error) {
		if st.Cwd == "" {
			return Session{}, Launch{}, fmt.Errorf("agent %q recorded no working directory: restart emberloop to enable compress", st.ID)
		}
		role := st.Role
		if role == "" {
			role = "plan"
		}
		sess := Session{
			ID:      next,
			Role:    role,
			Cwd:     st.Cwd,
			Model:   st.Model,
			Effort:  st.Effort,
			AgentID: st.ID,
			Prompt:  handoff,
		}
		launch, err := PrepareLaunch(st.Harness, sess)
		return sess, launch, err
	}
}

// compressSessionNow is the harness session the pane runs now: the worker
// record first, else the agent state. A waiter that finds a newer session
// was superseded by a forced compress.
func compressSessionNow(agentID string) string {
	if store, err := load(); err == nil {
		if w, ok := store[agentID]; ok && w.SessionID != "" {
			return w.SessionID
		}
	}
	if st, ok, err := ReadAgentState(agentID); err == nil && ok {
		return st.SessionID
	}
	return ""
}

// CompressPaneWithForce compresses the agent in pane by hand, like
// CompressPane, but force skips every boundary wait and compresses past a
// running tool call. Without force a running call queues to the real
// boundary with the queue mark set, so the header shows it however long the
// tool runs; a forced press meanwhile takes the mark over and stops the
// waiter as superseded instead of compressing twice. A failed wait clears
// the waiter's own mark and touches nothing.
func CompressPaneWithForce(run Exec, pane string, s tools.Settings, force bool) (string, error) {
	agentID, err := resolvePaneToAgent(pane, run)
	if err != nil {
		return "", err
	}
	if force {
		clearCompressQueued(agentID)
	}
	tgt, err := resolveCompressTarget(agentID, pane)
	if err != nil {
		return "", err
	}
	snap, err := summarizePath(tgt.harness, tgt.path)
	if err != nil {
		return "", err
	}
	// The boundary waits before the summary: a result landing mid-wait
	// belongs in the handoff, not after it.
	if !force && snap.Outstanding > 0 {
		markCompressQueued(agentID)
		if err := waitCompressBoundary(tgt.harness, tgt.path, agentID, pane); err != nil {
			clearCompressQueueOwned(agentID)
			return "", err
		}
		clearCompressQueueOwned(agentID)
		if now := compressSessionNow(agentID); now != tgt.sessionID {
			return "", fmt.Errorf("pane %q compressed as %s while this request waited: stopping", pane, now)
		}
	}
	model := s.EffectiveCompressModel(tgt.harness)
	// The claim keeps one pane on one compress across the manual key, the
	// force press and the automatic sweep: a live mark refuses instead of
	// summarizing twice over the same transcript.
	claimed, err := ClaimCompressing(agentID)
	if err != nil {
		return "", err
	}
	if !claimed {
		return "", fmt.Errorf("pane %q is already compressing", pane)
	}
	defer SetAgentCompressing(agentID, false) //nolint:errcheck
	handoff, err := SummarizeSession(run, tgt.harness, tgt.path, model, ModelWindow(model, s.ContextWindows()))
	if err != nil {
		return "", err
	}
	handoff = FinishHandoff(handoff, snap.Outstanding > 0)
	if err := keepCompressCopies(agentID, tgt.sessionID, tgt.path, handoff); err != nil {
		return "", err
	}
	return RestartPane(run, PaneRestart{
		Harness:      tgt.harness,
		Pane:         pane,
		AgentID:      agentID,
		OldSessionID: tgt.sessionID,
		Handoff:      handoff,
		Force:        force,
	}, tgt.launchFor(handoff))
}

// TopLevelSession is what the top-level emberloop records at startup so the
// compress key reaches it: the stable agent id, the herdr pane, how the
// session runs, and the harness session id a restart replaces.
type TopLevelSession struct {
	AgentID   string
	Pane      string
	Harness   string
	Model     string
	Effort    string
	Role      string
	Cwd       string
	SessionID string
}

// WriteTopLevelState records the top-level session's own agent state. The
// pane keeps the orchestrator name, so herdr and the header read it like
// every spawned pane. One pane holds one top-level record: an earlier run in
// the same pane is superseded and removed with its queue mark, so the key
// never dies on "matches multiple agents".
func WriteTopLevelState(s TopLevelSession) error {
	if err := supersedeTopLevelPane(s.Pane, s.AgentID); err != nil {
		return err
	}
	return WriteAgentState(AgentState{
		ID:        s.AgentID,
		Type:      "orchestrator",
		Pane:      s.Pane,
		Harness:   s.Harness,
		Model:     s.Model,
		Effort:    s.Effort,
		Role:      s.Role,
		Cwd:       s.Cwd,
		SessionID: s.SessionID,
	})
}

// supersedeTopLevelPane removes earlier top-level records for pane: a new
// run owns its pane outright, since two records on one pane break pane
// resolution for the key. Worker, reviewer and scout records never share a
// pane with a top-level run, so only orchestrator records go.
func supersedeTopLevelPane(pane, keep string) error {
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if id == keep {
			continue
		}
		st, ok, err := ReadAgentState(id)
		if err != nil || !ok || st.Type != "orchestrator" || st.Pane != pane {
			continue
		}
		_ = os.Remove(agentStatePath(id))
		clearCompressQueued(id)
	}
	return nil
}

// topLevelPanes lists the herdr panes of the recorded top-level sessions, so
// the automatic sweep reaches them like worker panes.
func topLevelPanes() []string {
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return nil
	}
	var panes []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		st, ok, err := ReadAgentState(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || !ok || st.Type != "orchestrator" || st.Pane == "" || st.SessionID == "" {
			continue
		}
		panes = append(panes, st.Pane)
	}
	return panes
}
