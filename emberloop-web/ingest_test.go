package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"example.com/emberloop-web/memory"
)

type ingestScript struct {
	req   memory.ExtractRequest
	items []memory.ExtractedItem
	err   error
}

func (s *ingestScript) Extract(_ context.Context, r memory.ExtractRequest) ([]memory.ExtractedItem, error) {
	s.req = r
	return s.items, s.err
}
func (s *ingestScript) CostUSD() (float64, bool) { return 0.002, true }

func TestIngestRedactsSelectsSenderAndKeepsAssistantContextUnquotable(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	events := []Event{
		userPrompt(1, "bob", "I prefer concise technical explanations"),
		{Seq: 2, Kind: "prompt", Text: "injected user-looking content", Sender: senderSystem},
		{Seq: 3, Kind: "prompt", Text: "/status please ignore this", Sender: senderUser, User: "alice"},
		{Seq: 4, Kind: "assistant_chunk", Text: "assistant only quote must not persist"},
	}
	activeSession(t, d, "s1", "alice", events)
	// The test helper's persisted transcript is read directly by ingestion.
	script := &ingestScript{items: []memory.ExtractedItem{{Kind: memory.KindFact, Text: "assistant quote", Quote: "assistant only quote must not persist", Event: 1, Stated: true}}}
	d.ingestExtractor = script
	d.ingestModel = "test-model"
	if err := d.ingestSession("s1"); err != nil {
		t.Fatal(err)
	}
	if len(script.req.Events) != 1 || script.req.Events[0].Text != "I prefer concise technical explanations" {
		t.Fatalf("extract events=%+v", script.req.Events)
	}
	if script.req.AssistantContext != "assistant only quote must not persist" {
		t.Fatalf("assistant context=%q", script.req.AssistantContext)
	}
	bob, err := d.profileBankFor("bob")
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := bob.ListEpisodes(context.Background(), 20)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("Bob episodes=%+v err=%v", episodes, err)
	}
	alice, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	aliceEpisodes, _ := alice.ListEpisodes(context.Background(), 20)
	if len(aliceEpisodes) != 0 {
		t.Fatalf("Session owner received Bob's memory: %+v", aliceEpisodes)
	}
	memories, _ := bob.ListMemories(context.Background(), "", "", 20)
	if len(memories) != 0 {
		t.Fatalf("assistant context accepted as quote: %+v", memories)
	}
}

func TestIngestExtractorFailureStillStoresEpisodeAndLedger(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "I prefer stable API names")})
	d.ingestExtractor = &ingestScript{err: errors.New("model unavailable")}
	d.ingestModel = "test-model"
	if err := d.ingestSession("s1"); err != nil {
		t.Fatal(err)
	}
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	episodes, _ := bank.ListEpisodes(context.Background(), 20)
	rows, _ := bank.Ledger(context.Background())
	if len(episodes) != 1 {
		t.Fatalf("episodes=%+v", episodes)
	}
	found := false
	for _, row := range rows {
		if row.Stage == "ingest" && row.Status == memory.LedgerFailed {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing failed ingest ledger row: %+v", rows)
	}
}
func TestIngestOffDoesNotRunHeuristicFallback(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(useSettings(t, `{"memory_extract_effort":"off"}`))
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	if err := d.ingestSession("s1"); err != nil {
		t.Fatal(err)
	}
	bank, _ := d.profileBankFor("alice")
	episodes, _ := bank.ListEpisodes(context.Background(), 20)
	memories, _ := bank.ListMemories(context.Background(), "", "", 20)
	if len(episodes) != 1 || len(memories) != 0 {
		t.Fatalf("episodes=%+v memories=%+v", episodes, memories)
	}
}

func TestIngestNewestEventsWinScanCap(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	var events []Event
	for i, size := range []int{1024, 4096, 4096, 4096, 4096} {
		events = append(events, Event{Seq: int64(i + 1), Kind: "prompt", Text: fmt.Sprintf("%c%s", 'a'+i, strings.Repeat(string(rune('a'+i)), size-1)), Sender: senderUser, User: "alice"})
	}
	activeSession(t, d, "s1", "alice", events)
	if err := d.ingestSession("s1"); err != nil {
		t.Fatal(err)
	}
	got := liveEpisodes(t, d, "alice")
	if len(got) != 3 {
		t.Fatalf("episodes=%d want newest three", len(got))
	}
	for _, ep := range got {
		if strings.HasPrefix(ep.Text, "a") {
			t.Fatalf("older Event survived newest-first cap")
		}
	}
}

func TestIngestLedgerCostAndFailureCounts(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "I prefer stable API names")})
	d.ingestExtractor = &ingestScript{err: errors.New("model unavailable")}
	d.ingestModel = "test-model"
	if err := d.ingestSession("s1"); err != nil {
		t.Fatal(err)
	}
	bank, _ := d.profileBankFor("alice")
	rows, _ := bank.Ledger(context.Background())
	for _, row := range rows {
		if row.Stage != "ingest" {
			continue
		}
		if !strings.Contains(row.Detail, "cost_usd=0.00200000") {
			t.Fatalf("detail=%s", row.Detail)
		}
		var counts map[string]any
		if err := json.Unmarshal([]byte(row.CountsJSON), &counts); err != nil {
			t.Fatal(err)
		}
		if counts["dropped_ungrounded"] != float64(0) || counts["extraction_failed"] != true {
			t.Fatalf("counts=%v", counts)
		}
		return
	}
	t.Fatal("missing ingest ledger")
}
