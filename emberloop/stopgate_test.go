package el

import (
	"strings"
	"testing"
)

// The decision is the one place the block/remind/give-up rules live: a submit,
// a provider error or pending children allow the stop; otherwise it blocks
// with reminder n of 3, pins on the 3rd attempt only where a harness can, and
// gives up after the 3rd block.
func TestGateDecisionBlocksRemindsPinsAndGivesUp(t *testing.T) {
	cases := []struct {
		name string
		in   GateInput
		want GateOutcome
	}{
		{"a submit this turn allows the stop", GateInput{Submitted: true}, GateAllow},
		{"a provider error allows the stop", GateInput{Failed: true}, GateAllow},
		{"children still owing a result allow the stop", GateInput{Pending: true}, GateAllow},
		{"the first stop without a submit blocks", GateInput{}, GateBlock},
		{"the second blocks", GateInput{Blocked: 1}, GateBlock},
		{"the third blocks without a pin where the harness cannot pin", GateInput{Blocked: 2}, GateBlock},
		{"the third pins where the harness can", GateInput{Blocked: 2, CanPin: true}, GatePin},
		{"the fourth gives up", GateInput{Blocked: 3}, GateGiveUp},
		{"the fourth gives up even where the harness can pin", GateInput{Blocked: 3, CanPin: true}, GateGiveUp},
	}
	for _, c := range cases {
		if got := DecideGate(c.in).Outcome; got != c.want {
			t.Errorf("%s: outcome %v, want %v", c.name, got, c.want)
		}
	}
}

// The reminder names the attempt and follows omp's wording: keep working is
// the default, submit only when done, blocked only for a real blocker.
func TestGateReminderNumberingAndWording(t *testing.T) {
	for n, want := range map[int]string{0: "reminder 1 of 3", 1: "reminder 2 of 3", 2: "reminder 3 of 3"} {
		if got := DecideGate(GateInput{Blocked: n}).Reminder; !strings.Contains(got, want) {
			t.Errorf("block %d: reminder must say %q, got %q", n, want, got)
		}
	}
	blocked := DecideGate(GateInput{}).Reminder
	for _, want := range []string{"submit", "tool call", "not an order to stop", "genuinely complete", "nameable blocker", "Never invent a blocker", "text only"} {
		if !strings.Contains(blocked, want) {
			t.Errorf("reminder missing %q:\n%s", want, blocked)
		}
	}
	// claude cannot pin a tool, so its 3rd block is its last and says so.
	if last := DecideGate(GateInput{Blocked: 2}).Reminder; !strings.Contains(last, "last") {
		t.Errorf("the 3rd claude block must say it is the last: %q", last)
	}
	// A harness that can pin does not claim the turn will be allowed to end: it
	// forces submit instead.
	pinned := DecideGate(GateInput{Blocked: 2, CanPin: true}).Reminder
	if !strings.Contains(pinned, "pinned") || strings.Contains(pinned, "allowed to end") {
		t.Errorf("the pinned reminder must reflect the pin, got %q", pinned)
	}
}
