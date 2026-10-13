package el

import (
	"strings"
	"testing"
	"time"
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
