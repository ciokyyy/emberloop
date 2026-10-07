package main

import (
	"strings"
	"testing"
)

// The compress command takes exactly one pane and compresses through the
// supervisor flow; a pane no agent runs in is a clear error.
func TestCompressNeedsAPane(t *testing.T) {
	var b strings.Builder
	if err := runCompress(nil, &b); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("compress without a pane = %v, want a usage error", err)
	}
}

func TestCompressUnknownPaneFails(t *testing.T) {
	settingsFile(t)
	// An unknown pane fails in state resolution with zero matches, before
	// any herdr call, so the test needs no fake exec.
	var b strings.Builder
	err := runCompress([]string{"pane-ghost"}, &b)
	if err == nil || !strings.Contains(err.Error(), "pane-ghost") {
		t.Errorf("compress on an unknown pane must fail naming it, got %v", err)
	}
}

func TestRequestCompressNeedsAPane(t *testing.T) {
	var b strings.Builder
	if err := runRequestCompress(nil, &b); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("request-compress without a pane = %v, want a usage error", err)
	}
}

func TestRequestCompressUnknownPaneFails(t *testing.T) {
	settingsFile(t)
	var b strings.Builder
	err := runRequestCompress([]string{"pane-ghost"}, &b)
	if err == nil || !strings.Contains(err.Error(), "pane-ghost") {
		t.Errorf("request-compress on an unknown pane must fail naming it, got %v", err)
	}
}

// --force is a flag, not a pane: it still resolves the pane it carries.
func TestCompressForceFlagKeepsThePane(t *testing.T) {
	settingsFile(t)
	var b strings.Builder
	err := runCompress([]string{"--force", "pane-ghost"}, &b)
	if err == nil || !strings.Contains(err.Error(), "pane-ghost") {
		t.Errorf("compress --force on an unknown pane must fail naming it, got %v", err)
	}
}
