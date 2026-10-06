package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam (tui 01): the dashboard binary — `wick-pig tui` execs the wick-tui
// installed beside it, falling back to PATH, and fails naming install when
// there is none. The starter is injected so tests never launch a terminal.

// fakeTUIBin drops an executable wick-tui onto PATH, so the lookup finds it
// everywhere except beside the test binary.
func fakeTUIBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, TUIBinaryName)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestFindTUIBinaryFallsBackToPATH(t *testing.T) {
	want := fakeTUIBin(t)
	got, err := FindTUIBinary()
	if err != nil {
		t.Fatalf("FindTUIBinary: %v", err)
	}
	if got != want {
		t.Errorf("FindTUIBinary = %q, want the PATH binary %q", got, want)
	}
}

func TestFindTUIBinaryMissingNamesInstall(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := FindTUIBinary(); err == nil {
		t.Fatal("with no wick-tui anywhere the lookup must fail")
	} else if !strings.Contains(err.Error(), "install") {
		t.Errorf("the failure must point at install, got: %v", err)
	}
}

func TestRunTUIStartsTheBinaryAndRefusesArgs(t *testing.T) {
	path := fakeTUIBin(t)
	old := tuiStarter
	var started string
	tuiStarter = func(bin string, argv []string) error {
		started = bin
		return nil
	}
	t.Cleanup(func() { tuiStarter = old })

	if err := RunTUI(nil); err != nil {
		t.Fatalf("RunTUI: %v", err)
	}
	if started != path {
		t.Errorf("started = %q, want %q", started, path)
	}
	tuiStarter = func(bin string, argv []string) error {
		t.Errorf("a refused RunTUI must not start anything, got %s %v", bin, argv)
		return nil
	}
	if err := RunTUI([]string{"--bogus"}); err == nil {
		t.Error("RunTUI with args must fail with usage")
	} else if !strings.Contains(err.Error(), "usage") {
		t.Errorf("the failure must print usage, got: %v", err)
	}
}
