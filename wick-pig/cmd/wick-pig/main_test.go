package main

import "testing"

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
