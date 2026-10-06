package wick

// Seam: the wrapper through a fake child and a fake terminal stream. The
// test asserts the exact bytes the terminal receives: the OSC palette at
// start, the reset at exit, and the per-run theme flags on the child.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

// The signal watcher resets the palette on the terminal stream, then
// re-raises: asserted on a fake stream and a fake channel, never a live
// signal.
func TestWrapSignalResetsPalette(t *testing.T) {
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	var stream bytes.Buffer
	resignalled := make(chan os.Signal, 1)
	go watchWrapSignals(ch, done,
		func() { _, _ = stream.WriteString(ThemeResetSequence()) },
		func(s os.Signal) { resignalled <- s })
	ch <- os.Interrupt
	select {
	case s := <-resignalled:
		if s != os.Interrupt {
			t.Errorf("re-raised %v, want interrupt", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("signal watcher never re-raised")
	}
	if stream.String() != ThemeResetSequence() {
		t.Errorf("signal wrote %q, want the OSC reset on the stream", stream.String())
	}
}

// A supervisor restart is a second WrapChild on the same stream: reset lands
// at the first exit and the palette sets again at the next start.
func TestWrapRestartResetsThenSets(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	run := func() {
		child := &fakeChild{}
		cfg := themeWrapConfig(child, &stdout, "agent", nil, true, dir)
		cfg.Source.Harness = ""
		if err := WrapChild(cfg); err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	out := stdout.String()
	if got := strings.Count(out, ThemeSetSequence(true)); got != 2 {
		t.Errorf("set sequences = %d, want 2 (one per start)", got)
	}
	if got := strings.Count(out, ThemeResetSequence()); got != 2 {
		t.Errorf("reset sequences = %d, want 2 (one per exit)", got)
	}
	firstReset := strings.Index(out, ThemeResetSequence())
	rest := out[firstReset+len(ThemeResetSequence()):]
	if !strings.HasPrefix(rest, ThemeSetSequence(true)) {
		t.Error("restart does not set the palette again right after the reset")
	}
}

// A claude child launches with the theme ref in its settings plus a per-run
// CLAUDE_CONFIG_DIR whose themes/ holds the resolving slug file.
func TestWrapClaudeThemeResolves(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	var gotArgs, gotEnv []string
	cfg := themeWrapConfig(child, &stdout, "claude", []string{"--settings", `{"hooks":{}}`}, true, t.TempDir())
	cfg.Source.Harness = "claude"
	cfg.Start = func(bin string, args, env []string, w, h int) (wrapPty, func() error, error) {
		gotArgs, gotEnv = append([]string{}, args...), append([]string{}, env...)
		child.Resize(w, h)
		return child, func() error { return nil }, nil
	}
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	ref := ""
	for i, a := range gotArgs {
		if a == "--settings" && i+1 < len(gotArgs) {
			var m map[string]any
			if err := json.Unmarshal([]byte(gotArgs[i+1]), &m); err != nil {
				t.Fatal(err)
			}
			ref, _ = m["theme"].(string)
		}
	}
	if ref != "custom:wick-mocha" {
		t.Errorf("claude theme ref = %q, want custom:wick-mocha", ref)
	}
	cfgDir := ""
	for _, e := range gotEnv {
		if strings.HasPrefix(e, "CLAUDE_CONFIG_DIR=") {
			cfgDir = strings.TrimPrefix(e, "CLAUDE_CONFIG_DIR=")
		}
	}
	if cfgDir == "" {
		t.Fatal("claude child misses per-run CLAUDE_CONFIG_DIR")
	}
	slug := strings.TrimPrefix(ref, "custom:")
	b, err := os.ReadFile(filepath.Join(cfgDir, "themes", slug+".json"))
	if err != nil {
		t.Fatalf("slug file %s missing under %s: %v", slug+".json", cfgDir, err)
	}
	var theme map[string]any
	if err := json.Unmarshal(b, &theme); err != nil {
		t.Fatal(err)
	}
	if theme["name"] != "Wick (Catppuccin Mocha)" {
		t.Errorf("slug file name = %v, want the generated mocha theme", theme["name"])
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

// Non-harness binaries run untouched: no theme flags, no extra env.
func TestWrapOtherBinUnthemed(t *testing.T) {
	bin, args, env := themedChildCommand(WrapConfig{
		Source:   HeaderSource{Harness: "pig"},
		Bin:      "bash",
		Args:     []string{"-c", "x"},
		Settings: tools.Settings{},
	}, true)
	if bin != "bash" || len(args) != 2 || len(env) != 0 {
		t.Errorf("other bin changed: %q %q %q", bin, args, env)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
