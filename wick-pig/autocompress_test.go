package wick

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/wick/tools"
)

// Seams under test (pre-agreed per spec Testing Decisions):
// - Supervisor behavior through the existing exec seam (the compressFake
//   below: fake pig one-off, fake herdr pane run, temp transcripts).
// - Context core as a pure function (DecideAutoCompress over hand-built
//   snapshots; expected values come from the ticket).
// No test touches the real home, a live agent, or the network.

// autoSettings names pig-cheap as the pig summarizer with a 100k threshold,
// so a 300k hand-built transcript crosses it by construction. The model's
// window override holds the transcript the way the settings CLI requires.
func autoSettings() tools.Settings {
	return tools.Settings{Context: &tools.ContextSettings{
		CompressThreshold: 100000,
		CompressModels:    map[string]string{"pig": "pig-cheap"},
		Windows:           map[string]int64{"pig-cheap": 1000000},
	}}
}

// pigBigUsageLine is one pig assistant line reporting input tokens and
// making one tool call.
func pigBigUsageLine(msgID string, input int64, callID, name string) string {
	return `{"type":"message","id":"` + msgID + `","timestamp":"2026-10-06T10:00:01Z",` +
		`"message":{"role":"assistant","provider":"p","model":"m",` +
		`"usage":{"input":` + itoa(int(input)) + `,"output":1,"cacheRead":0,"cacheWrite":0},` +
		`"stopReason":"toolUse","content":[{"type":"toolCall","id":"` + callID + `","name":"` + name + `"}]}}` + "\n"
}

// overThresholdIdle is a session past the 100k threshold at a boundary: one
// finished call, nothing outstanding.
func overThresholdIdle() string {
	return `{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}` + "\n" +
		pigBigUsageLine("a1", 300000, "call_1", "mcp__wick__read") +
		pigResultLine("r1", "call_1", "mcp__wick__read", "ok")
}

// overThresholdOpen is a session past the threshold with call_1 still
// running: the trigger must wait for its result.
func overThresholdOpen() string {
	return `{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}` + "\n" +
		pigBigUsageLine("a1", 300000, "call_1", "mcp__wick__read")
}

// underThreshold is a quiet session: nothing to compress.
func underThreshold() string {
	return `{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}` + "\n" +
		pigBigUsageLine("a1", 5000, "call_1", "mcp__wick__read") +
		pigResultLine("r1", "call_1", "mcp__wick__read", "ok")
}

// The trigger fires over the threshold at a boundary, waits while a tool
// call runs, stays off when disabled, without a model, below the threshold,
// and inside the cooldown. The first run never cools down.
func TestDecideAutoCompress(t *testing.T) {
	over := ContextSnapshot{ContextTokens: 300000, CompletedCalls: 6}
	open := ContextSnapshot{ContextTokens: 300000, Outstanding: 1, CompletedCalls: 6}
	under := ContextSnapshot{ContextTokens: 5000, CompletedCalls: 1}
	fresh := ContextSnapshot{ContextTokens: 300000}
	s := autoSettings()

	if d := DecideAutoCompress(over, s, "pig", 0); !d.Compress || d.Wait {
		t.Errorf("over at boundary = %+v, want compress", d)
	}
	if d := DecideAutoCompress(open, s, "pig", 0); d.Compress || !d.Wait {
		t.Errorf("over with a running call = %+v, want wait without compress", d)
	}
	if d := DecideAutoCompress(under, s, "pig", 0); d.Compress || d.Wait {
		t.Errorf("under threshold = %+v, want neither", d)
	}
	off := autoSettings()
	no := false
	off.Context.AutoCompress = &no
	if d := DecideAutoCompress(over, off, "pig", 0); d.Compress || d.Wait {
		t.Errorf("disabled = %+v, want neither", d)
	}
	if d := DecideAutoCompress(over, tools.Settings{}, "pig", 0); d.Compress || d.Wait {
		t.Errorf("no model = %+v, want neither", d)
	}
	if d := DecideAutoCompress(fresh, s, "pig", 0); !d.Compress {
		t.Errorf("first run with no calls yet = %+v, want compress (no cooldown before the first handoff)", d)
	}
	cool := ContextSnapshot{ContextTokens: 300000, CompletedCalls: 2}
	if d := DecideAutoCompress(cool, s, "pig", 1); d.Compress || d.Wait {
		t.Errorf("two calls after a compress = %+v, want cooldown silence", d)
	}
	past := ContextSnapshot{ContextTokens: 300000, CompletedCalls: 9}
	if d := DecideAutoCompress(past, s, "pig", 1); !d.Compress {
		t.Errorf("nine calls after a compress = %+v, want compress", d)
	}
}

// An idle session past the threshold compresses on its own: one summary,
// one relaunch, same pane and agent on a fresh session, and the handoff
// ends with the continue line so work resumes without the user.
func TestMaybeAutoCompressIdleSession(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	writeCompressWorker(t, overThresholdIdle())
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, did, err := MaybeAutoCompress(fake.run, "pane-1", autoSettings())
	if err != nil {
		t.Fatalf("auto compress: %v", err)
	}
	if !did || next == "" {
		t.Fatalf("did = %v next = %q, want a compress", did, next)
	}
	if fake.pig != 1 || fake.ran != 1 || fake.closed != 0 {
		t.Errorf("one summary, one relaunch, no close: pig=%d ran=%d closed=%d", fake.pig, fake.ran, fake.closed)
	}
	s, _, _ := ReadAgentState("w1")
	if s.SessionID != next || s.Compressed != 1 {
		t.Errorf("state = %+v, want session %q compressed 1", s, next)
	}
	handoff, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "compress", "w1", "sess-compress-old-handoff.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(handoff)), ContinueLine) {
		t.Errorf("auto handoff lacks the continue line:\n%s", handoff)
	}
	if st, _, _ := ReadAgentState("w1"); st.Compressing {
		t.Error("compressing mark stays set after the restart")
	}
}

// Crossing the threshold during a tool call waits: the compress starts at
// the boundary once the result lands.
func TestMaybeAutoCompressWaitsForTheBoundary(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	oldID := writeCompressWorker(t, overThresholdOpen())
	go func() {
		time.Sleep(40 * time.Millisecond)
		root := filepath.Join(os.Getenv("HOME"), ".pig", "agent", "sessions")
		f, err := os.OpenFile(filepath.Join(root, oldID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	}()
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, did, err := MaybeAutoCompress(fake.run, "pane-1", autoSettings())
	if err != nil {
		t.Fatalf("auto compress: %v", err)
	}
	if !did || next == "" || next == oldID {
		t.Fatalf("did = %v next = %q, want a fresh session after the boundary", did, next)
	}
}

// Below the threshold, switched off, without a model, or inside the cooldown
// the pane is never touched: no summary, no stop, no relaunch, no error.
// The missing model stays silent here because the header already warns.
func TestMaybeAutoCompressStaysOut(t *testing.T) {
	for _, tc := range []struct {
		name       string
		transcript string
		settings   func() tools.Settings
		compress   int
	}{
		{"below threshold", underThreshold(), autoSettings, 0},
		{"disabled", overThresholdIdle(), func() tools.Settings {
			s := autoSettings()
			no := false
			s.Context.AutoCompress = &no
			return s
		}, 0},
		{"no model", overThresholdIdle(), func() tools.Settings {
			return tools.Settings{Context: &tools.ContextSettings{CompressThreshold: 100000}}
		}, 0},
		{"cooldown", overThresholdIdle(), autoSettings, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateState(t)
			writeCompressWorker(t, tc.transcript)
			if tc.compress > 0 {
				s, ok, err := ReadAgentState("w1")
				if err != nil || !ok {
					t.Fatal(err)
				}
				s.Compressed = tc.compress
				if err := WriteAgentState(s); err != nil {
					t.Fatal(err)
				}
			}
			fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
			next, did, err := MaybeAutoCompress(fake.run, "pane-1", tc.settings())
			if err != nil {
				t.Fatalf("a refused auto-compress must not error: %v", err)
			}
			if did || next != "" {
				t.Errorf("did = %v next = %q, want silence", did, next)
			}
			if fake.pig != 0 || fake.ran != 0 || fake.closed != 0 {
				t.Errorf("refusal must touch nothing: pig=%d ran=%d closed=%d", fake.pig, fake.ran, fake.closed)
			}
			if !fake.alive {
				t.Error("refusal must not stop the child")
			}
		})
	}
}

// autoObserveFake records whether the agent reads as mid-compress while the
// pig one-off runs: the header's `compressing…` state during the work.
type autoObserveFake struct {
	compressFake
	observed bool
}

func (f *autoObserveFake) run(cmd string, args []string, o ExecOpts) (Result, error) {
	if cmd == "pig" {
		if st, ok, _ := ReadAgentState("w1"); ok && st.Compressing {
			f.observed = true
		}
	}
	return f.compressFake.run(cmd, args, o)
}

// The header reads `compressing…` while the summary runs: the mark is set
// around the work, not only after it.
func TestAutoCompressMarksCompressingDuringWork(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	writeCompressWorker(t, overThresholdIdle())
	fake := &autoObserveFake{compressFake: compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}}
	if _, _, err := MaybeAutoCompress(fake.run, "pane-1", autoSettings()); err != nil {
		t.Fatalf("auto compress: %v", err)
	}
	if !fake.observed {
		t.Error("the agent never read as compressing while the summarizer ran")
	}
}

// One sweep compresses the pane past its threshold and leaves the quiet one
// alone; a failing pane never stops the pass.
func TestSweepAutoCompress(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, "sess-over.jsonl"), overThresholdIdle())
	writeLines(t, filepath.Join(root, "sess-under.jsonl"), underThreshold())
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "pane-1", SessionID: "sess-over"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "w2", Type: "worker", Pane: "pane-2", SessionID: "sess-under"}); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := save(map[string]Worker{
		"w1": {ID: "w1", Pane: "pane-1", Harness: "pig", Worktree: wt, SessionID: "sess-over", AgentType: "worker"},
		"w2": {ID: "w2", Pane: "pane-2", Harness: "pig", Worktree: wt, SessionID: "sess-under", AgentType: "worker"},
	}); err != nil {
		t.Fatal(err)
	}
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	done := SweepAutoCompress(fake.run, autoSettings())
	if len(done) != 1 || done[0] != "pane-1" {
		t.Errorf("sweep = %v, want [pane-1]", done)
	}
	if s, _, _ := ReadAgentState("w2"); s.Compressed != 0 {
		t.Errorf("quiet pane compressed: %+v", s)
	}
}

// The supervisor loop compresses a pane past its threshold without a user
// key, then stops when told.
func TestSuperviseAutoCompress(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldAuto := pollEvery, paneStopWait, autoPollEvery
	pollEvery, paneStopWait, autoPollEvery = 5*time.Millisecond, 50*time.Millisecond, 10*time.Millisecond
	defer func() { pollEvery, paneStopWait, autoPollEvery = oldPoll, oldWait, oldAuto }()
	writeCompressWorker(t, overThresholdIdle())
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		SuperviseAutoCompress(fake.run, autoSettings, stop)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s, _, _ := ReadAgentState("w1")
		if s.Compressed == 1 {
			break
		}
		if !time.Now().Before(deadline) {
			close(stop)
			<-done
			t.Fatal("the supervisor never compressed the pane past its threshold")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	<-done
}

// writeAutoWorker records one worker under its own ids with a transcript
// under the harness session root, so tests isolate panes without sharing
// the failure map.
func writeAutoWorker(t *testing.T, agentID, pane, sessionID, transcript string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, sessionID+".jsonl"), transcript)
	if err := WriteAgentState(AgentState{ID: agentID, Type: "worker", Pane: pane, SessionID: sessionID}); err != nil {
		t.Fatal(err)
	}
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if store == nil {
		store = map[string]Worker{}
	}
	store[agentID] = Worker{ID: agentID, Pane: pane, Harness: "pig", Worktree: t.TempDir(), SessionID: sessionID, AgentType: "worker"}
	if err := save(store); err != nil {
		t.Fatal(err)
	}
}

// The claim keeps one pane on one compress: the second claimant reports
// false while the mark is live, true again after the clear, and steals a
// stale mark left by a crashed owner.
func TestClaimCompressingSingleFlight(t *testing.T) {
	isolateState(t)
	if err := WriteAgentState(AgentState{ID: "w1"}); err != nil {
		t.Fatal(err)
	}
	claimed, err := ClaimCompressing("w1")
	if err != nil || !claimed {
		t.Fatalf("first claim = %v %v, want true", claimed, err)
	}
	if claimed, _ := ClaimCompressing("w1"); claimed {
		t.Error("second claim on a live mark reports true, want false")
	}
	if err := SetAgentCompressing("w1", false); err != nil {
		t.Fatal(err)
	}
	if claimed, _ := ClaimCompressing("w1"); !claimed {
		t.Error("claim after clear reports false, want true")
	}
	oldStale := autoCompressStaleAfter
	autoCompressStaleAfter = 50 * time.Millisecond
	defer func() { autoCompressStaleAfter = oldStale }()
	s, _, _ := ReadAgentState("w1")
	s.Compressing, s.CompressingSince = true, time.Now().Add(-time.Hour).Unix()
	if err := WriteAgentState(s); err != nil {
		t.Fatal(err)
	}
	if claimed, _ := ClaimCompressing("w1"); !claimed {
		t.Error("claim on a stale mark reports false, want a steal")
	}
}

// concFake serializes one fake exec across goroutines, so the race is on
// the claim, never the counters.
type concFake struct {
	mu    sync.Mutex
	inner *compressFake
}

func (f *concFake) run(cmd string, args []string, o ExecOpts) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inner.run(cmd, args, o)
}

// Two triggers on one pane compress exactly once: the loser sees the live
// mark (or the cooldown the winner left) and returns without doing anything.
func TestConcurrentAutoCompressSingles(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	writeCompressWorker(t, overThresholdIdle())
	fake := &concFake{inner: &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}}
	var wg sync.WaitGroup
	dids := make([]bool, 2)
	for i := range dids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, dids[i], _ = MaybeAutoCompress(fake.run, "pane-1", autoSettings())
		}(i)
	}
	wg.Wait()
	n := 0
	for _, did := range dids {
		if did {
			n++
		}
	}
	if n != 1 {
		t.Errorf("concurrent dids = %v, want exactly one compress", dids)
	}
	if fake.inner.pig != 1 || fake.inner.ran != 1 {
		t.Errorf("one summary and one relaunch: pig=%d ran=%d", fake.inner.pig, fake.inner.ran)
	}
}

// A failed summary parks the pane: the next sweep spends no second model
// call on it.
func TestAutoFailureBackoff(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	writeAutoWorker(t, "w9", "pane-9", "sess-nine", overThresholdIdle())
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: "no sections here"}
	if _, _, err := MaybeAutoCompress(fake.run, "pane-9", autoSettings()); err == nil {
		t.Fatal("bad summary must report")
	}
	if fake.pig != 1 {
		t.Fatalf("want the one failed summary, got pig=%d", fake.pig)
	}
	SweepAutoCompress(fake.run, autoSettings())
	if fake.pig != 1 {
		t.Errorf("sweep retried the failed pane: pig=%d, want 1", fake.pig)
	}
	clearAutoFailure("pane-9")
}

// A manual compress refuses while a compress owns the pane instead of
// summarizing twice over the same transcript.
func TestCompressPaneRefusesWhileCompressing(t *testing.T) {
	isolateState(t)
	writeCompressWorker(t, overThresholdIdle())
	claimed, err := ClaimCompressing("w1")
	if err != nil || !claimed {
		t.Fatalf("claim = %v %v, want true", claimed, err)
	}
	defer SetAgentCompressing("w1", false) //nolint:errcheck
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	if _, err := CompressPane(fake.run, "pane-1", compressSettings()); err == nil {
		t.Fatal("manual compress during a live mark must fail")
	} else if !strings.Contains(err.Error(), "already compressing") {
		t.Errorf("error = %v, want the already-compressing refusal", err)
	}
	if fake.pig != 0 || fake.ran != 0 {
		t.Errorf("refusal must start nothing: pig=%d ran=%d", fake.pig, fake.ran)
	}
}

// A winner that finishes between the decision and the claim rotates the
// session: the loser sees the rotation after claiming and returns without
// summarizing the stale snapshot twice.
func TestMaybeAutoCompressSkipsRotatedSession(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	oldID := writeCompressWorker(t, overThresholdOpen())
	go func() {
		time.Sleep(40 * time.Millisecond)
		s, ok, err := ReadAgentState("w1")
		if err != nil || !ok {
			return
		}
		s.SessionID, s.Compressed = "sess-winner", 1
		_ = WriteAgentState(s)
		root := filepath.Join(os.Getenv("HOME"), ".pig", "agent", "sessions")
		f, err := os.OpenFile(filepath.Join(root, oldID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	}()
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, did, err := MaybeAutoCompress(fake.run, "pane-1", autoSettings())
	if err != nil {
		t.Fatalf("a rotated session must not error: %v", err)
	}
	if did || next != "" {
		t.Errorf("did = %v next = %q, want silence after the rotation", did, next)
	}
	if fake.pig != 0 || fake.ran != 0 {
		t.Errorf("rotation must summarize nothing: pig=%d ran=%d", fake.pig, fake.ran)
	}
}

// The top-level session is no worker, but the automatic sweep still
// compresses it past the threshold: it recorded its pane, harness and
// session at startup, and the restart rebuilds its own launch.
func TestSweepAutoCompressCoversTopLevel(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldID := "sess-top-auto"
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, oldID+".jsonl"), overThresholdIdle())
	if err := WriteTopLevelState(TopLevelSession{AgentID: "top-1", Pane: "top-pane", Harness: "pig", Model: "m", Effort: "e", Role: "plan", Cwd: t.TempDir(), SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	done := SweepAutoCompress(fake.run, autoSettings())
	if len(done) != 1 || done[0] != "top-pane" {
		t.Fatalf("sweep compressed %v, want the top-level pane", done)
	}
	if s, _, _ := ReadAgentState("top-1"); s.SessionID == oldID || s.Compressed != 1 {
		t.Errorf("top-level state = %+v, want a fresh session, compressed 1", s)
	}
}
