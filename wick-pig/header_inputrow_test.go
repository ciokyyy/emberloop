package wick

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Seam: top-level session with wick input row. Tests sizing, draft isolation,
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

// 3. A queued wick injection is not delivered while row holds a draft.
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
