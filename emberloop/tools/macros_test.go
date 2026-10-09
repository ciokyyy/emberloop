package tools

// Seam: the settings file. Macros live there as name/kind/template entries;
// load, merge, save and validation all run through LoadSettings and
// MergeSettings with a temp EL_SETTINGS file, never the real home.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A prompt template macro expands {{args}} with the row's trailing words.
func TestMacroExpandReplacesArgs(t *testing.T) {
	if got := ExpandMacro("review {{args}} now", "the auth flow"); got != "review the auth flow now" {
		t.Errorf("expand = %q, want the args spliced in", got)
	}
}

// Every {{args}} slot fills, not just the first.
func TestMacroExpandReplacesEverySlot(t *testing.T) {
	if got := ExpandMacro("{{args}} vs {{args}}", "a"); got != "a vs a" {
		t.Errorf("expand = %q, want every slot filled", got)
	}
}

// No slot means the template stands as is.
func TestMacroExpandWithoutSlotIsLiteral(t *testing.T) {
	if got := ExpandMacro("just do it", "ignored"); got != "just do it" {
		t.Errorf("expand = %q, want the literal template", got)
	}
}

// Row text starting with a slash splits into macro name and trailing args.
func TestParseMacroCallSplitsNameAndArgs(t *testing.T) {
	name, args, ok := ParseMacroCall("/review the auth flow")
	if !ok || name != "review" || args != "the auth flow" {
		t.Errorf("parse = %q %q %v, want review, the auth flow, true", name, args, ok)
	}
}

// A bare slash names nothing: it is plain text, not a macro call.
func TestParseMacroCallBareSlashIsNotACall(t *testing.T) {
	if _, _, ok := ParseMacroCall("/"); ok {
		t.Error("a bare slash must not parse as a macro call")
	}
}

// Text without a leading slash never parses as a macro call.
func TestParseMacroCallPlainTextIsNotACall(t *testing.T) {
	if _, _, ok := ParseMacroCall("review this"); ok {
		t.Error("plain text must not parse as a macro call")
	}
}

// Only el's own built-ins count: anything else is a normal message.
func TestIsBuiltinMacroOnlyNamesCompress(t *testing.T) {
	if !IsBuiltinMacro("compress") {
		t.Error("compress must read as the built-in")
	}
	if IsBuiltinMacro("review") {
		t.Error("a user macro must not read as a built-in")
	}
}

// Macros load from the settings file through the plain load seam.
func TestMacrosLoadFromSettingsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	body := `{"macros":[{"name":"review","kind":"prompt","template":"review {{args}}"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", path)
	s := LoadSettings()
	if len(s.Macros) != 1 || s.Macros[0].Name != "review" {
		t.Fatalf("macros = %+v, want the one review entry", s.Macros)
	}
	if m, ok := s.LookupMacro("review"); !ok || m.Template != "review {{args}}" {
		t.Errorf("lookup = %+v %v, want the review template", m, ok)
	}
}

// A name no macro defines looks up as missing: the row sends it as text.
func TestLookupMacroUnknownIsMissing(t *testing.T) {
	s := Settings{}
	if _, ok := s.LookupMacro("frobnicate"); ok {
		t.Error("an unknown macro must look up as missing")
	}
}

// A duplicate macro name is rejected at settings load.
func TestValidateMacrosRejectsDuplicates(t *testing.T) {
	s := Settings{Macros: []MacroSetting{
		{Name: "review", Kind: "prompt", Template: "first {{args}}"},
		{Name: "review", Kind: "prompt", Template: "second {{args}}"},
	}}
	valid, _, err := s.ValidateMacros()
	if err == nil {
		t.Fatal("a duplicate macro name must report")
	}
	if len(valid) != 1 || valid[0].Template != "first {{args}}" {
		t.Errorf("valid = %+v, want the first entry kept", valid)
	}
}

// A macro shadowing a built-in warns, and el's macro wins.
func TestValidateMacrosWarnsOnBuiltinShadow(t *testing.T) {
	s := Settings{Macros: []MacroSetting{
		{Name: "compress", Kind: "prompt", Template: "summarize {{args}}"},
	}}
	_, warnings, err := s.ValidateMacros()
	if err != nil {
		t.Fatalf("a shadow must warn, not reject: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "compress") {
		t.Fatalf("warnings = %q, want the compress collision named", warnings)
	}
	if _, ok := s.LookupMacro("compress"); ok {
		t.Error("a shadowing entry must not shadow the built-in: el wins")
	}
}

// An unknown kind and an empty prompt template are rejected, not kept.
func TestValidateMacrosRejectsBadKindAndEmptyTemplate(t *testing.T) {
	s := Settings{Macros: []MacroSetting{
		{Name: "weird", Kind: "sequence", Template: "x"},
		{Name: "empty", Kind: "prompt", Template: "  "},
		{Name: "good", Kind: "prompt", Template: "go {{args}}"},
	}}
	valid, _, err := s.ValidateMacros()
	if err == nil {
		t.Fatal("a bad kind and an empty template must report")
	}
	if len(valid) != 1 || valid[0].Name != "good" {
		t.Errorf("valid = %+v, want only the good entry", valid)
	}
}

// A leading slash in the settings name still matches the row's /name.
func TestLookupMacroStripsLeadingSlash(t *testing.T) {
	s := Settings{Macros: []MacroSetting{
		{Name: "/review", Kind: "prompt", Template: "review {{args}}"},
	}}
	if _, ok := s.LookupMacro("review"); !ok {
		t.Error("/review in settings must match a /review row call")
	}
}

// Project macros win over global ones wholesale.
func TestMergeSettingsProjectMacrosWin(t *testing.T) {
	global := Settings{Macros: []MacroSetting{{Name: "a", Kind: "prompt", Template: "global"}}}
	project := Settings{Macros: []MacroSetting{{Name: "b", Kind: "prompt", Template: "project"}}}
	out := MergeSettings(global, project)
	if len(out.Macros) != 1 || out.Macros[0].Name != "b" {
		t.Errorf("merged = %+v, want the project list", out.Macros)
	}
}

// Macros survive a settings save: unknown keys are not the only round trip.
func TestMacrosSurviveSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	t.Setenv("EL_SETTINGS", path)
	want := Settings{Macros: []MacroSetting{{Name: "review", Kind: "prompt", Template: "review {{args}}"}}}
	if err := SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	got := LoadSettings()
	if len(got.Macros) != 1 || got.Macros[0] != want.Macros[0] {
		t.Errorf("round trip = %+v, want %+v", got.Macros, want.Macros)
	}
}
