package tools

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Seam: strip settings — the header height (default two rows), the
// agent-colored frame (default off), the soft top edge versus the restored
// rule line (default soft), and the per-harness compress model the
// strip warns about when unset. Expected values come from the ticket, never
// the implementation.

func TestStripRowsDefaultsToTwo(t *testing.T) {
	s := writeContextSettings(t, `{}`)
	if got := s.EffectiveStripRows(); got != 2 {
		t.Errorf("default strip rows = %d, want 2", got)
	}
}

func TestStripRowsOneAndTwo(t *testing.T) {
	for _, want := range []int{1, 2} {
		s := writeContextSettings(t, `{"context":{"strip_rows":`+itoa(want)+`}}`)
		if got := s.EffectiveStripRows(); got != want {
			t.Errorf("strip rows = %d, want %d", got, want)
		}
	}
}

func TestStripRowsClamp(t *testing.T) {
	for _, rows := range []int{-1, 0, 3, 9} {
		s := writeContextSettings(t, `{"context":{"strip_rows":`+itoa(rows)+`}}`)
		if got := s.EffectiveStripRows(); got != 2 {
			t.Errorf("strip rows %d reads as %d, want default 2", rows, got)
		}
	}
}

func TestStripFrameDefaultsOff(t *testing.T) {
	if s := writeContextSettings(t, `{}`); s.StripFrameEnabled() {
		t.Error("default frame is on, want off")
	}
	if s := writeContextSettings(t, `{"context":{"strip_frame":false}}`); s.StripFrameEnabled() {
		t.Error("strip_frame false reads as on")
	}
	if s := writeContextSettings(t, `{"context":{"strip_frame":true}}`); !s.StripFrameEnabled() {
		t.Error("strip_frame true reads as off")
	}
}

// The borderless look keeps the soft half-block edge unless the setting
// restores the drawn rule line.
func TestStripRuleDefaultsSoft(t *testing.T) {
	if s := writeContextSettings(t, `{}`); s.StripRuleEnabled() {
		t.Error("default strip edge is the rule line, want the soft edge")
	}
	if s := writeContextSettings(t, `{"context":{"strip_rule":false}}`); s.StripRuleEnabled() {
		t.Error("strip_rule false reads as the rule line")
	}
	if s := writeContextSettings(t, `{"context":{"strip_rule":true}}`); !s.StripRuleEnabled() {
		t.Error("strip_rule true reads as the soft edge")
	}
}

func TestStripSettingsRoundTrip(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"strip_rows":1,"strip_frame":false,"strip_rule":true,"compress_models":{"pig":"m"}}}`)
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	back := LoadSettings()
	if got := back.EffectiveStripRows(); got != 1 {
		t.Errorf("round trip rows = %d, want 1", got)
	}
	if back.StripFrameEnabled() {
		t.Error("round trip frame is on, want off")
	}
	if !back.StripRuleEnabled() {
		t.Error("round trip rule is soft, want the rule line")
	}
	if got := back.EffectiveCompressModel("pig"); got != "m" {
		t.Errorf("round trip compress model = %q, want m", got)
	}
}

func TestStripProjectMerges(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, global)
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	globalBody := `{"context":{"strip_rows":1,"strip_frame":false,"strip_rule":false,"compress_models":{"pig":"g","claude":"g"}}}`
	if err := os.WriteFile(global, []byte(globalBody), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".wick-pig")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	projBody := `{"context":{"strip_rows":2,"strip_rule":true,"compress_models":{"pig":"p"}}}`
	if err := os.WriteFile(filepath.Join(proj, "settings.json"), []byte(projBody), 0o644); err != nil {
		t.Fatal(err)
	}
	s := EffectiveSettings(dir)
	if got := s.EffectiveStripRows(); got != 2 {
		t.Errorf("merged rows = %d, want project 2", got)
	}
	if s.StripFrameEnabled() {
		t.Error("merged frame is on, want global off kept")
	}
	if !s.StripRuleEnabled() {
		t.Error("merged rule is soft, want project on kept")
	}
	if got := s.EffectiveCompressModel("pig"); got != "p" {
		t.Errorf("merged pig model = %q, want project p", got)
	}
	if got := s.EffectiveCompressModel("claude"); got != "g" {
		t.Errorf("merged claude model = %q, want global g", got)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
