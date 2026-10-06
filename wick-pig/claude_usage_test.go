package wick

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"
)

const statusBlob = `{"session_id":"s","rate_limits":{"five_hour":{"used_percentage":36.6,"resets_at":2000},"seven_day":{"used_percentage":14,"resets_at":900000}}}`

func TestRunClaudeStatusStoresWindows(t *testing.T) {
	dir := t.TempDir()
	if err := RunClaudeStatus(strings.NewReader(statusBlob), dir); err != nil {
		t.Fatal(err)
	}
	u := ReadClaudeUsage(dir, time.Unix(1000, 0))
	if !u.Has || u.FiveHour.Pct != 37 || u.SevenDay.Pct != 14 || u.FiveHour.ResetsAt != 2000 {
		t.Errorf("usage = %+v", u)
	}
}

func TestRunClaudeStatusIgnoresBlobWithoutLimits(t *testing.T) {
	dir := t.TempDir()
	_ = RunClaudeStatus(strings.NewReader(statusBlob), dir)
	_ = RunClaudeStatus(strings.NewReader(`{"session_id":"s"}`), dir)
	_ = RunClaudeStatus(strings.NewReader(`not json`), dir)
	if u := ReadClaudeUsage(dir, time.Unix(1000, 0)); u.FiveHour.Pct != 37 {
		t.Errorf("stored usage was overwritten: %+v", u)
	}
}

func TestReadClaudeUsageRolledWindowReadsZero(t *testing.T) {
	dir := t.TempDir()
	_ = RunClaudeStatus(strings.NewReader(statusBlob), dir)
	u := ReadClaudeUsage(dir, time.Unix(5000, 0))
	if u.FiveHour.Pct != 0 || u.SevenDay.Pct != 14 {
		t.Errorf("after the 5h reset = %+v", u)
	}
}

func TestReadClaudeUsageMissingFile(t *testing.T) {
	if u := ReadClaudeUsage(t.TempDir(), time.Now()); u.Has {
		t.Errorf("empty dir has usage: %+v", u)
	}
}

func TestUsageResetIn(t *testing.T) {
	now := time.Unix(1000, 0)
	for in, want := range map[int64]string{
		0: "", 1000: "now", 1060: "1m", 1000 + 25*60: "25m", 1000 + 100*60: "1h40m",
		1000 + 26*3600: "1d2h", 1000 + 100*3600: "4d4h",
	} {
		if got := usageResetIn(in, now); got != want {
			t.Errorf("usageResetIn(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestUsagePie(t *testing.T) {
	for pct, want := range map[int]string{0: "○", 20: "◔", 50: "◑", 75: "◕", 100: "●"} {
		if got := usagePie(pct); got != want {
			t.Errorf("usagePie(%d) = %q, want %q", pct, got, want)
		}
	}
}

func TestSecondRowShowsClaudeUsage(t *testing.T) {
	snap := HeaderSnapshot{
		AgentName: "implementer", State: HeaderIdle, Window: 1000, ContextTokens: 100,
		Now:   time.Unix(0, 0),
		Usage: ClaudeUsage{Has: true, FiveHour: UsageWindow{Pct: 37, ResetsAt: 6000}, SevenDay: UsageWindow{Pct: 92, ResetsAt: 4 * 86400}}.Set(),
	}
	out := stripANSI(RenderHeader(snap, 120, HeaderOpts{Rows: 2, Dark: true, Profile: termenv.TrueColor}))
	rows := strings.Split(out, "\n")
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	for _, want := range []string{"5h ◔ 37% ↻1h40m", "wk ● 92% ↻4d0h"} {
		if !strings.Contains(rows[1], want) {
			t.Errorf("second row misses %q: %q", want, rows[1])
		}
	}
	snap.Usage = UsageSet{}
	if out := stripANSI(RenderHeader(snap, 120, HeaderOpts{Rows: 2, Dark: true, Profile: termenv.TrueColor})); strings.Contains(out, "5h") {
		t.Error("usage shown with none reported")
	}
}

func TestClaudeStopGateCarriesStatusLine(t *testing.T) {
	if !strings.Contains(claudeStopGate, `"statusLine":{"type":"command","command":"wick-pig claude-status"}`) {
		t.Error("claude settings lack the statusLine command")
	}
}

func TestTopLevelChipName(t *testing.T) {
	for _, c := range []struct{ typ, role, want string }{
		{"", "implement", "orchestrator"},
		{"", "", "orchestrator"},
		{"", "plan", "plan"},
		{"", "full", "full"},
		{"worker", "implement", "implementer"},
	} {
		if got := TopLevelChipName(c.typ, c.role); got != c.want {
			t.Errorf("TopLevelChipName(%q, %q) = %q, want %q", c.typ, c.role, got, c.want)
		}
	}
}

func TestStatusLineStoresLiveTranscriptForResume(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "resumed.jsonl")
	if err := os.WriteFile(tr, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_AGENT_ID", "a1")
	in := `{"session_id":"s2","transcript_path":` + strconv.Quote(tr) + `}`
	if err := RunClaudeStatus(strings.NewReader(in), dir); err != nil {
		t.Fatal(err)
	}
	got := resolveTranscript(HeaderSource{Harness: "claude", AgentID: "a1", SessionID: "s1", StateDir: dir})
	if got != tr {
		t.Fatalf("got %q, want %q", got, tr)
	}
}
