package main

import (
	"strings"
	"testing"
)

func TestInstallRefusesUnknownFlags(t *testing.T) {
	if err := run([]string{"install", "--bogus"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("run(install --bogus) must fail with usage, got %v", err)
	}
}

// `el deps` checks external dependencies. Here its dispatch and usage
// error are pinned; the installer itself is covered in the el package. The
// unknown flag fails before any install or download.
func TestDepsRefusesUnknownFlags(t *testing.T) {
	if err := run([]string{"deps", "--bogus"}); err == nil {
		t.Error("run(deps --bogus) must fail")
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	if err := run([]string{"tui"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("run(tui) must fail as unknown, got %v", err)
	}
}
