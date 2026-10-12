package el

// Seam: the input queue and the strip pending count. No socket, no PTY, no
// live agent: the queue is a FIFO of steer/queue items, and the strip renders
// its length from the snapshot.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The strip shows the pending count while items wait, and hides it when the
// queue is empty.
func TestStripShowsPendingCount(t *testing.T) {
	snap := wideSnapshot()
	snap.Pending = 2
	if plain := stripANSI(renderTrue(snap, 100)); !strings.Contains(plain, "2 pending") {
		t.Errorf("strip with 2 queued misses the count:\n%s", plain)
	}
	snap.Pending = 0
	if plain := stripANSI(renderTrue(snap, 100)); strings.Contains(plain, "pending") {
		t.Errorf("strip with an empty queue must hide the count:\n%s", plain)
	}
}

// The queue holds steer and queue items in arrival order.
func TestInputQueueHoldsKindsInOrder(t *testing.T) {
	q := newInputQueue()
	q.enqueue(QueueKindSteer, "steer it", "tell")
	q.enqueue(QueueKindQueue, "after the turn", "tell")
	if got := q.len(); got != 2 {
		t.Fatalf("queue length = %d, want 2", got)
	}
}

// User messages go before pending injections in arrival order.
func TestInputQueueUserBeforeInjections(t *testing.T) {
	q := newInputQueue()
	q.enqueue(QueueKindSteer, "inj1", "tell")
	q.enqueue(QueueKindQueue, "inj2", "reviewer")
	q.enqueue(QueueKindSteer, "user steer", "user")
	q.enqueue(QueueKindQueue, "user queued", "user")
	want := []string{"user steer", "user queued", "inj1", "inj2"}
	for i, w := range want {
		head, ok := q.head()
		if !ok || head.Text != w {
			t.Fatalf("item %d = %q (ok=%v), want %q", i, head.Text, ok, w)
		}
		q.pop()
	}
}

// The restart handoff pins ahead of everything: pending injections and the
// user's own messages wait behind it.
func TestInputQueueHandoffPinsAheadOfAll(t *testing.T) {
	q := newInputQueue()
	q.enqueue(QueueKindSteer, "inj1", "tell")
	q.enqueue(QueueKindSteer, "user msg", "user")
	q.enqueueHandoff("handoff text")
	want := []string{"handoff text", "user msg", "inj1"}
	for i, w := range want {
		head, ok := q.head()
		if !ok || head.Text != w {
			t.Fatalf("item %d = %q (ok=%v), want %q", i, head.Text, ok, w)
		}
		q.pop()
	}
}

// A user submit after a restart lands behind the handoff, ahead of
// pending injections.
func TestInputQueueUserLandsBehindHandoff(t *testing.T) {
	q := newInputQueue()
	q.enqueue(QueueKindSteer, "inj1", "tell")
	q.enqueueHandoff("handoff text")
	q.enqueue(QueueKindSteer, "user msg", "user")
	want := []string{"handoff text", "user msg", "inj1"}
	for i, w := range want {
		head, ok := q.head()
		if !ok || head.Text != w {
			t.Fatalf("item %d = %q (ok=%v), want %q", i, head.Text, ok, w)
		}
		q.pop()
	}
}

// Seam: the wrapper's control channel. A real unix socket in a temp state
// dir, a real queue behind it: enqueue through the socket, read the pending
// count back through it.

// An enqueue through the channel lands in the queue; the reply carries the
// pending count.
func TestCtrlEnqueueReachesQueue(t *testing.T) {
	spawnTestState(t)
	q := newInputQueue()
	stop, err := serveCtrl("w1", q)
	if err != nil {
		t.Fatalf("serveCtrl: %v", err)
	}
	defer stop()
	if n, err := ctrlEnqueue("w1", QueueKindSteer, "steer it", "tell"); err != nil || n != 1 {
		t.Fatalf("enqueue steer = %d, %v; want 1, nil", n, err)
	}
	if n, err := ctrlEnqueue("w1", QueueKindQueue, "after the turn", "tell"); err != nil || n != 2 {
		t.Fatalf("enqueue queue = %d, %v; want 2, nil", n, err)
	}
	if got := ctrlPending("w1"); got != 2 {
		t.Errorf("pending = %d, want 2", got)
	}
}

// An unknown kind is rejected and queues nothing.
func TestCtrlUnknownKindRejected(t *testing.T) {
	spawnTestState(t)
	q := newInputQueue()
	stop, err := serveCtrl("w1", q)
	if err != nil {
		t.Fatalf("serveCtrl: %v", err)
	}
	defer stop()
	if _, err := ctrlEnqueue("w1", QueueKind("bogus"), "x", "tell"); err == nil {
		t.Error("bogus kind must be rejected")
	}
	if got := q.len(); got != 0 {
		t.Errorf("queue length = %d, want 0", got)
	}
}

// With no wrapper running, the pending query reads as empty, never an
// error: the strip hides the count.
func TestCtrlPendingWithoutWrapperIsZero(t *testing.T) {
	spawnTestState(t)
	if got := ctrlPending("nobody"); got != 0 {
		t.Errorf("pending without a wrapper = %d, want 0", got)
	}
}

// Seam: the shared prompt-delivery helper. A wrapped agent (agent record
// plus a live channel) against a fake herdr that fails every call.

// A prompt for a wrapped agent arrives through the wrapper's channel, never
// through herdr send-text: the fake herdr refuses everything, and delivery
// still succeeds with one item pending.
func TestDeliverPromptUsesWrapperChannel(t *testing.T) {
	spawnTestState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "p1"}); err != nil {
		t.Fatal(err)
	}
	stop, err := serveCtrl("w1", newInputQueue())
	if err != nil {
		t.Fatalf("serveCtrl: %v", err)
	}
	defer stop()
	rec := &recorder{reply: func(string, []string) Result { return Result{Code: 1} }}
	if err := DeliverPrompt(rec.run, "p1", "steer it"); err != nil {
		t.Fatalf("DeliverPrompt through the channel: %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("channel delivery must not call herdr, got %v", rec.calls)
	}
	if got := ctrlPending("w1"); got != 1 {
		t.Errorf("pending = %d, want 1", got)
	}
}

// A tell to a wrapped agent arrives through the wrapper too: tell drives the
// same shared path.
func TestTellToWrappedAgentUsesWrapperChannel(t *testing.T) {
	spawnTestState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "p1"}); err != nil {
		t.Fatal(err)
	}
	stop, err := serveCtrl("w1", newInputQueue())
	if err != nil {
		t.Fatalf("serveCtrl: %v", err)
	}
	defer stop()
	rec := &recorder{reply: func(string, []string) Result { return Result{Code: 1} }}
	if err := Nudge(rec.run, "p1", "late prompt"); err != nil {
		t.Fatalf("Nudge through the channel: %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("channel delivery must not call herdr, got %v", rec.calls)
	}
	if got := ctrlPending("w1"); got != 1 {
		t.Errorf("pending = %d, want 1", got)
	}
}

// Seam: the delivery pump. A fake child PTY (a byte sink), scripted gates
// and a fake clock: the test asserts the exact bytes the child receives.

// pumpScript is the pump's world in a test: gate booleans per kind, a user
// turn count per text, a byte sink for the child, and a movable clock.
type pumpScript struct {
	boundary bool
	turnDone bool
	hasDraft bool
	turns    map[string]int
	sink     strings.Builder
	now      time.Time
}

func (s *pumpScript) gates() pumpGates {
	return pumpGates{
		gate: func(k QueueKind) bool {
			if k == QueueKindSteer {
				return s.boundary
			}
			return s.turnDone
		},
		countTurns: func(text string) int { return s.turns[text] },
		write:      func(b []byte) error { _, err := s.sink.Write(b); return err },
		now:        func() time.Time { return s.now },
		hasDraft:   func() bool { return s.hasDraft },
	}
}

// A steer waits for the tool boundary; the child receives the whole message
// plus Enter as the exact bytes.
func TestPumpSteerWaitsForToolBoundary(t *testing.T) {
	s := &pumpScript{turns: map[string]int{}, now: time.Now()}
	p := newQueuePump(newInputQueue(), s.gates())
	p.enqueue(QueueKindSteer, "steer it", "tell")
	p.step()
	if got := s.sink.String(); got != "" {
		t.Fatalf("steer before the boundary wrote %q, want nothing", got)
	}
	s.boundary = true
	p.step()
	if got := s.sink.String(); got != "steer it\r" {
		t.Errorf("steer at the boundary wrote %q, want %q", got, "steer it\r")
	}
}

// A queued message waits for the turn to end, even at a tool boundary.
func TestPumpQueueWaitsForTurnEnd(t *testing.T) {
	s := &pumpScript{boundary: true, turns: map[string]int{}, now: time.Now()}
	p := newQueuePump(newInputQueue(), s.gates())
	p.enqueue(QueueKindQueue, "after the turn", "tell")
	p.step()
	if got := s.sink.String(); got != "" {
		t.Fatalf("queued message before turn end wrote %q, want nothing", got)
	}
	s.turnDone = true
	p.step()
	if got := s.sink.String(); got != "after the turn\r" {
		t.Errorf("queued message at turn end wrote %q, want the message plus Enter", got)
	}
}

// An accepted item leaves the queue and the next item delivers: the
// transcript showed the turn.
func TestPumpAcceptedItemPops(t *testing.T) {
	s := &pumpScript{boundary: true, turnDone: true, turns: map[string]int{}, now: time.Now()}
	p := newQueuePump(newInputQueue(), s.gates())
	p.enqueue(QueueKindSteer, "first", "tell")
	p.enqueue(QueueKindSteer, "second", "tell")
	p.step()
	s.turns["first"] = 1
	p.step()
	if got := p.len(); got != 1 {
		t.Fatalf("pending after acceptance = %d, want 1", got)
	}
	p.step()
	if got := s.sink.String(); got != "first\rsecond\r" {
		t.Errorf("child got %q, want both messages in order", got)
	}
}

// El injections are held while the row holds a draft; a user submit goes
// before pending injections and delivers first.
func TestPumpHoldsInjectionsWhileDraft(t *testing.T) {
	oldSettle := submitSettle
	submitSettle = 0
	defer func() { submitSettle = oldSettle }()
	s := &pumpScript{boundary: true, turnDone: true, hasDraft: true, turns: map[string]int{}, now: time.Now()}
	p := newQueuePump(newInputQueue(), s.gates())
	p.enqueue(QueueKindSteer, "injection", "tell")
	p.step()
	if got := s.sink.String(); got != "" {
		t.Fatalf("injection during draft wrote %q, want nothing", got)
	}
	// User submits while injection is waiting
	p.enqueue(QueueKindSteer, "user steer", "user")
	s.hasDraft = false
	p.step()
	if got := s.sink.String(); got != "user steer\r" {
		t.Fatalf("first delivery wrote %q, want user steer first", got)
	}
	s.turns["user steer"] = 1
	p.step() // user steer pops
	p.step() // injection delivers
	if got := s.sink.String(); got != "user steer\rinjection\r" {
		t.Errorf("full delivery wrote %q, want user steer then injection", got)
	}
}

// Seam: acceptance against a real transcript file. The pump writes to the
// fake child; the test appends transcript lines by hand, the way the child
// would, and moves the fake clock.

// An item the transcript never shows stays first in the queue and is
// written again after the wait: a startup dialog swallowed the first
// attempt.
func TestPumpSwallowedItemStaysFirstAndRetries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(path, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &pumpScript{boundary: true, turnDone: true, turns: map[string]int{}, now: time.Now()}
	p := newQueuePump(newInputQueue(), pumpGates{
		gate:       s.gates().gate,
		countTurns: func(text string) int { return countUserTurns(path, "pig", text) },
		write:      s.gates().write,
		now:        s.gates().now,
	})
	p.enqueue(QueueKindSteer, "steer it", "tell")
	p.step()
	s.now = s.now.Add(queueAcceptWait + time.Second)
	p.step()
	if got := s.sink.String(); got != "steer it\rsteer it\r" {
		t.Errorf("swallowed item wrote %q, want the retry after the wait", got)
	}
	if got := p.len(); got != 1 {
		t.Errorf("swallowed item pending = %d, want 1 at the head", got)
	}
}

// Once the transcript shows the turn, the item leaves the queue.
func TestPumpTranscriptAcceptancePops(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(path, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &pumpScript{boundary: true, turnDone: true, turns: map[string]int{}, now: time.Now()}
	g := s.gates()
	p := newQueuePump(newInputQueue(), pumpGates{
		gate:       g.gate,
		countTurns: func(text string) int { return countUserTurns(path, "pig", text) },
		write:      g.write,
		now:        g.now,
	})
	p.enqueue(QueueKindSteer, "steer it", "tell")
	p.step()
	appendLine(t, path, pigUserTurn(t, "steer it"))
	p.step()
	if got := p.len(); got != 0 {
		t.Errorf("accepted item pending = %d, want 0", got)
	}
}

// Only real user turns count: assistant lines, tool results and other texts
// never accept an item.
func TestCountUserTurnsShapes(t *testing.T) {
	dir := t.TempDir()
	for harness, lines := range map[string]string{
		"pig": pigAssistantRunning() + pigToolResult() + pigUserTurn(t, "hello") + pigUserTurn(t, "other"),
		"claude": claudeAssistantRunning() + claudeToolResultOnly() + claudeUserTurn(t, "hello") +
			claudeUserTurn(t, "other"),
	} {
		path := filepath.Join(dir, harness+".jsonl")
		if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := countUserTurns(path, harness, "hello"); got != 1 {
			t.Errorf("%s: count(hello) = %d, want 1", harness, got)
		}
		if got := countUserTurns(path, harness, "missing"); got != 0 {
			t.Errorf("%s: count(missing) = %d, want 0", harness, got)
		}
	}
	if got := countUserTurns(filepath.Join(dir, "nope.jsonl"), "pig", "hello"); got != 0 {
		t.Errorf("missing transcript count = %d, want 0", got)
	}
}

// Empty prompts never enqueue: the channel answers an error and the shared
// path falls back to herdr.
func TestCtrlEmptyTextRejected(t *testing.T) {
	spawnTestState(t)
	stop, err := serveCtrl("w1", newInputQueue())
	if err != nil {
		t.Fatalf("serveCtrl: %v", err)
	}
	defer stop()
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "  ", "tell"); err == nil {
		t.Error("empty text must be rejected")
	}
	if got := ctrlPending("w1"); got != 0 {
		t.Errorf("pending = %d, want 0", got)
	}
}

// Transcript fixtures: one JSON line each, shaped like the real harnesses.
// The tool results carry the same word as the user turn, so only a reader
// that tells user turns from results passes.
func jsonText(t *testing.T, s string) string {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func pigAssistantRunning() string {
	return `{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"toolUse","content":[]}}` + "\n"
}

func pigToolResult() string {
	return `{"type":"message","id":"r1","message":{"role":"toolResult","toolCallId":"c1","toolName":"bash","content":[{"type":"text","text":"hello"}]}}` + "\n"
}

func pigUserTurn(t *testing.T, text string) string {
	t.Helper()
	return `{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":` + jsonText(t, text) + `}]}}` + "\n"
}

func claudeAssistantRunning() string {
	return `{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"working"}]}}` + "\n"
}

func claudeToolResultOnly() string {
	return `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"hello"}]}}` + "\n"
}

func claudeUserTurn(t *testing.T, text string) string {
	t.Helper()
	return `{"type":"user","message":{"role":"user","content":` + jsonText(t, text) + `}}` + "\n"
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

// Seam: the wrapped child. A blocking fake PTY under a real WrapChild with
// the channel served: enqueue through the socket, assert the exact PTY
// bytes and the strip text, all on the production gates and clock.

// blockingChild holds the wrapper's read loop open until the test releases
// it, then ends the child with EOF. A restart closes the old child instead:
// a closed child also reads EOF, so the loop moves to the new child.
type blockingChild struct {
	fakeChild
	release chan struct{}
}

func (b *blockingChild) Read(p []byte) (int, error) {
	b.fakeChild.mu.Lock()
	closed := b.fakeChild.closed
	b.fakeChild.mu.Unlock()
	if closed {
		return 0, io.EOF
	}
	<-b.release
	return 0, io.EOF
}

func waitForCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("timed out waiting for %s", what)
	}
}

// Channel, queue, pump and strip through a live wrapper: a tell-style
// enqueue reaches the child PTY as the exact bytes beside typed input, and
// the strip shows the pending count until the transcript accepts it.
func TestWrapCtrlEndToEnd(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	queuePumpEvery = 5 * time.Millisecond
	defer func() { queuePumpEvery = oldPump }()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}
	child := &blockingChild{release: make(chan struct{})}
	var stdout bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	cfg.ServeCtrl = true
	cfg.Poll = 5 * time.Millisecond
	cfg.Source.TranscriptPath = transcript
	// A bare Enter leaves the child box clean, so the injection goes out.
	cfg.Stdin = strings.NewReader("\r")
	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()
	socketUp := func() bool { _, err := os.Stat(ctrlSocketPath("w1")); return err == nil }
	waitForCond(t, "control socket", socketUp)
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "steer it", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	wantBytes := "\rsteer it\r"
	childGot := func() bool { return child.fakeChild.forwardedString() == wantBytes }
	waitForCond(t, "child bytes", childGot)
	if _, err := BuildHeaderSnapshot(cfg.Source, cfg.Settings); err != nil {
		t.Fatalf("BuildHeaderSnapshot: %v", err)
	}
	if got := ctrlPending("w1"); got != 1 {
		t.Fatalf("pending through socket = %d, want 1", got)
	}
	stdoutHasPending := func() bool { return strings.Contains(stripANSI(stdout.String()), "1 pending") }
	waitForCond(t, "wrapper strip output shows 1 pending", stdoutHasPending)
	close(child.release)
	select {
	case err := <-wrapped:
		if err != nil {
			t.Fatalf("WrapChild: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for wrapper exit")
	}
	if _, err := os.Stat(ctrlSocketPath("w1")); !os.IsNotExist(err) {
		t.Errorf("control socket survives the wrapper")
	}
}

// A transcript that appears after newWrapperPump is built still accepts the
// item: wrapperTurns resolves the path dynamically per step.
func TestWrapperTurnsDeferredTranscript(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "deferred.jsonl")
	src := HeaderSource{Harness: "pig", TranscriptPath: transcript}
	q := newInputQueue()
	var sink bytes.Buffer
	p := newWrapperPump(src, q, func(b []byte) error { _, err := sink.Write(b); return err }, nil, nil)
	p.enqueue(QueueKindSteer, "steer it", "tell")
	// Transcript does not exist yet: write happens, but item stays unaccepted.
	p.step()
	if got := sink.String(); got != "steer it\r" {
		t.Fatalf("first step wrote %q, want %q", got, "steer it\r")
	}
	if got := p.len(); got != 1 {
		t.Fatalf("pending before transcript = %d, want 1", got)
	}
	// Transcript appears with the user turn: next step accepts and pops.
	if err := os.WriteFile(transcript, []byte(pigUserTurn(t, "steer it")), 0o644); err != nil {
		t.Fatal(err)
	}
	p.step()
	if got := p.len(); got != 0 {
		t.Errorf("pending after transcript appeared = %d, want 0", got)
	}
}

// queueGate evaluates steer and queue against transcript state.
func TestQueueGateSteerAndQueueKinds(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "gate.jsonl")
	// An empty/missing transcript opens all gates.
	src := HeaderSource{Harness: "pig", TranscriptPath: transcript}
	if !queueGate(src, QueueKindSteer) || !queueGate(src, QueueKindQueue) {
		t.Fatal("missing transcript should open gates")
	}
	// Tool call running: steer blocked (outstanding > 0), queue blocked (not done).
	toolCall := `{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"toolCall","id":"c1","name":"bash","arguments":{"command":"ls"}}]}}` + "\n"
	if err := os.WriteFile(transcript, []byte(toolCall), 0o644); err != nil {
		t.Fatal(err)
	}
	if queueGate(src, QueueKindSteer) {
		t.Error("steer should be blocked while tool call is outstanding")
	}
	if queueGate(src, QueueKindQueue) {
		t.Error("queue should be blocked while turn is running")
	}
	// Tool done: tool result appended. Outstanding is 0, so steer passes, but turn not done.
	toolDone := `{"type":"message","id":"r1","message":{"role":"toolResult","toolCallId":"c1","toolName":"bash","content":[{"type":"text","text":"ok"}]}}` + "\n"
	appendLine(t, transcript, toolDone)
	if !queueGate(src, QueueKindSteer) {
		t.Error("steer should pass when outstanding is 0")
	}
	if queueGate(src, QueueKindQueue) {
		t.Error("queue should be blocked until turn is done")
	}
	// Turn done: stopReason endTurn. Both steer and queue pass.
	turnDone := `{"type":"message","id":"a2","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"finished"}]}}` + "\n"
	appendLine(t, transcript, turnDone)
	if !queueGate(src, QueueKindQueue) {
		t.Error("queue should pass when turn is done")
	}
}

// Enter never rides in the same write as the text: a TUI that reads a long
// burst as a paste would swallow it and leave the message unsent. The text
// goes first, then Enter alone, after the settle pause.
func TestPumpSendsEnterInItsOwnWrite(t *testing.T) {
	old := submitSettle
	submitSettle = 20 * time.Millisecond
	defer func() { submitSettle = old }()
	var writes []string
	var stamps []time.Time
	p := newQueuePump(newInputQueue(), pumpGates{
		gate: func(QueueKind) bool { return true },
		write: func(b []byte) error {
			writes = append(writes, string(b))
			stamps = append(stamps, time.Now())
			return nil
		},
		now: time.Now,
	})
	p.enqueue(QueueKindSteer, "a long\nmessage", "tell")
	p.step()
	if len(writes) != 2 || writes[0] != "a long message" || writes[1] != "\r" {
		t.Fatalf("writes = %q, want the line then a lone Enter", writes)
	}
	if gap := stamps[1].Sub(stamps[0]); gap < 15*time.Millisecond {
		t.Errorf("Enter followed the text after %v, want the settle pause", gap)
	}
}

// Typing in the child's own box holds injections; once the box sits stale
// past the timeout the wrapper clears it and delivers.
func TestWrapHoldsInjectionWhileTyping(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	queuePumpEvery = 5 * time.Millisecond
	defer func() { queuePumpEvery = oldPump }()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}
	child := &blockingChild{release: make(chan struct{})}
	var stdout bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	cfg.ServeCtrl = true
	cfg.Poll = 5 * time.Millisecond
	cfg.Source.TranscriptPath = transcript
	cfg.Source.Harness = "pig"
	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()
	waitForCond(t, "control socket", func() bool { _, err := os.Stat(ctrlSocketPath("w1")); return err == nil })
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "steer it", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	// Half the old 200ms: the pump polls every 5ms, so 100ms still gives it
	// twenty chances to wrongly forward while the box is dirty.
	time.Sleep(100 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); strings.Contains(got, "steer it") {
		t.Fatalf("injection leaked into a dirty box: %q", got)
	}
	close(child.release)
	<-wrapped
}
