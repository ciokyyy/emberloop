package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Seam (ticket 08): the herdr plugin's signal path through the existing Exec
// seam. A key press calls RequestCompress with the focused pane; a press
// during a running tool call queues (the header shows it) and a second press
// forces the compress now. No test touches live herdr, a live agent or the
// real home.

// A queued compress marks the agent: the header strip reads the mark.
func TestCompressQueuedMarkIsPerAgent(t *testing.T) {
	isolateState(t)
	if CompressQueued("w1") {
		t.Fatal("no request must read as queued")
	}
	markCompressQueued("w1")
	if !CompressQueued("w1") {
		t.Fatal("a queued request must read as queued")
	}
	if CompressQueued("w2") {
		t.Error("the mark must not leak to another agent")
	}
	clearCompressQueued("w1")
	if CompressQueued("w1") {
		t.Error("a cleared request must not read as queued")
	}
}

// A dead owner leaves no queue behind: a crashed wait must not pin the
// header on `compress queued` nor force the next press.
func TestCompressQueuedIgnoresADeadOwner(t *testing.T) {
	isolateState(t)
	markCompressQueued("w1")
	if !CompressQueued("w1") {
		t.Fatal("own request must read as queued")
	}
	if err := os.WriteFile(compressQueuedPath("w9"), []byte("99999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	if CompressQueued("w9") {
		t.Error("a dead owner's mark must read as no queue")
	}
	if _, err := os.Stat(compressQueuedPath("w9")); !os.IsNotExist(err) {
		t.Error("a dead owner's mark must be removed")
	}
}

// A live foreign owner still counts as queued: only the waiter itself tells
// a live queue from its own.
func TestCompressQueuedSeesALiveForeignOwner(t *testing.T) {
	isolateState(t)
	if err := os.MkdirAll(agentStateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compressQueuedPath("w1"), []byte(itoa(os.Getppid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if !CompressQueued("w1") {
		t.Error("a live owner's mark must read as queued")
	}
	if pid, ok := readCompressQueuePid("w1"); !ok || pid != os.Getppid() {
		t.Errorf("queue owner = %d, %v, want ppid %d", pid, ok, os.Getppid())
	}
}

// While a request waits at the boundary the header shows it.
func TestHeaderSnapshotShowsCompressQueued(t *testing.T) {
	isolateState(t)
	src := HeaderSource{AgentID: "w1", Harness: "pig"}
	snap, err := BuildHeaderSnapshot(src, compressSettings())
	if err != nil {
		t.Fatal(err)
	}
	if snap.State == HeaderQueued {
		t.Fatal("no request must not render queued")
	}
	markCompressQueued("w1")
	snap, err = BuildHeaderSnapshot(src, compressSettings())
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != HeaderQueued {
		t.Errorf("a queued request must render %q, got %q", HeaderQueued, snap.State)
	}
}

// A press at a tool boundary compresses at once through the exec seam.
func TestRequestCompressAtBoundaryCompressesAtOnce(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	oldID := writeCompressWorker(t,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`+"\n"+
			pigCallLine("s1", "a1", "call_1", "mcp__emberloop__read")+
			pigResultLine("r1", "call_1", "mcp__emberloop__read", "ok"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, err := RequestCompress(fake.run, "pane-1", compressSettings())
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if next == "" || next == oldID {
		t.Fatalf("new session = %q, want a fresh id", next)
	}
	if fake.pig != 1 || fake.ran != 1 {
		t.Errorf("one summary and one relaunch: pig=%d ran=%d", fake.pig, fake.ran)
	}
}

// A press during a running tool call queues: no summary starts, the mark is
// set, and the header would show it. The result lands, the wait ends at the
// boundary, and the pane restarts.
func TestRequestCompressMidToolCallQueuesToTheBoundary(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 5*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	oldID := writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__emberloop__bash"))
	go func() {
		time.Sleep(60 * time.Millisecond)
		root := filepath.Join(os.Getenv("HOME"), ".pig", "agent", "sessions")
		f, err := os.OpenFile(filepath.Join(root, oldID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__emberloop__bash", "ok"))
	}()
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	done := make(chan struct{})
	var next string
	var reqErr error
	go func() {
		defer close(done)
		next, reqErr = RequestCompress(fake.run, "pane-1", compressSettings())
	}()
	time.Sleep(30 * time.Millisecond)
	if !CompressQueued("w1") {
		t.Error("while the tool call runs the header must show the queue")
	}
	<-done
	if reqErr != nil {
		t.Fatalf("queued request: %v", reqErr)
	}
	if next == "" || next == oldID {
		t.Fatalf("new session = %q, want a fresh id", next)
	}
	if CompressQueued("w1") {
		t.Error("a finished request must clear the queue")
	}
}

// A second press while the first waits forces the compress now: it does not
// wait for the running tool call.
func TestRequestCompressSecondPressForces(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 5*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__emberloop__bash"))
	markCompressQueued("w1")
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	start := time.Now()
	next, err := RequestCompress(fake.run, "pane-1", compressSettings())
	if err != nil {
		t.Fatalf("forced request: %v", err)
	}
	if next == "" {
		t.Fatal("a forced request must return the new session")
	}
	if time.Since(start) > 4*time.Second {
		t.Error("a forced request must not wait out the boundary")
	}
	if fake.pig != 1 || fake.ran != 1 {
		t.Errorf("force summarizes and relaunches once: pig=%d ran=%d", fake.pig, fake.ran)
	}
	if CompressQueued("w1") {
		t.Error("a forced request must clear the queue")
	}
}

// --force compresses past a running tool call without waiting.
func TestCompressPaneForceSkipsTheBoundary(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 300*time.Millisecond
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__emberloop__bash"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, err := CompressPaneWithForce(fake.run, "pane-1", compressSettings(), true)
	if err != nil {
		t.Fatalf("forced compress: %v", err)
	}
	if next == "" {
		t.Fatal("a forced compress must return the new session")
	}
	if fake.pig != 1 || fake.ran != 1 {
		t.Errorf("force summarizes and relaunches once: pig=%d ran=%d", fake.pig, fake.ran)
	}
}

// Without --force the queue stands past any timeout: a tool call longer
// than the old boundary bound still compresses at its real boundary.
func TestCompressPaneWithoutForceWaitsPastTheOldBound(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	oldID := writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__emberloop__bash"))
	go func() {
		time.Sleep(400 * time.Millisecond)
		root := filepath.Join(os.Getenv("HOME"), ".pig", "agent", "sessions")
		f, err := os.OpenFile(filepath.Join(root, oldID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__emberloop__bash", "ok"))
	}()
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	done := make(chan struct{})
	var next string
	var reqErr error
	go func() {
		defer close(done)
		next, reqErr = CompressPaneWithForce(fake.run, "pane-1", compressSettings(), false)
	}()
	time.Sleep(200 * time.Millisecond)
	if !CompressQueued("w1") {
		t.Error("a long tool call must keep the queue standing")
	}
	<-done
	if reqErr != nil {
		t.Fatalf("queued compress past the old bound: %v", reqErr)
	}
	if next == "" || next == oldID {
		t.Fatalf("new session = %q, want a fresh id", next)
	}
}

// A forced press takes the mark over: the waiter stops as superseded instead
// of compressing twice.
func TestCompressWaiterStopsWhenForced(t *testing.T) {
	isolateState(t)
	oldPoll := pollEvery
	pollEvery = 5 * time.Millisecond
	defer func() { pollEvery = oldPoll }()
	writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__emberloop__bash"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	done := make(chan error, 1)
	go func() {
		_, err := CompressPaneWithForce(fake.run, "pane-1", compressSettings(), false)
		done <- err
	}()
	for i := 0; i < 200 && !CompressQueued("w1"); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !CompressQueued("w1") {
		t.Fatal("the waiter must mark the queue before the takeover")
	}
	clearCompressQueued("w1")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "forced press") {
			t.Fatalf("a taken-over waiter must stop as superseded, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a taken-over waiter must stop, not hang")
	}
	if fake.pig != 0 || fake.ran != 0 {
		t.Errorf("a superseded waiter must start nothing: pig=%d ran=%d", fake.pig, fake.ran)
	}
}

// The focused pane can be a claude worker: same signal path, same restart.
func TestRequestCompressCoversClaude(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldID := "sess-claude-old"
	root := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, oldID+".jsonl"),
		`{"type":"user","uuid":"u1","timestamp":"2026-10-06T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"do the thing"}}`+"\n"+
			`{"type":"assistant","uuid":"c1","timestamp":"2026-10-06T10:00:01Z","sessionId":"s1","message":{"id":"msg_1","model":"claude-test-model","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":100}}}`+"\n"+
			`{"type":"user","uuid":"c2","timestamp":"2026-10-06T10:00:02Z","sessionId":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}}`+"\n")
	if err := WriteAgentState(AgentState{ID: "c1", Type: "worker", Pane: "pane-c", Harness: "claude", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	if err := save(map[string]Worker{"c1": {ID: "c1", Pane: "pane-c", Harness: "claude", Worktree: t.TempDir(), SessionID: oldID, AgentType: "worker"}}); err != nil {
		t.Fatal(err)
	}
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, err := RequestCompress(fake.run, "pane-c", compressSettings())
	if err != nil {
		t.Fatalf("claude request: %v", err)
	}
	if next == "" || next == oldID {
		t.Fatalf("claude must restart at once, next=%q", next)
	}
	if fake.pig != 1 || fake.ran != 1 {
		t.Errorf("one summary and one relaunch: pig=%d ran=%d", fake.pig, fake.ran)
	}
	s, ok, err := ReadAgentState("c1")
	if err != nil || !ok {
		t.Fatalf("claude state: %v %v", ok, err)
	}
	if s.SessionID != next || s.Compressed != 1 {
		t.Errorf("claude state = %+v, want session %q compressed 1", s, next)
	}
}

// The focused pane can be the top-level session: no worker record, only the
// agent state the top level wrote at startup.
func TestRequestCompressCoversTopLevel(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldID := "sess-top-old"
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, oldID+".jsonl"),
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`+"\n"+
			pigCallLine("s1", "a1", "call_1", "mcp__emberloop__read")+
			pigResultLine("r1", "call_1", "mcp__emberloop__read", "ok"))
	cwd := t.TempDir()
	if err := WriteTopLevelState(TopLevelSession{AgentID: "top-1", Pane: "top-pane", Harness: "pig", Model: "m", Effort: "e", Role: "plan", Cwd: cwd, SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, err := RequestCompress(fake.run, "top-pane", compressSettings())
	if err != nil {
		t.Fatalf("top-level request: %v", err)
	}
	if next == "" || next == oldID {
		t.Fatalf("top-level must restart at once, next=%q", next)
	}
	s, ok, err := ReadAgentState("top-1")
	if err != nil || !ok {
		t.Fatalf("top-level state: %v %v", ok, err)
	}
	if s.SessionID != next || s.Compressed != 1 || s.Pane != "top-pane" {
		t.Errorf("top-level state = %+v, want session %q compressed 1", s, next)
	}
}

// A second top-level run in one pane supersedes the first: one pane holds
// one top-level record, so the key resolves it instead of dying on multiple
// agents.
func TestTopLevelStateSupersedesSamePane(t *testing.T) {
	isolateState(t)
	first := TopLevelSession{AgentID: "top-1", Pane: "top-pane", Harness: "pig", Role: "plan", Cwd: t.TempDir(), SessionID: "sess-1"}
	second := TopLevelSession{AgentID: "top-2", Pane: "top-pane", Harness: "pig", Role: "plan", Cwd: t.TempDir(), SessionID: "sess-2"}
	if err := WriteTopLevelState(first); err != nil {
		t.Fatal(err)
	}
	markCompressQueued("top-1")
	if err := WriteTopLevelState(second); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ReadAgentState("top-1"); ok {
		t.Error("the earlier run in the pane must go")
	}
	if CompressQueued("top-1") {
		t.Error("the earlier run's queue mark must go")
	}
	fake := &compressFake{t: t, alive: true, group: 4242}
	got, err := resolvePaneToAgent("top-pane", fake.run)
	if err != nil {
		t.Fatalf("one record must resolve: %v", err)
	}
	if got != "top-2" {
		t.Errorf("pane resolves to %q, want top-2", got)
	}
}
