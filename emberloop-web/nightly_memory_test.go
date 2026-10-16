package web

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

func TestNightlyFoldRetainsAndMarksEpisodes(t *testing.T) {
	t.Parallel()
	d, clock := nightlyTestDaemon(t, nil, nil)
	d.foldOneShot = func(_, _, _ string) (string, error) {
		return `{"items":[{"kind":"preference","text":"Prefers tabs for indentation.","quote":"I prefer tabs","episode_ids":[1]}]}`, nil
	}
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	id, err := bank.AddEpisode(context.Background(), memory.Episode{SessionID: "s1", EventRange: "event-1", Author: "alice", Text: "I prefer tabs for indentation"})
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(13 * time.Hour)
	res := d.nightlyFold(context.Background(), "fold-run", []string{"alice"})
	if res.Status != memory.LedgerSuccess {
		t.Fatalf("fold status=%s detail=%s", res.Status, res.Detail)
	}
	ep, err := bank.EpisodeByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if ep.ConsolidatedAt.IsZero() {
		t.Fatal("episode was not marked consolidated")
	}
	ms, err := bank.ListMemories(context.Background(), "", "", memory.MaxList)
	if err != nil || len(ms) != 1 {
		t.Fatalf("memories=%+v err=%v", ms, err)
	}
	if ms[0].Importance != .6 || ms[0].Veracity != memory.VeracityInferred {
		t.Fatalf("folded memory=%+v", ms[0])
	}
	d.nightlyFold(context.Background(), "fold-again", []string{"alice"})
	ms, _ = bank.ListMemories(context.Background(), "", "", memory.MaxList)
	if len(ms) != 1 {
		t.Fatalf("re-fold created %d memories", len(ms))
	}
}

func TestNightlyPromoteCreatesPendingQuotedRule(t *testing.T) {
	t.Parallel()
	d, _ := nightlyTestDaemon(t, nil, nil)
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	for i, session := range []string{"s1", "s2"} {
		id, err := bank.AddMemory(context.Background(), memory.MemoryItem{SessionID: session, EventID: "e", Author: "alice", Kind: memory.KindPreference, Text: "Use tabs for indentation.", Quote: "I prefer tabs", Importance: .8, Veracity: memory.VeracityStated, Embedding: memory.EncodeVector([]float32{1, 0}), EmbeddingModel: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			_, err = bank.BumpMemory(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	// Both mentions must meet the per-item recurrence threshold.
	items, _ := bank.ListMemories(context.Background(), "", memory.StatusActive, memory.MaxList)
	for _, item := range items {
		if item.MentionCount < 2 {
			_, err = bank.BumpMemory(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	res := d.nightlyPromote(context.Background(), "promote-run", []string{"alice"})
	if res.Counts["proposed"] != 1 {
		t.Fatalf("promotion=%+v", res)
	}
	queue, err := bank.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Kind != memory.QueueRule || queue[0].Status != memory.QueuePending {
		t.Fatalf("queue=%+v", queue)
	}
	proposal, err := memory.DecodeRememberProposal(queue[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Quotes) != 2 || proposal.Quotes[0] != "I prefer tabs" || proposal.Origin != "memory_candidate" {
		t.Fatalf("proposal=%+v", proposal)
	}
	if raw, err := json.Marshal(queue); err != nil || len(raw) == 0 {
		t.Fatalf("queue serialization err=%v", err)
	}
	d.nightlyPromote(context.Background(), "promote-again", []string{"alice"})
	queue, _ = bank.Queue(context.Background())
	if len(queue) != 1 {
		t.Fatalf("repeated stage created duplicate pending proposal: %d", len(queue))
	}
}

func TestNightlyMemoryConsolidationQueuesGeneralize(t *testing.T) {
	t.Parallel()
	d, _ := nightlyTestDaemon(t, nil, nil)
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for _, session := range []string{"s1", "s2"} {
		id, e := bank.AddMemory(context.Background(), memory.MemoryItem{SessionID: session, EventID: "e", Author: "alice", Kind: memory.KindPreference, Text: "Use tabs.", Quote: "I prefer tabs", Importance: .8, Veracity: memory.VeracityStated})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
	}
	d.consolidateOneShot = func(_, _, _ string) (string, error) {
		return fmt.Sprintf(`{"proposals":[{"action":"generalize","text":"Prefers tabs.","kind":"preference","source_ids":[%d,%d]}]}`, ids[0], ids[1]), nil
	}
	res := d.nightlyConsolidateMemories(context.Background(), "consolidate-run", []string{"alice"})
	if res.Counts["proposed"] != 1 {
		t.Fatalf("stage=%+v", res)
	}
	queue, err := bank.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Status != memory.QueuePending {
		t.Fatalf("generalization queue=%+v", queue)
	}
	proposal, err := memory.DecodeRememberProposal(queue[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Quotes) != 2 || proposal.Origin != "consolidated_memory" {
		t.Fatalf("proposal=%+v", proposal)
	}
}

func TestNightlyMemoryConsolidationMergeSoftSupersedes(t *testing.T) {
	t.Parallel()
	d, _ := nightlyTestDaemon(t, nil, nil)
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for _, session := range []string{"s1", "s2"} {
		id, e := bank.AddMemory(context.Background(), memory.MemoryItem{SessionID: session, EventID: "e", Author: "alice", Kind: memory.KindPreference, Text: "Use tabs.", Quote: "I prefer tabs", Importance: .8, Veracity: memory.VeracityStated})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
	}
	d.consolidateOneShot = func(_, _, _ string) (string, error) {
		return fmt.Sprintf(`{"proposals":[{"action":"merge","text":"Prefers tabs.","kind":"preference","source_ids":[%d,%d]}]}`, ids[0], ids[1]), nil
	}
	res := d.nightlyConsolidateMemories(context.Background(), "merge-run", []string{"alice"})
	if res.Counts["merged"] != 1 {
		t.Fatalf("stage=%+v", res)
	}
	for _, id := range ids {
		item, e := bank.MemoryByID(context.Background(), id)
		if e != nil || item.Status != memory.StatusSuperseded {
			t.Fatalf("source %d=%+v err=%v", id, item, e)
		}
	}
	items, err := bank.ListMemories(context.Background(), "", memory.StatusActive, memory.MaxList)
	if err != nil || len(items) != 1 || items[0].Text != "Prefers tabs." {
		t.Fatalf("active merged items=%+v err=%v", items, err)
	}
	for _, id := range ids {
		item, _ := bank.MemoryByID(context.Background(), id)
		if item.SupersededBy != items[0].ID {
			t.Fatalf("source %d superseded_by=%d, want %d", id, item.SupersededBy, items[0].ID)
		}
	}
}

func TestNightlyCatchupIngestUsesCursorOnce(t *testing.T) {
	t.Parallel()
	d, _ := nightlyTestDaemon(t, nil, nil)
	seedEndedSession(t, d, "catchup-s1", "alice", []Event{userPrompt(1, "alice", "A durable user preference with enough text")})
	n, err := d.nightlyCatchupIngest(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("catchup sessions=%d err=%v", n, err)
	}
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := bank.ListEpisodes(context.Background(), memory.MaxList)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("episodes=%+v err=%v", episodes, err)
	}
	if n, err = d.nightlyCatchupIngest(context.Background()); err != nil || n != 1 {
		t.Fatalf("second catchup sessions=%d err=%v", n, err)
	}
	episodes, err = bank.ListEpisodes(context.Background(), memory.MaxList)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("duplicate catchup episodes=%+v err=%v", episodes, err)
	}
}
