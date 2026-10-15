package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/emberloop-web/el/tools/systemone"
	"example.com/emberloop-web/memory"
)

const (
	ingestDebounce = 3 * time.Second
	maxIngestEvent = 4 << 10
	maxIngestScan  = 15 << 10
)

type ingestCounts struct {
	Scanned           int  `json:"scanned"`
	Kept              int  `json:"kept"`
	Extracted         int  `json:"extracted"`
	DroppedUngrounded int  `json:"dropped_ungrounded"`
	Stored            int  `json:"stored"`
	Duplicates        int  `json:"duplicates"`
	Superseded        int  `json:"superseded"`
	ExtractionFailed  bool `json:"extraction_failed,omitempty"`
}
type ingestEvent struct {
	event Event
	text  string
}

func (d *Daemon) ingestSession(sessionID string) error {
	started := time.Now()
	cur, err := memory.LoadCursor(d.minerCursorPath(sessionID))
	if err != nil {
		return err
	}
	chunk, next, _, err := memory.ReadNew(d.sessionDir(sessionID)+"/events.jsonl", cur)
	if err != nil {
		return err
	}
	var all []Event
	for _, line := range strings.Split(string(chunk), "\n") {
		var e Event
		if strings.TrimSpace(line) != "" && json.Unmarshal([]byte(line), &e) == nil {
			all = append(all, e)
		}
	}
	counts := ingestCounts{Scanned: len(all)}
	lastPrompt := -1
	for i, e := range all {
		if MinerReadable(e) {
			lastPrompt = i
		}
	}
	var assistantParts []string
	for i := lastPrompt + 1; i < len(all); i++ {
		if all[i].Kind == "assistant_chunk" && strings.TrimSpace(all[i].Text) != "" {
			assistantParts = append(assistantParts, all[i].Text)
		}
	}
	assistant := memory.Redact(cutUTF8(strings.Join(assistantParts, "\n"), 1024))
	var chosen []ingestEvent
	used := 0
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if !MinerReadable(e) || e.Sender != senderUser || e.Handoff || strings.HasPrefix(strings.TrimSpace(e.Text), "/") {
			continue
		}
		if utf8.RuneCountInString(strings.TrimSpace(e.Text)) < 12 {
			continue
		}
		// Redact before truncation so a secret crossing the byte boundary is never sent.
		text := cutUTF8(memory.Redact(strings.TrimSpace(e.Text)), maxIngestEvent)
		if text == "" || len(text)+used > maxIngestScan {
			break
		}
		used += len(text)
		chosen = append(chosen, ingestEvent{e, text})
	}
	for i, j := 0, len(chosen)-1; i < j; i, j = i+1, j-1 {
		chosen[i], chosen[j] = chosen[j], chosen[i]
	}
	counts.Kept = len(chosen)
	ctx := context.Background()
	banks := make(map[string]memory.Memory)
	episodeFailed := false
	for n, item := range chosen {
		author := strings.TrimSpace(item.event.User)
		if author == "" {
			continue
		}
		bank := banks[author]
		if bank == nil {
			bank, err = d.profileBankFor(author)
			if err != nil {
				episodeFailed = true
				continue
			}
			banks[author] = bank
		}
		if _, err = bank.AddEpisode(ctx, memory.Episode{SessionID: sessionID, EventRange: ingestEventID(item.event, n), Author: author, Text: item.text, Importance: 0.65}); err != nil {
			episodeFailed = true
		}
	}
	model, effort, extractor := d.turnExtractor()
	extractionFailed := false
	if len(chosen) > 0 && model != "" && extractor != nil {
		events := make([]memory.ExtractEvent, 0, len(chosen))
		for n, item := range chosen {
			events = append(events, memory.ExtractEvent{Number: n + 1, ID: ingestEventID(item.event, n), Text: item.text})
		}
		items, extractErr := extractor.Extract(ctx, memory.ExtractRequest{Events: events, AssistantContext: assistant})
		if extractErr != nil {
			extractionFailed = true
			counts.ExtractionFailed = true
		} else {
			valid, dropped := memory.ValidateExtractedItems(items, events)
			counts.Extracted = len(items)
			counts.DroppedUngrounded = len(dropped)
			for _, item := range valid {
				// The validator admits only unique event numbers in this 1-based list;
				// guard the indexing boundary too, so malformed output cannot panic.
				if item.Event < 1 || item.Event > len(events) {
					counts.DroppedUngrounded++
					continue
				}
				ev := events[item.Event-1]
				owner := strings.TrimSpace(chosen[item.Event-1].event.User)
				bank := banks[owner]
				if bank == nil {
					continue
				}
				veracity := memory.VeracityStated
				if !item.Stated {
					veracity = memory.VeracityInferred
				}
				_, err = bank.AddMemory(ctx, memory.MemoryItem{SessionID: sessionID, EventID: ev.ID, Author: owner, Kind: item.Kind, Text: item.Text, Quote: item.Quote, Importance: memory.MemoryImportance(item.Text, item.Stated), Veracity: veracity, ExtractorJSON: fmt.Sprintf(`{"model":%q,"effort":%q,"prompt":"x1"}`, model, effort)})
				if err == nil {
					counts.Stored++
				} else {
					counts.DroppedUngrounded++
				}
			}
		}
	}
	// The heuristic is only the fallback after a configured extractor fails.
	if extractionFailed {
		counts.Stored += d.storeIngestFallback(ctx, sessionID, chosen, banks)
	}
	countsJSON, _ := json.Marshal(counts)
	if len(banks) == 0 {
		d.mu.Lock()
		owner := ""
		if s := d.sessions[sessionID]; s != nil {
			owner = s.meta.Owner
		}
		d.mu.Unlock()
		if owner != "" {
			if b, e := d.profileBankFor(owner); e == nil {
				banks[owner] = b
			}
		}
	}
	status := memory.LedgerSuccess
	if episodeFailed || extractionFailed {
		status = memory.LedgerFailed
	} else if counts.Kept == 0 {
		status = memory.LedgerNoData
	}
	costText := "unknown"
	costKnown := extractor == nil || len(chosen) == 0
	if costKnown {
		costText = "0"
	}
	if reporter, ok := extractor.(interface{ CostUSD() (float64, bool) }); ok {
		if cost, known := reporter.CostUSD(); known {
			costText = fmt.Sprintf("%.8f", cost)
		}
	}
	detail := fmt.Sprintf("model=%s latency_ms=%d cost_usd=%s", model, time.Since(started).Milliseconds(), costText)
	for _, bank := range banks {
		_, _ = bank.RecordLedger(ctx, memory.LedgerRow{RunID: sessionID, Stage: "ingest", Status: status, CountsJSON: string(countsJSON), Detail: detail})
	}
	if episodeFailed {
		return fmt.Errorf("ingest: episode storage failed for session %s", sessionID)
	}
	return memory.SaveCursor(d.minerCursorPath(sessionID), next)
}
func ingestEventID(e Event, index int) string {
	if e.Seq != 0 {
		return fmt.Sprint(e.Seq)
	}
	return fmt.Sprintf("line-%d", index)
}
func (d *Daemon) storeIngestFallback(ctx context.Context, sessionID string, chosen []ingestEvent, banks map[string]memory.Memory) int {
	stored := 0
	for n, item := range chosen {
		if _, ok := memory.ClassifyHeuristic(item.text); !ok {
			continue
		}
		text := truncateIngestRunes(strings.TrimSpace(item.text), 300)
		owner := strings.TrimSpace(item.event.User)
		bank := banks[owner]
		if bank == nil || text == "" {
			continue
		}
		_, err := bank.AddMemory(ctx, memory.MemoryItem{SessionID: sessionID, EventID: ingestEventID(item.event, n), Author: owner, Kind: memory.KindPreference, Text: text, Quote: item.text, Importance: 0.4, Veracity: memory.VeracityInferred, ExtractorJSON: `{"model":"heuristic","prompt":"x1"}`})
		if err == nil {
			stored++
		}
	}
	return stored
}
func truncateIngestRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	n := 0
	for i := range s {
		if n == limit {
			return s[:i]
		}
		n++
	}
	return s
}
func cutUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
func (d *Daemon) turnExtractor() (string, string, memory.Extractor) {
	if d.ingestExtractor != nil {
		model := d.ingestModel
		if model == "" {
			model = memory.DefaultExtractorModel
		}
		return model, d.ingestEffort, d.ingestExtractor
	}
	all, err := d.settingsMap()
	if err != nil {
		return "", "", nil
	}
	if raw, ok := all["memory_extract_model"]; ok {
		var configured string
		_ = json.Unmarshal(raw, &configured)
		if strings.TrimSpace(configured) == "" {
			return "", "", nil
		}
	}
	var model, effort string
	_ = json.Unmarshal(all["memory_extract_model"], &model)
	_ = json.Unmarshal(all["memory_extract_effort"], &effort)
	if model == "" {
		model = memory.DefaultExtractorModel
	}
	if model == "off" || effort == "off" {
		return "", effort, nil
	}
	if key := systemone.APIKey(context.Background()); key != "" {
		return model, effort, memory.NewOpenRouterExtractor(key, model, effort)
	}
	return model, effort, nil
}
