package wick

// Seam: the wrapper through a fake child and a fake terminal stream. The
// test asserts the exact bytes the terminal receives: the OSC palette at
// start, the reset at exit, and the per-run theme flags on the child.

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/wick/tools"
)

func themeWrapConfig(child *fakeChild, stdout *bytes.Buffer, bin string, args []string, dark bool, dir string) WrapConfig {
	cfg := wrapTestConfig(child, stdout)
	cfg.Bin = bin
	cfg.Args = args
	cfg.ThemeDir = dir
	cfg.ThemeDark = &dark
	return cfg
}

// The wrapper sets the pane palette at start and resets it at exit, on a
// fake terminal stream.
func TestWrapThemeSetAndReset(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	cfg := themeWrapConfig(child, &stdout, "agent", nil, true, t.TempDir())
	cfg.Source.Harness = ""
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.HasPrefix(out, ThemeSetSequence(true)) {
		t.Errorf("terminal misses the OSC set first: %q", out[:minInt(60, len(out))])
	}
	if !strings.HasSuffix(out, ThemeResetSequence()) {
		t.Error("terminal misses the OSC reset last")
	}
}

// A pig child launches with the per-run theme flags; the saved pig
// settings are untouched.
func TestWrapPigThemeFlags(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	var gotArgs []string
	cfg := themeWrapConfig(child, &stdout, "pig", []string{"--session-id", "s"}, true, t.TempDir())
	cfg.Start = func(bin string, args, env []string, w, h int) (wrapPty, func() error, error) {
		gotArgs = append([]string{}, args...)
		child.Resize(w, h)
		return child, func() error { return nil }, nil
	}
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	theme, name := "", ""
	for i, a := range gotArgs {
		if a == "--theme" && i+1 < len(gotArgs) {
			theme = gotArgs[i+1]
		}
		if a == "--use-theme" && i+1 < len(gotArgs) {
			name = gotArgs[i+1]
		}
	}
	if name != "wick-mocha" || theme == "" {
		t.Errorf("pig child args = %q, want per-run --theme/--use-theme", gotArgs)
	}
	if len(child.sizes) == 0 {
		t.Error("child never started")
	}
}

// The signal watcher resets the palette, then re-raises: asserted on the
// fake channel, never a live signal.
func TestWrapSignalResetsPalette(t *testing.T) {
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	var resets int
	resignalled := make(chan os.Signal, 1)
	go watchWrapSignals(ch, done, func() { resets++ }, func(s os.Signal) { resignalled <- s })
	ch <- os.Interrupt
	select {
	case s := <-resignalled:
		if s != os.Interrupt {
			t.Errorf("re-raised %v, want interrupt", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("signal watcher never re-raised")
	}
	if resets != 1 {
		t.Errorf("resets = %d, want 1", resets)
	}
}

// A closed watcher ends quietly: no reset, no re-raise.
func TestWrapSignalDoneEndsQuietly(t *testing.T) {
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	close(done)
	resets := 0
	watchWrapSignals(ch, done, func() { resets++ }, func(os.Signal) {
		t.Error("must not re-raise after done")
	})
	if resets != 0 {
		t.Errorf("resets = %d, want 0", resets)
	}
}

// Non-harness binaries run untouched: no theme flags.
func TestWrapOtherBinUnthemed(t *testing.T) {
	bin, args := themedChildCommand(WrapConfig{
		Source:   HeaderSource{Harness: "pig"},
		Bin:      "bash",
		Args:     []string{"-c", "x"},
		Settings: tools.Settings{},
	}, true)
	if bin != "bash" || len(args) != 2 {
		t.Errorf("other bin changed: %q %q", bin, args)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
