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
