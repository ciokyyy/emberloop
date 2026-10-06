package main

import (
	"bytes"
	"strings"
	"testing"
)

// `wick-pig wrap` needs a known harness and a command; without a terminal
// the child runs plainly, which is what the success path asserts here.

func TestWrapRejectsUnknownHarness(t *testing.T) {
	var out bytes.Buffer
	err := runWrap([]string{"--harness", "nope", "--", "true"}, nil, &out)
	if err == nil || !strings.Contains(err.Error(), `unknown harness "nope"`) {
		t.Fatalf("wrap with a bad harness = %v, want unknown-harness", err)
	}
}

func TestWrapNeedsCommand(t *testing.T) {
	var out bytes.Buffer
	if err := runWrap([]string{"--harness", "pig"}, nil, &out); err == nil {
		t.Fatal("wrap without a command should be an error")
	}
}

func TestWrapRunsPlainWithoutTerminal(t *testing.T) {
	var out bytes.Buffer
	if err := runWrap([]string{"--harness", "pig", "--", "true"}, nil, &out); err != nil {
		t.Fatalf("wrap true = %v, want plain success", err)
	}
}

// The plain fallback keeps stderr its own sink: error text must never land
// in a stdout the caller reads.
func TestWrapPlainKeepsStderrSeparate(t *testing.T) {
	var out bytes.Buffer
	err := runWrap([]string{"--harness", "pig", "--", "sh", "-c", "echo oops >&2; exit 1"}, nil, &out)
	if err == nil {
		t.Fatal("wrap of a failing command should be an error")
	}
	if strings.Contains(out.String(), "oops") {
		t.Errorf("stderr leaked into stdout: %q", out.String())
	}
}
