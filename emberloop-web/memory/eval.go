// Offline eval helpers for ticket 30: replay recall@5, the repeat-correction
// rate, the embedder gap and Decider precision at the approve threshold.
//
// These helpers are pure: they compare strings with the same NormalizeQuote
// and Relation rules the miner's dedup uses, so canned logs give
// deterministic numbers without a database. The Daemon replay in
// ../replay.go builds the cases from Session histories and bank recall; the
// CLI in cmd/emberloop-web prints them.
package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// EvalDeciders are the Decider names the eval scores. Heuristic runs live
// from ClassifyHeuristic; llm, jev and laya read precomputed durable scores
// from a scores file (they need a model or a network), and report
// unavailable when a label has no score.
var EvalDeciders = []string{"heuristic", "llm", "jev", "laya"}

// CoversRule reports whether a stored Rule covers a later user correction:
// the dedup Relation of the two texts is duplicate, refines or contradicts.
// Contradictions count: a correction that contradicts a stored Rule restates
// it (the wrong-signal in the spec is a correction contradicting an injected
// Rule in the same Session).
func CoversRule(ruleText, correction string) bool {
	return Relation(ruleText, correction) != ""
}

// RecallAt5 is the share of cases covered by their top-5 hits. Zero cases
// give 0, not NaN, so an empty history prints cleanly.
func RecallAt5(covered, total int) float64 {
	if total <= 0 || covered < 0 {
		if total <= 0 {
			return 0
		}
		covered = 0
	}
	if covered > total {
		covered = total
	}
	return float64(covered) / float64(total)
}

// RepeatRate is the share of user corrections that restate a stored or
// capturable Rule. It shares the recall@5 math: covered over total.
func RepeatRate(covered, total int) float64 {
	return RecallAt5(covered, total)
}

// TopCovered reports whether any of the first top hits covers the query.
// Fewer hits than top still count; no hits never cover.
func TopCovered(hits []string, query string, top int) bool {
	if top <= 0 {
		top = 5
	}
	for i, h := range hits {
		if i >= top {
			break
		}
		if CoversRule(h, query) {
			return true
		}
	}
	return false
}

// HeuristicScore is the live heuristic durable score for eval: the
// ClassifyHeuristic confidence, or 0 when the text holds nothing durable.
func HeuristicScore(text string) float64 {
	if hit, ok := ClassifyHeuristic(text); ok {
		return hit.Confidence
	}
	return 0
}

// DeciderLabel is one hand-labelled user turn in the labelling file: one
// JSON object per line with id, text, durable and optional context and kind.
// Labels also come from the queue (rejected items are kept as labels): see
// QueueItemLabels, which maps queue rows into this shape.
type DeciderLabel struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Context string `json:"context,omitempty"`
	Durable bool   `json:"durable"`
	Kind    string `json:"kind,omitempty"`
}

// ParseDeciderLabels parses the labelling file: JSONL, one DeciderLabel per
// line, blank lines skipped. A missing id or text fails the whole file; an
// unknown kind fails the line. Lines carry hand labels; queue-derived labels
// arrive through QueueItemLabels in the same shape.
func ParseDeciderLabels(raw []byte) ([]DeciderLabel, error) {
	var out []DeciderLabel
	seen := map[string]bool{}
	scan := bufio.NewScanner(bytes.NewReader(raw))
	scan.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for scan.Scan() {
		line++
		text := strings.TrimSpace(scan.Text())
		if text == "" {
			continue
		}
		var l DeciderLabel
		if err := json.Unmarshal([]byte(text), &l); err != nil {
			return nil, fmt.Errorf("memory: labels line %d: %w", line, err)
		}
		l.ID = strings.TrimSpace(l.ID)
		if l.ID == "" {
			return nil, fmt.Errorf("memory: labels line %d: missing id", line)
		}
		if strings.TrimSpace(l.Text) == "" {
			return nil, fmt.Errorf("memory: labels line %d: missing text", line)
		}
		if l.Kind != "" && !validKind(l.Kind) {
			return nil, fmt.Errorf("memory: labels line %d: kind %q", line, l.Kind)
		}
		if seen[l.ID] {
			return nil, fmt.Errorf("memory: labels line %d: duplicate id %q", line, l.ID)
		}
		seen[l.ID] = true
		out = append(out, l)
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("memory: labels: %w", err)
	}
	return out, nil
}

// QueueItemLabels maps decided queue rows into DeciderLabel form: the
// candidate text comes from the payload's text, quote, statement or candidate
// field (in that order), and durable is true for approved rows, false for
// rejected ones. Pending rows are unjudged and skipped, so they never skew
// precision as false negatives; rows without extractable text are skipped,
// never fabricated. Rejected items are kept in the bank precisely so they
// serve as labels.
func QueueItemLabels(items []QueueItem) []DeciderLabel {
	var out []DeciderLabel
	for _, it := range items {
		if it.Status == QueuePending {
			continue
		}
		text := queueLabelText(it.Payload)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, DeciderLabel{
			ID:   fmt.Sprintf("queue-%d", it.ID),
			Text: text,
			// The queue payload may carry reason or context; keep the label
			// text-only so hand and queue labels score identically.
			Durable: it.Status == QueueApproved,
			Kind:    queueLabelKind(it.Payload),
		})
	}
	return out
}

// queueLabelText extracts the candidate text from a queue payload.
func queueLabelText(payload string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return ""
	}
	for _, k := range []string{"text", "quote", "statement", "candidate"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// queueLabelKind extracts the kind from a queue payload when valid.
func queueLabelKind(payload string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return ""
	}
	if v, ok := m["kind"]; ok {
		if s, ok := v.(string); ok && validKind(s) {
			return s
		}
	}
	return ""
}

// DeciderScores maps label ID to durable probability for one Decider. A
// missing entry means the Decider never answered that label (unavailable),
// never a no.
type DeciderScores map[string]float64

// ParseDeciderScores parses a scores file: JSON mapping decider name to a
// mapping of label ID to durable probability. Probabilities outside 0 to 1
// fail the file; unknown decider names are kept so jev and laya scores from
// a remote run load without an adapter.
func ParseDeciderScores(raw []byte) (map[string]DeciderScores, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]DeciderScores{}, nil
	}
	var m map[string]map[string]float64
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, fmt.Errorf("memory: scores: %w", err)
	}
	out := map[string]DeciderScores{}
	for decider, scores := range m {
		decider = strings.TrimSpace(decider)
		if decider == "" {
			return nil, fmt.Errorf("memory: scores: empty decider name")
		}
		ds := DeciderScores{}
		for id, p := range scores {
			if p < 0 || p > 1 {
				return nil, fmt.Errorf("memory: scores: decider %q label %q probability %v out of range", decider, id, p)
			}
			ds[id] = p
		}
		out[decider] = ds
	}
	return out, nil
}

// DeciderEval is one Decider's precision at the approve threshold.
type DeciderEval struct {
	Decider   string  `json:"decider"`
	Labels    int     `json:"labels"`
	Scored    int     `json:"scored"`
	Predicted int     `json:"predicted"`
	Actual    int     `json:"actual"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

// ScoreDecider scores one Decider at the approve threshold: a label counts
// as predicted when its durable score is at or above threshold. Labels the
// Decider never scored are unscored (unavailable), never negative.
func ScoreDecider(decider string, labels []DeciderLabel, scores DeciderScores, threshold float64) DeciderEval {
	ev := DeciderEval{Decider: decider, Labels: len(labels)}
	for _, l := range labels {
		if l.Durable {
			ev.Actual++
		}
		p, ok := scores[l.ID]
		if !ok {
			continue
		}
		ev.Scored++
		predicted := p >= threshold
		if predicted {
			ev.Predicted++
		}
		switch {
		case l.Durable && predicted:
			ev.TP++
		case !l.Durable && predicted:
			ev.FP++
		case !l.Durable && !predicted:
			ev.TN++
		default:
			ev.FN++
		}
	}
	if ev.TP+ev.FP > 0 {
		ev.Precision = float64(ev.TP) / float64(ev.TP+ev.FP)
	}
	if ev.TP+ev.FN > 0 {
		ev.Recall = float64(ev.TP) / float64(ev.TP+ev.FN)
	}
	return ev
}

// ScoreEvalDeciders scores every EvalDecider: heuristic from live
// ClassifyHeuristic confidence, the rest from the scores file. A Decider
// with no scores still reports its label counts with zero precision, so the
// report shows what ran and what was unavailable.
func ScoreEvalDeciders(labels []DeciderLabel, byDecider map[string]DeciderScores, threshold float64) []DeciderEval {
	if byDecider == nil {
		byDecider = map[string]DeciderScores{}
	}
	out := make([]DeciderEval, 0, len(EvalDeciders))
	for _, name := range EvalDeciders {
		scores := byDecider[name]
		if name == "heuristic" {
			live := DeciderScores{}
			for _, l := range labels {
				live[l.ID] = HeuristicScore(l.Text)
			}
			// Explicit scores still win when present (a recorded run), so a
			// checked-in scores file reproduces exactly.
			for id, p := range scores {
				live[id] = p
			}
			scores = live
		}
		if scores == nil {
			scores = DeciderScores{}
		}
		out = append(out, ScoreDecider(name, labels, scores, threshold))
	}
	return out
}

// EvalCase is one replay case for the embedder gap: the query correction and
// the two ranked hit lists (FTS5 and embedder) as Rule texts in rank order.
type EvalCase struct {
	Query        string
	FTSHits      []string
	EmbedderHits []string
}

// ScoreEvalCases reports FTS5 recall@5, embedder recall@5 and the gap
// (embedder minus FTS5) on the same cases, so the report decides whether
// embeddings are needed. top caps each list at 5 when positive.
func ScoreEvalCases(cases []EvalCase, top int) (fts, emb, gap float64) {
	if top <= 0 {
		top = 5
	}
	if len(cases) == 0 {
		return 0, 0, 0
	}
	fCovered, eCovered := 0, 0
	for _, c := range cases {
		if TopCovered(c.FTSHits, c.Query, top) {
			fCovered++
		}
		if TopCovered(c.EmbedderHits, c.Query, top) {
			eCovered++
		}
	}
	fts = RecallAt5(fCovered, len(cases))
	emb = RecallAt5(eCovered, len(cases))
	return fts, emb, emb - fts
}
