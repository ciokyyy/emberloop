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
	s := writeContextSettings(t, `{"context":{"strip_rows":1,"strip_frame":false,"strip_rule":true,"strip_symbols":"nerd","strip_separator":"pipe","compress_models":{"pig":"m"}}}`)
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
	if got := back.EffectiveStripSymbols(); got != StripSymbolsNerd {
		t.Errorf("round trip symbols = %q, want nerd", got)
	}
	if got := back.EffectiveStripSeparator(); got != StripSeparatorPipe {
		t.Errorf("round trip separator = %q, want pipe", got)
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

// Seam: strip symbol preset and separator (ticket 10).
func TestStripSymbolsDefaultsUnicode(t *testing.T) {
	if got := writeContextSettings(t, `{}`).EffectiveStripSymbols(); got != StripSymbolsUnicode {
		t.Errorf("default strip symbols = %q, want %q", got, StripSymbolsUnicode)
	}
}
func TestStripSymbolsPresets(t *testing.T) {
	for _, want := range []string{StripSymbolsNerd, StripSymbolsUnicode, StripSymbolsASCII} {
		s := writeContextSettings(t, `{"context":{"strip_symbols":`+strconv.Quote(want)+`}}`)
		if got := s.EffectiveStripSymbols(); got != want {
			t.Errorf("strip symbols = %q, want %q", got, want)
		}
	}
}
func TestStripSymbolsUnknownReadsDefault(t *testing.T) {
	for _, raw := range []string{"", "emoji", "nerd-font"} {
		s := writeContextSettings(t, `{"context":{"strip_symbols":`+strconv.Quote(raw)+`}}`)
		if got := s.EffectiveStripSymbols(); got != StripSymbolsUnicode {
			t.Errorf("strip symbols %q reads as %q, want default %q", raw, got, StripSymbolsUnicode)
		}
	}
	s := writeContextSettings(t, `{"context":{"strip_symbols":"Nerd "}}`)
	if got := s.EffectiveStripSymbols(); got != StripSymbolsNerd {
		t.Errorf("strip symbols %q reads as %q, want %q", "Nerd ", got, StripSymbolsNerd)
	}
}
func TestStripSeparatorDefaultsNone(t *testing.T) {
	if got := writeContextSettings(t, `{}`).EffectiveStripSeparator(); got != StripSeparatorNone {
		t.Errorf("default strip separator = %q, want %q", got, StripSeparatorNone)
	}
}
func TestStripSeparatorValues(t *testing.T) {
	for _, want := range []string{StripSeparatorPowerline, StripSeparatorPowerlineThin, StripSeparatorRounded, StripSeparatorSlash, StripSeparatorPipe, StripSeparatorNone} {
		s := writeContextSettings(t, `{"context":{"strip_separator":`+strconv.Quote(want)+`}}`)
		if got := s.EffectiveStripSeparator(); got != want {
			t.Errorf("strip separator = %q, want %q", got, want)
		}
	}
}
func TestStripSeparatorUnknownReadsDefault(t *testing.T) {
	for _, raw := range []string{"", "arrow", "chevron"} {
		s := writeContextSettings(t, `{"context":{"strip_separator":`+strconv.Quote(raw)+`}}`)
		if got := s.EffectiveStripSeparator(); got != StripSeparatorNone {
			t.Errorf("strip separator %q reads as %q, want default %q", raw, got, StripSeparatorNone)
		}
	}
	s := writeContextSettings(t, `{"context":{"strip_separator":"Powerline-Thin "}}`)
	if got := s.EffectiveStripSeparator(); got != StripSeparatorPowerlineThin {
		t.Errorf("strip separator reads as %q, want %q", got, StripSeparatorPowerlineThin)
	}
}
func TestStripSymbolsProjectMerges(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, global)
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(global, []byte(`{"context":{"strip_symbols":"ascii","strip_separator":"slash"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".wick-pig")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "settings.json"), []byte(`{"context":{"strip_separator":"pipe"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := EffectiveSettings(dir)
	if got := s.EffectiveStripSymbols(); got != StripSymbolsASCII {
		t.Errorf("merged symbols = %q, want global ascii kept", got)
	}
	if got := s.EffectiveStripSeparator(); got != StripSeparatorPipe {
		t.Errorf("merged separator = %q, want project pipe", got)
	}
	if got := s.SourceOf("context.strip_symbols"); got != "file" {
		t.Errorf("symbols source = %q, want file", got)
	}
	if got := s.SourceOf("context.strip_separator"); got != "project" {
		t.Errorf("separator source = %q, want project", got)
	}
}
