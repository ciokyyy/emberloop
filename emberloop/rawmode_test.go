package el

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Seam: raw passthrough mode. Unit seam is the dirty flag and the harness
// table; wrapper seam is the fake child PTY plus fake clock and the control
// channel, asserting exact child bytes and strip text.

func TestRawHarnessTableHasClaudeAndPig(t *testing.T) {
	for _, h := range []string{"claude", "pig"} {
		keys, ok := harnessKeysFor(h)
		if !ok {
			t.Fatalf("harnessKeysFor(%q) = false, want true", h)
		}
		if len(keys.clear) == 0 {
			t.Errorf("%s clear is empty", h)
		}
		for _, b := range keys.clear {
			if b == 0x03 {
				t.Errorf("%s clear contains Ctrl+C, never allowed", h)
			}
		}
		if len(keys.newline) == 0 {
			t.Errorf("%s newline is empty", h)
		}
		if len(keys.interrupt) == 0 {
			t.Errorf("%s interrupt is empty", h)
		}
	}
	if _, ok := harnessKeysFor("unknown-harness"); ok {
		t.Error("unknown harness must have no entry")
	}
}

func TestRawClearIsSixCtrlU(t *testing.T) {
	for _, h := range []string{"claude", "pig"} {
		seq, ok := rawClearSequence(h)
		if !ok {
			t.Fatalf("rawClearSequence(%q) = false, want true", h)
		}
		want := strings.Repeat("\x15", 6)
		if string(seq) != want {
			t.Errorf("%s clear = %q, want %q", h, seq, want)
		}
	}
	if _, ok := rawClearSequence("unknown-harness"); ok {
		t.Error("unknown harness must never get a clear")
	}
}

func TestRawDirtyTransitions(t *testing.T) {
	m := newRawMode("pig", time.Now)
	m.observeForward([]byte("hi"))
	if !m.dirty() {
		t.Fatal("typed text must set dirty")
	}
	m.observeForward([]byte("\r"))
	if m.dirty() {
		t.Fatal("bare Enter must reset dirty")
	}
	m.observeForward([]byte("x"))
	if !m.dirty() {
		t.Fatal("typed text must set dirty")
	}
	m.observeForward([]byte{0x03})
	if m.dirty() {
		t.Fatal("Ctrl+C must reset dirty")
	}
	// A chunk mixing text with a trailing Enter stays dirty: the flag errs
	// towards dirty and only a bare Enter chunk resets it.
	m.observeForward([]byte("abc\r"))
	if !m.dirty() {
		t.Fatal("mixed text plus Enter must stay dirty")
	}
	m.observeForward([]byte("\r"))
	m.observeForward([]byte("y"))
	m.elClear()
	if m.dirty() {
		t.Fatal("el clear must reset dirty")
	}
}

func TestStripShowsRawMode(t *testing.T) {
	snap := wideSnapshot()
	snap.Raw = true
	if plain := stripANSI(renderTrue(snap, 100)); !strings.Contains(plain, "raw") {
		t.Errorf("strip in raw mode misses raw:\n%s", plain)
	}
	snap.Pending = 2
	plain := stripANSI(renderTrue(snap, 100))
	if !strings.Contains(plain, "raw, 2 queued") {
		t.Errorf("strip raw with queue = %q, want raw, 2 queued", plain)
	}
	if strings.Contains(plain, "pending") {
		t.Errorf("raw queue must read queued, not pending:\n%s", plain)
	}
	for _, state := range []HeaderState{HeaderQueued, HeaderCompressing, HeaderCompressFailed} {
		snap.State = state
		plain := stripANSI(renderTrue(snap, 100))
		if strings.Contains(plain, "raw,") {
			t.Errorf("compress state %q hidden by raw:\n%s", state, plain)
		}
	}
}

// Wrapper seam: toggle, hold, stale clear and unknown-harness default, all
// through a fake child PTY, a fake clock and the control channel.

func TestWrapRawToggleShowsMode(t *testing.T) {
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
	if strings.Contains(stripANSI(stdout.String()), "raw") {
		t.Fatal("strip shows raw before the toggle")
	}
	_, _ = pw.Write([]byte{rawToggleByte})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	// In raw mode typing reaches the child at once.
	_, _ = pw.Write([]byte("passthrough"))
	waitForCond(t, "raw typing reaches child", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "passthrough")
	})
	_, _ = pw.Write([]byte{rawToggleByte})
	// Back in the row typing stays out of the child until submit.
	_, _ = pw.Write([]byte("back in row"))
	waitForCond(t, "row draft renders", func() bool {
		return strings.Contains(stdout.String(), "back in row")
	})
	time.Sleep(50 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); strings.Contains(got, "back in row") {
		t.Fatalf("row typing reached child after returning from raw: %q", got)
	}
	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "row submit reaches child", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "back in row\r")
	})
	close(child.release)
	_ = pw.Close()
	<-wrapped
}

func TestWrapRawHoldsWhileDirty(t *testing.T) {
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
	_, _ = pw.Write([]byte{rawToggleByte})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	_, _ = pw.Write([]byte("dirty typing"))
	waitForCond(t, "raw typing reaches child", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "dirty typing")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "held injection", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); strings.Contains(got, "held injection") {
		t.Fatalf("injection delivered while raw dirty: %q", got)
	}
	waitForCond(t, "strip shows raw queue", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw, 1 queued")
	})
	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "injection delivered after Enter", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "held injection\r")
	})
	close(child.release)
	_ = pw.Close()
	<-wrapped
}

func TestWrapRawDeliversAtOnceWhenClean(t *testing.T) {
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
	_, _ = pw.Write([]byte{rawToggleByte})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "clean injection", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	waitForCond(t, "clean injection delivers", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "clean injection\r")
	})
	close(child.release)
	_ = pw.Close()
	<-wrapped
}

func TestWrapRawStaleClearThenDeliver(t *testing.T) {
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
	_, _ = pw.Write([]byte{rawToggleByte})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	_, _ = pw.Write([]byte("stale draft"))
	waitForCond(t, "raw typing reaches child", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "stale draft")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "queued behind stale", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); strings.Contains(got, "queued behind stale") {
		t.Fatalf("stale injection delivered early: %q", got)
	}
	advanceNow(11 * time.Second)
	waitForCond(t, "stale clear then deliver", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), strings.Repeat("\x15", 6)+"queued behind stale\r")
	})
	got := child.fakeChild.forwardedString()
	if strings.Contains(got, "\x03") {
		t.Errorf("stale clear must never use Ctrl+C: %q", got)
	}
	close(child.release)
	_ = pw.Close()
	<-wrapped
}

func TestWrapRawUnknownHarnessHoldsWithoutClear(t *testing.T) {
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
	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.ServeCtrl = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Clock = getNow
	cfg.Source.Harness = "unknown-harness"
	cfg.Source.TranscriptPath = transcript
	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()
	waitForCond(t, "control socket", func() bool {
		_, err := os.Stat(ctrlSocketPath("w1"))
		return err == nil
	})
	_, _ = pw.Write([]byte{rawToggleByte})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	_, _ = pw.Write([]byte("dirty"))
	waitForCond(t, "raw typing reaches child", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "dirty")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "held injection", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	nowMu.Lock()
	fakeNow = fakeNow.Add(11 * time.Second)
	nowMu.Unlock()
	time.Sleep(50 * time.Millisecond)
	got := child.fakeChild.forwardedString()
	if strings.Contains(got, "held injection") {
		t.Fatalf("unknown harness must hold while dirty: %q", got)
	}
	if strings.Contains(got, "\x15") {
		t.Fatalf("unknown harness must never get a clear: %q", got)
	}
	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "Enter releases the hold", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "held injection\r")
	})
	close(child.release)
	_ = pw.Close()
	<-wrapped
}

func TestWrapRawCtrlCClearsDirty(t *testing.T) {
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
	_, _ = pw.Write([]byte{rawToggleByte})
	waitForCond(t, "strip shows raw", func() bool {
		return strings.Contains(stripANSI(stdout.String()), "raw")
	})
	_, _ = pw.Write([]byte("dirty"))
	waitForCond(t, "raw typing reaches child", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "dirty")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "after interrupt", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); strings.Contains(got, "after interrupt") {
		t.Fatalf("injection delivered while dirty: %q", got)
	}
	_, _ = pw.Write([]byte{0x03})
	waitForCond(t, "injection delivered after Ctrl+C", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "after interrupt\r")
	})
	if got := child.fakeChild.forwardedString(); !strings.Contains(got, "\x03") {
		t.Errorf("Ctrl+C must reach the child: %q", got)
	}
	close(child.release)
	_ = pw.Close()
	<-wrapped
}
