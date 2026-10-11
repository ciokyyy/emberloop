// Queue approval, subject keys and supersession.
//
// A candidate reaches the queue only on /remember (the remember tool always
// enqueues) or when its subject key recurs in two or more Sessions on
// different tasks. Generalizing language counts for promotion, situational
// language against it. The approver sees one proposed generalization with its
// quotes and can approve, edit or reject; rejected items stay as Decider
// labels. Subject keys are proposed by the Decider (ProposeSubjectKey is the
// heuristic fallback) and normalized; same key, same author, newer wins
// deterministically, while a different author is a disagreement for a human.
package memory

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// QueueBudget caps the pending queue before the UI warns and the nightly
// summary records pressure. It is a budget, not a limit: nothing is dropped
// when it is exceeded, because rows are never deleted outside purge.
const QueueBudget = 50

// MaxSubjectKeyRunes caps a normalized subject key.
const MaxSubjectKeyRunes = 64

// NormalizeSubjectKey lowercases s, replaces every run of non-alphanumeric
// characters with one dot, trims leading and trailing dots and caps the
// length. "Go.Errors Wrapping!" becomes "go.errors.wrapping". Empty stays
// empty: an observation without a topic carries no key and never promotes.
func NormalizeSubjectKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDot := true // trim leading dots
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDot = false
			continue
		}
		if !prevDot {
			b.WriteRune('.')
			prevDot = true
		}
	}
	out := strings.Trim(b.String(), ".")
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", ".")
	}
	if utf8.RuneCountInString(out) > MaxSubjectKeyRunes {
		out = firstRunes(out, MaxSubjectKeyRunes)
		out = strings.Trim(out, ".")
	}
	return out
}

// validSubjectKey reports whether k is a normalized subject key or empty.
func validSubjectKey(k string) bool {
	if k == "" {
		return true
	}
	if utf8.RuneCountInString(k) > MaxSubjectKeyRunes {
		return false
	}
	for i, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	if strings.HasPrefix(k, ".") || strings.HasSuffix(k, ".") || strings.Contains(k, "..") {
		return false
	}
	return true
}

// ProposeSubjectKey is the heuristic subject-key proposal: the first three
// significant topic tokens of text, joined with dots. Polarity tokens
// (negations and always/never-type adverbs) are dropped so "always return
// wrapped errors" and "never return wrapped errors" share the key
// "return.wrapped.errors" and meet as a contradiction instead of missing
// each other. Filler stopwords are dropped the same way. Fewer than two
// topic tokens gives "", which never promotes: a single vague mention is
// not a subject.
func ProposeSubjectKey(text string) string {
	toks := tokenizeQuote(text)
	var topics []string
	for _, w := range toks {
		if negationTokens[w] {
			continue
		}
		// Signal words name the generality of the statement, not its
		// topic: they must not fork the key.
		switch w {
		case "here", "repo", "project", "function", "now":
			continue
		}
		clean := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, w)
		if clean == "" {
			continue
		}
		topics = append(topics, clean)
		if len(topics) == 3 {
			break
		}
	}
	if len(topics) < 2 {
		return ""
	}
	return NormalizeSubjectKey(strings.Join(topics, "."))
}

// Generalizing and situational phrases from the spec. Generalizing language
// counts for queue promotion, situational language against it.
var (
	generalizingPhrases = []string{"always", "in this repo", "from now on"}
	situationalPhrases  = []string{"here", "this function", "for now"}
)

// GeneralizationScore counts +1 per generalizing phrase present and -1 per
// situational phrase present, case-insensitive. Single words match whole
// words only, so "where" never counts as the situational "here". "Always use
// tabs in this repo, here for now" scores 0: the signals cancel.
func GeneralizationScore(text string) int {
	lower := strings.ToLower(text)
	wordSet := make(map[string]bool)
	for _, w := range words(lower) {
		wordSet[w] = true
	}
	score := 0
	for _, p := range generalizingPhrases {
		if strings.Contains(p, " ") {
			if strings.Contains(lower, p) {
				score++
			}
		} else if wordSet[p] {
			score++
		}
	}
	for _, p := range situationalPhrases {
		if strings.Contains(p, " ") {
			if strings.Contains(lower, p) {
				score--
			}
		} else if wordSet[p] {
			score--
		}
	}
	return score
}

// IsGeneralizing reports whether text carries generalizing language.
func IsGeneralizing(text string) bool { return GeneralizationScore(text) > 0 }

// IsSituational reports whether text carries situational language.
func IsSituational(text string) bool { return GeneralizationScore(text) < 0 }

// RuleProposal is the JSON payload of a kind=rule queue item: one proposed
// generalization with its quotes and provenance. Origin is "remember" for
// remember-tool proposals and "mined" for recurrence promotions. Relation and
// RelatedRuleID carry contradiction pairing: the FTS5 relation to the stored
// Rule the candidate overlaps, so the approver sees the conflict without
// recomputing it.
type RuleProposal struct {
	Text                 string   `json:"text"`
	Kind                 string   `json:"kind,omitempty"`
	SubjectKey           string   `json:"subject_key,omitempty"`
	Quotes               []string `json:"quotes,omitempty"`
	Sessions             []string `json:"sessions,omitempty"`
	Author               string   `json:"author,omitempty"`
	Origin               string   `json:"origin,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	SourceObservationIDs []int64  `json:"source_observation_ids,omitempty"`
	Relation             string   `json:"relation,omitempty"`
	RelatedRuleID        string   `json:"related_rule_id,omitempty"`
}

// EncodeRuleProposal marshals p with a normalized subject key.
func EncodeRuleProposal(p RuleProposal) string {
	p.SubjectKey = NormalizeSubjectKey(p.SubjectKey)
	raw, _ := json.Marshal(p)
	return string(raw)
}

// rememberCompat is the pre-ticket-22 remember payload shape, plus the
// subject_key and kind ticket 22 adds to remember proposals. Approve still
// reads both: Quote becomes the single quote, SessionID the single session.
type rememberCompat struct {
	Text       string `json:"text"`
	Reason     string `json:"reason"`
	Quote      string `json:"quote"`
	EventID    string `json:"eventId"`
	SessionID  string `json:"sessionId"`
	Author     string `json:"author"`
	Origin     string `json:"origin"`
	SubjectKey string `json:"subject_key"`
	Kind       string `json:"kind"`
}

// DecodeRememberProposal converts either the current RuleProposal shape or
// the older remember shape into a RuleProposal. Unknown JSON fails. Old
// payloads without quotes, sessions, kind or subject key are upgraded: the
// quote and session become the single-element lists, the kind defaults and
// the key is proposed from the text.
func DecodeRememberProposal(payload string) (RuleProposal, error) {
	var p RuleProposal
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return RuleProposal{}, fmt.Errorf("%w: rule payload is not JSON", ErrInvalid)
	}
	if strings.TrimSpace(p.Text) == "" {
		return RuleProposal{}, fmt.Errorf("%w: rule proposal text empty", ErrInvalid)
	}
	var old rememberCompat
	if err := json.Unmarshal([]byte(payload), &old); err == nil {
		if len(p.Quotes) == 0 && old.Quote != "" {
			p.Quotes = []string{old.Quote}
		}
		if len(p.Sessions) == 0 && old.SessionID != "" {
			p.Sessions = []string{old.SessionID}
		}
		if p.SubjectKey == "" && old.SubjectKey != "" {
			p.SubjectKey = old.SubjectKey
		}
		if p.Kind == "" && old.Kind != "" {
			p.Kind = old.Kind
		}
		if p.Reason == "" {
			p.Reason = old.Reason
		}
		if p.Author == "" {
			p.Author = old.Author
		}
		if p.Origin == "" {
			p.Origin = old.Origin
		}
	}
	if p.SubjectKey == "" {
		p.SubjectKey = ProposeSubjectKey(p.Text)
	}
	p.SubjectKey = NormalizeSubjectKey(p.SubjectKey)
	return p, nil
}

// GroupObservationsBySubject groups observations by normalized subject key.
// Empty keys are skipped: keyless observations never promote.
func GroupObservationsBySubject(obs []Observation) map[string][]Observation {
	out := make(map[string][]Observation)
	for _, o := range obs {
		key := NormalizeSubjectKey(o.SubjectKey)
		if key == "" {
			continue
		}
		out[key] = append(out[key], o)
	}
	return out
}

// distinctSessions counts distinct SessionIDs in group.
func distinctSessions(group []Observation) int {
	seen := make(map[string]bool)
	for _, o := range group {
		seen[o.SessionID] = true
	}
	return len(seen)
}

// RecursAcrossSessions reports whether group holds the same normalized
// subject key in two or more distinct Sessions. Sessions stand in for tasks:
// each Session is one unit of work, so repeats inside one Session never
// promote, while the same subject in two Sessions counts as two tasks. One
// mention, however emphatic, never promotes alone.
func RecursAcrossSessions(group []Observation) bool {
	if len(group) < 2 {
		return false
	}
	return distinctSessions(group) >= 2
}

// ShouldPromote reports whether group belongs in the queue: the subject key
// recurs across Sessions and the quotes are not situational on balance.
// Generalizing language counts for promotion, situational language against
// it; a neutral group (no signals) promotes.
func ShouldPromote(group []Observation) bool {
	if !RecursAcrossSessions(group) {
		return false
	}
	score := 0
	for _, o := range group {
		score += GeneralizationScore(o.Quote)
	}
	return score >= 0
}

// ProposeGeneralization returns the one proposed generalization the approver
// sees, with its supporting quotes. The pick is the quote with the highest
// generalization score, breaking ties toward the longest quote, so an
// "always ..." statement wins over a bare "here ..." one. The text is never
// clipped: a proposal over MaxRuleChars stays whole and the approval refuses
// it until a human edits it down. The full quotes stay beside it as evidence.
// Empty group gives an error.
func ProposeGeneralization(group []Observation) (string, []string, error) {
	if len(group) == 0 {
		return "", nil, fmt.Errorf("%w: no observations", ErrInvalid)
	}
	best := group[0]
	bestScore := GeneralizationScore(best.Quote)
	for _, o := range group[1:] {
		s := GeneralizationScore(o.Quote)
		if s > bestScore || (s == bestScore && utf8.RuneCountInString(o.Quote) > utf8.RuneCountInString(best.Quote)) {
			best, bestScore = o, s
		}
	}
	text := strings.TrimSpace(best.Quote)
	seen := make(map[string]bool)
	var quotes []string
	for _, o := range group {
		q := strings.TrimSpace(o.Quote)
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		quotes = append(quotes, q)
	}
	return text, quotes, nil
}

// BuildRuleProposal builds the queue proposal for group when it should
// promote, or reports false. The proposal carries one generalization, every
// quote, every session, the author of the newest observation and the source
// IDs, so the approver sees the evidence and the approval records
// provenance. Empty subject key or a non-recurring group never builds.
func BuildRuleProposal(group []Observation, kind, author, origin string) (RuleProposal, bool) {
	if len(group) == 0 {
		return RuleProposal{}, false
	}
	key := NormalizeSubjectKey(group[0].SubjectKey)
	if key == "" {
		return RuleProposal{}, false
	}
	for _, o := range group[1:] {
		if NormalizeSubjectKey(o.SubjectKey) != key {
			return RuleProposal{}, false
		}
	}
	if !ShouldPromote(group) {
		return RuleProposal{}, false
	}
	text, quotes, err := ProposeGeneralization(group)
	if err != nil {
		return RuleProposal{}, false
	}
	if !validKind(kind) {
		kind = KindPreference
	}
	seenSession := make(map[string]bool)
	var sessions []string
	var sources []int64
	newest := group[0]
	for _, o := range group {
		if !seenSession[o.SessionID] {
			seenSession[o.SessionID] = true
			sessions = append(sessions, o.SessionID)
		}
		if o.ID != 0 {
			sources = append(sources, o.ID)
		}
		if o.CreatedAt.After(newest.CreatedAt) {
			newest = o
		}
	}
	if author == "" {
		author = newest.Author
	}
	return RuleProposal{
		Text: text, Kind: kind, SubjectKey: key,
		Quotes: quotes, Sessions: sessions, Author: author, Origin: origin,
		SourceObservationIDs: sources,
	}, true
}

// latestLiveRules keeps the latest version per rule ID among live rows,
// ordered by rule ID so pairing and partition read deterministically.
func latestLiveRules(rules []Rule) []Rule {
	latest := make(map[string]Rule)
	for _, r := range rules {
		if cur, ok := latest[r.RuleID]; !ok || r.Version > cur.Version {
			latest[r.RuleID] = r
		}
	}
	var out []Rule
	for _, r := range latest {
		if live(r.Status) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out
}

// PartitionSubjectRules splits the live latest versions carrying subjectKey
// into same-author and different-author sets, both ordered by rule ID.
// Authors are observer identities resolved by the caller: ruleAuthor reports
// the User behind a Rule's observations (its source observations' single
// distinct author, else the approver fallback the caller applied), and
// candidateAuthor is the candidate's resolved observer. "" on either side
// means unknown or mixed and never matches, so nothing supersedes without a
// known author on both sides. A newer statement from a different User lands
// in different and stays in the queue for a human; it never supersedes.
func PartitionSubjectRules(rules []Rule, subjectKey, candidateAuthor string, ruleAuthor func(Rule) string) (same, different []Rule) {
	key := NormalizeSubjectKey(subjectKey)
	if key == "" {
		return nil, nil
	}
	for _, r := range latestLiveRules(rules) {
		if NormalizeSubjectKey(r.SubjectKey) != key {
			continue
		}
		if candidateAuthor != "" {
			if author := ruleAuthor(r); author != "" && author == candidateAuthor {
				same = append(same, r)
				continue
			}
		}
		different = append(different, r)
	}
	return same, different
}

// PairCandidate pairs a candidate against stored Rules for the queue payload:
// the lowest-ID live Rule with the same subject key, else the lowest-ID Rule
// with a Relation (duplicate, refines, contradicts). It returns the related
// Rule ID and the relation, or empty when nothing relates. Callers store both
// on the proposal so the approver sees the conflict.
func PairCandidate(candidateText, subjectKey string, rules []Rule) (string, string) {
	key := NormalizeSubjectKey(subjectKey)
	liveRules := latestLiveRules(rules)
	for _, r := range liveRules {
		if key != "" && NormalizeSubjectKey(r.SubjectKey) == key {
			if rel := Relation(candidateText, r.Text); rel != "" {
				return r.RuleID, rel
			}
			return r.RuleID, RelRefines
		}
	}
	for _, r := range liveRules {
		if rel := Relation(candidateText, r.Text); rel != "" {
			return r.RuleID, rel
		}
	}
	return "", ""
}

// QueueStats describes the decision queue: its budget, its size and how long
// approvals take. Latencies are decided_at minus created_at over decided
// items; pending age is now minus the oldest pending item.
type QueueStats struct {
	Pending             int           `json:"pending"`
	Approved            int           `json:"approved"`
	Rejected            int           `json:"rejected"`
	Total               int           `json:"total"`
	Budget              int           `json:"budget"`
	OverBudget          bool          `json:"overBudget"`
	OldestPendingAge    time.Duration `json:"oldestPendingAgeNanos"`
	AvgApprovalLatency  time.Duration `json:"avgApprovalLatencyNanos"`
	OldestPendingAgeHrs float64       `json:"oldestPendingAgeHours"`
	AvgLatencyHrs       float64       `json:"avgApprovalLatencyHours"`
}

// DescribeQueue folds items into stats at now. Rejected items stay in the
// counts: they are the Decider labels.
func DescribeQueue(items []QueueItem, now time.Time) QueueStats {
	st := QueueStats{Budget: QueueBudget}
	var oldest time.Time
	var latencySum time.Duration
	var decided int
	for _, it := range items {
		st.Total++
		switch it.Status {
		case QueuePending:
			st.Pending++
			if oldest.IsZero() || it.CreatedAt.Before(oldest) {
				oldest = it.CreatedAt
			}
		case QueueApproved:
			st.Approved++
		case QueueRejected:
			st.Rejected++
		}
		if (it.Status == QueueApproved || it.Status == QueueRejected) && !it.DecidedAt.IsZero() && !it.CreatedAt.IsZero() {
			latencySum += it.DecidedAt.Sub(it.CreatedAt)
			decided++
		}
	}
	st.OverBudget = st.Pending > st.Budget
	if !oldest.IsZero() && !now.IsZero() {
		st.OldestPendingAge = now.Sub(oldest)
		st.OldestPendingAgeHrs = st.OldestPendingAge.Hours()
	}
	if decided > 0 {
		st.AvgApprovalLatency = latencySum / time.Duration(decided)
		st.AvgLatencyHrs = st.AvgApprovalLatency.Hours()
	}
	return st
}

// QueueItemsForSession returns the queue items whose payload names sessionID
// in a sessions list or a sessionId field (both proposal shapes). Unparsable
// payloads never match.
func QueueItemsForSession(items []QueueItem, sessionID string) []QueueItem {
	var out []QueueItem
	for _, it := range items {
		if queueItemSession(it.Payload, sessionID) {
			out = append(out, it)
		}
	}
	return out
}

// queueItemSession reports whether payload names sessionID.
func queueItemSession(payload, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	var doc struct {
		Sessions  []string `json:"sessions"`
		SessionID string   `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return false
	}
	if doc.SessionID == sessionID {
		return true
	}
	for _, s := range doc.Sessions {
		if s == sessionID {
			return true
		}
	}
	return false
}

// QueueItemSubjectKey returns the normalized subject key of a rule payload,
// or "" when the payload carries none or does not parse.
func QueueItemSubjectKey(payload string) string {
	var doc struct {
		SubjectKey string `json:"subject_key"`
	}
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return ""
	}
	return NormalizeSubjectKey(doc.SubjectKey)
}

// PendingForSubject returns the pending rule items with the same normalized
// subject key, so promotion enqueues a subject once instead of once per mine.
func PendingForSubject(items []QueueItem, subjectKey string) []QueueItem {
	key := NormalizeSubjectKey(subjectKey)
	if key == "" {
		return nil
	}
	var out []QueueItem
	for _, it := range items {
		if it.Kind != QueueRule || it.Status != QueuePending {
			continue
		}
		if QueueItemSubjectKey(it.Payload) == key {
			out = append(out, it)
		}
	}
	return out
}
