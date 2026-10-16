package memory

import (
	"context"
	"testing"
	"time"
)

type costTestEmbedder struct{ *FakeEmbedder }

func (e *costTestEmbedder) CostUSD() (float64, bool) { return .0042, true }

func TestSuccessfulEmbeddingCostIsWrittenToLedger(t *testing.T) {
	eng, err := Open(tempDir(t), Options{Embedder: &costTestEmbedder{&FakeEmbedder{Vector: []float32{1, 0}}}, EmbeddingModel: "m"})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	bank, err := eng.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bank.AddMemory(context.Background(), MemoryItem{SessionID: "s", EventID: "e", Author: "alice", Kind: KindFact, Text: "fact", Quote: "fact"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rows, err := bank.Ledger(context.Background())
		if err == nil {
			for _, row := range rows {
				if row.Stage == "embedding" && row.Status == LedgerSuccess && row.Detail == "cost_usd=0.00420000" {
					return
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("successful embedding cost was not recorded")
}
