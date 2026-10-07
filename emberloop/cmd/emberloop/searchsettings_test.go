package main

import (
	"bytes"
	"strings"
	"testing"

	"example.com/emberloop/tools"
)

// Seam: settings search command — search.provider and search.mode are
// settable through emberloop settings, validated, and printed with sources.

func TestSettingsSearchSetsProvider(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"search", "provider", "exa"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveSearchProvider(); got != "exa" {
		t.Fatalf("saved search provider is %q, want exa", got)
	}
	if !strings.Contains(out.String(), "exa") {
		t.Fatalf("output %q does not confirm the new provider", out.String())
	}
}

func TestSettingsSearchSetsMode(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"search", "mode", "parallel"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveSearchMode(); got != "parallel" {
		t.Fatalf("saved search mode is %q, want parallel", got)
	}
}

func TestSettingsSearchRejectsUnknownValues(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"search", "provider", "tavily"}, &out, noModels); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
	if err := runSettings([]string{"search", "mode", "turbo"}, &out, noModels); err == nil {
		t.Fatal("expected an error for an unknown mode")
	}
	if err := runSettings([]string{"search", "engine", "exa"}, &out, noModels); err == nil {
		t.Fatal("expected an error for an unknown search field")
	}
}

func TestSettingsPrintShowsSearch(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"search:", "auto", "fallback"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("settings output %q does not mention %q", out.String(), want)
		}
	}
}
