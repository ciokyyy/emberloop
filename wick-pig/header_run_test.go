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
	"github.com/muesli/termenv"
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
		Start: func(bin string, args, env []string, w, h int, stderr io.Writer) (wrapPty, func() error, error) {
			child.Resize(w, h)
			return child, func() error { return nil }, nil
		},
		Snapshot: func(HeaderSource, tools.Settings) (HeaderSnapshot, error) {
			return HeaderSnapshot{AgentName: "implementer", State: HeaderWorking}, nil
		},
	}
}

// A pig child under the strip keeps its own stderr: with ChildStderr set the
// fake writes its errors there while its terminal output stays on the PTY,
// the strip still paints, and the log receives each raw line stamped.
func TestWrapPipesChildStderrToLog(t *testing.T) {
	child := &fakeChild{pending: []byte("hello")}
	var stdout, log bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	stderr, tail := TeeStderr(&stampWriter{w: &log, label: "s"})
	cfg.ChildStderr = stderr
	cfg.Start = func(bin string, args, env []string, w, h int, stderr io.Writer) (wrapPty, func() error, error) {
		child.Resize(w, h)
		if stderr == nil {
			t.Error("a wrapped child with ChildStderr set must get its own stderr pipe")
		} else {
			_, _ = io.WriteString(stderr, "openai.Stream: sending 8 tools (tools=8)\nopenai.Stream: sending 7 tools (tools=7)\n")
		}
		return child, func() error { return nil }, nil
	}
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "tools=8") || !strings.Contains(lines[1], "tools=7") {
		t.Errorf("stderr lines must reach the log one at a time, got %q", log.String())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "20") || !strings.Contains(line, "Z ") {
			t.Errorf("each log line must lead with a timestamp, got %q", line)
		}
	}
	if got := tail(); !strings.Contains(got, "tools=8") {
		t.Errorf("the stderr tail must keep the child's lines, got %q", got)
	}
	out := stdout.String()
	for _, want := range []string{"hello", "implementer"} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal misses %q:\n%q", want, out)
		}
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

// A wrapped claude child is reported to herdr as an agent: working while it
// starts (never idle before its prompt shows), then released when the child ends. Pig reports itself through its
// extension, so a pig child is never reported here.
func TestWrapReportsClaudeToHerdr(t *testing.T) {
	var calls [][]string
	child := &fakeChild{}
	var stdout bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	cfg.Source.Harness = "claude"
	cfg.Source.AgentName = "code-reviewer"
	cfg.PaneID = "w:p1"
	cfg.Report = func(args []string) { calls = append(calls, args) }
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("herdr calls = %v, want a report then a release", calls)
	}
	join := func(a []string) string { return strings.Join(a, " ") }
	if got := join(calls[0]); !strings.HasPrefix(got, "pane report-agent w:p1 --source wick --agent code-reviewer --state working --seq ") {
		t.Errorf("first call = %q", got)
	}
	if got := join(calls[1]); !strings.HasPrefix(got, "pane release-agent w:p1 --source wick --agent code-reviewer --seq ") {
		t.Errorf("last call = %q", got)
	}
}

func TestWrapDoesNotReportPigToHerdr(t *testing.T) {
	called := false
	child := &fakeChild{}
	var stdout bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	cfg.PaneID = "w:p1"
	cfg.Report = func([]string) { called = true }
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("a pig child was reported by the wrapper")
	}
}

func TestAgentReporterTracksState(t *testing.T) {
	var calls []string
	r := &agentReporter{run: func(a []string) { calls = append(calls, strings.Join(a, " ")) }, pane: "p", label: "x", seq: 10, ready: true}
	r.report(HeaderIdle)
	r.report(HeaderIdle) // unchanged: no call
	r.report(HeaderWorking)
	r.report(HeaderCompressing) // still busy: no call
	r.report(HeaderIdle)
	r.release()
	r.release()             // once only
	r.report(HeaderWorking) // after release: no call
	want := []string{
		"pane report-agent p --source wick --agent x --state idle --seq 11",
		"pane report-agent p --source wick --agent x --state working --seq 12",
		"pane report-agent p --source wick --agent x --state idle --seq 13",
		"pane release-agent p --source wick --agent x --seq 14",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

// The setting restores the drawn rule line: the agent-hue dashes return in
// place of the soft edge, while the child keeps the full borderless width.
func TestWrapRuleLineRestored(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	cfg := wrapTestConfig(child, &stdout)
	rule := true
	cfg.Settings = tools.Settings{Context: &tools.ContextSettings{StripRule: &rule}}
	if err := WrapChild(cfg); err != nil {
		t.Fatal(err)
	}
	if len(child.sizes) == 0 || child.sizes[0] != [2]int{100, 21} {
		t.Errorf("child sizes = %v, want first 100x21", child.sizes)
	}
	got := stdout.String()
	if !strings.Contains(got, "─") {
		t.Error("strip with strip_rule on draws no rule line")
	}
	if strings.Contains(got, "▄") {
		t.Error("strip with strip_rule on keeps the soft edge")
	}
}

// The soft edge is a full-width row of lower half blocks in the strip
// background at every color profile, dark and light: no drawn line survives
// a downgrade.
func TestTopRuleSoftEdgeProfiles(t *testing.T) {
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.ANSI} {
		for _, dark := range []bool{true, false} {
			w := &wrapper{
				cfg:     WrapConfig{Source: HeaderSource{AgentID: "w1", AgentType: "worker"}},
				termW:   100,
				childW:  100,
				dark:    dark,
				profile: profile,
			}
			plain := stripANSI(w.topRule())
			if want := strings.Repeat("▄", 100); plain != want {
				t.Errorf("profile %v dark %v soft edge = %q, want %q", profile, dark, plain, want)
			}
			if strings.Contains(plain, "─") {
				t.Errorf("profile %v dark %v soft edge draws a line", profile, dark)
			}
		}
	}
}

// The strip background sits one step off the pane background, dark and
// light: mantle against base, so tint — not a line — separates the areas.
// The dim tone the borders dim to is the surface tone, dark and light.
// Expected values are the probe (run-borderless.sh Mocha base, the wickb
// themes' surface0 for dark, Latte surface0 for light); the code reads the
// strip side of the shared palette in header.go, so drift breaks the test.
func TestStripBgStandsOffPane(t *testing.T) {
	for _, tc := range []struct {
		dark      bool
		stripBg   string
		paneBg    string
		surfaceBg string
	}{
		{true, "#181825", "#1e1e2e", "#313244"},
		{false, "#e6e9ef", "#eff1f5", "#ccd0da"},
	} {
		if got := pick(headerStripBg, tc.dark); got != tc.stripBg {
			t.Errorf("dark %v strip bg = %s, want %s", tc.dark, got, tc.stripBg)
		}
		if tc.stripBg == tc.paneBg {
			t.Errorf("dark %v strip bg matches the pane bg %s, want one step off", tc.dark, tc.paneBg)
		}
		if got := pick(headerBarDim, tc.dark); got != tc.surfaceBg {
			t.Errorf("dark %v surface tone = %s, want probe surface0 %s", tc.dark, got, tc.surfaceBg)
		}
	}
}

// A resize keeps the borderless geometry: the child is re-sized to the full
// width minus nothing and the strip height, the filter learns the new
// height, and the repaint keeps the soft edge with no margins.
func TestWrapperResizeKeepsBorderlessSize(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	w := &wrapper{
		cfg:     wrapTestConfig(child, &stdout),
		stdout:  &stdout,
		termW:   100,
		termH:   24,
		childW:  100,
		childH:  21,
		rows:    2,
		dark:    true,
		profile: termenv.TrueColor,
		child:   child,
		snapshot: func(HeaderSource, tools.Settings) (HeaderSnapshot, error) {
			return HeaderSnapshot{AgentName: "implementer", State: HeaderWorking}, nil
		},
		filter: &wrapFilter{childH: 21},
	}
	w.resized(80, 20)
	if len(child.sizes) == 0 || child.sizes[len(child.sizes)-1] != [2]int{80, 17} {
		t.Errorf("child sizes = %v, want last 80x17", child.sizes)
	}
	out, _ := w.filter.push([]byte("\x1b[19;1H"))
	if s := string(out); s != "\x1b[17;1H" {
		t.Errorf("post-resize clamp = %q, want row 17", s)
	}
	// Leaving alt-screen after the resize still repaints the soft strip
	// behind it, with no line and no margins.
	if _, repaint := w.filter.push([]byte("\x1b[?1049h")); repaint {
		t.Error("alt-screen entry asks for a repaint")
	}
	if _, repaint := w.filter.push([]byte("\x1b[?1049l")); !repaint {
		t.Error("alt-screen exit after resize does not repaint the strip")
	}
	w.forced()
	got := stdout.String()
	if !strings.Contains(got, "▄") {
		t.Error("post-resize repaint loses the soft edge")
	}
	if strings.Contains(got, "─") || strings.Contains(got, "\x1b[?69h") {
		t.Errorf("post-resize repaint restores a line or margins:\n%q", got)
	}
}

// The wrapper wipes the screen before it claims the strip rows, so shell
// output left behind by the launching pane (a greeting, the command line)
// never shows through a child that redraws only what changed.
func TestWrapClearsScreenBeforeRegion(t *testing.T) {
	child := &fakeChild{}
	var stdout bytes.Buffer
	if err := WrapChild(wrapTestConfig(child, &stdout)); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	clear := strings.Index(got, "\x1b[2J\x1b[H")
	region := strings.Index(got, "\x1b[1;21r")
	if clear < 0 || region < 0 || clear > region {
		t.Errorf("clear at %d, scroll region at %d: want the clear first\n%q", clear, region, got)
	}
}

// A starting claude is reported working until its input prompt shows, so
// herdr's wait for idle cannot return while the first prompt would be typed
// into a screen that is not listening yet.
func TestAgentReporterIdleOnlyOnceReady(t *testing.T) {
	var calls []string
	r := &agentReporter{run: func(a []string) { calls = append(calls, strings.Join(a, " ")) }, pane: "p", label: "x", seq: 10, readyAfter: time.Now().Add(time.Hour)}
	r.report(HeaderIdle)
	r.observe([]byte("\x1b[2J Welcome to Claude Code\r\n"))
	r.report(HeaderIdle)
	if len(calls) != 1 || !strings.Contains(calls[0], "--state working") {
		t.Fatalf("before the prompt: calls = %v, want one working report", calls)
	}
	r.observe([]byte("\x1b[1m\xe2\x8f\xb5\xe2\x8f\xb5 auto mode on\x1b[0m (shift+tab\x1b[1Cto\x1b[1Ccycle)"))
	if len(calls) != 2 || !strings.Contains(calls[1], "--state idle") {
		t.Fatalf("after the prompt: calls = %v, want idle second", calls)
	}
}

// A claude UI that never shows a known footer still becomes idle after the
// wait limit, so a spawn slows down instead of hanging.
func TestAgentReporterIdleAfterLimit(t *testing.T) {
	var calls []string
	r := &agentReporter{run: func(a []string) { calls = append(calls, strings.Join(a, " ")) }, pane: "p", label: "x", seq: 10, readyAfter: time.Now().Add(-time.Second)}
	r.observe([]byte("some unknown screen"))
	if len(calls) != 1 || !strings.Contains(calls[0], "--state idle") {
		t.Errorf("calls = %v, want idle after the limit", calls)
	}
}
