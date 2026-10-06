package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The context command prints one session's numbers without a header or a
// supervisor, so the core slice is verifiable on its own.
func TestContextNeedsASessionID(t *testing.T) {
	var b strings.Builder
	if err := runContext(nil, &b); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("context without a session = %v, want a usage error", err)
	}
}

func writeContextTranscript(t *testing.T, id string) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "usage", "context-pig.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-10-06_"+id+".jsonl"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestContextPrintsSessionNumbers(t *testing.T) {
	settingsFile(t)
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	dir := writeContextTranscript(t, "sess1")
	var b strings.Builder
	if err := runContext([]string{"--harness", "pig", "--dir", dir, "sess1"}, &b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"muse-spark-test", "9400", "outstanding tool calls: 1"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("context output %q lacks %q", b.String(), want)
		}
	}
}

func TestContextUnknownSessionFails(t *testing.T) {
	settingsFile(t)
	var b strings.Builder
	err := runContext([]string{"--harness", "pig", "--dir", t.TempDir(), "missing"}, &b)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("an unknown session must fail naming it, got %v", err)
	}
}

// The settings command carries the threshold (global with a per-harness
// override) and the window overrides.
func TestSettingsCompressThreshold(t *testing.T) {
	settingsFile(t)
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	var b strings.Builder
	if err := runSettings([]string{"compress-threshold", "150000"}, &b, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "150000") {
		t.Errorf("settings output %q lacks 150000", b.String())
	}
	var c strings.Builder
	if err := runSettings([]string{"compress-threshold", "pig", "300000"}, &c, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.String(), "300000") {
		t.Errorf("settings output %q lacks 300000", c.String())
	}
}

func TestSettingsWindowOverride(t *testing.T) {
	settingsFile(t)
	var b strings.Builder
	if err := runSettings([]string{"window", "my-model", "1000000"}, &b, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "my-model") {
		t.Errorf("settings output %q lacks my-model", b.String())
	}
}

// The bare settings printout carries the threshold and the overrides with
// their sources, and a cleared window persists as gone.
func TestSettingsPrintShowsContext(t *testing.T) {
	settingsFile(t)
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	var b strings.Builder
	if err := runSettings([]string{"compress-threshold", "150000"}, &b, nil); err != nil {
		t.Fatal(err)
	}
	if err := runSettings([]string{"window", "my-model", "1000000"}, &b, nil); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := runSettings(nil, &out, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"compress-threshold: 150000 (file)", "window my-model: 1000000 (file)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("settings print %q lacks %q", out.String(), want)
		}
	}
}

func TestSettingsWindowClearRemovesTheObject(t *testing.T) {
	p := settingsFile(t)
	t.Setenv("WICK_COMPRESS_THRESHOLD", "")
	var b strings.Builder
	if err := runSettings([]string{"window", "my-model", "1000000"}, &b, nil); err != nil {
		t.Fatal(err)
	}
	if err := runSettings([]string{"window", "my-model", "0"}, &b, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "my-model") || strings.Contains(string(raw), `"context"`) {
		t.Errorf("cleared settings %s still hold context", raw)
	}
}
