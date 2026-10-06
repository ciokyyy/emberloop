package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// settingsEnv points the store at path and clears the env overrides, so a test
// reads only what it wrote and never the machine's own environment.
func settingsEnv(t *testing.T, path string) {
	t.Helper()
	t.Setenv("WICK_SETTINGS", path)
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	t.Setenv("WICK_SEARCH_PROVIDER", "")
	t.Setenv("WICK_SEARCH_MODE", "")
}

// homeEnv makes the default path resolve under a temp home with the override
// unset, which is what the old-file fallback tests need.
func homeEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("WICK_SETTINGS", "")
	t.Setenv("HOME", home)
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	t.Setenv("WICK_SEARCH_PROVIDER", "")
	t.Setenv("WICK_SEARCH_MODE", "")
}

// The default store lives beside the user's skills, not in machine state.
func TestSettingsPathDefaultsToWickPig(t *testing.T) {
	settingsEnv(t, "")
	t.Setenv("HOME", "/home/example")
	if got, want := SettingsPath(), filepath.Join("/home/example", ".wick-pig", "settings.json"); got != want {
		t.Fatalf("SettingsPath() = %q, want %q", got, want)
	}
}

func TestSettingsPathKeepsTheOverride(t *testing.T) {
	settingsEnv(t, "/tmp/other.json")
	if got, want := SettingsPath(), "/tmp/other.json"; got != want {
		t.Fatalf("SettingsPath() = %q, want %q", got, want)
	}
}

// The old state-dir file is read when the new one is absent, so an upgrade
// loses nothing.
func TestSettingsReadsTheOldFileWhenTheNewOneIsAbsent(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
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
	settingsEnv(t, filepath.Join(t.TempDir(), "missing.json"))
	if s := LoadSettings(); s.DecisionModel != "" {
		t.Fatalf("override read the old file: %+v", s)
	}
}

// Saving always writes the new path, even when the values came from the old
// file.
func TestSettingsSaveWritesTheNewPathOverTheOldFile(t *testing.T) {
	home := t.TempDir()
	homeEnv(t, home)
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
	settingsEnv(t, p)
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

// A key wick does not know inside a role object survives too, so a role can
// grow without losing what a newer wick wrote there.
func TestSettingsKeepsUnknownRoleFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	settingsEnv(t, p)
	if err := os.WriteFile(p, []byte(`{"roles":{"scout":{"harness":"claude","future_field":"kept"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	s.Roles["scout"] = Role{Harness: "claude", Model: "haiku"}
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Roles map[string]map[string]json.RawMessage `json:"roles"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Roles["scout"]["future_field"]) != `"kept"` {
		t.Fatalf("unknown role field dropped: %s", raw)
	}
	if string(got.Roles["scout"]["model"]) != `"haiku"` {
		t.Fatalf("known role field lost: %s", raw)
	}
}

func TestSettingsRoundTripsRoleSlots(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	settingsEnv(t, p)
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
	settingsEnv(t, p)
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

// A file value that cannot be parsed reads as the default, and the source says
// default rather than claiming the file owns a value it does not.
func TestSettingsSourceIsDefaultWhenTheFileValueDoesNotParse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	settingsEnv(t, p)
	if err := os.WriteFile(p, []byte(`{"threshold":"soon"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if s.Threshold != 0 {
		t.Fatalf("a bad threshold should read as unset, got %v", s.Threshold)
	}
	if got := s.SourceOf("threshold"); got != "default" {
		t.Fatalf("threshold source = %q, want default", got)
	}
}

// A case variant of a known key is the known key: encoding/json reads it into
// the field, and the save must not also keep it as an unknown copy.
func TestSettingsNormalisesCaseVariantKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	settingsEnv(t, p)
	if err := os.WriteFile(p, []byte(`{"Decision_Model":"m","Roles":{"scout":{"Harness":"claude","future":"kept"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if s.DecisionModel != "m" || s.Roles["scout"].Harness != "claude" {
		t.Fatalf("case-variant keys not read: %+v", s)
	}
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
	if _, ok := got["Decision_Model"]; ok {
		t.Fatalf("a case variant of a known key was kept as an unknown: %s", raw)
	}
	if string(got["decision_model"]) != `"m"` {
		t.Fatalf("known key lost: %s", raw)
	}
	var roles struct {
		Roles map[string]map[string]json.RawMessage `json:"roles"`
	}
	if err := json.Unmarshal(raw, &roles); err != nil {
		t.Fatal(err)
	}
	if _, ok := roles.Roles["scout"]["Harness"]; ok {
		t.Fatalf("a case variant of a known role key was kept: %s", raw)
	}
	if string(roles.Roles["scout"]["harness"]) != `"claude"` {
		t.Fatalf("role key lost: %s", raw)
	}
	if string(roles.Roles["scout"]["future"]) != `"kept"` {
		t.Fatalf("unknown role field dropped under a case-variant roles key: %s", raw)
	}
}
