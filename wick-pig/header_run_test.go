package wick

// Seam: the wrapper through a fake child. No PTY, no live agent: the fake
// answers the same start/read/write/resize surface the real PTY does, so
// the test proves forwarding, the reduced child size and strip survival.

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/wick/tools"
)

// fakeChild is the test double for the PTY: preloaded child output drains
// through Read, wrapper input lands in forwarded, and every resize is
// recorded.
type fakeChild struct {
	mu        sync.Mutex
	pending   []byte
	forwarded bytes.Buffer
	sizes     [][2]int
	closed    bool
}

func (f *fakeChild) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

func (f *fakeChild) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forwarded.Write(p)
}

func (f *fakeChild) Resize(w, h int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{w, h})
}

func (f *fakeChild) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeChild) forwardedString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forwarded.String()
}

func wrapTestConfig(child *fakeChild, stdout *bytes.Buffer) WrapConfig {
	return WrapConfig{
		Source:   HeaderSource{AgentID: "w1", AgentType: "worker", AgentName: "implementer", Harness: "pig"},
		Settings: tools.Settings{},
		Bin:      "agent",
		Stdin:    strings.NewReader("typed\n"),
		Stdout:   stdout,
		TermW:    100,
		TermH:    24,
		Poll:     time.Hour,
		Start: func(bin string, args, env []string, w, h int) (wrapPty, func() error, error) {
			child.Resize(w, h)
			return child, func() error { return nil }, nil
		},
		Snapshot: func(HeaderSource, tools.Settings) (HeaderSnapshot, error) {
			return HeaderSnapshot{AgentName: "implementer", State: HeaderWorking}, nil
		},
	}
}

// The wrapper forwards both directions, starts the child at the reduced
// size, constrains its columns to the frame, and keeps the strip — top
// rule included — on screen across a region reset.
func TestWrapFakeChild(t *testing.T) {
	child := &fakeChild{pending: []byte("hello\x1b[rworld")}
	var stdout bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	frame := true
	cfg.Settings = tools.Settings{Context: &tools.ContextSettings{StripFrame: &frame}}
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	if len(child.sizes) == 0 || child.sizes[0] != [2]int{98, 21} {
		t.Errorf("child sizes = %v, want first 98x21", child.sizes)
	}
	deadline := time.Now().Add(2 * time.Second)
	for child.forwardedString() != "typed\n" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := child.forwardedString(); got != "typed\n" {
		t.Errorf("child got %q, want the typed input", got)
	}
	out := stdout.String()
	for _, want := range []string{"hello", "world", "implementer", "ctx", "\x1b[1;21r", "\x1b[?69h", "\x1b[2;99s", "\u2584", "\x1b7", "\x1b8"} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal misses %q:\n%q", want, out)
		}
	}
	// With the margins enabled CSI s would set margins, never save the
	// cursor: the wrapper must not emit it.
	if strings.Contains(out, "\x1b[s") {
		t.Errorf("wrapper saves the cursor with CSI s, want ESC 7:\n%q", out)
	}
}

// A clear erases the strip with the rest, so the wrapper repaints it right
// after instead of waiting for the numbers to change: the chip lands on
// screen twice, once at start and once after the clear.
func TestWrapRepaintsAfterClear(t *testing.T) {
	child := &fakeChild{pending: []byte("a\x1b[2Jb")}
	var stdout bytes.Buffer
	if err := WrapChild(wrapTestConfig(child, &stdout)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(stdout.String(), "implementer"); got < 2 {
		t.Errorf("chip paints %d times, want at least 2 (start plus clear)", got)
	}
}

// On exit the wrapper hands the terminal back: the scroll region resets,
// the strip rows clear, and the cursor lands on the bottom row.
func TestWrapRestoresTerminalOnExit(t *testing.T) {
	child := &fakeChild{pending: []byte("done")}
	var stdout bytes.Buffer
	if err := WrapChild(wrapTestConfig(child, &stdout)); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{"\x1b[r", "\x1b[2K", "\x1b[24;1H"} {
		if !strings.Contains(out, want) {
			t.Errorf("exit leaves no %q:\n%q", want, out)
		}
	}
}

// A region reset split across reads is still repaired: the filter holds the
// partial sequence until the rest arrives.
func TestWrapFilterHoldsPartialSequence(t *testing.T) {
	f := &wrapFilter{childH: 22}
	first, repaint := f.push([]byte("hi\x1b"))
	if strings.Contains(string(first), "\x1b") {
		t.Errorf("partial escape leaks: %q", first)
	}
	if repaint {
		t.Error("partial escape asks for a repaint")
	}
	second, _ := f.push([]byte("[rthere"))
	if s := string(second); !strings.Contains(s, "\x1b[1;22r") || !strings.Contains(s, "there") {
		t.Errorf("split reset repairs to %q, want region plus text", s)
	}
}

// Leaving the child's alt screen repaints the strip behind it.
func TestWrapFilterAltScreenExitRepaints(t *testing.T) {
	f := &wrapFilter{childH: 22}
	if _, repaint := f.push([]byte("\x1b[?1049h")); repaint {
		t.Error("alt-screen entry asks for a repaint")
	}
	if _, repaint := f.push([]byte("\x1b[?1049l")); !repaint {
		t.Error("alt-screen exit does not repaint the strip")
	}
}

// A resize reaches the filter too: later chunks clamp against the new
// height, never the stale one.
func TestWrapFilterFollowsResize(t *testing.T) {
	f := &wrapFilter{childH: 22}
	f.setSize(10, "\x1b[2;99s")
	out, _ := f.push([]byte("\x1b[15;1H"))
	if s := string(out); s != "\x1b[10;1H" {
		t.Errorf("post-resize clamp = %q, want row 10", s)
	}
}

// The default look is borderless: no frame columns, so the child gets the
// full width, and the strip's top edge is a soft half-block row, never a
// drawn line.
func TestWrapDefaultIsBorderless(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	if err := WrapChild(wrapTestConfig(child, &stdout)); err != nil {
		t.Fatal(err)
	}
	if len(child.sizes) == 0 || child.sizes[0] != [2]int{100, 21} {
		t.Errorf("child sizes = %v, want first 100x21", child.sizes)
	}
	got := stdout.String()
	if !strings.Contains(got, "\u2584") {
		t.Error("strip top edge is not the soft half-block row")
	}
	if strings.Contains(got, "\u2500") {
		t.Error("strip draws a rule line")
	}
	if strings.Contains(got, "\x1b[?69h") {
		t.Error("column margins set with the frame off")
	}
}
