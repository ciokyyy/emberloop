package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The default store lives beside the user's skills, not in machine state.
func TestSettingsPathDefaultsToWickPig(t *testing.T) {
	t.Setenv("WICK_SETTINGS", "")
	t.Setenv("HOME", "/home/example")
	if got, want := SettingsPath(), filepath.Join("/home/example", ".wick-pig", "settings.json"); got != want {
		t.Fatalf("SettingsPath() = %q, want %q", got, want)
	}
}

func TestSettingsPathKeepsTheOverride(t *testing.T) {
	t.Setenv("WICK_SETTINGS", "/tmp/other.json")
	if got, want := SettingsPath(), "/tmp/other.json"; got != want {
		t.Fatalf("SettingsPath() = %q, want %q", got, want)
	}
}

// The old state-dir file is read when the new one is absent, so an upgrade
// loses nothing.
func TestSettingsReadsTheOldFileWhenTheNewOneIsAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WICK_SETTINGS", "")
	t.Setenv("HOME", home)
	old := filepath.Join(home, ".pig", "state", "wick", "settings.json")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"decision_model":"old/model","threshold":0.3,"roles":{"scout":{"harness":"claude","model":"haiku"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if s.DecisionModel != "old/model" || s.Threshold != 0.3 {
		t.Fatalf("old file not read: %+v", s)
	}
	if got := s.Roles["scout"]; got.Harness != "claude" || got.Model != "haiku" {
		t.Fatalf("old roles not read: %+v", got)
	}
}

// An explicit WICK_SETTINGS file is the whole world: the old file must not
// leak in and turn a hermetic test into a reading of the machine.
func TestSettingsOverrideDoesNotFallBackToTheOldFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := filepath.Join(home, ".pig", "state", "wick", "settings.json")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"decision_model":"old/model"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	if s := LoadSettings(); s.DecisionModel != "" {
		t.Fatalf("override read the old file: %+v", s)
	}
}

// Saving always writes the new path, even when the values came from the old
// file.
func TestSettingsSaveWritesTheNewPathOverTheOldFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WICK_SETTINGS", "")
	t.Setenv("HOME", home)
	old := filepath.Join(home, ".pig", "state", "wick", "settings.json")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"threshold":0.3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	s.Threshold = 0.5
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(home, ".wick-pig", "settings.json")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new file not written: %v", err)
	}
	if s := LoadSettings(); s.Threshold != 0.5 {
		t.Fatalf("new file holds %+v, want threshold 0.5", s)
	}
	if raw, err := os.ReadFile(old); err != nil || string(raw) != `{"threshold":0.3}` {
		t.Fatalf("old file changed: %q, %v", raw, err)
	}
}

// A key wick does not know yet survives a save, so the file can grow.
func TestSettingsKeepsUnknownKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	t.Setenv("WICK_SETTINGS", p)
	if err := os.WriteFile(p, []byte(`{"decision_model":"m","future_thing":{"a":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	s.Threshold = 0.4
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["future_thing"]; !ok {
		t.Fatalf("unknown key dropped: %s", raw)
	}
	if s2 := LoadSettings(); s2.DecisionModel != "m" || s2.Threshold != 0.4 {
		t.Fatalf("known keys lost: %+v", s2)
	}
}

func TestSettingsRoundTripsRoleSlots(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	t.Setenv("WICK_SETTINGS", p)
	want := Role{Harness: "claude", Model: "opus", Effort: "high"}
	if err := SaveSettings(Settings{Roles: map[string]Role{"orchestrator": want}}); err != nil {
		t.Fatal(err)
	}
	if got := LoadSettings().Roles["orchestrator"]; got != want {
		t.Fatalf("role round trip = %+v, want %+v", got, want)
	}
}

func TestIsRoleSlotKnowsTheSlots(t *testing.T) {
	for _, slot := range []string{"orchestrator", "implementer", "code-review", "spec-review", "scout"} {
		if !IsRoleSlot(slot) {
			t.Errorf("%q should be a role slot", slot)
		}
	}
	for _, bad := range []string{"", "worker", "review"} {
		if IsRoleSlot(bad) {
			t.Errorf("%q should not be a role slot", bad)
		}
	}
}

// The decision model and threshold report where their value came from, so the
// CLI can print it.
func TestSettingsSourceTracksFileEnvAndDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	t.Setenv("WICK_SETTINGS", p)
	t.Setenv("WICK_DECISION_MODEL", "")
	if err := os.WriteFile(p, []byte(`{"threshold":0.3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if got := s.SourceOf("threshold"); got != "file" {
		t.Fatalf("threshold source = %q, want file", got)
	}
	if got := s.SourceOf("decision_model"); got != "default" {
		t.Fatalf("decision_model source = %q, want default", got)
	}
	t.Setenv("WICK_DECISION_MODEL", "respan/span-01")
	if got := LoadSettings().SourceOf("decision_model"); got != "env" {
		t.Fatalf("env did not win the source: %q", got)
	}
}
