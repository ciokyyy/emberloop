package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/wick/tools"
)

// settingsFile points the settings store at a temp file and clears the env
// overrides, so a test reads and writes only what the command wrote.
func settingsFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("WICK_SETTINGS", p)
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	return p
}

func noModels() ([]string, error) { return nil, nil }

func TestSettingsPrintsCurrentSettings(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"decision_model":"typesafe/jev-1.13","threshold":0.3}`)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"typesafe/jev-1.13", "0.3"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("settings output %q does not mention %q", out.String(), want)
		}
	}
}

func TestSettingsSavesDecisionModel(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"decision-model", "openai/gpt-5"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "openai/gpt-5") {
		t.Fatalf("output %q does not confirm the new model", out.String())
	}
	if got := tools.LoadSettings().DecisionModel; got != "openai/gpt-5" {
		t.Fatalf("saved decision model is %q, want openai/gpt-5", got)
	}
}

func TestSettingsSavesThreshold(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"threshold", "0.42"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Threshold; got != 0.42 {
		t.Fatalf("saved threshold is %v, want 0.42", got)
	}
}

func TestSettingsRejectsBadThreshold(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"threshold", "soon"}, &out, noModels); err == nil {
		t.Fatal("a non-numeric threshold should be an error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected threshold must not be saved")
	}
}

func TestSettingsModelsListsFetchedIds(t *testing.T) {
	settingsFile(t)
	called := false
	var out bytes.Buffer
	fetch := func() ([]string, error) {
		called = true
		return []string{"typesafe/jev-1.13", "openai/gpt-5"}, nil
	}
	if err := runSettings([]string{"models"}, &out, fetch); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("models must ask for the id list")
	}
	for _, want := range []string{"typesafe/jev-1.13", "openai/gpt-5"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("models output %q does not list %q", out.String(), want)
		}
	}
}

func TestSettingsUnknownSubcommandIsUsageError(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"colour"}, &out, noModels)
	if err == nil {
		t.Fatal("an unknown settings subcommand should fail")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("error %q should carry the usage line", err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The printed slots name their value's source, so an unset slot reads as the
// default in force and a stored one as the file's.
func TestSettingsPrintsSlotsWithTheirSource(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"decision_model":"typesafe/jev-1.13","threshold":0.3,"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"}}}`)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"typesafe/jev-1.13 (file)",
		"0.3 (file)",
		"orchestrator: harness claude (file), model opus (file), effort high (file)",
		"implementer: harness pig (default), model - (default), effort - (default)",
		"scout: harness pig (default), model - (default), effort - (default)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings output does not mention %q:\n%s", want, got)
		}
	}
	for _, slot := range []string{"orchestrator", "implementer", "code-review", "spec-review", "scout"} {
		if !strings.Contains(got, slot+":") {
			t.Errorf("settings output omits the %s slot:\n%s", slot, got)
		}
	}
}

func TestSettingsSetsRoleFields(t *testing.T) {
	p := settingsFile(t)
	cases := []struct {
		argv []string
		want tools.Role
	}{
		{[]string{"role", "scout", "harness", "claude"}, tools.Role{Harness: "claude"}},
		{[]string{"role", "scout", "model", "haiku"}, tools.Role{Harness: "claude", Model: "haiku"}},
		{[]string{"role", "scout", "effort", "low"}, tools.Role{Harness: "claude", Model: "haiku", Effort: "low"}},
	}
	var out bytes.Buffer
	for _, c := range cases {
		if err := runSettings(c.argv, &out, noModels); err != nil {
			t.Fatalf("%v: %v", c.argv, err)
		}
		if got := tools.LoadSettings().Roles["scout"]; got != c.want {
			t.Fatalf("after %v scout is %+v, want %+v", c.argv, got, c.want)
		}
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("role settings were not written to %s: %v", p, err)
	}
}

// A model and effort chosen for one harness mean nothing to another, so a
// harness change clears them.
func TestSettingsChangingRoleHarnessClearsModelAndEffort(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"pig","model":"m","effort":"high"}}}`)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "harness", "claude"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != (tools.Role{Harness: "claude"}) {
		t.Fatalf("harness change left %+v, want only the harness", got)
	}
}

// Setting the harness it already has is not a change, so it keeps model and
// effort.
func TestSettingsSamenessKeepsModelAndEffort(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"}}}`)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "harness", "claude"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != (tools.Role{Harness: "claude", Model: "opus", Effort: "high"}) {
		t.Fatalf("same-harness set changed %+v", got)
	}
}

func TestSettingsRejectsAnUnknownSlot(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"role", "worker", "model", "m"}, &out, noModels)
	if err == nil || !strings.Contains(err.Error(), "role slot") {
		t.Fatalf("unknown slot must fail naming the slot, got %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected slot must not be saved")
	}
}

func TestSettingsRejectsAnUnknownHarness(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"role", "orchestrator", "harness", "gemini"}, &out, noModels)
	if err == nil || !strings.Contains(err.Error(), "harness") {
		t.Fatalf("unknown harness must fail naming the harness, got %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected harness must not be saved")
	}
}

func TestSettingsRejectsAnUnknownRoleField(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "temperature", "0.5"}, &out, noModels); err == nil {
		t.Fatal("an unknown role field should fail")
	}
}

func TestSettingsRolePrintsTheNewValue(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "spec-review", "model", "opus"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "spec-review: harness pig (default), model opus (file)") {
		t.Fatalf("output %q does not confirm the new model with its source", out.String())
	}
}
