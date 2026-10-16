package memory

import (
	"context"
	"testing"
)

func TestDedupAndStoreMemoryTextAndSupersession(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			m := fac.open(t, newTestClock())
			ctx := context.Background()
			put := func(session, event, author, text string) MemoryStoreResult {
				result, err := DedupAndStoreMemory(ctx, m, MemoryItem{SessionID: session, EventID: event, Author: author, Kind: KindInstruction, Text: text, Quote: text, Importance: .7, Veracity: VeracityStated})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}

			first := put("s1", "e1", "alice", "always return wrapped errors")
			if first.ID == 0 || first.Duplicate {
				t.Fatalf("first result = %+v", first)
			}
			dup := put("s2", "e2", "alice", "Always, return wrapped errors!")
			if !dup.Duplicate || dup.ID != first.ID {
				t.Fatalf("duplicate = %+v, want id %d", dup, first.ID)
			}
			stored, err := m.GetMemory(ctx, first.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.MentionCount != 2 || stored.Importance != .75 {
				t.Fatalf("bumped memory = %+v", stored)
			}
			rows, err := m.ListMemories(ctx, "", "", 20)
			if err != nil || len(rows) != 1 {
				t.Fatalf("duplicate inserted row: %d, %v", len(rows), err)
			}

			newer := put("s4", "e4", "alice", "never return wrapped errors")
			if newer.Relation != RelContradicts || !newer.Superseded {
				t.Fatalf("same-author contradiction = %+v", newer)
			}
			stored, _ = m.GetMemory(ctx, first.ID)
			if stored.Status != StatusSuperseded || stored.SupersededBy != newer.ID {
				t.Fatalf("old memory = %+v", stored)
			}
			latest, _ := m.GetMemory(ctx, newer.ID)
			if latest.Status != StatusActive || latest.SupersededBy != 0 {
				t.Fatalf("new memory = %+v", latest)
			}
			crossAuthor := put("s5", "e5", "bob", "always return wrapped errors")
			if crossAuthor.Relation != RelContradicts || crossAuthor.Superseded {
				t.Fatalf("cross-author contradiction = %+v", crossAuthor)
			}
			latest, _ = m.GetMemory(ctx, newer.ID)
			if latest.Status != StatusActive {
				t.Fatalf("cross-author contradiction superseded row: %+v", latest)
			}
		})
	}
}

func TestMemoryRelationVectorThresholdsAndNullFallback(t *testing.T) {
	base := MemoryItem{Text: "always return wrapped errors", Embedding: EncodeVector([]float32{1, 0}), EmbeddingModel: "m"}
	other := MemoryItem{Text: "always return wrapped errors consistently", Embedding: EncodeVector([]float32{1, 0}), EmbeddingModel: "m"}
	if got := memoryRelation(other, base); got != RelDuplicate {
		t.Fatalf("cosine duplicate = %q", got)
	}
	other.Text = "never return wrapped errors"
	if got := memoryRelation(other, base); got != RelContradicts {
		t.Fatalf("cosine contradiction = %q", got)
	}
	other.Embedding = nil
	if got := memoryRelation(other, base); got != RelContradicts {
		t.Fatalf("null vector text fallback = %q", got)
	}
	other = MemoryItem{Text: "this preference is entirely different", Embedding: EncodeVector([]float32{0, 1}), EmbeddingModel: "m"}
	if got := memoryRelation(other, base); got != "" {
		t.Fatalf("null-free unrelated text = %q", got)
	}
}

func TestDedupAndStoreUsesSuppliedCandidateVector(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			m := fac.open(t, newTestClock())
			ctx := context.Background()
			first, err := m.AddMemory(ctx, MemoryItem{SessionID: "s1", EventID: "e1", Author: "alice", Kind: KindInstruction, Text: "always return wrapped errors", Quote: "always return wrapped errors"})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.SetMemoryEmbedding(ctx, first, EncodeVector([]float32{1, 0}), "dedup-model"); err != nil {
				t.Fatal(err)
			}
			result, err := m.DedupAndStoreMemory(ctx, MemoryItem{SessionID: "s2", EventID: "e2", Author: "alice", Kind: KindInstruction, Text: "always return wrapped errors carefully", Quote: "always return wrapped errors carefully", Importance: .7, Embedding: EncodeVector([]float32{.98, .2}), EmbeddingModel: "dedup-model"})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Duplicate || result.ID != first {
				t.Fatalf("vector duplicate = %+v, want original id %d", result, first)
			}
			rows, err := m.ListMemories(ctx, "", "", 10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("vector duplicate inserted row: %d, %v", len(rows), err)
			}
		})
	}
}

func TestNormalizedTextDuplicateWinsOverVectorDifference(t *testing.T) {
	a := MemoryItem{Text: "Always return wrapped errors", Embedding: EncodeVector([]float32{1, 0}), EmbeddingModel: "m"}
	b := MemoryItem{Text: "always return wrapped errors!", Embedding: EncodeVector([]float32{0, 1}), EmbeddingModel: "m"}
	if got := memoryRelation(a, b); got != RelDuplicate {
		t.Fatalf("normalized duplicate = %q", got)
	}
}

func TestBankWideDuplicatePrecedesSameAuthorSupersession(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			m := fac.open(t, newTestClock())
			ctx := context.Background()
			put := func(session, event, author, text string) MemoryStoreResult {
				result, err := m.DedupAndStoreMemory(ctx, MemoryItem{SessionID: session, EventID: event, Author: author, Kind: KindInstruction, Text: text, Quote: text, Importance: .7, Veracity: VeracityStated})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			alice := put("s1", "e1", "alice", "always return wrapped errors")
			bob := put("s2", "e2", "bob", "never return wrapped errors")
			incoming := put("s3", "e3", "alice", "never return wrapped errors")
			if !incoming.Duplicate || incoming.ID != bob.ID || incoming.Superseded {
				t.Fatalf("bank-wide duplicate = %+v, want duplicate id %d", incoming, bob.ID)
			}
			aliceItem, err := m.GetMemory(ctx, alice.ID)
			if err != nil || aliceItem.Status != StatusActive {
				t.Fatalf("contradiction target changed: %+v, %v", aliceItem, err)
			}
			bobItem, err := m.GetMemory(ctx, bob.ID)
			if err != nil || bobItem.MentionCount != 2 {
				t.Fatalf("duplicate not bumped: %+v, %v", bobItem, err)
			}
			rows, err := m.ListMemories(ctx, "", "", 10)
			if err != nil || len(rows) != 2 {
				t.Fatalf("duplicate inserted row: %d, %v", len(rows), err)
			}
		})
	}
}
