package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam: search settings — the default provider and mode come from the
// settings file's search object, the project file wins field by field, and
// the environment wins over both, in the style of the existing overrides.

// Nothing stored means auto provider and fallback mode.
func TestSearchDefaultsToAutoFallback(t *testing.T) {
	settingsEnv(t, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "")
	s := LoadSettings()
	if got := s.EffectiveSearchProvider(); got != "auto" {
		t.Fatalf("provider = %q, want auto", got)
	}
	if got := s.EffectiveSearchMode(); got != "fallback" {
		t.Fatalf("mode = %q, want fallback", got)
	}
}

// The file's search object is read, with its source.
func TestSearchReadsTheFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, p)
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "")
	if err := os.WriteFile(p, []byte(`{"search":{"provider":"exa","mode":"parallel"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if got := s.EffectiveSearchProvider(); got != "exa" {
		t.Fatalf("provider = %q, want exa", got)
	}
	if got := s.EffectiveSearchMode(); got != "parallel" {
		t.Fatalf("mode = %q, want parallel", got)
	}
	if got := s.SourceOf("search.provider"); got != "file" {
		t.Fatalf("provider source = %q, want file", got)
	}
}

// The project file wins field by field over the global file.
func TestSearchProjectWinsFieldByField(t *testing.T) {
	global := filepath.Join(t.TempDir(), "global.json")
	settingsEnv(t, global)
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "")
	if err := os.WriteFile(global, []byte(`{"search":{"provider":"exa","mode":"fallback"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("EMBERLOOP_PROJECT_SETTINGS", filepath.Join(root, "settings.json"))
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"search":{"mode":"parallel"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := EffectiveSettings(root)
	if got := s.EffectiveSearchProvider(); got != "exa" {
		t.Fatalf("provider = %q, want exa from global", got)
	}
	if got := s.EffectiveSearchMode(); got != "parallel" {
		t.Fatalf("mode = %q, want parallel from project", got)
	}
}

// The environment wins over both files, like the other overrides.
func TestSearchEnvWinsOverFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, p)
	if err := os.WriteFile(p, []byte(`{"search":{"provider":"exa","mode":"parallel"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "brave")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "fallback")
	s := LoadSettings()
	if got := s.EffectiveSearchProvider(); got != "brave" {
		t.Fatalf("provider = %q, want brave from env", got)
	}
	if got := s.EffectiveSearchMode(); got != "fallback" {
		t.Fatalf("mode = %q, want fallback from env", got)
	}
	if got := s.SourceOf("search.provider"); got != "env" {
		t.Fatalf("provider source = %q, want env", got)
	}
}

// A value outside the vocabulary reads as the default, never an error.
func TestSearchInvalidValuesReadAsDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, p)
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "")
	if err := os.WriteFile(p, []byte(`{"search":{"provider":"tavily","mode":"turbo"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if got := s.EffectiveSearchProvider(); got != "auto" {
		t.Fatalf("provider = %q, want auto", got)
	}
	if got := s.EffectiveSearchMode(); got != "fallback" {
		t.Fatalf("mode = %q, want fallback", got)
	}
}

// The search object survives a save, with unknown subfields kept.
func TestSearchRoundTripsThroughSave(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, p)
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "")
	if err := os.WriteFile(p, []byte(`{"search":{"provider":"brave","future":"keep"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	s.Search.Mode = "parallel"
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	again := LoadSettings()
	if got := again.EffectiveSearchProvider(); got != "brave" {
		t.Fatalf("provider = %q, want brave", got)
	}
	if got := again.EffectiveSearchMode(); got != "parallel" {
		t.Fatalf("mode = %q, want parallel", got)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "keep") {
		t.Fatalf("save dropped the unknown search field: %s", raw)
	}
}

// A save without search writes no search key: omitempty on the pointer
// keeps an untouched settings file free of an empty object.
func TestSaveOmitsSearchWhenUnset(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	settingsEnv(t, p)
	t.Setenv("EMBERLOOP_SEARCH_PROVIDER", "")
	t.Setenv("EMBERLOOP_SEARCH_MODE", "")
	if err := SaveSettings(LoadSettings()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "search") {
		t.Fatalf("save wrote a search key for unset search: %s", raw)
	}
}
