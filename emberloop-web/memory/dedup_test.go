package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeKeepsNegation(t *testing.T) {
	a := NormalizeQuote("Always return wrapped errors")
	b := NormalizeQuote("Never return wrapped errors")
	if !strings.Contains(a, "always") || !strings.Contains(b, "never") {
		t.Fatalf("negation dropped: %q vs %q", a, b)
	}
	if a == b {
		t.Fatalf("opposites normalized equal: %q", a)
	}
	if got := NormalizeQuote("Please always wrap errors in this repo"); !strings.Contains(got, "always") {
		t.Fatalf("polarity adverb dropped: %q", got)
	}
	if got := NormalizeQuote("the quick brown fox"); strings.Contains(got, "the") {
		t.Fatalf("filler kept: %q", got)
	}
}

func TestRelationCases(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want string
	}{
		{"duplicate case-insensitive", "Always wrap errors", "always wrap errors!", RelDuplicate},
		{"duplicate punctuation", "No, use tabs here.", "no use tabs here", RelDuplicate},
		{"contradicts always-never", "always return wrapped errors", "never return wrapped errors", RelContradicts},
		{"contradicts negation", "don't add comments to this code", "add comments to this code", RelContradicts},
		{"refines contains", "always wrap errors", "always wrap errors in this repo", RelRefines},
		{"unrelated", "always wrap errors in Go", "prefer tabs for indentation", ""},
		{"empty", "", "always wrap errors", ""},
	}
	for _, c := range cases {
		if got := Relation(c.a, c.b); got != c.want {
			t.Errorf("%s: Relation(%q, %q) = %q, want %q", c.name, c.a, c.b, got, c.want)
		}
		if got := Relation(c.b, c.a); got != c.want {
			t.Errorf("%s: Relation is not symmetric: %q", c.name, got)
		}
	}
}

func TestDedupAndStore(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			mk := func(session, event, quote string) Observation {
				return Observation{SessionID: session, EventID: event, Author: "u1", Quote: quote, Kind: KindPreference}
			}
			first, rel, err := DedupAndStore(ctx, m, mk("s1", "e1", "always wrap errors in this repo"))
			if err != nil || rel != "" || first == 0 {
				t.Fatalf("first store = %d %q %v, want a novel insert", first, rel, err)
			}
			// Same Event replayed: same ID, no bump.
			again, rel, err := DedupAndStore(ctx, m, mk("s1", "e1", "always wrap errors in this repo"))
			if err != nil || again != first || rel != RelDuplicate {
				t.Fatalf("replay = %d %q %v, want %d duplicate", again, rel, err, first)
			}
			obs, err := m.SearchObservations(ctx, "wrap errors", 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(obs) != 1 || obs[0].EvidenceCount != 1 {
				t.Fatalf("replay bumped evidence: %+v", obs)
			}
			// Same text, new Event: duplicate bumps evidence_count.
			dup, rel, err := DedupAndStore(ctx, m, mk("s2", "e2", "always wrap errors in this repo"))
			if err != nil || rel != RelDuplicate {
				t.Fatalf("duplicate = %d %q %v, want duplicate", dup, rel, err)
			}
			obs, _ = m.SearchObservations(ctx, "wrap errors", 10)
			if len(obs) != 1 || obs[0].EvidenceCount != 2 {
				t.Fatalf("duplicate did not bump evidence: %+v", obs)
			}
			// Contradiction: stored, not merged.
			contra, rel, err := DedupAndStore(ctx, m, mk("s3", "e3", "never wrap errors in this repo"))
			if err != nil || rel != RelContradicts || contra == 0 {
				t.Fatalf("contradiction = %d %q %v, want a contradicts insert", contra, rel, err)
			}
			obs, _ = m.SearchObservations(ctx, "wrap errors", 10)
			if len(obs) != 2 {
				t.Fatalf("contradiction merged: %d observations, want 2", len(obs))
			}
		})
	}
}

func TestBumpObservationEvidence(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			id, err := m.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "bump me", Kind: KindFact})
			if err != nil {
				t.Fatal(err)
			}
			updated, err := m.BumpObservationEvidence(ctx, id)
			if err != nil || updated.EvidenceCount != 2 || updated.ID != id {
				t.Fatalf("bump = %+v %v, want evidence 2", updated, err)
			}
			if _, err := m.BumpObservationEvidence(ctx, 999999); !errors.Is(err, ErrNotFound) {
				t.Fatalf("bump missing err = %v, want ErrNotFound", err)
			}
		})
	}
}
