package main

import (
	"strings"
	"testing"
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
