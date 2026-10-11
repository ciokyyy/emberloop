package memory

import (
	"context"
	"testing"
)

// TestAnnotateQueueRelation runs the pairing write-back against each
// implementation: annotating a pending rule proposal stores the relation and
// the related Rule on its payload, decided items and other kinds refuse, and
// bad relations and missing items fail without touching the row.
func TestAnnotateQueueRelation(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			rule, err := m.AddRule(ctx, Rule{Text: "always use tabs", Kind: KindPreference, Status: StatusActive, SubjectKey: "use.tabs", ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			id, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: EncodeRuleProposal(RuleProposal{
				Text: "always use tabs here", Kind: KindPreference, SubjectKey: "use.tabs",
				Quotes: []string{"always use tabs here"}, Sessions: []string{"s1"}, Author: "u1", Origin: "mined",
			})})
			if err != nil {
				t.Fatal(err)
			}
			related, rel := PairCandidate("always use tabs here", "use.tabs", []Rule{rule})
			if related == "" || rel == "" {
				t.Fatalf("PairCandidate = %q %q, want a relation to the stored rule", related, rel)
			}
			if err := m.AnnotateQueueRelation(ctx, id, rel, related); err != nil {
				t.Fatal(err)
			}
			items, err := m.Queue(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var found *QueueItem
			for i := range items {
				if items[i].ID == id {
					found = &items[i]
				}
			}
			if found == nil {
				t.Fatalf("queue item %d missing after annotate", id)
			}
			got, err := DecodeRememberProposal(found.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if got.Relation != rel || got.RelatedRuleID != related {
				t.Fatalf("payload relation = %q %q, want %q %q", got.Relation, got.RelatedRuleID, rel, related)
			}
			if got.Text != "always use tabs here" || len(got.Quotes) != 1 {
				t.Fatalf("annotate rewrote more than the relation: %+v", got)
			}
			// Annotating with the same relation is a no-op success.
			if err := m.AnnotateQueueRelation(ctx, id, rel, related); err != nil {
				t.Fatalf("repeat annotate = %v, want nil", err)
			}
			// Bad relations, other kinds and decided items refuse.
			if err := m.AnnotateQueueRelation(ctx, id, "merges", related); err == nil {
				t.Fatal("bad relation accepted")
			}
			if err := m.AnnotateQueueRelation(ctx, id, rel, ""); err == nil {
				t.Fatal("relation without a related rule accepted")
			}
			if err := m.AnnotateQueueRelation(ctx, id+9999, rel, related); err == nil {
				t.Fatal("missing item annotated")
			}
			other, err := m.Enqueue(ctx, QueueItem{Kind: QueueProfilePatch, Payload: `{"ops":[]}`})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.AnnotateQueueRelation(ctx, other, rel, related); err == nil {
				t.Fatal("non-rule item annotated")
			}
			if err := m.Decide(ctx, id, QueueRejected, "u1"); err != nil {
				t.Fatal(err)
			}
			if err := m.AnnotateQueueRelation(ctx, id, rel, related); err == nil {
				t.Fatal("decided item annotated: labels stay immutable")
			}
		})
	}
}
