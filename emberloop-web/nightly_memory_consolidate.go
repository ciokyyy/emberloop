package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/emberloop-web/memory"
)

type consolidateMemoryInput struct {
	ID         int64   `json:"id"`
	SessionID  string  `json:"session_id"`
	Kind       string  `json:"kind"`
	Text       string  `json:"text"`
	Quote      string  `json:"quote"`
	Importance float64 `json:"importance"`
}
type memoryConsolidation struct {
	Action    string  `json:"action"`
	Text      string  `json:"text"`
	Kind      string  `json:"kind"`
	SourceIDs []int64 `json:"source_ids"`
}

// nightlyConsolidateMemories validates model proposals against the exact
// Memory set sent to it. Generalizations remain pending; merges create a new
// immutable Memory and soft-supersede their sources.
func (d *Daemon) nightlyConsolidateMemories(ctx context.Context, runID string, owners []string) NightlyStageResult {
	const stage = "nightly_memory_consolidate"
	if fail := d.nightlyInjectedFailure(ctx, runID, stage, owners); fail != nil {
		return *fail
	}
	proposed, merged, rejected, failed := 0, 0, 0, 0
	setting := strings.TrimSpace(d.deciderSettingsOf().Model)
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			failed++
			continue
		}
		items, err := bank.ListMemories(ctx, "", memory.StatusActive, memory.MaxList)
		if err != nil {
			failed++
			continue
		}
		queue, err := bank.Queue(ctx)
		if err != nil {
			failed++
			continue
		}
		queuedQuotes := map[string]bool{}
		pending := 0
		for _, q := range queue {
			if q.Kind == memory.QueueRule && q.Status == memory.QueuePending {
				pending++
			}
			if p, e := memory.DecodeRememberProposal(q.Payload); e == nil && p.Origin == "consolidated_memory" {
				for _, quote := range p.Quotes {
					queuedQuotes[quote] = true
				}
			}
		}
		cutoff := d.now().Add(-48 * time.Hour)
		pool := make([]memory.MemoryItem, 0)
		for _, item := range items {
			if !item.CreatedAt.Before(cutoff) {
				item.Embedding, item.EmbeddingModel = nil, ""
				pool = append(pool, item)
			}
		}
		if len(pool) < 2 || (setting == "" && d.consolidateOneShot == nil) {
			continue
		}
		payload := make([]consolidateMemoryInput, 0, len(pool))
		selected := make([]memory.MemoryItem, 0, len(pool))
		size := 0
		for _, item := range pool {
			redacted := consolidateMemoryInput{ID: item.ID, SessionID: item.SessionID, Kind: item.Kind, Text: memory.Redact(item.Text), Quote: memory.Redact(item.Quote), Importance: item.Importance}
			encoded, _ := json.Marshal(redacted)
			if size+len(encoded) > nightlyDistillCap {
				break
			}
			payload, selected, size = append(payload, redacted), append(selected, item), size+len(encoded)
		}
		pool = selected
		if len(pool) < 2 {
			continue
		}
		input, _ := json.Marshal(payload)
		reply, err := d.nightlyLLMCall(d.consolidateOneShot, setting, "Read recent user Memories. Propose supported merges or generalizations. Return JSON only: {\"proposals\":[{\"action\":\"merge|generalize\",\"text\":\"<=300 chars\",\"kind\":\"preference|instruction|decision|fact\",\"source_ids\":[memory IDs]}]}. Each needs two distinct input IDs, valid kind and redaction-safe text. Use none when unsure.", string(input))
		if err != nil {
			failed++
			continue
		}
		if strings.EqualFold(strings.TrimSpace(reply), "none") {
			continue
		}
		var result struct {
			Proposals []memoryConsolidation `json:"proposals"`
		}
		if json.Unmarshal([]byte(reply), &result) != nil || len(result.Proposals) > 10 {
			failed++
			continue
		}
		byID := make(map[int64]memory.MemoryItem, len(pool))
		for _, item := range pool {
			byID[item.ID] = item
		}
		for _, proposal := range result.Proposals {
			valid := (proposal.Action == "merge" || proposal.Action == "generalize") && validMemoryNightlyKind(proposal.Kind) && strings.TrimSpace(proposal.Text) != "" && utf8.RuneCountInString(proposal.Text) <= 300 && memory.Redact(proposal.Text) == proposal.Text && len(proposal.SourceIDs) >= 2
			seen := map[int64]bool{}
			sources := make([]memory.MemoryItem, 0, len(proposal.SourceIDs))
			for _, id := range proposal.SourceIDs {
				item, ok := byID[id]
				if !ok || seen[id] {
					valid = false
					break
				}
				seen[id] = true
				sources = append(sources, item)
			}
			if !valid {
				rejected++
				continue
			}
			quotes, sessions := make([]string, 0, len(sources)), make([]string, 0, len(sources))
			for _, item := range sources {
				quotes = append(quotes, item.Quote)
				sessions = append(sessions, item.SessionID)
			}
			alreadyQueued := false
			for _, source := range sources {
				if queuedQuotes[source.Quote] {
					alreadyQueued = true
				}
			}
			if alreadyQueued {
				continue
			}
			if proposal.Action == "generalize" {
				if pending >= memory.QueueBudget {
					continue
				}
				_, err = bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: memory.EncodeRuleProposal(memory.RuleProposal{Text: strings.TrimSpace(proposal.Text), Kind: proposal.Kind, Author: owner, Origin: "consolidated_memory", Reason: "generalized recurring Memories", Quotes: quotes, Sessions: sessions})})
				if err != nil {
					failed++
				} else {
					proposed++
					pending++
					for _, quote := range quotes {
						queuedQuotes[quote] = true
					}
				}
				continue
			}
			first := sources[0]
			newID, err := bank.AddMemory(ctx, memory.MemoryItem{SessionID: first.SessionID, EventID: first.EventID, Author: owner, Kind: proposal.Kind, Text: strings.TrimSpace(proposal.Text), Quote: first.Quote, Importance: first.Importance, Veracity: memory.VeracityInferred})
			if err != nil {
				failed++
				continue
			}
			for _, source := range sources {
				if err = bank.SetMemorySuperseded(ctx, source.ID, newID); err != nil {
					failed++
				}
			}
			merged++
		}
	}
	status := memory.LedgerSuccess
	if failed > 0 {
		status = memory.LedgerFailed
	}
	if proposed+merged+rejected == 0 && failed == 0 {
		status = memory.LedgerNoData
	}
	counts := map[string]any{"proposed": proposed, "merged": merged, "rejected": rejected, "failed": failed}
	detail := fmt.Sprintf("consolidated Memories: %d pending generalizations, %d merges, %d rejected", proposed, merged, rejected)
	d.nightlyLedger(ctx, runID, stage, status, counts, detail, owners)
	return NightlyStageResult{Stage: stage, Status: status, Counts: counts, Detail: detail}
}
