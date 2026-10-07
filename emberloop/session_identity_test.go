package emberloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An agent keeps its identity across a session restart: the stable agent id
// names the state, and the state records the harness session id currently in
// use. A restart gives the same agent a fresh harness session id without
// losing any state.
func TestAgentStateRecordsItsCurrentSession(t *testing.T) {
	isolateState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p1", SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	s, ok, err := ReadAgentState("a1")
	if err != nil || !ok {
		t.Fatalf("ReadAgentState: %v %v", ok, err)
	}
	if s.SessionID != "s1" {
		t.Errorf("current session = %q, want s1", s.SessionID)
	}
}

// Rotating the session hands the same agent a fresh harness session id: a new
// UUID, never the agent id again, with everything else (pane, group, submit
// path) untouched.
func TestRotateAgentSessionKeepsTheAgentLosesTheSession(t *testing.T) {
	isolateState(t)
	if err := WriteAgentState(AgentState{
		ID: "a1", Type: "scout", Pane: "p1", Tab: "t1",
		ParentID: "parent", GroupID: "g1", Deliver: true, SessionID: "s1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "a1", Status: "done", Report: "r"}); err != nil {
		t.Fatal(err)
	}
	next, err := RotateAgentSession("a1")
	if err != nil {
		t.Fatal(err)
	}
	if next == "" || next == "s1" || next == "a1" {
		t.Errorf("rotated session = %q, want a fresh id", next)
	}
	s, ok, err := ReadAgentState("a1")
	if err != nil || !ok {
		t.Fatalf("ReadAgentState: %v %v", ok, err)
	}
	if s.SessionID != next {
		t.Errorf("current session = %q, want %q", s.SessionID, next)
	}
	if s.Pane != "p1" || s.GroupID != "g1" || !s.Deliver || s.ParentID != "parent" {
		t.Errorf("rotation must keep the rest of the state: %+v", s)
	}
	sub, ok, err := ReadAgentSubmit("a1")
	if err != nil || !ok || sub.Report != "r" {
		t.Errorf("rotation must keep the submit: %+v %v %v", sub, ok, err)
	}
}

// A rotated id is a fresh UUID every time, so an old harness session is never
// reused by accident.
func TestRotateAgentSessionMintsFreshUUIDs(t *testing.T) {
	isolateState(t)
	if err := WriteAgentState(AgentState{ID: "a1", SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{"s1": true}
	for i := 0; i < 3; i++ {
		next, err := RotateAgentSession("a1")
		if err != nil {
			t.Fatal(err)
		}
		if len(next) != 36 || seen[next] {
			t.Errorf("rotated session = %q, want a fresh UUID", next)
		}
		seen[next] = true
	}
}

// Rotating an unknown agent is an error, not a new record.
func TestRotateAgentSessionRefusesUnknownAgents(t *testing.T) {
	isolateState(t)
	if _, err := RotateAgentSession("nobody"); err == nil {
		t.Error("rotating an unknown agent must fail")
	}
}

// A launch records the harness session it started as the agent's current
// session, so later lookups read it instead of a stale copy.
func TestScoutLaunchRecordsItsCurrentSession(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "plan")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "where is it?"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if len(ex.ids) != 1 {
		t.Fatalf("launched %d scouts, want 1", len(ex.ids))
	}
	s, ok, err := ReadAgentState(ex.ids[0])
	if err != nil || !ok {
		t.Fatalf("ReadAgentState: %v %v", ok, err)
	}
	launched := envIn(ex.lines[0], "EMBERLOOP_SESSION_ID")
	if launched == "" {
		t.Fatal("the scout launch names no harness session")
	}
	if s.SessionID != launched {
		t.Errorf("current session = %q, want the launched %q", s.SessionID, launched)
	}
}

// A fresh reviewer records its harness session; a follow-up keeps the
// current one, so a restart in between is not overwritten with the old id.
func TestWriteReviewStateKeepsTheCurrentSession(t *testing.T) {
	isolateState(t)
	p := &reviewPacket{tree: "/tree", head: "abc"}
	if err := writeReviewState("r1", "spec-reviewer", p, "parent", "pid", "g1", "pane1", "tab1"); err != nil {
		t.Fatal(err)
	}
	s, _, _ := ReadAgentState("r1")
	if s.SessionID != "r1" {
		t.Errorf("fresh reviewer session = %q, want r1", s.SessionID)
	}
	next, err := RotateAgentSession("r1")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeReviewState("r1", "spec-reviewer", p, "parent", "pid", "g1", "pane1", "tab1"); err != nil {
		t.Fatal(err)
	}
	s, _, _ = ReadAgentState("r1")
	if s.SessionID != next {
		t.Errorf("follow-up session = %q, want the rotated %q", s.SessionID, next)
	}
}

// The usage tailer follows the agent's current session: after a restart
// it tails the new transcript, and the rows keep accumulating on the
// agent id.
func TestTailUsageFollowsTheCurrentSession(t *testing.T) {
	isolateState(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A private state dir: usage rows never collide with another test's
	// agent, even when the environment shares one database.
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	t.Setenv("EMBERLOOP_HARNESS", "pig")
	t.Setenv("EMBERLOOP_SESSION_ID", "sess-old")
	t.Setenv("EMBERLOOP_AGENT_ID", "agent-rot")
	cfg, ok := UsageTailFromEnv()
	if !ok {
		t.Fatal("tail config must resolve from the session environment")
	}
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, "sess-old.jsonl"), pigLine("r1", 7))
	if err := TailUsageFile(cfg); err != nil {
		t.Fatal(err)
	}
	if got := readUsageLog(t, "agent-rot"); len(got) != 1 {
		t.Fatalf("rows after the old session = %d, want 1", len(got))
	}
	// The restart gives the same agent a new harness session id. The
	// tailer still holds the old config, the way a running emberloop-mcp does.
	if err := WriteAgentState(AgentState{ID: "agent-rot", SessionID: "sess-new"}); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, "sess-new.jsonl"), pigLine("r2", 9)+pigLine("r3", 11))
	if err := TailUsageFile(cfg); err != nil {
		t.Fatal(err)
	}
	got := readUsageLog(t, "agent-rot")
	if len(got) != 3 {
		t.Fatalf("rows after the restart = %d, want 3 on the agent id", len(got))
	}
}

// Reply waiting reads the agent's current session: after a restart the
// reply comes from the new transcript, not the stale launch id.
func TestReadOnlyReplyFollowsTheCurrentSession(t *testing.T) {
	isolateState(t)
	oldTimeout := readOnlyTimeout
	readOnlyTimeout = 5 * time.Second
	t.Cleanup(func() { readOnlyTimeout = oldTimeout })
	dir := t.TempDir()
	old := Session{Role: "review", ID: "sess-old", AgentID: "agent-1", Dir: dir}
	agent := readOnlyAgent{Pane: "p1", Harness: "pig", Session: old}
	// Only the restarted session has a transcript, carrying the answer.
	rec, _ := json.Marshal(map[string]any{"type": "message", "message": map[string]any{
		"role": "assistant", "stopReason": "stop",
		"content": []any{map[string]any{"type": "text", "text": "fresh answer"}}}})
	writeLines(t, filepath.Join(dir, "sess-new.jsonl"), string(rec)+"\n")
	if err := WriteAgentState(AgentState{ID: "agent-1", SessionID: "sess-new"}); err != nil {
		t.Fatal(err)
	}
	stub := &recorder{reply: func(string, []string) Result { return Result{Code: 1} }}
	got, err := agent.reply(stub.run, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh answer" {
		t.Errorf("reply = %q, want the restarted session's answer", got)
	}
	if size := agent.transcriptSize(); size <= 0 {
		t.Errorf("transcript size = %d, want the restarted transcript's size", size)
	}
}

// A worker's record names its harness session too, so the rotation moves
// it while the worker id stays the key.
func TestRotateAgentSessionFollowsTheWorkerRecord(t *testing.T) {
	isolateState(t)
	if err := save(map[string]Worker{"w1": {ID: "w1", SessionID: "s1", Pane: "p1"}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "w1", Type: "implement", SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	next, err := RotateAgentSession("w1")
	if err != nil {
		t.Fatal(err)
	}
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if store["w1"].SessionID != next {
		t.Errorf("worker session = %q, want the rotated %q", store["w1"].SessionID, next)
	}
	if store["w1"].Pane != "p1" {
		t.Errorf("rotation must keep the worker record: %+v", store["w1"])
	}
}
