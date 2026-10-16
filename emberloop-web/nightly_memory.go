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

const foldAge = 12 * time.Hour

type foldedMemory struct {
	Kind       string  `json:"kind"`
	Text       string  `json:"text"`
	Quote      string  `json:"quote"`
	EpisodeIDs []int64 `json:"episode_ids"`
}

func (d *Daemon) nightlyReembed(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageReembed, owners); fail != nil {
		return *fail
	}
	failed, memories, episodes := 0, 0, 0
	embedder, model := d.memoryEmbeddingOptions()
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			failed++
			continue
		}
		if embedder == nil || model == "" {
			continue
		}
		for _, status := range []string{memory.StatusActive, memory.StatusParked, memory.StatusSuperseded, memory.StatusForgotten} {
			ms, err := bank.ListMemories(ctx, "", status, -1)
			if err != nil {
				failed++
				continue
			}
			for _, item := range ms {
				if len(item.Embedding) > 0 && item.EmbeddingModel == model {
					continue
				}
				v, e := embedder.Embed(ctx, model, []string{memory.ForEmbedder(item.Text)})
				if e != nil || len(v) != 1 || bank.SetMemoryEmbedding(ctx, item.ID, memory.EncodeVector(v[0]), model) != nil {
					failed++
					continue
				}
				memories++
			}
		}
		eps, err := bank.ListEpisodes(ctx, -1)
		if err != nil {
			failed++
			continue
		}
		for _, ep := range eps {
			if len(ep.Embedding) > 0 && ep.EmbeddingModel == model {
				continue
			}
			v, e := embedder.Embed(ctx, model, []string{memory.ForEmbedder(ep.Text)})
			if e != nil || len(v) != 1 || bank.SetEpisodeEmbedding(ctx, ep.ID, memory.EncodeVector(v[0]), model) != nil {
				failed++
				continue
			}
			episodes++
		}
	}
	status := memory.LedgerSuccess
	if failed > 0 {
		status = memory.LedgerFailed
	}
	if memories+episodes == 0 && failed == 0 {
		status = memory.LedgerNoData
	}
	counts := map[string]any{"memories": memories, "episodes": episodes, "failed": failed}
	detail := fmt.Sprintf("reembedded %d Memories and %d Episodes", memories, episodes)
	d.nightlyLedger(ctx, runID, nightlyStageReembed, status, counts, detail, owners)
	return NightlyStageResult{Stage: nightlyStageReembed, Status: status, Counts: counts, Detail: detail}
}

func (d *Daemon) nightlyFold(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageFold, owners); fail != nil {
		return *fail
	}
	model := strings.TrimSpace(d.deciderSettingsOf().Model)
	made, folded, failed := 0, 0, 0
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			failed++
			continue
		}
		episodes, err := bank.ListEpisodes(ctx, -1)
		if err != nil {
			failed++
			continue
		}
		groups := map[string][]memory.Episode{}
		for _, ep := range episodes {
			if ep.ConsolidatedAt.IsZero() && ep.CreatedAt.Before(d.now().Add(-foldAge)) {
				groups[ep.SessionID] = append(groups[ep.SessionID], ep)
			}
		}
		for session, group := range groups {
			if model == "" && d.foldOneShot == nil {
				continue
			}
			input, _ := json.Marshal(group)
			reply, e := d.nightlyLLMCall(d.foldOneShot, model, "Summarize these redacted episodes into at most five durable inferred memory items. Return JSON only: {\"items\":[{\"kind\":\"preference|instruction|decision|fact\",\"text\":\"...\",\"quote\":\"verbatim source substring\",\"episode_ids\":[id]}]}. Quote must be verbatim; never invent.", string(input))
			if e != nil {
				failed++
				continue
			}
			var result struct {
				Items []foldedMemory `json:"items"`
			}
			if json.Unmarshal([]byte(reply), &result) != nil || len(result.Items) > 5 {
				failed++
				continue
			}
			valid := true
			epByID := map[int64]memory.Episode{}
			for _, ep := range group {
				epByID[ep.ID] = ep
			}
			for _, item := range result.Items {
				if !validMemoryNightlyKind(item.Kind) || strings.TrimSpace(item.Text) == "" || utf8.RuneCountInString(item.Text) > 300 || memory.Redact(item.Text) != item.Text || item.Quote == "" || len(item.EpisodeIDs) == 0 {
					valid = false
					break
				}
				grounded := false
				for _, id := range item.EpisodeIDs {
					ep, ok := epByID[id]
					if !ok {
						valid = false
						break
					}
					if strings.Contains(ep.Text, item.Quote) {
						grounded = true
					}
				}
				if !valid || !grounded {
					valid = false
					break
				}
			}
			if !valid {
				failed++
				continue
			}
			for _, item := range result.Items {
				if len(item.EpisodeIDs) == 0 {
					continue
				}
				ep, e := bank.EpisodeByID(ctx, item.EpisodeIDs[0])
				if e != nil {
					failed++
					valid = false
					break
				}
				if _, e = bank.AddMemory(ctx, memory.MemoryItem{SessionID: session, EventID: ep.EventRange, Author: owner, Kind: item.Kind, Text: strings.TrimSpace(item.Text), Quote: item.Quote, Importance: .6, Veracity: memory.VeracityInferred}); e != nil {
					failed++
					valid = false
					break
				}
				made++
			}
			if !valid {
				continue
			}
			for _, ep := range group {
				if bank.SetEpisodeConsolidated(ctx, ep.ID, d.now()) == nil {
					folded++
				} else {
					failed++
				}
			}
		}
	}
	status := memory.LedgerSuccess
	if failed > 0 {
		status = memory.LedgerFailed
	}
	if folded == 0 && failed == 0 {
		status = memory.LedgerNoData
	}
	counts := map[string]any{"memories": made, "episodes": folded, "failed": failed}
	detail := fmt.Sprintf("folded %d Episodes into %d inferred Memories", folded, made)
	d.nightlyLedger(ctx, runID, nightlyStageFold, status, counts, detail, owners)
	return NightlyStageResult{Stage: nightlyStageFold, Status: status, Counts: counts, Detail: detail}
}

func validMemoryNightlyKind(kind string) bool {
	return kind == memory.KindPreference || kind == memory.KindInstruction || kind == memory.KindDecision || kind == memory.KindFact
}

func (d *Daemon) nightlyPromote(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStagePromote, owners); fail != nil {
		return *fail
	}
	candidates, proposed, failed := 0, 0, 0
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			failed++
			continue
		}
		items, err := bank.ListMemories(ctx, "", memory.StatusActive, -1)
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
		for _, q := range queue {
			if p, e := memory.DecodeRememberProposal(q.Payload); e == nil && p.Origin == "memory_candidate" {
				for _, quote := range p.Quotes {
					queuedQuotes[quote] = true
				}
			}
		}
		liveRules, err := bank.LiveRules(ctx)
		if err != nil {
			failed++
			continue
		}
		embedder, model := d.memoryEmbeddingOptions()
		var ruleVectors [][]float32
		ruleCheckReady := len(liveRules) == 0
		if embedder != nil && model != "" {
			ruleCheckReady = true
			for _, rule := range liveRules {
				v, e := embedder.Embed(ctx, model, []string{memory.ForEmbedder(rule.Text)})
				if e != nil || len(v) != 1 {
					ruleCheckReady = false
					continue
				}
				ruleVectors = append(ruleVectors, v[0])
			}
		}
		used := map[int64]bool{}
		for i, m := range items {
			if used[m.ID] || queuedQuotes[m.Quote] || m.MentionCount < 2 || m.Importance < .7 || (m.Kind != memory.KindPreference && m.Kind != memory.KindInstruction && m.Kind != memory.KindDecision) {
				continue
			}
			cluster := []memory.MemoryItem{m}
			sessions := map[string]bool{m.SessionID: true}
			for _, other := range items[i+1:] {
				if used[other.ID] || other.Kind != m.Kind || len(m.Embedding) == 0 || len(other.Embedding) == 0 || m.EmbeddingModel != other.EmbeddingModel {
					continue
				}
				a, _ := memory.DecodeVector(m.Embedding)
				b, _ := memory.DecodeVector(other.Embedding)
				if memory.Cosine(a, b) >= .80 {
					cluster = append(cluster, other)
					sessions[other.SessionID] = true
				}
			}
			if len(sessions) < 2 {
				continue
			}
			if len(liveRules) > 0 && (!ruleCheckReady || m.EmbeddingModel != model) {
				continue
			}
			if len(ruleVectors) > 0 && m.EmbeddingModel == model {
				mv, _ := memory.DecodeVector(m.Embedding)
				blocked := false
				for _, rv := range ruleVectors {
					if memory.Cosine(mv, rv) >= .88 {
						blocked = true
						break
					}
				}
				if blocked {
					continue
				}
			}
			candidates++
			quotes, sessionIDs := []string{}, []string{}
			for _, v := range cluster {
				used[v.ID] = true
				quotes = append(quotes, v.Quote)
				sessionIDs = append(sessionIDs, v.SessionID)
			}
			_, err = bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: memory.EncodeRuleProposal(memory.RuleProposal{Text: m.Text, Kind: m.Kind, Author: owner, Origin: "memory_candidate", Reason: "recurring Memory candidate", Quotes: quotes, Sessions: sessionIDs})})
			if err != nil {
				failed++
			} else {
				proposed++
				for _, source := range cluster {
					countsJSON, _ := json.Marshal(map[string]int64{"count": 1, "memory_id": source.ID})
					if _, ledgerErr := bank.RecordLedger(ctx, memory.LedgerRow{RunID: runID, Stage: "memory_promoted", Status: memory.LedgerSuccess, CountsJSON: string(countsJSON)}); ledgerErr != nil {
						failed++
					}
				}
			}
		}
	}
	status := memory.LedgerSuccess
	if failed > 0 {
		status = memory.LedgerFailed
	}
	if candidates == 0 {
		status = memory.LedgerNoData
	}
	counts := map[string]any{"candidates": candidates, "proposed": proposed, "failed": failed}
	detail := fmt.Sprintf("promoted %d recurring Memory candidates to the pending Rule queue", proposed)
	d.nightlyLedger(ctx, runID, nightlyStagePromote, status, counts, detail, owners)
	return NightlyStageResult{Stage: nightlyStagePromote, Status: status, Counts: counts, Detail: detail}
}
