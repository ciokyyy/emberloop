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
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
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
	t.Setenv("WICK_COMPRESS_THRESHOLD", "50000")
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
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(global, []byte(`{"context":{"compress_threshold":150000,"windows":{"a":1,"b":2}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".wick-pig")
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
	t.Setenv("WICK_COMPRESS_THRESHOLD", "50000")
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
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	if err := os.WriteFile(global, []byte(`{"context":{"compress_threshold":150000,"windows":{"a":1}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".wick-pig")
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
