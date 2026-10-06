package wick

// Header wrapper tests (ticket 04): geometry, the child-output filter and
// the snapshot builder are pure or temp-dir bound, so no live agent runs.
// The PTY runner itself is tested with a fake child in TestWrapFakeChild.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/wick/statedb"
	"example.com/wick/tools"
)

// The child runs in the terminal minus the strip rows and its top rule,
// and minus one column per side when the frame is on. Degenerate sizes
// clamp, never zero.
func TestChildSize(t *testing.T) {
	w, h := ChildSize(80, 24, 2, true)
	if w != 78 || h != 21 {
		t.Errorf("framed 80x24 rows 2 = %dx%d, want 78x21", w, h)
	}
	w, h = ChildSize(80, 24, 2, false)
	if w != 80 || h != 21 {
		t.Errorf("frameless 80x24 rows 2 = %dx%d, want 80x21", w, h)
	}
	w, h = ChildSize(80, 24, 1, true)
	if w != 78 || h != 22 {
		t.Errorf("framed 80x24 rows 1 = %dx%d, want 78x22", w, h)
	}
	w, h = ChildSize(3, 2, 2, true)
	if w < 1 || h < 1 {
		t.Errorf("tiny terminal = %dx%d, want at least 1x1", w, h)
	}
}

// A child that resets the scroll region gets it re-applied: the strip
// survives DECSTBM resets and DECSTR soft resets inline. A full reset
// clears the screen, so it also asks for a repaint.
func TestFilterReappliesScrollRegion(t *testing.T) {
	for _, seq := range []string{"\x1b[r", "\x1b[!p"} {
		out, repaint := FilterChildOutput([]byte("hi"+seq+"there"), 22, "")
		s := string(out)
		if !strings.Contains(s, "hi") || !strings.Contains(s, "there") {
			t.Errorf("filter drops child text around %q: %q", seq, s)
		}
		if !strings.Contains(s, "\x1b[1;22r") {
			t.Errorf("filter does not re-apply the region after %q: %q", seq, s)
		}
		if repaint {
			t.Errorf("filter asks for a redraw after %q, want inline re-apply", seq)
		}
	}
	out, repaint := FilterChildOutput([]byte("hi\x1bcthere"), 22, "\x1b[?69h\x1b[2;79s")
	if s := string(out); !strings.Contains(s, "\x1b[1;22r") || !strings.Contains(s, "\x1b[?69h\x1b[2;79s") {
		t.Errorf("full reset repairs to %q, want region plus margin setup", s)
	}
	if !repaint {
		t.Error("full reset does not ask for a repaint")
	}
}

// A full-screen clear passes through and asks for a strip repaint right
// after, so no frame is left without the strip.
func TestFilterClearScreenRedraws(t *testing.T) {
	out, repaint := FilterChildOutput([]byte("\x1b[2J"), 22, "")
	if !strings.Contains(string(out), "\x1b[2J") {
		t.Errorf("filter eats the clear: %q", out)
	}
	if !repaint {
		t.Error("filter does not ask for a repaint after a clear")
	}
}

// Cursor addresses past the child area clamp to its last row: absolute
// addresses with and without a column, vertical addresses, and a region
// that ends past the child. The rest of the child's output, including
// colors, passes through untouched.
func TestFilterClampsCursorAndPassesText(t *testing.T) {
	out, repaint := FilterChildOutput([]byte("\x1b[30;1H\x1b[31mred\x1b[0m ok"), 22, "")
	if s := string(out); s != "\x1b[22;1H\x1b[31mred\x1b[0m ok" {
		t.Errorf("filter = %q, want clamped address with text intact", s)
	}
	if repaint {
		t.Error("filter asks for a redraw on plain text")
	}
	out, _ = FilterChildOutput([]byte("\x1b[5;1H\x1b[22;1f\x1b[H"), 22, "")
	if s := string(out); s != "\x1b[5;1H\x1b[22;1f\x1b[H" {
		t.Errorf("filter touches in-bounds addresses: %q", s)
	}
	out, _ = FilterChildOutput([]byte("\x1b[30H\x1b[25d\x1b[1;30r"), 22, "")
	if s := string(out); s != "\x1b[22;1H\x1b[22d\x1b[1;22r" {
		t.Errorf("filter = %q, want clamped row, VPA and region", s)
	}
}

// The snapshot builder folds a transcript the way the context core reads it:
// tokens and model from the latest request, tools from result sizes, the
// window from settings, the cost from the usage rows, and a warning when no
// compress model is set. Worked from the pig context fixture by hand.
func TestBuildHeaderSnapshot(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	raw, err := os.ReadFile(filepath.Join("testdata", "usage", "context-pig.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "wick.db")
	db, err := statedb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []float64{1.50, 0.50} {
		cost := c
		if _, err := statedb.InsertUsageRow(db, statedb.UsageRow{
			Agent: "w1", At: "2026-10-06T10:00:00Z", Harness: "pig",
			Kind: statedb.KindRequest, ReportedCost: &cost,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	settings := tools.Settings{Context: &tools.ContextSettings{
		Windows:        map[string]int64{"muse-spark-test": 100000},
		CompressModels: map[string]string{"pig": "cheap-long"},
	}}
	src := HeaderSource{
		AgentID: "w1", AgentType: "worker", AgentName: "worker",
		Harness: "pig", TranscriptPath: transcript, StateDir: dir,
		CompressCount: 1,
	}
	snap, err := BuildHeaderSnapshot(src, settings)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ContextTokens != 9400 {
		t.Errorf("tokens = %d, want 9400", snap.ContextTokens)
	}
	if snap.Window != 100000 {
		t.Errorf("window = %d, want settings 100000", snap.Window)
	}
	if snap.Model != "muse-spark-test" {
		t.Errorf("model = %q, want muse-spark-test", snap.Model)
	}
	if !snap.HasCost || snap.Cost != 2.0 {
		t.Errorf("cost = %v %v, want 2.0 reported", snap.Cost, snap.HasCost)
	}
	if len(snap.TopTools) != 3 || snap.TopTools[0].Name != "mcp__wick__bash" {
		t.Errorf("top tools = %+v, want the fixture's three", snap.TopTools)
	}
	if snap.CompressCount != 1 {
		t.Errorf("compress count = %d, want 1", snap.CompressCount)
	}
	if snap.Warning != "" {
		t.Errorf("warning = %q, want empty with a compress model set", snap.Warning)
	}
}

// Without a compress model the strip warns instead of staying silent about
// a compress that could never run.
func TestBuildHeaderSnapshotWarnsWithoutCompressModel(t *testing.T) {
	src := HeaderSource{
		AgentID: "w1", AgentType: "worker", AgentName: "worker",
		Harness: "pig", TranscriptPath: filepath.Join(t.TempDir(), "missing.jsonl"),
		StateDir: t.TempDir(),
	}
	snap, err := BuildHeaderSnapshot(src, tools.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Warning == "" {
		t.Error("no warning without a compress model")
	}
	if snap.HasCost {
		t.Error("missing database reports a cost")
	}
}

// While a compress runs the strip shows `compressing…` instead of the
// working state, so the user knows why the agent paused. A missing model
func TestBuildHeaderSnapshotShowsCompressing(t *testing.T) {
	isolateState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "pane-1", Compressing: true, CompressingSince: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	settings := tools.Settings{Context: &tools.ContextSettings{
		CompressModels: map[string]string{"pig": "cheap-long"},
	}}
	src := HeaderSource{
		AgentID: "w1", AgentType: "worker", AgentName: "worker",
		Harness: "pig", TranscriptPath: filepath.Join(t.TempDir(), "missing.jsonl"),
		StateDir: t.TempDir(),
	}
	snap, err := BuildHeaderSnapshot(src, settings)
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != HeaderCompressing {
		t.Errorf("state = %q, want compressing", snap.State)
	}
	if plain := stripANSI(renderTrue(snap, 100)); !strings.Contains(plain, "compressing…") {
		t.Errorf("strip misses the compressing state:\n%s", plain)
	}
	if snap.Warning != "" {
		t.Errorf("warning = %q, want empty with a compress model set", snap.Warning)
	}
}

// A stale mark from a crashed owner reads as absent: the strip shows the
// working state, never a stuck `compressing…`.
func TestBuildHeaderSnapshotIgnoresStaleCompressing(t *testing.T) {
	isolateState(t)
	oldStale := autoCompressStaleAfter
	autoCompressStaleAfter = 50 * time.Millisecond
	defer func() { autoCompressStaleAfter = oldStale }()
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "pane-1", Compressing: true, CompressingSince: time.Now().Add(-time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	settings := tools.Settings{Context: &tools.ContextSettings{
		CompressModels: map[string]string{"pig": "cheap-long"},
	}}
	src := HeaderSource{
		AgentID: "w1", AgentType: "worker", AgentName: "worker",
		Harness: "pig", TranscriptPath: filepath.Join(t.TempDir(), "missing.jsonl"),
		StateDir: t.TempDir(),
	}
	snap, err := BuildHeaderSnapshot(src, settings)
	if err != nil {
		t.Fatal(err)
	}
	if snap.State == HeaderCompressing {
		t.Error("stale mark still reads as compressing")
	}
}
