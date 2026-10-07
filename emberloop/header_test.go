package emberloop

// Header strip tests (ticket 04): the strip is a pure function of a
// snapshot and a width, so every tier, state and theme asserts here.
// header_golden_test.go locks the wide layout byte for byte. Expected
// values come from the spec mockup, never from the implementation.

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"
)

var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiSeq.ReplaceAllString(s, "") }

func wideSnapshot() HeaderSnapshot {
	return HeaderSnapshot{
		AgentID:       "worker-04",
		AgentName:     "orchestrator",
		Model:         "opus",
		ContextTokens: 142000,
		Window:        200000,
		Cost:          1.82,
		HasCost:       true,
		TopTools: []ToolSize{
			{Name: "bash", Bytes: 31000},
			{Name: "read", Bytes: 22000},
			{Name: "grep", Bytes: 9000},
		},
		CompressCount: 2,
		State:         HeaderWorking,
		Running:       ToolCall{Name: "mcp__emberloop__read", Arg: "/home/dev/emberloop/header.go"},
		Now:           time.Unix(0, 0),
		Usage: UsageSet{Has: true, Windows: []UsageWindow{
			{Label: "5h", Pct: 37, ResetsAt: 6000}, {Label: "wk", Pct: 14, ResetsAt: 4 * 86400},
		}},
	}
}

func renderTrue(snap HeaderSnapshot, width int) string {
	return RenderHeader(snap, width, HeaderOpts{Dark: true, Profile: termenv.TrueColor})
}

// The wide tier shows the mockup's two rows: context against the window
// with the bar, cost, model and compress count, then the running tool on
// the left and the account usage on the right.
func TestRenderHeaderWide(t *testing.T) {
	rows := strings.Split(strings.TrimRight(renderTrue(wideSnapshot(), 100), "\n"), "\n")
	if len(rows) != 2 {
		t.Fatalf("wide strip is %d rows, want 2:\n%s", len(rows), renderTrue(wideSnapshot(), 100))
	}
	plain := stripANSI(strings.Join(rows, "\n"))
	for _, want := range []string{"orchestrator", "ctx", "71%", "142k", "200k", "~$1.82", "opus", "×2", "reading", "emberloop/header.go", "5h ◔ 37% ↻1h40m", "wk ◔ 14% ↻4d0h"} {
		if !strings.Contains(plain, want) {
			t.Errorf("wide strip misses %q:\n%s", want, plain)
		}
	}
	if got := strings.Count(rows[0], "▰") + strings.Count(rows[0], "▱"); got != 24 {
		t.Errorf("wide bar has %d cells, want 24", got)
	}
	if got := strings.Count(rows[0], "▰"); got != 17 {
		t.Errorf("wide bar fills %d cells at 71%%, want 17", got)
	}
}

// The medium tier drops the tools row and shrinks the bar.
func TestRenderHeaderMedium(t *testing.T) {
	out := renderTrue(wideSnapshot(), 80)
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 1 {
		t.Fatalf("medium strip is %d rows, want 1:\n%s", len(rows), out)
	}
	plain := stripANSI(rows[0])
	for _, want := range []string{"71%", "142k", "~$1.82", "working"} {
		if !strings.Contains(plain, want) {
			t.Errorf("medium strip misses %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "top") {
		t.Errorf("medium strip keeps the tools row:\n%s", plain)
	}
	if got := strings.Count(rows[0], "▰") + strings.Count(rows[0], "▱"); got != 16 {
		t.Errorf("medium bar has %d cells, want 16", got)
	}
}

// The narrow tier keeps only the percent and the cost beside the state.
func TestRenderHeaderNarrow(t *testing.T) {
	out := renderTrue(wideSnapshot(), 40)
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 1 {
		t.Fatalf("narrow strip is %d rows, want 1:\n%s", len(rows), out)
	}
	plain := stripANSI(rows[0])
	for _, want := range []string{"ctx", "71%", "~$1.82", "working"} {
		if !strings.Contains(plain, want) {
			t.Errorf("narrow strip misses %q:\n%s", want, plain)
		}
	}
	if strings.Contains(rows[0], "▰") || strings.Contains(rows[0], "▱") {
		t.Errorf("narrow strip keeps the bar:\n%s", plain)
	}
}

// One row in settings forces the single-row layout even on a wide pane.
func TestRenderHeaderSingleRow(t *testing.T) {
	out := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Rows: 1})
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 1 {
		t.Fatalf("single-row strip is %d rows, want 1:\n%s", len(rows), out)
	}
	if plain := stripANSI(rows[0]); !strings.Contains(plain, "working") {
		t.Errorf("single-row strip drops the state:\n%s", plain)
	}
}

// Threshold colors change at 60 and 80 percent.
func TestThresholdClass(t *testing.T) {
	for pct, want := range map[int64]string{0: "calm", 34: "calm", 59: "calm", 60: "warm", 71: "warm", 79: "warm", 80: "hot", 93: "hot", 150: "hot"} {
		if got := ThresholdClass(pct); got != want {
			t.Errorf("ThresholdClass(%d) = %q, want %q", pct, got, want)
		}
	}
}

// The orchestrator is always mauve; any other agent keeps one stable hue
// from the identity set, never a threshold color.
func TestAgentHue(t *testing.T) {
	dark, light := AgentHue("anything", "orchestrator")
	if dark != "#cba6f7" || light != "#8839ef" {
		t.Fatalf("orchestrator hue = %s / %s, want mauve #cba6f7 / #8839ef", dark, light)
	}
	if dark, light := AgentHue("anything", ""); dark != "#cba6f7" || light != "#8839ef" {
		t.Fatalf("top-level session hue = %s / %s, want mauve", dark, light)
	}
	a1, _ := AgentHue("worker-04", "worker")
	a2, _ := AgentHue("worker-04", "worker")
	if a1 != a2 {
		t.Fatalf("agent hue unstable: %s vs %s", a1, a2)
	}
	for _, id := range []string{"w1", "w2", "w3", "w4", "w5", "w6", "w7"} {
		dark, _ := AgentHue(id, "worker")
		for _, banned := range []string{"#a6e3a1", "#f9e2af", "#f38ba8"} {
			if dark == banned {
				t.Errorf("agent %s takes threshold color %s", id, dark)
			}
		}
	}
}

// Numbers are humanized and rounded; empty stats are hidden, never zero.
func TestRenderHeaderHidesEmpty(t *testing.T) {
	snap := wideSnapshot()
	snap.Cost, snap.HasCost = 0, false
	snap.Model = ""
	snap.CompressCount = 0
	snap.TopTools = nil
	plain := stripANSI(renderTrue(snap, 100))
	for _, banned := range []string{"~$", "opus", "×", "0 tok", "top"} {
		if strings.Contains(plain, banned) {
			t.Errorf("strip prints empty stat %q:\n%s", banned, plain)
		}
	}
}

// A warning replaces the state pill, in the hot color.
func TestRenderHeaderWarning(t *testing.T) {
	snap := wideSnapshot()
	snap.Warning = "no compressModel"
	plain := stripANSI(renderTrue(snap, 100))
	if !strings.Contains(plain, "no compressModel") {
		t.Errorf("strip misses the warning:\n%s", plain)
	}
}

// The compressing and queued states each render their own pill; an idle
// agent shows no state word at all.
func TestRenderHeaderStates(t *testing.T) {
	for state, want := range map[HeaderState]string{
		HeaderCompressing:    "compressing…",
		HeaderQueued:         "compress queued",
		HeaderCompressFailed: "compress failed",
	} {
		snap := wideSnapshot()
		snap.State = state
		if plain := stripANSI(renderTrue(snap, 100)); !strings.Contains(plain, want) {
			t.Errorf("state %q renders without %q:\n%s", state, want, plain)
		}
	}
}

func TestRenderHeaderIdleShowsNoStateWord(t *testing.T) {
	snap := wideSnapshot()
	snap.State = HeaderIdle
	snap.Running = ToolCall{}
	plain := stripANSI(renderTrue(snap, 100))
	for _, bad := range []string{"idle", "working", "reading"} {
		if strings.Contains(plain, bad) {
			t.Errorf("idle strip shows %q:\n%s", bad, plain)
		}
	}
	if !strings.Contains(plain, "5h") {
		t.Errorf("idle strip dropped the usage:\n%s", plain)
	}
}

// Downgraded profiles keep the same text without truecolor sequences.
func TestRenderHeaderDowngrade(t *testing.T) {
	full := stripANSI(renderTrue(wideSnapshot(), 100))
	for _, profile := range []termenv.Profile{termenv.ANSI256, termenv.ANSI} {
		out := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: true, Profile: profile})
		if strings.Contains(out, "38;2") {
			t.Errorf("profile %v keeps truecolor sequences", profile)
		}
		if got := stripANSI(out); got != full {
			t.Errorf("profile %v changes the text:\n%s\nwant:\n%s", profile, got, full)
		}
	}
}

// The light theme renders the same layout from the Latte palette.
func TestRenderHeaderLight(t *testing.T) {
	out := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: false, Profile: termenv.TrueColor})
	plain := stripANSI(out)
	for _, want := range []string{"71%", "142k", "~$1.82", "reading"} {
		if !strings.Contains(plain, want) {
			t.Errorf("light strip misses %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(out, "38;2") {
		t.Error("light truecolor strip has no styled output at all")
	}
}

// A Gemini worker with 228k tokens shows ~22% against the 1M window, not 114%.
func TestRenderHeaderGeminiWindowAt228k(t *testing.T) {
	snap := wideSnapshot()
	snap.Model = "antigravity-gemini-3.8-flash"
	snap.ContextTokens = 228364
	snap.Window = ModelWindow(snap.Model, nil)
	out := renderTrue(snap, 100)
	plain := stripANSI(out)
	if !strings.Contains(plain, "22%") {
		t.Errorf("expected 22%% for 228k/1M, got:\n%s", plain)
	}
	if strings.Contains(plain, "114%") {
		t.Errorf("strip must not show 114%%, got:\n%s", plain)
	}
	if !strings.Contains(plain, "1000k") {
		t.Errorf("strip must show 1000k window, got:\n%s", plain)
	}
}

func TestPillShowsQueuedCountdown(t *testing.T) {
	snap := wideSnapshot()
	snap.Pending = 3
	snap.ClearIn = 7
	got := stripANSI(RenderHeader(snap, 200, HeaderOpts{}))
	if !strings.Contains(got, "queued 3 messages") || !strings.Contains(got, "draft clears in 7s") {
		t.Errorf("strip lacks queued countdown:\n%s", got)
	}
	snap.ClearIn = 0
	got = stripANSI(RenderHeader(snap, 200, HeaderOpts{}))
	if !strings.Contains(got, "3 pending") || strings.Contains(got, "clears in") {
		t.Errorf("strip without countdown must show plain pending:\n%s", got)
	}
}
