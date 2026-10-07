package emberloop

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/emberloop/tools"
	"github.com/muesli/termenv"
)

// Seam: top-level session with emberloop input row. Tests sizing, draft isolation,
// injection holding, stale draft timeout with fake clock, submission ordering,
// steering vs queueing, multiline, paste, history and interrupts.

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// 1. Child PTY is sized to terminal minus strip and input row (reduced height).
// Resize preserves input row and strip and forwards reduced height to child.
func TestWrapInputRowGeometryAndResize(t *testing.T) {
	child := &fakeChild{}
	var stdout safeBuffer
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.TermW = 100
	cfg.TermH = 24
	pr, pw := io.Pipe()
	cfg.Stdin = pr

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	// First size: 100 cols, 24 - 2(strip) - 1(rule) - 1(row) = 20 rows
	waitForCond(t, "child start size", func() bool {
		child.mu.Lock()
		defer child.mu.Unlock()
		return len(child.sizes) > 0
	})
	if got := child.sizes[0]; got != [2]int{100, 20} {
		t.Fatalf("child start size = %v, want [100, 20]", got)
	}

	// Stdio output has row prompt and strip content
	waitForCond(t, "stdout row and strip", func() bool {
		out := stdout.String()
		return strings.Contains(out, "❯") && strings.Contains(out, "implementer")
	})

	_ = pw.Close()
	child.Close()
}

// 2. Typing in the row never reaches the child until submit.
// Row and strip text are visible in terminal output.
func TestWrapInputRowTypingNeverReachesChildUntilSubmit(t *testing.T) {
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	// Wait for start
	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})

	// User types draft
	_, _ = pw.Write([]byte("drafting something"))
	waitForCond(t, "draft in row stdout", func() bool {
		return strings.Contains(stdout.String(), "drafting something")
	})

	// Must NOT reach child
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Fatalf("child received typed text before submit: %q", got)
	}

	// Submit via Enter
	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "child received submitted text", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "drafting something\r")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// 3. A queued emberloop injection is not delivered while row holds a draft.
// After user submits, user message delivers before pending injection.
func TestWrapInputRowHoldsInjectionsUntilSubmit(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.ServeCtrl = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Source.TranscriptPath = transcript

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "control socket", func() bool {
		_, err := os.Stat(ctrlSocketPath("w1"))
		return err == nil
	})

	// User types in row
	_, _ = pw.Write([]byte("user message"))
	waitForCond(t, "draft rendered", func() bool {
		return strings.Contains(stdout.String(), "user message")
	})

	// Injected prompt arrives via control channel
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "injection text", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}

	// Must NOT be delivered because row holds draft
	time.Sleep(50 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Fatalf("child got text while draft active: %q", got)
	}

	// User submits draft with Enter
	_, _ = pw.Write([]byte("\r"))

	// Child must receive user message first!
	waitForCond(t, "user message delivered first", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "user message\r")
	})

	// Transcript accepts user message
	appendLine(t, transcript, pigUserTurn(t, "user message"))

	// Now injection text delivers
	waitForCond(t, "injection delivered after user", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "injection text\r")
	})

	wantBytes := "user message\rinjection text\r"
	if got := child.fakeChild.forwardedString(); got != wantBytes {
		t.Fatalf("forwarded bytes = %q, want %q", got, wantBytes)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// 4. Stale draft: after 10 s of no keystrokes the row clears and held injections deliver.
func TestWrapInputRowStaleDraftTimeout(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}

	var nowMu sync.Mutex
	fakeNow := time.Now()
	getNow := func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return fakeNow
	}
	advanceNow := func(d time.Duration) {
		nowMu.Lock()
		defer nowMu.Unlock()
		fakeNow = fakeNow.Add(d)
	}

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.ServeCtrl = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Clock = getNow
	cfg.Source.TranscriptPath = transcript

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "control socket", func() bool {
		_, err := os.Stat(ctrlSocketPath("w1"))
		return err == nil
	})

	// User types draft
	_, _ = pw.Write([]byte("forgotten draft"))
	waitForCond(t, "draft rendered", func() bool {
		return strings.Contains(stdout.String(), "forgotten draft")
	})

	// Injection arrives
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "background injection", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Fatalf("injection delivered before timeout: %q", got)
	}

	// Advance clock past 10 seconds
	advanceNow(11 * time.Second)

	// Held injection delivers after draft clears!
	waitForCond(t, "held injection delivered", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "background injection\r")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// 5. Multiline, paste, history, Ctrl+C and Esc keys.
func TestWrapInputRowSpecialKeys(t *testing.T) {
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})

	// Shift+Enter inserts newline in draft
	_, _ = pw.Write([]byte("line1"))
	_, _ = pw.Write([]byte("\x1b[13;2u")) // Shift+Enter
	_, _ = pw.Write([]byte("line2\r"))    // submit
	waitForCond(t, "multiline submitted", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "line1 line2\r")
	})

	// Up recalls history
	_, _ = pw.Write([]byte("\x1b[A")) // Up
	waitForCond(t, "history in stdout", func() bool {
		return strings.Contains(stdout.String(), "line1")
	})

	// Ctrl+C forwards \x03
	_, _ = pw.Write([]byte{0x03})
	waitForCond(t, "ctrl+c forwarded", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "\x03")
	})

	// Esc forwards \x1b
	_, _ = pw.Write([]byte{0x1b})
	waitForCond(t, "esc forwarded", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "\x1b")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// 6. Alt+Enter queues until turn end, while Enter steers at tool boundary.
func TestWrapInputRowAltEnterWaitsForTurnEnd(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	toolRunning := `{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"toolCall","id":"c1","name":"bash"}]}}` + "\n"
	if err := os.WriteFile(transcript, []byte(toolRunning), 0o644); err != nil {
		t.Fatal(err)
	}

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Source.TranscriptPath = transcript

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})

	// User sends Alt+Enter (queue message)
	_, _ = pw.Write([]byte("queued task"))
	_, _ = pw.Write([]byte("\x1b\r"))

	// Wait a moment: turn is not done (stopReason is toolUse), so queued task must NOT deliver
	time.Sleep(50 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Fatalf("queued task delivered before turn end: %q", got)
	}

	// Turn finishes
	turnDone := `{"type":"message","id":"a2","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"finished"}]}}` + "\n"
	appendLine(t, transcript, turnDone)

	// Now queued task delivers!
	waitForCond(t, "queued task delivered after turn end", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "queued task\r")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// A compress restart keeps the wrapper up: the row draft and history are
// unchanged, pending items survive, and the handoff pins ahead of them. It
// waits on the draft like any injection, then delivers first through the
// same pump, accepted as the transcript shows each turn.
func TestWrapRestartKeepsDraftQueueAndHistory(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}

	oldChild := &blockingChild{release: make(chan struct{})}
	newChild := &blockingChild{release: make(chan struct{})}
	starts := 0
	start := func(bin string, args, env []string, w, h int, stderr io.Writer) (wrapPty, func() error, error) {
		starts++
		if starts == 1 {
			oldChild.Resize(w, h)
			return oldChild, func() error { return nil }, nil
		}
		newChild.Resize(w, h)
		return newChild, func() error { return nil }, nil
	}

	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(oldChild, &stdout)
	cfg.Start = start
	cfg.InputRow = true
	cfg.ServeCtrl = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Source.TranscriptPath = transcript

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})

	// A submitted message writes to the old child and waits for acceptance.
	_, _ = pw.Write([]byte("first msg\r"))
	waitForCond(t, "first msg on old child", func() bool {
		return strings.Contains(oldChild.forwardedString(), "first msg\r")
	})

	// A draft plus a held injection wait behind it.
	_, _ = pw.Write([]byte("my draft"))
	waitForCond(t, "draft rendered", func() bool {
		return strings.Contains(stdout.String(), "my draft")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "queued before", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	// The supervisor restarts through the channel: same socket, new child.
	if _, err := ctrlRestartAgent("w1", ctrlRestart{Handoff: "handoff text", SessionID: "sess-new", Bin: "agent"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	waitForCond(t, "restart keeps 3 pending", func() bool {
		return ctrlPending("w1") == 3
	})

	// The handoff waits on the draft like any injection, and the strip shows
	// the queue behind it.
	time.Sleep(30 * time.Millisecond)
	if got := newChild.forwardedString(); got != "" {
		t.Fatalf("handoff delivered past a held draft: %q", got)
	}
	waitForCond(t, "strip shows 3 queued with countdown", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "queued 3 messages")
	})
	if got := oldChild.forwardedString(); got != "first msg\r" {
		t.Fatalf("old child got %q, want only the pre-restart message", got)
	}
	if out := stdout.String(); !strings.Contains(out, "my draft") {
		t.Fatalf("restart cleared the row draft:\n%q", stripANSI(out))
	}

	// Submitting the draft unblocks the queue behind the handoff.
	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "handoff on new child", func() bool {
		return strings.Contains(newChild.forwardedString(), "handoff text\r")
	})

	// Each item is accepted as the transcript shows its turn: handoff, the
	// resubmitted user message, the submitted draft, then the injection.
	appendLine(t, transcript, pigUserTurn(t, "handoff text"))
	waitForCond(t, "user msg redelivered", func() bool {
		return strings.Contains(newChild.forwardedString(), "handoff text\rfirst msg\r")
	})
	appendLine(t, transcript, pigUserTurn(t, "first msg"))
	waitForCond(t, "draft delivered", func() bool {
		return strings.Contains(newChild.forwardedString(), "handoff text\rfirst msg\rmy draft\r")
	})
	appendLine(t, transcript, pigUserTurn(t, "my draft"))
	waitForCond(t, "injection delivered", func() bool {
		return strings.Contains(newChild.forwardedString(), "handoff text\rfirst msg\rmy draft\rqueued before\r")
	})
	appendLine(t, transcript, pigUserTurn(t, "queued before"))
	waitForCond(t, "queue drains", func() bool {
		return ctrlPending("w1") == 0
	})
	if got, want := newChild.forwardedString(), "handoff text\rfirst msg\rmy draft\rqueued before\r"; got != want {
		t.Fatalf("new child got %q, want %q", got, want)
	}

	// History survived the restart: two Ups walk back past the submitted draft
	// to the first message. The gap keeps the reads apart. Nothing yet shows
	// the first message: the single-write submit painted an empty row.
	if strings.Contains(stdout.String(), "first msg") {
		t.Fatalf("history text leaked before recall:\n%q", stripANSI(stdout.String()))
	}
	_, _ = pw.Write([]byte("\x1b[A"))
	time.Sleep(100 * time.Millisecond)
	_, _ = pw.Write([]byte("\x1b[A"))
	waitForCond(t, "history after restart", func() bool {
		return strings.Contains(stdout.String(), "first msg")
	})

	close(oldChild.release)
	close(newChild.release)
	_ = pw.Close()
	<-wrapped
}

// A restart in raw mode keeps the mode and the dirty state: the restart
// keeps w.raw itself, so a compress never drops them.
func TestRestartKeepsRawModeAndDirty(t *testing.T) {
	q := newInputQueue()
	q.enqueue(QueueKindSteer, "queued before", "tell")
	row := newInputRow()
	_, _ = row.handleInput([]byte("my draft"))
	raw := newRawMode("pig", nil)
	raw.toggle()
	raw.observeForward([]byte("x"))
	if !raw.isEnabled() || !raw.dirty() {
		t.Fatal("raw setup did not take")
	}
	var sink bytes.Buffer
	pump := newQueuePump(q, pumpGates{
		gate:  func(QueueKind) bool { return true },
		write: func(b []byte) error { _, err := sink.Write(b); return err },
		now:   time.Now,
	})
	var stdout bytes.Buffer
	oldChild := &fakeChild{}
	w := &wrapper{
		cfg:     WrapConfig{Source: HeaderSource{AgentID: "w1", AgentType: "worker", Harness: "pig"}},
		stdout:  &stdout,
		termW:   100,
		termH:   24,
		childW:  100,
		childH:  20,
		dark:    true,
		profile: termenv.TrueColor,
		child:   oldChild,
		snapshot: func(HeaderSource, tools.Settings) (HeaderSnapshot, error) {
			return HeaderSnapshot{AgentName: "implementer", State: HeaderWorking}, nil
		},
		filter:   &wrapFilter{childH: 20},
		inputRow: row,
		queue:    q,
		pump:     pump,
		raw:      raw,
	}
	newChild := &fakeChild{}
	var waited atomic.Bool
	w.wait = func() error { waited.Store(true); return nil }
	w.cfg.Start = func(bin string, args, env []string, cw, ch int, stderr io.Writer) (wrapPty, func() error, error) {
		return newChild, func() error { return nil }, nil
	}
	if err := w.restartChild("agent", nil, nil, HeaderSource{AgentID: "w1", AgentType: "worker", Harness: "pig", SessionID: "s2"}, "handoff text"); err != nil {
		t.Fatalf("restartChild: %v", err)
	}
	if !w.raw.isEnabled() || !w.raw.dirty() {
		t.Error("restart dropped raw mode or its dirty state")
	}
	if got := row.draft(); got != "my draft" {
		t.Errorf("restart dropped the row draft: %q", got)
	}
	if got := q.len(); got != 2 {
		t.Fatalf("pending after restart = %d, want the handoff plus the survivor", got)
	}
	head, _ := q.head()
	if head.Text != "handoff text" {
		t.Errorf("head = %q, want the handoff first", head.Text)
	}
	if w.child != wrapPty(newChild) {
		t.Error("restart kept the old child")
	}
	if !oldChild.closed {
		t.Error("restart left the old child open")
	}
	waitForCond(t, "old child reaped", waited.Load)
	if w.cfg.Source.SessionID != "s2" {
		t.Errorf("source session = %q, want s2", w.cfg.Source.SessionID)
	}
	// The swapped-out pump stays quiet: a captured step must not write the
	// head twice to the new child.
	pump.step()
	if got := sink.String(); got != "" {
		t.Errorf("old pump wrote %q after the swap, want silence", got)
	}
}

// A restart relaunches a harness child themed: the same per-run palette the
// first start applied, so a compress never drops the strip's colors. Any
// other binary restarts untouched.
func TestRestartRelaunchesHarnessChildThemed(t *testing.T) {
	spawnTestState(t)
	q := newInputQueue()
	var stdout bytes.Buffer
	w := &wrapper{
		cfg:     WrapConfig{Source: HeaderSource{AgentID: "w1", AgentType: "worker", Harness: "pig"}},
		stdout:  &stdout,
		termW:   100,
		termH:   24,
		childW:  100,
		childH:  20,
		dark:    true,
		profile: termenv.TrueColor,
		child:   &fakeChild{},
		snapshot: func(HeaderSource, tools.Settings) (HeaderSnapshot, error) {
			return HeaderSnapshot{AgentName: "implementer", State: HeaderWorking}, nil
		},
		filter: &wrapFilter{childH: 20},
		queue:  q,
		pump: newQueuePump(q, pumpGates{
			gate:  func(QueueKind) bool { return true },
			write: func(b []byte) error { return nil },
			now:   time.Now,
		}),
	}
	var gotBin string
	var gotArgs []string
	w.cfg.Start = func(bin string, args, env []string, cw, ch int, stderr io.Writer) (wrapPty, func() error, error) {
		gotBin, gotArgs = bin, args
		return &fakeChild{}, func() error { return nil }, nil
	}
	src := HeaderSource{AgentID: "w1", AgentType: "worker", Harness: "pig", SessionID: "s2"}
	if err := w.restartChild("pig", []string{"--session-id", "s2"}, nil, src, "handoff"); err != nil {
		t.Fatalf("restartChild: %v", err)
	}
	if gotBin != "pig" {
		t.Fatalf("restarted bin = %q, want pig", gotBin)
	}
	if !slices.Contains(gotArgs, "--theme") || !slices.Contains(gotArgs, "--use-theme") {
		t.Errorf("restarted pig args = %q, want the per-run theme flags", gotArgs)
	}
	if err := w.restartChild("agent", []string{"x"}, nil, src, "handoff"); err != nil {
		t.Fatalf("restartChild: %v", err)
	}
	if gotBin != "agent" || len(gotArgs) != 1 || gotArgs[0] != "x" {
		t.Errorf("non-harness restart = %q %q, want agent [x] untouched", gotBin, gotArgs)
	}
}

// A restart in raw mode keeps the mode and the dirty state end to end: the
// strip shows raw before and after, the handoff waits on the dirty box like
// any injection, and a bare Enter resets the flag and releases it.
func TestWrapRestartKeepsRawModeEndToEnd(t *testing.T) {
	spawnTestState(t)
	oldPump := queuePumpEvery
	oldSettle := submitSettle
	queuePumpEvery = 5 * time.Millisecond
	submitSettle = 0
	defer func() {
		queuePumpEvery = oldPump
		submitSettle = oldSettle
	}()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}

	oldChild := &blockingChild{release: make(chan struct{})}
	newChild := &blockingChild{release: make(chan struct{})}
	starts := 0
	start := func(bin string, args, env []string, w, h int, stderr io.Writer) (wrapPty, func() error, error) {
		starts++
		if starts == 1 {
			oldChild.Resize(w, h)
			return oldChild, func() error { return nil }, nil
		}
		newChild.Resize(w, h)
		return newChild, func() error { return nil }, nil
	}

	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(oldChild, &stdout)
	cfg.Start = start
	cfg.InputRow = true
	cfg.ServeCtrl = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Source.TranscriptPath = transcript

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})

	// Raw on, then one forwarded byte dirties the box.
	_, _ = pw.Write([]byte{0x0f})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	_, _ = pw.Write([]byte("x"))
	waitForCond(t, "raw byte forwarded", func() bool {
		return strings.Contains(oldChild.forwardedString(), "x")
	})

	// The restart keeps the mode: the strip paints raw again after it, and the
	// handoff waits on the dirty box.
	before := strings.Count(stripANSI(stdout.String()), "raw")
	if _, err := ctrlRestartAgent("w1", ctrlRestart{Handoff: "handoff text", SessionID: "sess-new", Bin: "agent"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	waitForCond(t, "strip still shows raw", func() bool {
		return strings.Count(stripANSI(stdout.String()), "raw") > before
	})
	time.Sleep(30 * time.Millisecond)
	if got := newChild.forwardedString(); got != "" {
		t.Fatalf("handoff delivered past a dirty raw box: %q", got)
	}

	// A bare Enter resets the flag and releases the handoff through the same
	// pump, accepted when the transcript shows its turn. The Enter itself
	// forwards to the child first, as raw passthrough does.
	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "handoff on new child", func() bool {
		return strings.Contains(newChild.forwardedString(), "\rhandoff text\r")
	})
	appendLine(t, transcript, pigUserTurn(t, "handoff text"))
	waitForCond(t, "queue drains", func() bool {
		return ctrlPending("w1") == 0
	})
	if got, want := newChild.forwardedString(), "\rhandoff text\r"; got != want {
		t.Fatalf("new child got %q, want %q", got, want)
	}

	close(oldChild.release)
	close(newChild.release)
	_ = pw.Close()
	<-wrapped
}
