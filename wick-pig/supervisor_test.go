package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Seams under test (pre-agreed per spec Testing Decisions):
// - Supervisor behavior through the existing Exec seam (fake exec below).
// - Temp transcript files as the fake child (no real agent or harness).
// - AgentState files for identity (same agent ID, new session ID, count).
// No test touches the real home, a live agent, or the network.

// pigCallLine is one pig assistant line making tool call id with name.
func pigCallLine(id, msgID, callID, name string) string {
	return `{"type":"message","id":"` + msgID + `","timestamp":"2026-10-06T10:00:01Z",` +
		`"message":{"role":"assistant","provider":"p","model":"m",` +
		`"usage":{"input":10,"output":1,"cacheRead":0,"cacheWrite":0},` +
		`"stopReason":"toolUse","content":[{"type":"toolCall","id":"` + callID + `","name":"` + name + `"}]}}` + "\n"
}

// pigResultLine is one pig toolResult line answering callID.
func pigResultLine(msgID, callID, name, text string) string {
	return `{"type":"message","id":"` + msgID + `","timestamp":"2026-10-06T10:00:02Z",` +
		`"message":{"role":"toolResult","toolCallId":"` + callID + `","toolName":"` + name + `",` +
		`"content":[{"type":"text","text":"` + text + `"}]}}` + "\n"
}

// A compress signal during a running tool call waits: the boundary is met
// only once every call has its result.
func TestBoundaryWaitsForRunningToolCall(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	writeLines(t, path, pigCallLine("s1", "a1", "call_1", "mcp__wick__bash"))

	oldPoll, oldTimeout := pollEvery, restartBoundaryTimeout
	pollEvery, restartBoundaryTimeout = 5*time.Millisecond, 2*time.Second
	defer func() { pollEvery, restartBoundaryTimeout = oldPoll, oldTimeout }()

	// The fake child finishes its tool call shortly after the wait starts.
	go func() {
		time.Sleep(30 * time.Millisecond)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__wick__bash", "done"))
	}()

	if err := WaitForToolBoundary("pig", path); err != nil {
		t.Fatalf("boundary wait: %v", err)
	}
}

// restartFake is the fake child behind the exec seam: herdr pane answers,
// temp transcripts are the child's output. runErr fails the relaunch;
// promptErr plus typeErr fail late delivery (agent prompt refused, typing
// refused too, so the shared path reports).
type restartFake struct {
	t         *testing.T
	alive     bool
	group     int
	ran       []string
	prompted  []string
	closed    []string
	renamed   []string
	runErr    bool
	promptErr bool
	typeErr   bool
}

func (f *restartFake) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	if cmd == "kill" {
		f.alive = false
		return Result{}, nil
	}
	if cmd != herdrBin() {
		return Result{Stdout: "{}"}, nil
	}
	if len(args) < 2 {
		return Result{Stdout: "{}"}, nil
	}
	switch args[0] + " " + args[1] {
	case "pane process-info":
		fg := `[]`
		group := 0
		if f.alive {
			fg = `[{"pid":` + itoa(f.group) + `}]`
			group = f.group
		}
		return Result{Stdout: `{"result":{"process_info":{"shell_pid":111,"foreground_process_group_id":` + itoa(group) + `,"foreground_processes":` + fg + `}}}`}, nil
	case "pane run":
		f.ran = append(f.ran, args[3])
		if f.runErr {
			return Result{Code: 1, Stderr: "launch failed"}, nil
		}
		f.alive = true
		return Result{Stdout: "{}"}, nil
	case "agent wait":
		return Result{Stdout: "{}"}, nil
	case "agent prompt":
		f.prompted = append(f.prompted, args[3])
		if f.promptErr {
			return Result{Code: 1, Stdout: `{"error":{"code":"agent_not_ready"}}`}, nil
		}
		return Result{Stdout: "{}"}, nil
	case "pane close", "tab close":
		f.closed = append(f.closed, args[0]+" "+args[1])
		return Result{Stdout: "{}"}, nil
	case "pane rename":
		f.renamed = append(f.renamed, args[2]+"="+args[3])
		return Result{Stdout: "{}"}, nil
	case "pane send-text":
		if f.typeErr {
			return Result{Code: 1, Stderr: "send-text failed"}, nil
		}
		return Result{Stdout: "{}"}, nil
	case "pane send-keys":
		if f.typeErr {
			return Result{Code: 1, Stderr: "send-keys failed"}, nil
		}
		return Result{Stdout: "{}"}, nil
	}
	return Result{Stdout: "{}"}, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// The restart keeps the pane and the agent, issues a new harness session,
// and counts the handoff: same pane, same agent ID, new session ID,
// `compressed xN` incremented. The pane stays open: nothing closes it.
func TestRestartKeepsPaneAndAgentIssuesNewSession(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()

	dir := t.TempDir()
	oldID := "sess-old"
	writeLines(t, filepath.Join(dir, oldID+".jsonl"), pigCallLine("s", "a1", "call_1", "mcp__wick__bash")+pigResultLine("r1", "call_1", "mcp__wick__bash", "ok"))
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "pane-1", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	if err := save(map[string]Worker{"w1": {ID: "w1", SessionID: oldID, Pane: "pane-1"}}); err != nil {
		t.Fatal(err)
	}

	fake := &restartFake{t: t, alive: true, group: 4242}
	handoff := "handoff text\nsecond line"
	template := Session{Role: "agent", Cwd: dir, Label: "w1", AgentID: "w1", AgentType: "worker"}
	launchFor := func(newID string) (Session, Launch, error) {
		sess := template
		sess.ID = newID
		sess.Prompt = handoff
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	}
	next, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-1", AgentID: "w1", OldSessionID: oldID, SessionDir: dir, Handoff: handoff}, launchFor)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if next == "" || next == oldID || next == "w1" {
		t.Errorf("new session = %q, want a fresh id", next)
	}
	if len(next) != 36 {
		t.Errorf("new session = %q, want a fresh UUID", next)
	}
	s, ok, err := ReadAgentState("w1")
	if err != nil || !ok {
		t.Fatalf("ReadAgentState: %v %v", ok, err)
	}
	if s.SessionID != next {
		t.Errorf("current session = %q, want %q", s.SessionID, next)
	}
	if s.Pane != "pane-1" || s.ID != "w1" {
		t.Errorf("restart must keep pane and agent, got %+v", s)
	}
	if s.Compressed != 1 {
		t.Errorf("compressed = %d, want 1", s.Compressed)
	}
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if store["w1"].SessionID != next {
		t.Errorf("worker session = %q, want %q", store["w1"].SessionID, next)
	}
	if len(fake.ran) != 1 {
		t.Fatalf("want exactly one pane run, got %d", len(fake.ran))
	}
	cmd := launchCmd(fake.ran[0])
	found := false
	for _, a := range cmd.Args {
		if a == handoff {
			found = true
		}
	}
	if !found {
		t.Errorf("new child must receive the handoff as its first message, got %q", cmd.Args)
	}
	if len(fake.closed) != 0 {
		t.Errorf("pane stays open: closes = %v", fake.closed)
	}

	// A second handoff counts again on the same pane and agent.
	writeLines(t, filepath.Join(dir, next+".jsonl"), pigCallLine("s", "a2", "call_9", "mcp__wick__read")+pigResultLine("r9", "call_9", "mcp__wick__read", "ok"))
	next2, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-1", AgentID: "w1", OldSessionID: next, SessionDir: dir, Handoff: "second handoff"}, func(id string) (Session, Launch, error) {
		sess := template
		sess.ID = id
		sess.Prompt = "second handoff"
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	})
	if err != nil {
		t.Fatalf("second restart: %v", err)
	}
	s, _, _ = ReadAgentState("w1")
	if s.SessionID != next2 || s.Compressed != 2 {
		t.Errorf("after two restarts = %+v, want session %q compressed 2", s, next2)
	}
}

// A claude child receives the handoff late as the message itself, with
// newlines intact, through the shared DeliverPrompt path.
func TestRestartDeliversHandoffLateForClaude(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-old-claude"
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "pane-c", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: false, group: 0}
	handoff := "handoff line one\nhandoff line two"
	var launched Session
	next, err := RestartPane(fake.run, PaneRestart{Harness: "claude", Pane: "pane-c", AgentID: "c1", OldSessionID: oldID, SessionDir: dir, Handoff: handoff}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "review", Cwd: dir, AgentID: "c1", AgentType: "scout", ID: id, Prompt: handoff}
		launch, err := PrepareLaunch("claude", sess)
		launched = sess
		return sess, launch, err
	})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if len(fake.prompted) != 1 {
		t.Fatalf("late handoff must prompt exactly once, got %q", fake.prompted)
	}
	if fake.prompted[0] != handoff {
		t.Errorf("late handoff must arrive verbatim, got %q want %q", fake.prompted[0], handoff)
	}
	if _, err := os.Stat(mcpConfigPath(launched)); !os.IsNotExist(err) {
		t.Errorf("the relaunch must drop its MCP config, stat err = %v", err)
	}
	s, _, _ := ReadAgentState("c1")
	if s.SessionID != next || s.Compressed != 1 {
		t.Errorf("after claude restart = %+v, want session %q compressed 1", s, next)
	}
	if len(fake.closed) != 0 {
		t.Errorf("pane stays open: closes = %v", fake.closed)
	}
}

// A failed relaunch keeps the pane open and leaves the agent where it was:
// same session, same count, nothing closed.
func TestFailedRelaunchKeepsPaneOpen(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-old-fail"
	writeLines(t, filepath.Join(dir, oldID+".jsonl"), pigCallLine("s", "a1", "call_1", "mcp__wick__bash")+pigResultLine("r1", "call_1", "mcp__wick__bash", "ok"))
	if err := WriteAgentState(AgentState{ID: "w9", Type: "worker", Pane: "pane-9", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: true, group: 999, runErr: true}
	_, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-9", AgentID: "w9", OldSessionID: oldID, SessionDir: dir, Handoff: "handoff"}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "agent", Cwd: dir, Label: "w9", AgentID: "w9", ID: id, Prompt: "handoff"}
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	})
	if err == nil {
		t.Fatal("a failed pane run must fail the restart")
	}
	if len(fake.closed) != 0 {
		t.Errorf("a failed relaunch must not close the pane: closes = %v", fake.closed)
	}
	s, _, _ := ReadAgentState("w9")
	if s.SessionID != oldID || s.Compressed != 0 {
		t.Errorf("a failed relaunch must leave identity alone, got %+v", s)
	}
}

// The top-level session restarts the same way with no prior state: the first
// compress starts its record on the same pane and agent.
func TestRestartTopLevelWithoutPriorState(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	fake := &restartFake{t: t, alive: false, group: 0}
	handoff := "top-level handoff"
	next, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "top-pane", AgentID: "top-agent", OldSessionID: "sess-never-wrote", SessionDir: dir, Handoff: handoff}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "plan", Cwd: dir, AgentID: "top-agent", ID: id, Prompt: handoff}
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	s, ok, err := ReadAgentState("top-agent")
	if err != nil || !ok {
		t.Fatalf("top-level restart must start agent state: %v %v", ok, err)
	}
	if s.SessionID != next || s.Pane != "top-pane" || s.Compressed != 1 {
		t.Errorf("top-level state = %+v, want pane top-pane session %q compressed 1", s, next)
	}
	if len(fake.ran) != 1 || len(fake.closed) != 0 {
		t.Errorf("top-level relaunch runs once and closes nothing: ran=%d closed=%v", len(fake.ran), fake.closed)
	}
}

// The child stops only after the boundary: the restart succeeds once the
// fake child's outstanding result lands.
func TestRestartStopsOnlyAfterTheBoundary(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-racing"
	path := filepath.Join(dir, oldID+".jsonl")
	writeLines(t, path, pigCallLine("s", "a1", "call_1", "mcp__wick__bash"))
	if err := WriteAgentState(AgentState{ID: "wr", Type: "worker", Pane: "pane-r", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: true, group: 31337}
	landed := make(chan time.Time, 1)
	go func() {
		time.Sleep(40 * time.Millisecond)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__wick__bash", "ok"))
		landed <- time.Now()
	}()
	handoff := "racing handoff"
	if _, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-r", AgentID: "wr", OldSessionID: oldID, SessionDir: dir, Handoff: handoff}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "agent", Cwd: dir, Label: "wr", AgentID: "wr", ID: id, Prompt: handoff}
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	select {
	case <-landed:
	default:
		t.Fatal("the fake child never wrote its result")
	}
	if len(fake.ran) != 1 {
		t.Fatalf("want one pane run after the boundary, got %d", len(fake.ran))
	}
}

// A launchFor error leaves the pane with the child stopped and the state
// untouched: the pane stays open at its shell for a retry.
func TestLaunchForErrorLeavesStateUntouched(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-launch-err"
	writeLines(t, filepath.Join(dir, oldID+".jsonl"), pigCallLine("s", "a1", "call_1", "mcp__wick__bash")+pigResultLine("r1", "call_1", "mcp__wick__bash", "ok"))
	if err := WriteAgentState(AgentState{ID: "wl", Type: "worker", Pane: "pane-l", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: true, group: 5150}
	_, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-l", AgentID: "wl", OldSessionID: oldID, SessionDir: dir, Handoff: "h"}, func(string) (Session, Launch, error) {
		return Session{}, Launch{}, fmt.Errorf("no launch today")
	})
	if err == nil {
		t.Fatal("a launchFor error must fail the restart")
	}
	if len(fake.ran) != 0 || len(fake.closed) != 0 {
		t.Errorf("no relaunch and no close on a launchFor error: ran=%d closed=%v", len(fake.ran), fake.closed)
	}
	s, _, _ := ReadAgentState("wl")
	if s.SessionID != oldID || s.Compressed != 0 {
		t.Errorf("a launchFor error must leave identity alone, got %+v", s)
	}
}

// Guards refuse a restart with no agent, no pane, no handoff, or no harness
// before touching the pane.
func TestRestartGuardsRefuseBeforeTouchingThePane(t *testing.T) {
	isolateState(t)
	for _, req := range []PaneRestart{
		{Harness: "pig", Pane: "p", AgentID: "", Handoff: "h"},
		{Harness: "pig", Pane: "", AgentID: "a", Handoff: "h"},
		{Harness: "pig", Pane: "p", AgentID: "a", Handoff: "  "},
		{Harness: "nope", Pane: "p", AgentID: "a", Handoff: "h"},
	} {
		fake := &restartFake{t: t, alive: true, group: 1}
		if _, err := RestartPane(fake.run, req, func(id string) (Session, Launch, error) {
			return Session{}, Launch{Line: "true"}, nil
		}); err == nil {
			t.Errorf("restart %+v must fail", req)
		}
		if len(fake.ran) != 0 || len(fake.closed) != 0 {
			t.Errorf("a refused restart must not touch the pane: %+v ran=%d closed=%v", req, len(fake.ran), fake.closed)
		}
	}
}

// A tool that never answers ends the wait: the child is not stopped and no
// relaunch runs.
func TestBoundaryTimeoutStopsNothing(t *testing.T) {
	isolateState(t)
	oldPoll, oldTimeout := pollEvery, restartBoundaryTimeout
	pollEvery, restartBoundaryTimeout = 5*time.Millisecond, 25*time.Millisecond
	defer func() { pollEvery, restartBoundaryTimeout = oldPoll, oldTimeout }()
	if err := WaitForToolBoundary("pig", filepath.Join(t.TempDir(), "absent.jsonl")); err != nil {
		t.Fatalf("a missing transcript is quiet, got %v", err)
	}
	dir := t.TempDir()
	oldID := "sess-hung"
	writeLines(t, filepath.Join(dir, oldID+".jsonl"), pigCallLine("s", "a1", "call_1", "mcp__wick__bash"))
	if err := WriteAgentState(AgentState{ID: "wh", Type: "worker", Pane: "pane-h", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: true, group: 717}
	if _, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-h", AgentID: "wh", OldSessionID: oldID, SessionDir: dir, Handoff: "h"}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "agent", Cwd: dir, Label: "wh", AgentID: "wh", ID: id, Prompt: "h"}
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	}); err == nil {
		t.Fatal("a hung tool call must fail the restart")
	}
	if len(fake.ran) != 0 || len(fake.closed) != 0 {
		t.Errorf("a timed-out boundary must stop nothing: ran=%d closed=%v", len(fake.ran), fake.closed)
	}
	if !fake.alive {
		t.Error("a timed-out boundary must not stop the child")
	}
	s, _, _ := ReadAgentState("wh")
	if s.SessionID != oldID || s.Compressed != 0 {
		t.Errorf("a timed-out boundary must leave identity alone, got %+v", s)
	}
}

// Once the new child is live its session is the answer even when late
// delivery fails: the state already names it and the pane stays open.
func TestLateDeliveryFailureKeepsTheNewSession(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-late-fail"
	if err := WriteAgentState(AgentState{ID: "cl", Type: "scout", Pane: "pane-cl", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: false, group: 0, promptErr: true, typeErr: true}
	handoff := "late handoff\ntwo lines"
	next, err := RestartPane(fake.run, PaneRestart{Harness: "claude", Pane: "pane-cl", AgentID: "cl", OldSessionID: oldID, SessionDir: dir, Handoff: handoff}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "review", Cwd: dir, AgentID: "cl", AgentType: "scout", ID: id, Prompt: handoff}
		launch, err := PrepareLaunch("claude", sess)
		return sess, launch, err
	})
	if err == nil {
		t.Fatal("a failed late delivery must report")
	}
	if next == "" || next == oldID {
		t.Fatalf("the pane runs the new session, got %q", next)
	}
	s, _, _ := ReadAgentState("cl")
	if s.SessionID != next || s.Compressed != 1 {
		t.Errorf("state must name the live session: %+v want %q compressed 1", s, next)
	}
	if len(fake.ran) != 1 || len(fake.closed) != 0 {
		t.Errorf("one relaunch, nothing closed: ran=%d closed=%v", len(fake.ran), fake.closed)
	}
}

// Usage, submits and gate state stay keyed by the agent: the restart moves
// only the harness session and the count.
func TestRestartKeepsSubmitOnTheAgent(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-submit"
	writeLines(t, filepath.Join(dir, oldID+".jsonl"), pigCallLine("s", "a1", "call_1", "mcp__wick__bash")+pigResultLine("r1", "call_1", "mcp__wick__bash", "ok"))
	if err := WriteAgentState(AgentState{ID: "ws", Type: "worker", Pane: "pane-s", SessionID: oldID, Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "ws", Type: "worker", Status: "done", Report: "r"}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: false, group: 0}
	if _, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-s", AgentID: "ws", OldSessionID: oldID, SessionDir: dir, Handoff: "h"}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "agent", Cwd: dir, Label: "ws", AgentID: "ws", ID: id, Prompt: "h"}
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	sub, ok, err := ReadAgentSubmit("ws")
	if err != nil || !ok || sub.Report != "r" {
		t.Errorf("restart must keep the submit on the agent: %+v %v %v", sub, ok, err)
	}
}

// A restart re-asserts the pane name for the agent's type, so the herdr
// sidebar agrees with the header strip chip after every handoff.
func TestRestartReassertsPaneName(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	dir := t.TempDir()
	oldID := "sess-rename"
	writeLines(t, filepath.Join(dir, oldID+".jsonl"), pigCallLine("s", "a1", "call_1", "mcp__wick__read")+pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	if err := WriteAgentState(AgentState{ID: "wn", Type: "worker", Pane: "pane-n", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &restartFake{t: t, alive: true, group: 4242}
	if _, err := RestartPane(fake.run, PaneRestart{Harness: "pig", Pane: "pane-n", AgentID: "wn", OldSessionID: oldID, SessionDir: dir, Handoff: "h"}, func(id string) (Session, Launch, error) {
		sess := Session{Role: "agent", Cwd: dir, Label: "wn", AgentID: "wn", ID: id, Prompt: "h"}
		launch, err := PrepareLaunch("pig", sess)
		return sess, launch, err
	}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if len(fake.renamed) != 1 || fake.renamed[0] != "pane-n=implementer" {
		t.Errorf("restart must rename the pane for the agent type, got %v", fake.renamed)
	}
}
