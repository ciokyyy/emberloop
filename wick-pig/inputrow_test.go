package wick

import (
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"
)

// Seam: inputRow unit tests. Tests typing, Alt+Enter, Shift+Enter, paste,
// history, Ctrl+C / Esc interrupts, 10s stale timeout, and rendering.

func TestInputRowTypingAndSubmit(t *testing.T) {
	row := newInputRow()
	action, text := row.handleInput([]byte("hello world"))
	if action != rowActionNone {
		t.Fatalf("action = %v, want none", action)
	}
	if !row.hasDraft() {
		t.Fatal("hasDraft = false, want true")
	}
	if got := row.draft(); got != "hello world" {
		t.Fatalf("draft = %q, want %q", got, "hello world")
	}

	action, text = row.handleInput([]byte("\r"))
	if action != rowActionSubmit || text != "hello world" {
		t.Fatalf("submit = (%v, %q), want (submit, %q)", action, text, "hello world")
	}
	if row.hasDraft() {
		t.Fatal("hasDraft after submit = true, want false")
	}
}

func TestInputRowAltEnterQueues(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("queued prompt"))
	action, text := row.handleInput([]byte("\x1b\r"))
	if action != rowActionQueue || text != "queued prompt" {
		t.Fatalf("alt+enter = (%v, %q), want (queue, %q)", action, text, "queued prompt")
	}
	if row.hasDraft() {
		t.Fatal("hasDraft after queue = true, want false")
	}

	// CSI u encoding of Alt+Enter: \x1b[13;3u
	row.handleInput([]byte("second"))
	action, text = row.handleInput([]byte("\x1b[13;3u"))
	if action != rowActionQueue || text != "second" {
		t.Fatalf("csi u alt+enter = (%v, %q), want (queue, %q)", action, text, "second")
	}
}

func TestInputRowShiftEnterInsertsNewline(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("first line"))
	// Shift+Enter CSI u: \x1b[13;2u
	action, _ := row.handleInput([]byte("\x1b[13;2u"))
	if action != rowActionNone {
		t.Fatalf("shift+enter action = %v, want none", action)
	}
	row.handleInput([]byte("second line"))
	if got := row.draft(); got != "first line\nsecond line" {
		t.Fatalf("draft = %q, want %q", got, "first line\nsecond line")
	}

	// Submit via Enter
	action, text := row.handleInput([]byte("\r"))
	if action != rowActionSubmit || text != "first line\nsecond line" {
		t.Fatalf("submit = (%v, %q), want (submit, %q)", action, text, "first line\nsecond line")
	}

	// Bare newline (\n) also works as Shift+Enter
	row.handleInput([]byte("a"))
	row.handleInput([]byte("\n"))
	row.handleInput([]byte("b"))
	if got := row.draft(); got != "a\nb" {
		t.Fatalf("draft with bare newline = %q, want %q", got, "a\nb")
	}
}

func TestInputRowBracketedPaste(t *testing.T) {
	row := newInputRow()
	paste := "\x1b[200~multi\nline\npaste\x1b[201~"
	action, _ := row.handleInput([]byte(paste))
	if action != rowActionNone {
		t.Fatalf("paste action = %v, want none", action)
	}
	if got := row.draft(); got != "multi\nline\npaste" {
		t.Fatalf("draft = %q, want %q", got, "multi\nline\npaste")
	}
	action, text := row.handleInput([]byte("\r"))
	if action != rowActionSubmit || text != "multi\nline\npaste" {
		t.Fatalf("submit = (%v, %q), want (submit, %q)", action, text, "multi\nline\npaste")
	}
}

func TestInputRowHistoryRecall(t *testing.T) {
	row := newInputRow()
	// Submit 2 items
	row.handleInput([]byte("msg1"))
	row.handleInput([]byte("\r"))
	row.handleInput([]byte("msg2"))
	row.handleInput([]byte("\r"))

	// Up arrow recalls msg2
	row.handleInput([]byte("\x1b[A"))
	if got := row.draft(); got != "msg2" {
		t.Fatalf("history up 1 = %q, want msg2", got)
	}

	// Up arrow again recalls msg1
	row.handleInput([]byte("\x1b[A"))
	if got := row.draft(); got != "msg1" {
		t.Fatalf("history up 2 = %q, want msg1", got)
	}

	// Down arrow moves forward to msg2
	row.handleInput([]byte("\x1b[B"))
	if got := row.draft(); got != "msg2" {
		t.Fatalf("history down 1 = %q, want msg2", got)
	}

	// Down arrow moves forward to empty draft
	row.handleInput([]byte("\x1b[B"))
	if got := row.draft(); got != "" {
		t.Fatalf("history down 2 = %q, want empty", got)
	}
}

func TestInputRowInterruptAndEsc(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("drafting"))
	action, _ := row.handleInput([]byte{0x03})
	if action != rowActionInterrupt {
		t.Fatalf("ctrl+c action = %v, want interrupt", action)
	}

	action, _ = row.handleInput([]byte{0x1b})
	if action != rowActionEsc {
		t.Fatalf("esc action = %v, want esc", action)
	}
}

func TestInputRowStaleDraftClearedAfter10s(t *testing.T) {
	now := time.Now()
	row := newInputRow()
	row.now = func() time.Time { return now }

	row.handleInput([]byte("stale text"))
	if !row.hasDraft() {
		t.Fatal("hasDraft = false, want true")
	}

	now = now.Add(9 * time.Second)
	if row.checkStale() {
		t.Fatal("checkStale at 9s = true, want false")
	}
	if !row.hasDraft() {
		t.Fatal("hasDraft at 9s = false, want true")
	}

	now = now.Add(1 * time.Second) // 10s total
	if !row.checkStale() {
		t.Fatal("checkStale at 10s = false, want true")
	}
	if row.hasDraft() {
		t.Fatal("hasDraft after stale clear = true, want false")
	}
	if got := row.draft(); got != "" {
		t.Fatalf("draft after stale clear = %q, want empty", got)
	}
}

func TestInputRowRender(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("hello"))
	rendered := row.render(80, true, termenv.TrueColor)
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("render output missing text: %q", rendered)
	}
}
