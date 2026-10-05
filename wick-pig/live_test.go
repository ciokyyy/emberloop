package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveReadOnly runs a real reviewer pane. WICK_LIVE=pig|claude, inside herdr.
func TestLiveReadOnly(t *testing.T) {
	h := os.Getenv("WICK_LIVE")
	if h == "" {
		t.Skip("set WICK_LIVE=pig or claude, inside herdr")
	}
	t.Setenv("WICK_READONLY_HARNESS", h)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	got, err := ReadOnly(osExec, mustAbs(t, ".."), "", "Use read on wick-pig/go.mod. Reply with only its first line.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "module example.com/wick") {
		t.Errorf("reply = %q", got)
	}
}

func mustAbs(t *testing.T, p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
