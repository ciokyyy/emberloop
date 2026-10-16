package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryRecallFiltersAndKindDecay(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	root := tempDir(t)
	b, err := openBank(filepath.Join(root, "u.db"), filepath.Join(root, "snapshots"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	add := func(kind, status, text string, created time.Time) int64 {
		id, e := b.AddMemory(context.Background(), MemoryItem{SessionID: "s", EventID: text, Author: "u", Kind: kind, Text: text, Quote: text, Importance: .8, Veracity: VeracityStated, Status: status, CreatedAt: created})
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	add(KindFact, StatusActive, "A durable fact about blue lanterns", now.AddDate(-1, 0, 0))
	add(KindFact, StatusActive, "A durable fact about blue lanterns", now)
	add(KindPreference, StatusActive, "A preference for blue lanterns", now.AddDate(-5, 0, 0))
	add(KindFact, StatusParked, "A parked fact about blue lanterns", now)
	hits, err := b.RecallMemories(context.Background(), "blue lanterns", RecallWeights{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits=%d want active rows only", len(hits))
	}
	if hits[0].Memory.Status != StatusActive {
		t.Fatal("inactive memory recalled")
	}
	var old, fresh, preference float64
	for _, h := range hits {
		switch {
		case h.Memory.Kind == KindPreference:
			preference = h.Score
		case h.Memory.CreatedAt.Equal(now):
			fresh = h.Score
		default:
			old = h.Score
		}
	}
	if !(fresh > old) {
		t.Fatalf("old fact score %f >= fresh %f", old, fresh)
	}
	if preference == 0 {
		t.Fatal("old preference decayed to zero")
	}
}

func TestRecallWeightsRenormalizeWithoutVector(t *testing.T) {
	w := normalizedWeights(RecallWeights{}, false)
	if w.Vector != 0 || w.FTS+w.Importance < .999 || w.FTS+w.Importance > 1.001 {
		t.Fatalf("weights=%+v", w)
	}
}

func TestMemoryRecallFindsSemanticParaphraseWithoutKeyword(t *testing.T) {
	root := tempDir(t)
	embedder := &FakeEmbedder{Vector: []float32{1, 0}}
	b, err := openBank(filepath.Join(root, "u.db"), filepath.Join(root, "snapshots"), Options{Embedder: embedder, EmbeddingModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	id, err := b.AddMemory(context.Background(), MemoryItem{SessionID: "s", EventID: "e", Author: "u", Kind: KindFact, Text: "The user rides a bicycle", Quote: "I ride a bicycle", Importance: .8, Veracity: VeracityStated})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetMemoryEmbedding(context.Background(), id, EncodeVector([]float32{1, 0}), "test-model"); err != nil {
		t.Fatal(err)
	}
	hits, err := b.RecallMemories(context.Background(), "two-wheeled commute", RecallWeights{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != id {
		t.Fatalf("semantic hits=%+v", hits)
	}
}
