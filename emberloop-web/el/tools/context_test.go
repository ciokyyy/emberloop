package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// Seam: context settings — the compress threshold (global with a
// per-harness override, default 200k) and the model window overrides.
// Expected values come from the ticket, never the implementation.

// writeContextSettings writes body as the settings file and returns the
// effective settings read back from it.
func writeContextSettings(t *testing.T, body string) Settings {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, p)
	t.Setenv("EL_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return LoadSettings()
}

func TestCompressThresholdDefaultsTo200k(t *testing.T) {
	s := writeContextSettings(t, `{}`)
	if got := s.EffectiveCompressThreshold("pig"); got != 200000 {
		t.Errorf("default threshold = %d, want 200000", got)
	}
	if got := s.EffectiveCompressThreshold("claude"); got != 200000 {
		t.Errorf("default threshold = %d, want 200000", got)
	}
}

func TestCompressThresholdGlobalAndPerHarness(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_threshold":150000,"harness_thresholds":{"pig":300000}}}`)
	if got := s.EffectiveCompressThreshold("pig"); got != 300000 {
		t.Errorf("pig threshold = %d, want 300000", got)
	}
	if got := s.EffectiveCompressThreshold("claude"); got != 150000 {
		t.Errorf("claude threshold = %d, want global 150000", got)
	}
}

func TestCompressThresholdEnvWins(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_threshold":150000}}`)
	t.Setenv("EL_COMPRESS_THRESHOLD", "50000")
	s = LoadSettings()
	if got := s.EffectiveCompressThreshold("pig"); got != 50000 {
		t.Errorf("env threshold = %d, want 50000", got)
	}
}

func TestCompressThresholdNonPositiveMeansDefault(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_threshold":-5,"harness_thresholds":{"pig":0}}}`)
	if got := s.EffectiveCompressThreshold("pig"); got != 200000 {
		t.Errorf("non-positive threshold = %d, want 200000", got)
	}
}

func TestContextWindowsRoundTrip(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"windows":{"my-model":123456}}}`)
	if got := s.ContextWindows()["my-model"]; got != 123456 {
		t.Fatalf("windows = %+v, want my-model 123456", s.ContextWindows())
	}
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	back := LoadSettings()
	if got := back.ContextWindows()["my-model"]; got != 123456 {
		t.Errorf("round trip windows = %+v, want my-model 123456", back.ContextWindows())
	}
}

func TestContextProjectMergesByKey(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, global)
	t.Setenv("EL_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(global, []byte(`{"context":{"compress_threshold":150000,"windows":{"a":1,"b":2}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".emberloop")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "settings.json"), []byte(`{"context":{"windows":{"b":3}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := EffectiveSettings(dir)
	if got := s.EffectiveCompressThreshold("pig"); got != 150000 {
		t.Errorf("merged threshold = %d, want 150000", got)
	}
	w := s.ContextWindows()
	if w["a"] != 1 || w["b"] != 3 {
		t.Errorf("merged windows = %+v, want a:1 b:3", w)
	}
}

// The environment names one global value: it wins over the file's global
// and per-harness values alike.
func TestCompressThresholdEnvBeatsHarnessOverride(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_threshold":150000,"harness_thresholds":{"pig":300000}}}`)
	t.Setenv("EL_COMPRESS_THRESHOLD", "50000")
	s = LoadSettings()
	if got := s.EffectiveCompressThreshold("pig"); got != 50000 {
		t.Errorf("pig threshold = %d, want env 50000", got)
	}
	if got := s.EffectiveCompressThreshold("claude"); got != 50000 {
		t.Errorf("claude threshold = %d, want env 50000", got)
	}
}

// A non-positive project value is not a value: the global one stands, and
// no bogus override appears.
func TestNegativeProjectValuesAreIgnored(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, global)
	t.Setenv("EL_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(global, []byte(`{"context":{"compress_threshold":150000,"windows":{"a":1}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".emberloop")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "settings.json"), []byte(`{"context":{"compress_threshold":-5,"harness_thresholds":{"pig":0},"windows":{"b":-2}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := EffectiveSettings(dir)
	if got := s.EffectiveCompressThreshold("pig"); got != 150000 {
		t.Errorf("merged threshold = %d, want 150000", got)
	}
	if w := s.ContextWindows(); len(w) != 1 || w["a"] != 1 {
		t.Errorf("merged windows = %+v, want only a:1", w)
	}
}

// Seam: compress model — one model per harness names the summarizer. Unset
// means compression refuses; the header warns instead of defaulting.
func TestCompressModelUnsetByDefault(t *testing.T) {
	s := writeContextSettings(t, `{}`)
	if got := s.EffectiveCompressModel("pig"); got != "" {
		t.Errorf("default compress model = %q, want unset", got)
	}
}

func TestCompressModelPerHarness(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_models":{"pig":"pig-cheap","claude":"claude-cheap"}}}`)
	if got := s.EffectiveCompressModel("pig"); got != "pig-cheap" {
		t.Errorf("pig compress model = %q, want pig-cheap", got)
	}
	if got := s.EffectiveCompressModel("claude"); got != "claude-cheap" {
		t.Errorf("claude compress model = %q, want claude-cheap", got)
	}
	if got := s.EffectiveCompressModel("unknown-harness"); got != "" {
		t.Errorf("unknown harness compress model = %q, want unset", got)
	}
}

func TestCompressModelRoundTrip(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_models":{"pig":"pig-cheap"}}}`)
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	back := LoadSettings()
	if got := back.EffectiveCompressModel("pig"); got != "pig-cheap" {
		t.Errorf("round trip compress model = %q, want pig-cheap", got)
	}
}

// Seam: auto-compress guards — the disable switch (default on, manual still
// works) and the cooldown in tool calls after a compress (default 5).
// Expected values come from the ticket, never the implementation.
func TestAutoCompressDefaultsOn(t *testing.T) {
	s := writeContextSettings(t, `{}`)
	if !s.EffectiveAutoCompress() {
		t.Error("default auto-compress is off, want on")
	}
}

func TestAutoCompressDisableSwitch(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"auto_compress":false}}`)
	if s.EffectiveAutoCompress() {
		t.Error("auto_compress false reads on, want off")
	}
	s = writeContextSettings(t, `{"context":{"auto_compress":true}}`)
	if !s.EffectiveAutoCompress() {
		t.Error("auto_compress true reads off, want on")
	}
}

func TestCompressCooldownDefaultsTo5Calls(t *testing.T) {
	s := writeContextSettings(t, `{}`)
	if got := s.EffectiveCompressCooldown(); got != 5 {
		t.Errorf("default cooldown = %d, want 5", got)
	}
}

func TestCompressCooldownFromFile(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_cooldown":3}}`)
	if got := s.EffectiveCompressCooldown(); got != 3 {
		t.Errorf("cooldown = %d, want 3", got)
	}
}

func TestCompressCooldownNonPositiveMeansDefault(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"compress_cooldown":0}}`)
	if got := s.EffectiveCompressCooldown(); got != 5 {
		t.Errorf("zero cooldown = %d, want 5", got)
	}
}

func TestAutoCompressRoundTrip(t *testing.T) {
	s := writeContextSettings(t, `{"context":{"auto_compress":false,"compress_cooldown":3}}`)
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	back := LoadSettings()
	if back.EffectiveAutoCompress() {
		t.Error("round trip auto-compress is on, want off")
	}
	if got := back.EffectiveCompressCooldown(); got != 3 {
		t.Errorf("round trip cooldown = %d, want 3", got)
	}
}

func TestAutoCompressProjectMerges(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, global)
	t.Setenv("EL_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(global, []byte(`{"context":{"auto_compress":false,"compress_cooldown":9}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".emberloop")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "settings.json"), []byte(`{"context":{"auto_compress":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := EffectiveSettings(dir)
	if !s.EffectiveAutoCompress() {
		t.Error("merged auto-compress is off, want the project's true")
	}
	if got := s.EffectiveCompressCooldown(); got != 9 {
		t.Errorf("merged cooldown = %d, want the global 9", got)
	}
}
