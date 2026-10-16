package memory

import (
	"context"
	"testing"
)

func TestRecallMemoryItemsRequiresOptInForSuperseded(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			m := fac.open(t, newTestClock())
			ctx := context.Background()
			old, err := m.DedupAndStoreMemory(ctx, MemoryItem{SessionID: "s1", EventID: "e1", Author: "alice", Kind: KindInstruction, Text: "always return wrapped errors", Quote: "always return wrapped errors"})
			if err != nil {
				t.Fatal(err)
			}
			newer, err := m.DedupAndStoreMemory(ctx, MemoryItem{SessionID: "s2", EventID: "e2", Author: "alice", Kind: KindInstruction, Text: "never return wrapped errors", Quote: "never return wrapped errors"})
			if err != nil {
				t.Fatal(err)
			}
			if !newer.Superseded {
				t.Fatalf("new memory did not supersede old: %+v", newer)
			}
			regular, err := m.RecallMemoryItems(ctx, "wrapped errors", 10, false)
			if err != nil || len(regular) != 1 || regular[0].ID != newer.ID {
				t.Fatalf("default recall = %+v, %v", regular, err)
			}
			withOld, err := m.RecallMemoryItems(ctx, "wrapped errors", 10, true)
			if err != nil || len(withOld) != 2 {
				t.Fatalf("include-superseded recall = %+v, %v", withOld, err)
			}
			foundOld := false
			for _, item := range withOld {
				if item.ID == old.ID && item.Status == StatusSuperseded {
					foundOld = true
				}
			}
			if !foundOld {
				t.Fatalf("superseded Memory missing from opt-in results: %+v", withOld)
			}
		})
	}
}
