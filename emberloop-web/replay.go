package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/emberloop-web/memory"
)

// NewReplayBank opens a throwaway FTS5 bank for the replay command: a real
// SQLite bank in a temp dir, so the numbers reflect FTS5 ranking, not the
// Fake's term-count order. The caller closes it with cleanup.
func NewReplayBank() (memory.Memory, func(), error) {
	dir, err := os.MkdirTemp("", "el-replay-")
	if err != nil {
		return nil, nil, err
	}
	eng, err := memory.Open(dir, memory.Options{})
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}
	bank, err := eng.Bank("replay-eval")
	if err != nil {
		_ = eng.Close()
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}
	return bank, func() {
		_ = eng.Close()
		_ = os.RemoveAll(dir)
	}, nil
}

// Offline replay and Decider eval for ticket 30.
//
// Replay builds memory from Sessions before Session N and checks whether the
// Rules that would be injected cover the corrections the User made in N
// (recall@5). It also reports the repeat-correction rate: the share of user
// corrections that restate a stored or capturable Rule. Both read the same
// Session histories the miner reads (FilterMinerEvents plus the heuristic),
// so canned logs give deterministic numbers.
//
// The embedder comparison runs on the same cases: for each query the FTS5
// top-20 from the bank is re-ranked by caller-supplied embedder scores and
// both top-5 lists are checked for coverage. The gap (embedder minus FTS5)
// decides whether embeddings are needed.
//
// The labelling file is JSONL (see memory.ParseDeciderLabels): hand labels
// plus queue-derived labels (memory.QueueItemLabels, read from a User bank
// with QueueLabelsForUser and merged by the eval command) score each Decider
// at the approve threshold through memory.ScoreEvalDeciders. SampleLabelCandidates
// builds the hand-labelling set: heuristic hits first, then misses to fill n.
//
// The Run A/B hook is a Run setting: every Nth ticket in ticket order starts
// with recall off (SessionMeta.NoRecall). Ticket order keeps the arm stable
// across retries, pauses and resumes. The setting and the per-ticket marks
// persist in the Run file; reviewers compare verdicts across the pair.

// ReplaySession is one Session's history for the replay, in the same shape
// the miner reads: full Events with sender typing intact.
type ReplaySession struct {
	ID        string
	CreatedAt time.Time
	ProjectID string
	Events    []Event
}

// ReplayCase is one correction in Session N with the hits the replay found.
type ReplayCase struct {
	SessionID string
	EventID   string
	Query     string
	FTSHits   []string
	Covered   bool
}

// ReplayResult aggregates Rules-only and recall-with-Memories arms.
type ReplayResult struct {
	Cases           int     `json:"cases"`
	Covered         int     `json:"covered"`
	RecallAt5       float64 `json:"recallAt5"`
	MemoryCovered   int     `json:"memoryCovered"`
	MemoryRecallAt5 float64 `json:"memoryRecallAt5"`
	RepeatTotal     int     `json:"repeatTotal"`
	RepeatCovered   int     `json:"repeatCovered"`
	RepeatRate      float64 `json:"repeatRate"`
}

// replayCorrection is one heuristic hit with its Event identity.
type replayCorrection struct {
	sessionID string
	eventID   string
	text      string
}

// replayCorrectionsOf returns the miner-readable corrections of one Session:
// FilterMinerEvents prompts judged by the heuristic, in log order.
func replayCorrectionsOf(s ReplaySession) []replayCorrection {
	var out []replayCorrection
	for i, e := range s.Events {
		if !MinerReadable(e) {
			continue
		}
		if _, ok := memory.ClassifyHeuristic(e.Text); !ok {
			continue
		}
		id := eventKey(e.Seq, i)
		out = append(out, replayCorrection{sessionID: s.ID, eventID: id, text: e.Text})
	}
	return out
}

// eventKey names one Event for replay Rules and embedder scores: the
// sequence number when present, else the line index.
func eventKey(seq int64, i int) string {
	if seq != 0 {
		return fmt.Sprintf("%d", seq)
	}
	return fmt.Sprintf("line-%d", i)
}

// truncateRuleText caps a replay Rule at the 300-char bank limit on a rune
// boundary, so a long correction still stores.
func truncateRuleText(s string) string {
	if utf8.RuneCountInString(s) <= memory.MaxRuleChars {
		return s
	}
	n := 0
	for pos := range s {
		if n == memory.MaxRuleChars {
			return s[:pos]
		}
		n++
	}
	return s
}

// ReplayEvaluate runs the offline replay over sessions in time order with
// one bank built incrementally: for Session N the bank holds Rules from
// Sessions before N (each prior correction as one Rule), then each
// correction in N recalls the top 20, cuts to 5 and checks coverage with
// memory.CoversRule. The repeat rate checks each correction against every
// earlier correction (prior Sessions plus earlier in the same Session), the
// capturable-Rule denominator. newBank supplies the bank (the Fake in tests,
// a real FTS5 bank in the CLI).
func ReplayEvaluate(sessions []ReplaySession, newBank func() memory.Memory) (ReplayResult, []ReplayCase, error) {
	ordered := append([]ReplaySession(nil), sessions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	if newBank == nil {
		return ReplayResult{}, nil, fmt.Errorf("replay: no bank")
	}
	bank := newBank()
	memoryBank := newBank()
	ctx := context.Background()
	var (
		res       ReplayResult
		cases     []ReplayCase
		priorSeen []string // every earlier correction text, for the repeat rate
	)
	for _, s := range ordered {
		corrections := replayCorrectionsOf(s)
		for _, c := range corrections {
			res.Cases++
			res.RepeatTotal++
			// Repeat: restates any earlier correction.
			for _, p := range priorSeen {
				if memory.CoversRule(p, c.text) {
					res.RepeatCovered++
					break
				}
			}
			// Recall@5: FTS top-20 cut to 5 covers the query.
			var hits []string
			if rules, err := bank.Recall(ctx, memory.Redact(c.text), memory.MaxRecall); err == nil {
				for _, r := range rules {
					hits = append(hits, r.Text)
				}
			} else {
				return res, cases, fmt.Errorf("replay: recall: %w", err)
			}
			top := hits
			if len(top) > 5 {
				top = top[:5]
			}
			covered := memory.TopCovered(top, c.text, 5)
			if covered {
				res.Covered++
			}
			memoryHits, err := memoryBank.RecallMemoryItems(ctx, memory.Redact(c.text), memory.MaxRecall, false)
			if err != nil {
				return res, cases, fmt.Errorf("replay: memory recall: %w", err)
			}
			memoryTexts := make([]string, 0, len(memoryHits))
			for _, hit := range memoryHits {
				memoryTexts = append(memoryTexts, hit.Text)
			}
			if memory.TopCovered(memoryTexts, c.text, 5) {
				res.MemoryCovered++
			}
			cases = append(cases, ReplayCase{
				SessionID: c.sessionID, EventID: c.eventID, Query: c.text,
				FTSHits: hits, Covered: covered,
			})
		}
		// Session N becomes memory for N+1: each correction is one Rule.
		for _, c := range corrections {
			text := truncateRuleText(strings.TrimSpace(c.text))
			if text == "" {
				priorSeen = append(priorSeen, c.text)
				continue
			}
			_, _ = bank.AddRule(ctx, memory.Rule{
				RuleID: fmt.Sprintf("replay-%s-%s", c.sessionID, c.eventID),
				Text:   text, Kind: memory.KindPreference,
				Status: memory.StatusActive, ApprovedBy: "replay",
			})
			_, _ = memoryBank.AddMemory(ctx, memory.MemoryItem{
				SessionID: c.sessionID, EventID: c.eventID, Author: "replay",
				Kind: memory.KindPreference, Text: text, Quote: c.text,
				Importance: .7, Veracity: memory.VeracityStated, Status: memory.StatusActive,
			})
			priorSeen = append(priorSeen, c.text)
		}
	}
	res.RecallAt5 = memory.RecallAt5(res.Covered, res.Cases)
	res.MemoryRecallAt5 = memory.RecallAt5(res.MemoryCovered, res.Cases)
	res.RepeatRate = memory.RepeatRate(res.RepeatCovered, res.RepeatTotal)
	return res, cases, nil
}

// RankWithScores re-ranks candidates by embedder scores (higher first):
// rules with a score beat rules without one, ties keep bank order. It never
// drops a candidate; the caller cuts to 5 after.
func RankWithScores(candidates []memory.Rule, scores map[string]float64) []memory.Rule {
	out := append([]memory.Rule(nil), candidates...)
	sort.SliceStable(out, func(i, j int) bool {
		pi, iok := scores[out[i].Text]
		pj, jok := scores[out[j].Text]
		if iok != jok {
			return iok
		}
		if !iok {
			return false
		}
		return pi > pj
	})
	return out
}

// ReplayCompare runs the embedder comparison on the same cases as
// ReplayEvaluate: for each query the FTS5 top-20 from the incremental bank
// is re-ranked by the embedder scores for that Event key
// ("sessionID:eventID"), and both top-5 lists are checked. It returns the
// FTS5 recall@5, the embedder recall@5 and the gap (embedder minus FTS5).
// A missing key falls back to FTS order for that case, so a partial scores
// file still prints.
func ReplayCompare(sessions []ReplaySession, newBank func() memory.Memory, embedder map[string]map[string]float64) (fts, emb, gap float64, res ReplayResult, err error) {
	ordered := append([]ReplaySession(nil), sessions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	if newBank == nil {
		return 0, 0, 0, ReplayResult{}, fmt.Errorf("replay: no bank")
	}
	bank := newBank()
	ctx := context.Background()
	var cases []memory.EvalCase
	var priorSeen []string
	for _, s := range ordered {
		corrections := replayCorrectionsOf(s)
		for _, c := range corrections {
			res.Cases++
			res.RepeatTotal++
			for _, p := range priorSeen {
				if memory.CoversRule(p, c.text) {
					res.RepeatCovered++
					break
				}
			}
			rules, rerr := bank.Recall(ctx, memory.Redact(c.text), memory.MaxRecall)
			if rerr != nil {
				return 0, 0, 0, res, fmt.Errorf("replay: recall: %w", rerr)
			}
			var ftsHits []string
			for _, r := range rules {
				ftsHits = append(ftsHits, r.Text)
			}
			top := ftsHits
			if len(top) > 5 {
				top = top[:5]
			}
			if memory.TopCovered(top, c.text, 5) {
				res.Covered++
			}
			key := c.sessionID + ":" + c.eventID
			ranked := RankWithScores(rules, embedder[key])
			var embHits []string
			for _, r := range ranked {
				embHits = append(embHits, r.Text)
			}
			cases = append(cases, memory.EvalCase{Query: c.text, FTSHits: ftsHits, EmbedderHits: embHits})
		}
		for _, c := range corrections {
			text := truncateRuleText(strings.TrimSpace(c.text))
			if text == "" {
				priorSeen = append(priorSeen, c.text)
				continue
			}
			_, _ = bank.AddRule(ctx, memory.Rule{
				RuleID: fmt.Sprintf("replay-%s-%s", c.sessionID, c.eventID),
				Text:   text, Kind: memory.KindPreference,
				Status: memory.StatusActive, ApprovedBy: "replay",
			})
			priorSeen = append(priorSeen, c.text)
		}
	}
	res.RecallAt5 = memory.RecallAt5(res.Covered, res.Cases)
	res.RepeatRate = memory.RepeatRate(res.RepeatCovered, res.RepeatTotal)
	fts, emb, gap = memory.ScoreEvalCases(cases, 5)
	return fts, emb, gap, res, nil
}

// LoadReplaySessions reads every Session history under stateDir for the
// replay command: meta.json (time, project) plus events.jsonl. A non-empty
// project keeps only that Project's Sessions. Corrupt lines are skipped;
// Sessions with no Events still order the replay but add no cases.
// since and until bound the period (the ticket's repeat rate over a period):
// sessions outside [since, until) are skipped; zero times are unbounded.
func LoadReplaySessions(stateDir, project string, since, until time.Time) ([]ReplaySession, error) {
	root := filepath.Join(stateDir, "enderbloom", "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ReplaySession
	for _, en := range entries {
		if !en.IsDir() {
			continue
		}
		id := en.Name()
		metaRaw, err := os.ReadFile(filepath.Join(root, id, "meta.json"))
		if err != nil {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			continue
		}
		if project != "" && meta.ProjectID != project {
			continue
		}
		if !since.IsZero() && meta.CreatedAt.Before(since) {
			continue
		}
		if !until.IsZero() && !meta.CreatedAt.Before(until) {
			continue
		}
		events := loadReplayEvents(filepath.Join(root, id, "events.jsonl"), id)
		out = append(out, ReplaySession{
			ID: id, CreatedAt: meta.CreatedAt, ProjectID: meta.ProjectID, Events: events,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// loadReplayEvents reads one events.jsonl file, skipping torn or corrupt
// lines like the miner does.
func loadReplayEvents(path, id string) []Event {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []Event
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.SessionID == "" {
			e.SessionID = id
		}
		out = append(out, e)
	}
	return out
}

// LoadEmbedderScores reads the embedder scores file: JSON mapping Event key
// ("sessionID:eventID") to a mapping of Rule text to similarity score. rule
// texts are matched verbatim against the FTS hits of that case.
func LoadEmbedderScores(path string) (map[string]map[string]float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]map[string]float64
	if err := json.Unmarshal(bytes.TrimSpace(raw), &m); err != nil {
		return nil, fmt.Errorf("replay: embedder scores: %w", err)
	}
	if m == nil {
		m = map[string]map[string]float64{}
	}
	return m, nil
}

// FormatReplayReport renders the replay command output: recall@5, the
// repeat-correction rate, and, when embedder scores were given, the embedder
// recall@5 and the gap on the same cases.
func FormatReplayReport(res ReplayResult, sessions int, fts, emb, gap float64, embedderOn bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "sessions: %d\n", sessions)
	fmt.Fprintf(&b, "cases: %d\n", res.Cases)
	fmt.Fprintf(&b, "recall@5: %.3f (%d/%d) [Rules]\n", res.RecallAt5, res.Covered, res.Cases)
	fmt.Fprintf(&b, "recall@5 (Memories): %.3f (%d/%d)\n", res.MemoryRecallAt5, res.MemoryCovered, res.Cases)
	fmt.Fprintf(&b, "repeat-correction rate: %.3f (%d/%d)\n", res.RepeatRate, res.RepeatCovered, res.RepeatTotal)
	if embedderOn {
		fmt.Fprintf(&b, "fts recall@5: %.3f\n", fts)
		fmt.Fprintf(&b, "embedder recall@5: %.3f\n", emb)
		fmt.Fprintf(&b, "gap (embedder-fts): %+.3f\n", gap)
	}
	return b.String()
}

// FormatDeciderReport renders the decider eval output: the label mix (hand
// plus queue) and one line per Decider with scored counts and precision at
// the approve threshold. A Decider that scored nothing prints precision n/a
// (unscored) rather than a misleading zero; likewise precision with no
// positive predictions and recall with no positive labels read n/a.
func FormatDeciderReport(evals []memory.DeciderEval, threshold float64, handLabels, queueLabels int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "threshold: %.2f\n", threshold)
	fmt.Fprintf(&b, "labels: %d (hand %d, queue %d)\n", handLabels+queueLabels, handLabels, queueLabels)
	for _, e := range evals {
		fmt.Fprintf(&b, "%s: precision %s (%d/%d predicted) recall %s scored %d/%d labels\n",
			e.Decider, evalScore(e.Precision, e.Predicted), e.TP, e.Predicted, evalScore(e.Recall, e.Actual), e.Scored, e.Labels)
	}
	return b.String()
}

// evalScore renders one eval ratio: n/a when its denominator is zero, so an
// unevaluated Decider never reads as a measured zero.
func evalScore(v float64, denom int) string {
	if denom <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", v)
}

// AssignRecallOff reports whether the ticket at 0-based index in the Run's
// ticket order runs with recall off: every everyNth ticket (3rd, 6th, ...)
// when everyN is positive, else none. Ticket order (not start order) keeps
// the arm stable across retries, pauses and resumes; the Run records everyN
// and each assigned ticket, so reviewer verdicts give the paired comparison.
func AssignRecallOff(startIndex, everyN int) bool {
	if everyN <= 0 {
		return false
	}
	return (startIndex+1)%everyN == 0
}

// RecallOffForTicket reports whether ticketID runs with recall off under
// everyN: the ticket's index in the Run's ticket order decides, so the arm
// is stable whatever order Workers start in. Unknown IDs never match.
func RecallOffForTicket(tickets []staticTicket, ticketID string, everyN int) bool {
	for i, t := range tickets {
		if t.ID == ticketID {
			return AssignRecallOff(i, everyN)
		}
	}
	return false
}

// QueueLabelsForUser reads userID's decision queue from the banks under
// stateDir and maps the decided rows into DeciderLabel form for the eval
// command: labels come from the queue as well as from hand labels. It reads
// only; it never decides. A missing bank is an error naming the user.
func QueueLabelsForUser(stateDir, userID string) ([]memory.DeciderLabel, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("replay: queue user is required")
	}
	root := filepath.Join(stateDir, "enderbloom", "memory")
	if _, err := os.Stat(filepath.Join(root, "users", userID+".db")); err != nil {
		return nil, fmt.Errorf("replay: no memory bank for user %q", userID)
	}
	eng, err := memory.Open(root, memory.Options{})
	if err != nil {
		return nil, err
	}
	defer eng.Close()
	bank, err := eng.Bank(userID)
	if err != nil {
		return nil, err
	}
	items, err := bank.Queue(context.Background())
	if err != nil {
		return nil, err
	}
	return memory.QueueItemLabels(items), nil
}

// SampledTurn is one labelling candidate for the hand-labelled set: the
// user turn with the live heuristic hint. The human sets durable; the hint
// only oversamples, never labels.
type SampledTurn struct {
	ID        string
	Text      string
	Context   string
	Heuristic float64
}

// SampleLabelCandidates builds the hand-labelling set from sessions in time
// order: every miner-readable user prompt with its preceding tool call or
// diff hunk as context and its live heuristic score. Hits oversample: all
// heuristic hits come first in time order, then misses fill up to n, so a
// 200-to-300 turn set holds the durable candidates instead of chatter.
// Deterministic: no sampling seed, time order throughout. n <= 0 returns
// every candidate; the labelling file stays on the host for hand labelling.
func SampleLabelCandidates(sessions []ReplaySession, n int) []SampledTurn {
	ordered := append([]ReplaySession(nil), sessions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	var hits, misses []SampledTurn
	for _, s := range ordered {
		var lastCtx string
		for i, e := range s.Events {
			switch {
			case e.Kind == "tool_call_started" || e.Kind == "tool_call_updated" || e.Kind == "diff":
				lastCtx = e.Text
			case MinerReadable(e):
				t := SampledTurn{
					ID: s.ID + ":" + eventKey(e.Seq, i), Text: e.Text,
					Context:   lastCtx,
					Heuristic: memory.HeuristicScore(e.Text),
				}
				if _, ok := memory.ClassifyHeuristic(e.Text); ok {
					hits = append(hits, t)
				} else {
					misses = append(misses, t)
				}
				lastCtx = ""
			}
		}
	}
	out := hits
	if n > 0 && len(out) > n {
		return out[:n]
	}
	if n <= 0 {
		return append(out, misses...)
	}
	if need := n - len(out); need > 0 {
		if need > len(misses) {
			need = len(misses)
		}
		out = append(out, misses[:need]...)
	}
	return out
}
