package web

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

func seedObs(t *testing.T, d *Daemon, owner, session, event, quote, key string) int64 {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	id, err := bank.AddObservation(context.Background(), memory.Observation{
		SessionID: session, EventID: event, Author: owner, Quote: quote,
		Kind: memory.KindPreference, SubjectKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func consolidateDaemon(t *testing.T, reply func(user string) string, seen *[]string) (*Daemon, *fakeClock) {
	t.Helper()
	d, clock := nightlyTestDaemon(t, nil, nil)
	d.consolidateOneShot = func(model, system, user string) (string, error) {
		if seen != nil {
			*seen = append(*seen, user)
		}
		return reply(user), nil
	}
	return d, clock
}

func proposalJSON(sources []int64, mutate func(m map[string]any)) string {
	m := map[string]any{
		"action": "generalize", "text": "Prefer tabs over spaces in Go files.", "kind": "preference",
		"subject_key": "go.formatting.indent", "source_ids": sources, "related_rule_id": "",
		"rationale": "Two statements ask for tabs.", "confidence": 0.9,
	}
	if mutate != nil {
		mutate(m)
	}
	raw, _ := json.Marshal(map[string]any{"proposals": []any{m}})
	return string(raw)
}

func queueRuleItems(t *testing.T, d *Daemon, owner string) []memory.QueueItem {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	items, err := bank.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestConsolidateProposesGeneralizationAndSkipsCoveredSources(t *testing.T) {
	var a, b int64
	calls := 0
	d, _ := consolidateDaemon(t, func(string) string {
		calls++
		return proposalJSON([]int64{a, b}, nil)
	}, nil)
	a = seedObs(t, d, "alice", "s1", "1", "Use tabs, not spaces", "")
	b = seedObs(t, d, "alice", "s2", "1", "Indent with tabs from now on", "")
	res := d.nightlyConsolidate(context.Background(), "run1", []string{"alice"})
	if res.Status != memory.LedgerSuccess || res.Counts["proposed"] != 1 {
		t.Fatalf("stage = %+v, want one proposal", res)
	}
	items := queueRuleItems(t, d, "alice")
	if len(items) != 1 || items[0].Status != memory.QueuePending {
		t.Fatalf("queue = %+v, want one pending item", items)
	}
	p, err := memory.DecodeRememberProposal(items[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if p.Origin != "consolidated" || p.Author != "alice" || len(p.Quotes) != 2 || len(p.Sessions) != 2 ||
		len(p.SourceObservationIDs) != 2 || p.Quotes[0] != "Use tabs, not spaces" {
		t.Fatalf("proposal = %+v, want quotes and sessions from the source observations", p)
	}
	// No Rule is created: the human gate stays.
	bank, _ := d.profileBankFor("alice")
	if rules, _ := bank.ListRules(context.Background(), 10); len(rules) != 0 {
		t.Fatalf("rules = %+v, want none before approval", rules)
	}
	// The next run finds both sources in the queue and asks nothing.
	before := calls
	res = d.nightlyConsolidate(context.Background(), "run2", []string{"alice"})
	if calls != before || res.Status != memory.LedgerNoData {
		t.Fatalf("second run: calls %d->%d, stage %+v, want no call and NO_DATA", before, calls, res)
	}
}

func TestConsolidateRejectsUngroundedOrWeakProposals(t *testing.T) {
	cases := map[string]func(sources []int64) string{
		"unknown source": func(s []int64) string { return proposalJSON([]int64{s[0], 9999}, nil) },
		"single source":  func(s []int64) string { return proposalJSON(s[:1], nil) },
		"low confidence": func(s []int64) string { return proposalJSON(s, func(m map[string]any) { m["confidence"] = 0.4 }) },
		"no rationale":   func(s []int64) string { return proposalJSON(s, func(m map[string]any) { m["rationale"] = " " }) },
		"over 300 chars": func(s []int64) string {
			return proposalJSON(s, func(m map[string]any) { m["text"] = strings.Repeat("x", 301) })
		},
		"bad kind":       func(s []int64) string { return proposalJSON(s, func(m map[string]any) { m["kind"] = "vibes" }) },
		"unknown action": func(s []int64) string { return proposalJSON(s, func(m map[string]any) { m["action"] = "delete" }) },
		"refine no rule": func(s []int64) string { return proposalJSON(s, func(m map[string]any) { m["action"] = "refine" }) },
		"secret in text": func(s []int64) string {
			return proposalJSON(s, func(m map[string]any) { m["text"] = "Use key sk-abcdefghijklmnopqrstuvwxyz0123456789ABCD for deploys." })
		},
		"repeated source": func(s []int64) string { return proposalJSON([]int64{s[0], s[0]}, nil) },
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			var ids []int64
			d, _ := consolidateDaemon(t, func(string) string { return mk(ids) }, nil)
			ids = []int64{
				seedObs(t, d, "alice", "s1", "1", "Use tabs, not spaces", ""),
				seedObs(t, d, "alice", "s2", "1", "Indent with tabs from now on", ""),
			}
			res := d.nightlyConsolidate(context.Background(), "run", []string{"alice"})
			if res.Counts["proposed"] != 0 || res.Counts["rejected"] != 1 {
				t.Fatalf("stage = %+v, want the proposal rejected", res)
			}
			if items := queueRuleItems(t, d, "alice"); len(items) != 0 {
				t.Fatalf("queue = %+v, want empty after a rejected proposal", items)
			}
		})
	}
}

func TestConsolidateNoneAndBadReply(t *testing.T) {
	d, _ := consolidateDaemon(t, func(string) string { return "none" }, nil)
	seedObs(t, d, "alice", "s1", "1", "Use tabs, not spaces", "")
	res := d.nightlyConsolidate(context.Background(), "run", []string{"alice"})
	if res.Status != memory.LedgerSuccess || res.Counts["proposed"] != 0 {
		t.Fatalf("none reply: %+v, want SUCCESS with zero proposals", res)
	}
	d2, _ := consolidateDaemon(t, func(string) string { return "I think tabs." }, nil)
	seedObs(t, d2, "alice", "s1", "1", "Use tabs, not spaces", "")
	res = d2.nightlyConsolidate(context.Background(), "run", []string{"alice"})
	if res.Status != memory.LedgerFailed {
		t.Fatalf("prose reply: %+v, want FAILED", res)
	}
}

func TestConsolidateRefineNamesStoredRuleAndSendsRedactedInput(t *testing.T) {
	var seen []string
	var obsID int64
	var ruleID string
	d, _ := consolidateDaemon(t, func(string) string {
		return proposalJSON([]int64{obsID}, func(m map[string]any) {
			m["action"] = "refine"
			m["related_rule_id"] = ruleID
			m["text"] = "Prefer tabs in Go, except generated files."
		})
	}, &seen)
	bank, _ := d.profileBankFor("alice")
	rule, err := bank.AddRule(context.Background(), memory.Rule{Text: "Prefer tabs over spaces in Go files.",
		Kind: memory.KindPreference, SubjectKey: "go.formatting.indent", Status: memory.StatusActive, ApprovedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	ruleID = rule.RuleID
	obsID = seedObs(t, d, "alice", "s1", "1", "Tabs in Go but not generated files; key sk-abcdefghijklmnopqrstuvwxyz0123456789ABCD", "")
	res := d.nightlyConsolidate(context.Background(), "run", []string{"alice"})
	if res.Counts["proposed"] != 1 {
		t.Fatalf("stage = %+v, want the refine proposed", res)
	}
	if len(seen) != 1 || strings.Contains(seen[0], "sk-abcdefghijklmnop") || !strings.Contains(seen[0], ruleID) {
		t.Fatalf("model input = %v, want redacted quotes and the stored rule", seen)
	}
	p, _ := memory.DecodeRememberProposal(queueRuleItems(t, d, "alice")[0].Payload)
	if p.Relation != memory.RelRefines || p.RelatedRuleID != ruleID {
		t.Fatalf("proposal = %+v, want refines %s", p, ruleID)
	}
}

func TestConsolidateIgnoresOldObservationsAndNeverDeletes(t *testing.T) {
	calls := 0
	d, clock := consolidateDaemon(t, func(string) string { calls++; return "none" }, nil)
	seedObs(t, d, "alice", "s1", "1", "Use tabs, not spaces", "")
	clock.advance(consolidateWindow + time.Hour)
	res := d.nightlyConsolidate(context.Background(), "run", []string{"alice"})
	if calls != 0 || res.Status != memory.LedgerNoData {
		t.Fatalf("old observations: calls=%d stage=%+v, want no call and NO_DATA", calls, res)
	}
	bank, _ := d.profileBankFor("alice")
	if obs, _ := bank.ListObservations(context.Background(), 10); len(obs) != 1 {
		t.Fatalf("observations = %d, want the original kept", len(obs))
	}
}

func TestConsolidateSkipsWithoutModel(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"decider_mode":"heuristic"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedObs(t, d, "alice", "s1", "1", "Use tabs, not spaces", "")
	res := d.nightlyConsolidate(context.Background(), "run", []string{"alice"})
	if res.Status != memory.LedgerNoData || !strings.Contains(res.Detail, "no memory model") {
		t.Fatalf("stage = %+v, want NO_DATA for the missing model", res)
	}
}
