package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/wick"
)

// A bare launch is a planning session. full is reachable, but only by asking
// for it with the role flag.
func TestBareLaunchPlansAndTheRoleFlagSelectsFull(t *testing.T) {
	cases := []struct {
		sub, flag, want string
	}{
		{"", "", "plan"},
		{"", "full", "full"},
		{"grill", "", "plan"},
		{"to-spec", "", "plan"},
		{"implement", "", "implement"},
	}
	for _, c := range cases {
		got, err := sessionRole(c.sub, c.flag)
		if err != nil {
			t.Errorf("sessionRole(%q, %q): %v", c.sub, c.flag, err)
			continue
		}
		if got != c.want {
			t.Errorf("sessionRole(%q, %q) = %q, want %q", c.sub, c.flag, got, c.want)
		}
	}
}

func TestRoleFlagRefusesAnUnknownRole(t *testing.T) {
	if _, err := sessionRole("", "nope"); err == nil {
		t.Error("an unknown role must be refused at the launcher, not a broken session")
	}
}

// The flag belongs to a bare session: a subcommand's prompt is written for its
// own role, so pairing it with a different one is refused rather than launched.
func TestRoleFlagIsRefusedBesideASubcommand(t *testing.T) {
	for _, sub := range []string{"implement", "grill"} {
		if _, err := sessionRole(sub, "full"); err == nil {
			t.Errorf("--role full beside %q must be refused", sub)
		}
	}
}

// implement must run in a herdr pane: its workers' results are delivered into
// that pane as notices, so a session outside herdr could never hear them. The
// refusal comes before anything is created or spawned.
func TestImplementRequiresAHerdrPane(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	err := run([]string{"implement", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "herdr") {
		t.Fatalf("implement outside herdr must fail naming herdr, got %v", err)
	}
}

// The headless parent path is gone: an implement session refuses -p rather
// than run one-shot where its workers' notices cannot reach it.
func TestImplementRefusesHeadless(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	err := run([]string{"implement", "-p", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "-p") {
		t.Fatalf("implement -p must be refused, got %v", err)
	}
}

// A top-level session with no flags resolves the orchestrator slot; a flag
// still wins over it. This is the slot `wick-pig implement` launches with.
func TestTopLevelSessionResolvesTheOrchestratorSlot(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"}}}`)
	if got := topLevelChoice(wick.Choice{}); got.Harness != "claude" || got.Model != "opus" || got.Effort != "high" {
		t.Errorf("orchestrator slot = %+v, want claude/opus/high", got)
	}
	over := topLevelChoice(wick.Choice{Harness: "pig", Model: "m", Effort: "low"})
	if over.Harness != "pig" || over.Model != "m" || over.Effort != "low" {
		t.Errorf("flags must win = %+v, want pig/m/low", over)
	}
}

// --claude against a pig orchestrator slot drops the slot's model and effort:
// a pig model and an effort claude does not have must not leak onto claude.
func TestClaudeFlagDropsThePigOrchestratorSlotModel(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"pig","model":"pig-model","effort":"high"}}}`)
	if got := topLevelChoice(wick.Choice{Harness: "claude"}); got != (wick.Choice{Harness: "claude"}) {
		t.Errorf("orchestrator slot with --claude = %+v, want {claude}", got)
	}
}

// --effort is a launcher flag, and the harness turns it into its own: a pig
// session launches with --thinking <level>.
func TestEffortFlagReachesTheHarness(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_PANE_ID", "")
	out := filepath.Join(home, "args")
	script := filepath.Join(home, "pig")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_PIG", script)
	if err := run([]string{"-p", "--effort", "high", "what is it"}); err != nil {
		t.Fatalf("run with --effort: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Fields(string(got))
	i := indexOf(args, "--thinking")
	if i < 0 || i+1 >= len(args) || args[i+1] != "high" {
		t.Errorf("pig must launch with --thinking high, got %q", string(got))
	}
}

func indexOf(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}
