package memory

import (
	"context"
	"testing"
)

// TestLiveRulesContract runs the LiveRules promise against each
// implementation: the latest live version per rule ID, ordered by rule ID,
// with superseded and dormant versions hidden and parked kept.
func TestLiveRulesContract(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			active, err := m.AddRule(ctx, Rule{Text: "live active rule", Kind: KindPreference, Status: StatusActive, ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			parked, err := m.AddRule(ctx, Rule{Text: "live parked rule", Kind: KindFact, Status: StatusParked, ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			dormant, err := m.AddRule(ctx, Rule{Text: "dormant rule", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.SetRuleStatus(ctx, dormant.RuleID, StatusDormant); err != nil {
				t.Fatal(err)
			}
			old, err := m.AddRule(ctx, Rule{Text: "old version", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			next := old
			next.Text = "new version"
			if _, err := m.ReviseRule(ctx, next); err != nil {
				t.Fatal(err)
			}
			live, err := m.LiveRules(ctx)
			if err != nil {
				t.Fatal(err)
			}
			byID := make(map[string]Rule)
			for _, r := range live {
				if _, dup := byID[r.RuleID]; dup {
					t.Fatalf("rule %s listed twice in %+v", r.RuleID, live)
				}
				byID[r.RuleID] = r
			}
			if _, ok := byID[active.RuleID]; !ok {
				t.Fatalf("active rule missing in %+v", live)
			}
			if _, ok := byID[parked.RuleID]; !ok {
				t.Fatalf("parked rule missing in %+v: parked stays live for manual recall", live)
			}
			if _, ok := byID[dormant.RuleID]; ok {
				t.Fatalf("dormant rule present in %+v", live)
			}
			got, ok := byID[old.RuleID]
			if !ok || got.Text != "new version" || got.Version != 2 {
				t.Fatalf("revised rule = %+v, want version 2 with the new text", got)
			}
			for i := 1; i < len(live); i++ {
				if live[i-1].RuleID >= live[i].RuleID {
					t.Fatalf("live rules not ordered by rule ID: %+v", live)
				}
			}
		})
	}
}
