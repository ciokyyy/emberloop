package web

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/emberloop-web/memory"
)

// Nightly consolidation: the offline pass over what was learned recently.
// Turn-end mining (mine_live.go) fills the bank with grounded observations
// all day; this stage reads the new ones together with their neighbours and
// the stored Rules they touch, and asks one cheap LLM call per User bank to
// cluster paraphrases, generalize recurring statements and flag refinements
// and contradictions. Shapes follow the engines we studied: hippo-memory's
// partitioned merge with rejected-digest skip, MemOS Dream's actions that
// must carry a rationale and a confidence, graphiti's "never fabricate, unsure
// means none", ReMe's same/related/unrelated classification.
//
// The LLM only proposes. Go validates every field, and the result is a
// pending `rule` queue item: the human gate is untouched. Nothing is deleted
// or edited here.

const (
	// consolidateWindow bounds the "new" observations the stage reads.
	consolidateWindow = 48 * time.Hour
	// consolidateMaxObs caps observations per call, new plus neighbours.
	consolidateMaxObs = 40
	// consolidateMaxRules caps stored Rules shown to the model.
	consolidateMaxRules = 15
	// consolidateMaxProposals caps proposals taken from one reply.
	consolidateMaxProposals = 10
	// consolidateNeighbours is how many older observations each new one pulls in.
	consolidateNeighbours = 3
	// consolidateMaxTokens bounds the reply.
	consolidateMaxTokens = 2048
)

// Consolidation actions the model may return.
const (
	consolidateGeneralize = "generalize"
	consolidateRefine     = "refine"
	consolidateContradict = "contradict"
)

type consolidateProposal struct {
	Action        string  `json:"action"`
	Text          string  `json:"text"`
	Kind          string  `json:"kind"`
	SubjectKey    string  `json:"subject_key"`
	SourceIDs     []int64 `json:"source_ids"`
	RelatedRuleID string  `json:"related_rule_id"`
	Rationale     string  `json:"rationale"`
	Confidence    float64 `json:"confidence"`
}

type consolidateReply struct {
	Proposals []consolidateProposal `json:"proposals"`
}

type consolidateObs struct {
	ID    int64  `json:"id"`
	Quote string `json:"quote"`
	Kind  string `json:"kind,omitempty"`
	Key   string `json:"key,omitempty"`
}

type consolidateRule struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// consolidateSystemPrompt states the task, the output schema and the refusal
// rules. "none" is the default answer.
func consolidateSystemPrompt() string {
	return "You consolidate a user's recorded statements into durable Rule proposals. " +
		"Input is JSON: observations (id, quote, kind, key) the user really said, and stored rules (id, text). " +
		"Reply with JSON only: {\"proposals\":[{\"action\":\"generalize\"|\"refine\"|\"contradict\"," +
		"\"text\":\"<=300 chars\",\"kind\":\"preference\"|\"decision\"|\"antipattern\"|\"fact\"|\"procedure\"," +
		"\"subject_key\":\"dotted.lowercase.key\",\"source_ids\":[observation ids],\"related_rule_id\":\"stored rule id or empty\"," +
		"\"rationale\":\"why\",\"confidence\":0.0-1.0}]}. " +
		"generalize: two or more observations state the same durable preference, decision or correction in different words; write one Rule that restates them. " +
		"refine: an observation narrows or extends a stored rule; name it in related_rule_id. " +
		"contradict: an observation says the opposite of a stored rule, explicitly; name it in related_rule_id. " +
		"Never invent content that no quote says. Never use situational statements (this function, for now, here). " +
		"Use only ids from the input. When unsure, or when nothing is durable, reply with exactly none. No other text."
}

// parseConsolidateReply reads the model reply: "none" is an empty answer,
// anything that is not the schema is an error. A code fence around the JSON
// is tolerated.
func parseConsolidateReply(raw string) ([]consolidateProposal, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("empty consolidate reply")
	}
	if strings.EqualFold(s, "none") {
		return nil, nil
	}
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	var r consolidateReply
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("consolidate reply is not the schema: %w", err)
	}
	return r.Proposals, nil
}

// validateConsolidate is the Go gate on one proposal. The model never
// decides what is allowed: ids must come from the input, a generalization
// needs at least two sources, a refine or contradict must name a stored rule
// shown to the model, and confidence must reach confirmHigh.
func validateConsolidate(p consolidateProposal, pool map[int64]memory.Observation, rules map[string]bool, high float64) error {
	switch p.Action {
	case consolidateGeneralize, consolidateRefine, consolidateContradict:
	default:
		return fmt.Errorf("unknown action %q", p.Action)
	}
	text := strings.TrimSpace(p.Text)
	if text == "" {
		return fmt.Errorf("empty text")
	}
	if utf8.RuneCountInString(text) > memory.MaxRuleChars {
		return fmt.Errorf("text over %d characters", memory.MaxRuleChars)
	}
	if memory.Redact(text) != text {
		return fmt.Errorf("text holds redactable content")
	}
	if !validMineKind(p.Kind) {
		return fmt.Errorf("invalid kind %q", p.Kind)
	}
	if strings.TrimSpace(p.Rationale) == "" {
		return fmt.Errorf("no rationale")
	}
	if p.Confidence < high || p.Confidence > 1 {
		return fmt.Errorf("confidence %.2f below %.2f", p.Confidence, high)
	}
	if len(p.SourceIDs) == 0 {
		return fmt.Errorf("no sources")
	}
	seen := make(map[int64]bool)
	for _, id := range p.SourceIDs {
		if _, ok := pool[id]; !ok {
			return fmt.Errorf("source %d not in input", id)
		}
		if seen[id] {
			return fmt.Errorf("source %d repeated", id)
		}
		seen[id] = true
	}
	if p.Action == consolidateGeneralize && len(p.SourceIDs) < 2 {
		return fmt.Errorf("generalize needs two or more sources")
	}
	if p.Action != consolidateGeneralize {
		if p.RelatedRuleID == "" || !rules[p.RelatedRuleID] {
			return fmt.Errorf("%s names no stored rule from the input", p.Action)
		}
	} else if p.RelatedRuleID != "" && !rules[p.RelatedRuleID] {
		return fmt.Errorf("related rule %q not in input", p.RelatedRuleID)
	}
	return nil
}

// coveredObservationIDs returns the source observation IDs of every rule
// proposal already in the queue, pending or decided. A rejected proposal is a
// label: its sources are not proposed again.
func coveredObservationIDs(items []memory.QueueItem) map[int64]bool {
	covered := make(map[int64]bool)
	for _, it := range items {
		if it.Kind != memory.QueueRule {
			continue
		}
		p, err := memory.DecodeRememberProposal(it.Payload)
		if err != nil {
			continue
		}
		for _, id := range p.SourceObservationIDs {
			covered[id] = true
		}
	}
	return covered
}

// consolidatePool picks the observations one bank shows the model: its new,
// uncovered observations (newest first), then up to consolidateNeighbours
// older uncovered neighbours of each, found by FTS5 over the quote.
func consolidatePool(ctx context.Context, bank memory.Memory, covered map[int64]bool, now time.Time) ([]memory.Observation, error) {
	recent, err := bank.ListObservations(ctx, memory.MaxList)
	if err != nil {
		return nil, err
	}
	pool := make(map[int64]memory.Observation)
	var order []int64
	add := func(o memory.Observation) {
		if o.ID == 0 || covered[o.ID] || len(order) >= consolidateMaxObs {
			return
		}
		if _, ok := pool[o.ID]; ok {
			return
		}
		pool[o.ID] = o
		order = append(order, o.ID)
	}
	var fresh []memory.Observation
	for _, o := range recent {
		if now.Sub(o.CreatedAt) <= consolidateWindow && !covered[o.ID] {
			fresh = append(fresh, o)
			add(o)
		}
	}
	if len(fresh) == 0 {
		return nil, nil
	}
	for _, o := range fresh {
		found, err := bank.SearchObservations(ctx, o.Quote, consolidateNeighbours+1)
		if err != nil {
			continue // neighbours are best effort
		}
		n := 0
		for _, c := range found {
			if n >= consolidateNeighbours {
				break
			}
			if c.ID == o.ID {
				continue
			}
			add(c)
			n++
		}
	}
	out := make([]memory.Observation, 0, len(order))
	for _, id := range order {
		out = append(out, pool[id])
	}
	return out, nil
}

// consolidateInput builds the JSON input within the 15 KB cap, whole entries
// only so the JSON stays valid. It returns the observations and rules that
// made it in.
func consolidateInput(obs []memory.Observation, rules []memory.Rule) (string, map[int64]memory.Observation, map[string]bool) {
	inObs := make(map[int64]memory.Observation)
	inRules := make(map[string]bool)
	var oEntries []consolidateObs
	var rEntries []consolidateRule
	size := 64
	for _, o := range obs {
		e := consolidateObs{ID: o.ID, Quote: memory.Redact(o.Quote), Kind: o.Kind, Key: o.SubjectKey}
		b, _ := json.Marshal(e)
		if size+len(b)+1 > nightlyDistillCap {
			break
		}
		size += len(b) + 1
		oEntries = append(oEntries, e)
		inObs[o.ID] = o
	}
	for _, r := range rules {
		e := consolidateRule{ID: r.RuleID, Text: memory.Redact(r.Text)}
		b, _ := json.Marshal(e)
		if size+len(b)+1 > nightlyDistillCap || len(rEntries) >= consolidateMaxRules {
			break
		}
		size += len(b) + 1
		rEntries = append(rEntries, e)
		inRules[r.RuleID] = true
	}
	raw, _ := json.Marshal(map[string]any{"observations": oEntries, "rules": rEntries})
	return string(raw), inObs, inRules
}

// consolidateRules gathers the stored Rules nearest the pool: FTS5 recall
// per observation, deduped by Rule ID, latest live version.
func consolidateRules(ctx context.Context, bank memory.Memory, obs []memory.Observation) []memory.Rule {
	seen := make(map[string]bool)
	var out []memory.Rule
	for _, o := range obs {
		rs, err := bank.Recall(ctx, o.Quote, 3)
		if err != nil {
			continue
		}
		for _, r := range rs {
			if seen[r.RuleID] {
				continue
			}
			seen[r.RuleID] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out
}

// nightlyConsolidate runs the consolidation pass over every User bank. A bank
// with no new observations, a full queue or no memory model is skipped; a bad
// reply or a rejected proposal counts, it never stops the other banks. The
// stage only enqueues pending rule proposals.
func (d *Daemon) nightlyConsolidate(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageConsolidate, owners); fail != nil {
		return *fail
	}
	finish := func(status string, counts map[string]any, detail string) NightlyStageResult {
		d.nightlyLedger(ctx, runID, nightlyStageConsolidate, status, counts, detail, owners)
		return NightlyStageResult{Stage: nightlyStageConsolidate, Status: status, Counts: counts, Detail: detail}
	}
	if len(owners) == 0 {
		return NightlyStageResult{Stage: nightlyStageConsolidate, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": 0}, Detail: "no banks to consolidate"}
	}
	set := d.deciderSettingsOf()
	model := strings.TrimSpace(set.Model)
	if model == "" && d.consolidateOneShot == nil {
		return finish(memory.LedgerNoData, map[string]any{"owners": len(owners)},
			"no memory model: consolidation skipped")
	}
	banks, failed, firstErr := d.nightlyOpenBanks(owners)
	proposed, rejected, banksRun, skippedFull := 0, 0, 0, 0
	now := d.now()
	for _, owner := range owners {
		bank, ok := banks[owner]
		if !ok {
			continue
		}
		items, err := bank.Queue(ctx)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		pending := 0
		for _, it := range items {
			if it.Kind == memory.QueueRule && it.Status == memory.QueuePending {
				pending++
			}
		}
		if pending >= memory.QueueBudget {
			skippedFull++
			continue
		}
		pool, err := consolidatePool(ctx, bank, coveredObservationIDs(items), now)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		if len(pool) == 0 {
			continue
		}
		input, inObs, inRules := consolidateInput(pool, consolidateRules(ctx, bank, pool))
		reply, err := d.nightlyLLMCall(d.consolidateOneShot, model, consolidateSystemPrompt(), input)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		banksRun++
		props, err := parseConsolidateReply(reply)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		taken := 0
		for _, p := range props {
			if taken >= consolidateMaxProposals || pending+taken >= memory.QueueBudget {
				break
			}
			if err := validateConsolidate(p, inObs, inRules, set.High); err != nil {
				rejected++
				continue
			}
			if _, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule,
				Payload: memory.EncodeRuleProposal(consolidateToRuleProposal(owner, p, inObs))}); err != nil {
				failed++
				if firstErr == "" {
					firstErr = err.Error()
				}
				continue
			}
			taken++
			proposed++
			// Sources are covered from now on; a later pass over the same
			// observations finds them in the queue and skips them.
		}
	}
	counts := map[string]any{"owners": len(owners), "banks": banksRun, "proposed": proposed,
		"rejected": rejected, "queue_full": skippedFull, "failed": failed}
	detail := fmt.Sprintf("consolidated %d banks: %d proposed, %d rejected by validation", banksRun, proposed, rejected)
	status := memory.LedgerSuccess
	switch {
	case failed > 0:
		status = memory.LedgerFailed
		detail = fmt.Sprintf("consolidate partial: %d failed (%s); %s", failed, firstErr, detail)
	case banksRun == 0 && skippedFull == 0:
		status = memory.LedgerNoData
		detail = "no new observations to consolidate"
	}
	return finish(status, counts, detail)
}

// consolidateToRuleProposal builds the queue payload. Quotes and Sessions
// come from the source observations, never from the model.
func consolidateToRuleProposal(owner string, p consolidateProposal, pool map[int64]memory.Observation) memory.RuleProposal {
	rp := memory.RuleProposal{
		Text:                 strings.TrimSpace(p.Text),
		Kind:                 p.Kind,
		SubjectKey:           p.SubjectKey,
		Author:               owner,
		Origin:               "consolidated",
		Reason:               strings.TrimSpace(p.Rationale),
		SourceObservationIDs: append([]int64(nil), p.SourceIDs...),
		RelatedRuleID:        p.RelatedRuleID,
	}
	switch p.Action {
	case consolidateRefine:
		rp.Relation = memory.RelRefines
	case consolidateContradict:
		rp.Relation = memory.RelContradicts
	}
	seen := make(map[string]bool)
	for _, id := range p.SourceIDs {
		o := pool[id]
		rp.Quotes = append(rp.Quotes, o.Quote)
		if o.SessionID != "" && !seen[o.SessionID] {
			seen[o.SessionID] = true
			rp.Sessions = append(rp.Sessions, o.SessionID)
		}
	}
	return rp
}
