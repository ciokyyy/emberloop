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

			crossAuthor := put("s3", "e3", "bob", "never return wrapped errors")
			if crossAuthor.Relation != RelContradicts || crossAuthor.Superseded {
				t.Fatalf("cross-author contradiction = %+v", crossAuthor)
			}
			stored, _ = m.GetMemory(ctx, first.ID)
			if stored.Status != StatusActive {
				t.Fatalf("cross-author contradiction superseded row: %+v", stored)
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

func TestBankDedupEmbedsBeforeInsertForVectorComparison(t *testing.T) {
	ctx := context.Background()
	embedder := &FakeEmbedder{Vector: []float32{1, 0}}
	eng, err := Open(tempDir(t), Options{Embedder: embedder, EmbeddingModel: "dedup-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	bank, err := eng.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	put := func(event, text string) MemoryStoreResult {
		result, err := bank.DedupAndStoreMemory(ctx, MemoryItem{SessionID: "session", EventID: event, Author: "alice", Kind: KindInstruction, Text: text, Quote: text, Importance: .7})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := put("one", "always return wrapped errors")
	second := put("two", "always return wrapped errors carefully")
	if second.Relation != RelDuplicate || !second.Duplicate || second.ID != first.ID {
		t.Fatalf("vector duplicate = %+v, want original id %d", second, first.ID)
	}
	rows, err := bank.ListMemories(ctx, "", "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("vector duplicate inserted row: %d, %v", len(rows), err)
	}
	stored, err := bank.GetMemory(ctx, first.ID)
	if err != nil || len(stored.Embedding) == 0 || stored.EmbeddingModel != "dedup-model" {
		t.Fatalf("new row missing precomputed vector: %+v, %v", stored, err)
	}
	embedder.mu.Lock()
	defer embedder.mu.Unlock()
	if len(embedder.Inputs) != 2 {
		t.Fatalf("embed requests = %d, want one per candidate", len(embedder.Inputs))
	}
}

func TestNormalizedTextDuplicateWinsOverVectorDifference(t *testing.T) {
	a := MemoryItem{Text: "Always return wrapped errors", Embedding: EncodeVector([]float32{1, 0}), EmbeddingModel: "m"}
	b := MemoryItem{Text: "always return wrapped errors!", Embedding: EncodeVector([]float32{0, 1}), EmbeddingModel: "m"}
	if got := memoryRelation(a, b); got != RelDuplicate {
		t.Fatalf("normalized duplicate = %q", got)
	}
}
