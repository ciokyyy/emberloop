package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/emberloop-web/memory"
)

// Rule queue: pending Rule candidates with one proposed generalization and
// its quotes. A candidate reaches the queue only on /remember (the remember
// tool always enqueues) or when its subject key recurs in two or more
// Sessions. The approver sees the proposal and can approve, edit or reject;
// rejected items stay as Decider labels. Same key, same author, newer wins
// deterministically (the approval supersedes the old Rule); a different
// author is a disagreement for a human (both Rules stay live).

// queueLedgerRunID is the ledger RunID for rule-queue mutations
// (recordQueueLedger). The Memory page status treats it as non-nightly when
// it picks the last nightly run, so the two must stay in step.
const queueLedgerRunID = "queue"

// ruleDecideBody is POST /api/rules/queue/{id}/decide: approve or reject,
// with an optional edit. Text, Kind and SubjectKey replace the proposal when
// present; absent fields keep the proposal. The proposal payload is never
// rewritten, so an edit keeps both the proposal and the final Rule text.
type ruleDecideBody struct {
	Status     string `json:"status"`
	Text       string `json:"text,omitempty"`
	Kind       string `json:"kind,omitempty"`
	SubjectKey string `json:"subject_key,omitempty"`
}

// ruleBatchBody is POST /api/rules/queue/batch: approve or reject many items
// at once. Either IDs or SessionID selects the items; SessionID approves the
// whole per-Session batch. IDs win when both are set.
type ruleBatchBody struct {
	IDs       []int64 `json:"ids,omitempty"`
	SessionID string  `json:"sessionId,omitempty"`
	Status    string  `json:"status"`
}

// ruleBatchResult is one item of a batch decision.
type ruleBatchResult struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	RuleID string `json:"ruleId,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ruleUserOf resolves the signed-in User behind a rule queue call. The
// machine token is no User: it answers 401 like /api/users.
func (d *Daemon) ruleUserOf(w http.ResponseWriter, r *http.Request) (User, bool) {
	who, ok := d.requesterOf(r)
	if !ok || who.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return User{}, false
	}
	return *who.user, true
}

// approveRuleQueueItem approves (or edits then approves) one rule queue item
// in one bank transaction (memory.ApproveRule): it creates the Rule and marks
// the item approved together, so a failed approval leaves no Rule, no
// supersession and a still-pending item. The queue payload is never rewritten,
// so an edit keeps both the proposal (in the queue row) and the final text
// (in the Rule). Supersession is deterministic on observer identity and date:
// live Rules with the same normalized key from the same observer (the User
// behind the source observations) become superseded only when the candidate's
// newest observation is at least as new as theirs; a different observer is a
// disagreement and both Rules stay live for a human to resolve. Text over
// MaxRuleChars is refused, never clipped.
func (d *Daemon) approveRuleQueueItem(ctx context.Context, owner, decidedBy string, itemID int64, edit *ruleDecideBody) (memory.Rule, error) {
	bank, err := d.profileBankFor(owner)
	if err != nil {
		return memory.Rule{}, err
	}
	items, err := bank.Queue(ctx)
	if err != nil {
		return memory.Rule{}, err
	}
	var item *memory.QueueItem
	for i := range items {
		if items[i].ID == itemID {
			item = &items[i]
			break
		}
	}
	if item == nil {
		return memory.Rule{}, fmt.Errorf("%w: queue item %d", memory.ErrNotFound, itemID)
	}
	if item.Kind != memory.QueueRule {
		return memory.Rule{}, fmt.Errorf("%w: queue item %d is %q, not a rule", memory.ErrInvalid, itemID, item.Kind)
	}
	if item.Status != memory.QueuePending {
		return memory.Rule{}, fmt.Errorf("%w: queue item %d already decided", memory.ErrInvalid, itemID)
	}
	proposal, err := memory.DecodeRememberProposal(item.Payload)
	if err != nil {
		return memory.Rule{}, fmt.Errorf("%w: rule payload: %v", memory.ErrInvalid, err)
	}
	text := strings.TrimSpace(proposal.Text)
	kind := proposal.Kind
	key := memory.NormalizeSubjectKey(proposal.SubjectKey)
	if edit != nil {
		if strings.TrimSpace(edit.Text) != "" {
			text = strings.TrimSpace(edit.Text)
		}
		if strings.TrimSpace(edit.Kind) != "" {
			kind = strings.TrimSpace(edit.Kind)
		}
		if strings.TrimSpace(edit.SubjectKey) != "" {
			key = memory.NormalizeSubjectKey(edit.SubjectKey)
		} else if key == "" && text != "" {
			// An old proposal without a key gets one from the final text,
			// so the Rule carries the subject it was approved under.
			key = memory.NormalizeSubjectKey(memory.ProposeSubjectKey(text))
		}
	} else if key == "" && text != "" {
		key = memory.NormalizeSubjectKey(memory.ProposeSubjectKey(text))
	}
	if text == "" {
		return memory.Rule{}, fmt.Errorf("%w: rule text empty", memory.ErrInvalid)
	}
	if utf8.RuneCountInString(text) > memory.MaxRuleChars {
		return memory.Rule{}, fmt.Errorf("%w: rule text %d chars, max %d", memory.ErrInvalid, utf8.RuneCountInString(text), memory.MaxRuleChars)
	}
	if !validMineKind(kind) {
		kind = memory.KindPreference
		if proposal.Kind != "" && validMineKind(proposal.Kind) {
			kind = proposal.Kind
		}
	}
	candidateAuthor := d.candidateObserverAuthor(ctx, bank, proposal, decidedBy)
	candidateTime := d.candidateObservedTime(ctx, bank, proposal, item.CreatedAt)
	var supersede []string
	stillborn := false
	if key != "" {
		existing, err := bank.RulesBySubjectKey(ctx, key)
		if err != nil {
			return memory.Rule{}, err
		}
		authors := d.ruleObserverAuthors(ctx, bank, existing)
		same, _ := memory.PartitionSubjectRules(existing, key, candidateAuthor, func(r memory.Rule) string {
			return authors[r.RuleID]
		})
		for _, old := range same {
			// Newer observation wins: only a candidate at least as new as
			// the stored Rule supersedes it. When any same-observer Rule is
			// newer than the candidate, the new Rule lands stillborn
			// superseded while the older ones still fall, so exactly the
			// newest text stays live.
			if !candidateTime.Before(d.ruleObservedTime(ctx, bank, old)) {
				supersede = append(supersede, old.RuleID)
			} else {
				stillborn = true
			}
		}
	}
	status := memory.StatusActive
	if stillborn {
		status = memory.StatusSuperseded
	}
	// Provenance: the source observations stay on the Rule, the quotes stay
	// in the queue row. Redact the final text: approval is an egress from the
	// Daemon's store to future prompts.
	rule, err := bank.ApproveRule(ctx, itemID, decidedBy, memory.Rule{
		Text: memory.Redact(text), Kind: kind, SubjectKey: key,
		ObservedCommit:       observedCommitFor(item.Payload),
		Status:               status,
		SourceObservationIDs: proposal.SourceObservationIDs,
		ApprovedBy:           decidedBy,
	}, supersede)
	if err != nil {
		return memory.Rule{}, err
	}
	return rule, nil
}

// candidateObserverAuthor resolves the observer behind a proposal: the single
// distinct author of its source observations, else the proposal author, else
// the decider. Mixed or unknown authors resolve to "": nothing supersedes
// without one known author.
func (d *Daemon) candidateObserverAuthor(ctx context.Context, bank memory.Memory, proposal memory.RuleProposal, decidedBy string) string {
	if authors := observationAuthors(ctx, bank, proposal.SourceObservationIDs); len(authors) == 1 {
		return authors[0]
	} else if len(authors) > 1 {
		return ""
	}
	if strings.TrimSpace(proposal.Author) != "" {
		return proposal.Author
	}
	return decidedBy
}

// candidateObservedTime reports the newest source-observation time of a
// proposal, falling back to the queue item's creation when it has no
// resolvable sources.
func (d *Daemon) candidateObservedTime(ctx context.Context, bank memory.Memory, proposal memory.RuleProposal, fallback time.Time) time.Time {
	if newest := newestObservationTime(ctx, bank, proposal.SourceObservationIDs); !newest.IsZero() {
		return newest
	}
	return fallback
}

// ruleObserverAuthors resolves the observer behind each Rule: the single
// distinct author of its source observations, else its approver. Mixed
// authors resolve to "": such a Rule never supersedes and always disagrees.
func (d *Daemon) ruleObserverAuthors(ctx context.Context, bank memory.Memory, rules []memory.Rule) map[string]string {
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		out[r.RuleID] = d.ruleObserverAuthor(ctx, bank, r)
	}
	return out
}

// ruleObserverAuthor resolves the observer behind one Rule.
func (d *Daemon) ruleObserverAuthor(ctx context.Context, bank memory.Memory, r memory.Rule) string {
	if authors := observationAuthors(ctx, bank, r.SourceObservationIDs); len(authors) == 1 {
		return authors[0]
	} else if len(authors) > 1 {
		return ""
	}
	return r.ApprovedBy
}

// ruleObservedTime reports the newest source-observation time of a Rule,
// falling back to its approval when it has no resolvable sources.
func (d *Daemon) ruleObservedTime(ctx context.Context, bank memory.Memory, r memory.Rule) time.Time {
	if newest := newestObservationTime(ctx, bank, r.SourceObservationIDs); !newest.IsZero() {
		return newest
	}
	return r.ApprovedAt
}

// observationAuthors resolves the distinct authors of the named source
// observations in first-seen order, skipping missing or authorless rows, so
// one helper serves every observer and date lookup with one query per source.
func observationAuthors(ctx context.Context, bank memory.Memory, ids []int64) []string {
	seen := make(map[string]bool)
	var authors []string
	for _, id := range ids {
		o, err := bank.ObservationByID(ctx, id)
		if err != nil || strings.TrimSpace(o.Author) == "" {
			continue
		}
		if !seen[o.Author] {
			seen[o.Author] = true
			authors = append(authors, o.Author)
		}
	}
	return authors
}

// newestObservationTime reports the newest creation time among the named
// source observations, or zero when none resolve.
func newestObservationTime(ctx context.Context, bank memory.Memory, ids []int64) time.Time {
	var newest time.Time
	for _, id := range ids {
		o, err := bank.ObservationByID(ctx, id)
		if err != nil || o.CreatedAt.IsZero() {
			continue
		}
		if o.CreatedAt.After(newest) {
			newest = o.CreatedAt
		}
	}
	return newest
}

// observedCommitFor returns the observed commit recorded in a rule payload,
// or "" when the payload carries none (older remember items).
func observedCommitFor(payload string) string {
	var doc struct {
		ObservedCommit string `json:"observed_commit"`
	}
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return ""
	}
	return doc.ObservedCommit
}

// handleRulesQueue serves GET /api/rules/queue: the User's rule proposals
// with their quotes and source Sessions, oldest first.
func (d *Daemon) handleRulesQueue(w http.ResponseWriter, r *http.Request) {
	user, ok := d.ruleUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bank, err := d.profileBankFor(user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := bank.Queue(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := []memory.QueueItem{}
	for _, it := range items {
		if it.Kind == memory.QueueRule {
			out = append(out, it)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRulesQueueStats serves GET /api/rules/queue/stats: the queue budget,
// its size and the approval latency, so the Memory page and the nightly
// summary read them from one place. Rejected items stay in the counts: they
// are the Decider labels.
func (d *Daemon) handleRulesQueueStats(w http.ResponseWriter, r *http.Request) {
	user, ok := d.ruleUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bank, err := d.profileBankFor(user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := bank.Queue(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, memory.DescribeQueue(items, d.now()))
}

// handleRulesQueueDecide serves POST /api/rules/queue/{id}/decide
// {"status":"approved"|"rejected","text"?,"kind"?,"subject_key"?}. An approval
// creates the Rule (superseding the same observer's older Rule) and keeps the
// proposal in the queue row; a rejection keeps the item as a label and writes
// nothing. Only kind=rule items decide here: a profile_patch ID answers 400.
func (d *Daemon) handleRulesQueueDecide(w http.ResponseWriter, r *http.Request, id int64) {
	user, ok := d.ruleUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body ruleDecideBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	bank, err := d.profileBankFor(user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch body.Status {
	case memory.QueueRejected:
		if err := d.rejectRuleQueueItem(r.Context(), bank, id, user.ID); err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		d.recordQueueLedger(r.Context(), user.ID, "queue_reject")
		writeJSON(w, http.StatusOK, map[string]any{"status": memory.QueueRejected})
	case memory.QueueApproved:
		rule, err := d.approveRuleQueueItem(r.Context(), user.ID, user.ID, id, &body)
		if err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		d.recordQueueLedger(r.Context(), user.ID, "queue_approve")
		writeJSON(w, http.StatusOK, rule)
	default:
		http.Error(w, "status must be approved or rejected", http.StatusBadRequest)
	}
}

// rejectRuleQueueItem rejects one pending kind=rule queue item. The row stays
// as a Decider label. Non-rule IDs are refused so the rule queue can never
// decide a profile_patch item.
func (d *Daemon) rejectRuleQueueItem(ctx context.Context, bank memory.Memory, id int64, decidedBy string) error {
	items, err := bank.Queue(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.ID != id {
			continue
		}
		if it.Kind != memory.QueueRule {
			return fmt.Errorf("%w: queue item %d is %q, not a rule", memory.ErrInvalid, id, it.Kind)
		}
		return bank.Decide(ctx, id, memory.QueueRejected, decidedBy)
	}
	return fmt.Errorf("%w: queue item %d", memory.ErrNotFound, id)
}

// handleRulesQueueBatch serves POST /api/rules/queue/batch
// {"ids"|"sessionId","status"}: one decision for many items. A per-Session
// batch approves every pending rule proposal of that Session, so the Memory
// page clears a Session's candidates together. Each item is decided on its
// own: one failure never stops the rest.
func (d *Daemon) handleRulesQueueBatch(w http.ResponseWriter, r *http.Request) {
	user, ok := d.ruleUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body ruleBatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if body.Status != memory.QueueApproved && body.Status != memory.QueueRejected {
		http.Error(w, "status must be approved or rejected", http.StatusBadRequest)
		return
	}
	bank, err := d.profileBankFor(user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := bank.Queue(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var ids []int64
	if len(body.IDs) > 0 {
		ids = body.IDs
	} else if body.SessionID != "" {
		for _, it := range memory.QueueItemsForSession(items, body.SessionID) {
			if it.Kind == memory.QueueRule && it.Status == memory.QueuePending {
				ids = append(ids, it.ID)
			}
		}
	} else {
		http.Error(w, "ids or sessionId is required", http.StatusBadRequest)
		return
	}
	results := make([]ruleBatchResult, 0, len(ids))
	byID := make(map[int64]memory.QueueItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	succeeded := false
	for _, id := range ids {
		res := ruleBatchResult{ID: id, Status: body.Status}
		if it, ok := byID[id]; ok && it.Kind != memory.QueueRule {
			res.Error = fmt.Errorf("%w: queue item %d is %q, not a rule", memory.ErrInvalid, id, it.Kind).Error()
			results = append(results, res)
			continue
		}
		switch body.Status {
		case memory.QueueRejected:
			if err := bank.Decide(r.Context(), id, memory.QueueRejected, user.ID); err != nil {
				res.Error = err.Error()
			} else {
				succeeded = true
			}
		case memory.QueueApproved:
			rule, err := d.approveRuleQueueItem(r.Context(), user.ID, user.ID, id, nil)
			if err != nil {
				res.Error = err.Error()
			} else {
				res.RuleID = rule.RuleID
				succeeded = true
			}
		}
		results = append(results, res)
	}
	// The batch records its counts only when it decided something: an empty
	// selection or an all-failed batch writes no ledger row.
	if succeeded {
		d.recordQueueLedger(r.Context(), user.ID, "queue_batch")
	}
	writeJSON(w, http.StatusOK, results)
}

// promoteSubject enqueues the recurrence proposal for subjectKey in the
// owner's bank when the key recurs across Sessions. A single mention never
// reaches the queue; the same subject key in two Sessions on different tasks
// does, unless situational language counts against it. A pending item for the
// same key is returned as-is instead of enqueued twice. Pairing runs before
// the enqueue: a live Rule with the same key from a different observer marks
// the proposal as a disagreement (relation plus related rule) for the human;
// otherwise the FTS5 relation to an overlapping stored Rule goes on it.
func (d *Daemon) promoteSubject(ctx context.Context, owner, subjectKey, kind string) (int64, bool, error) {
	key := memory.NormalizeSubjectKey(subjectKey)
	if key == "" {
		return 0, false, fmt.Errorf("%w: subject key empty", memory.ErrInvalid)
	}
	bank, err := d.profileBankFor(owner)
	if err != nil {
		return 0, false, err
	}
	group, err := bank.ObservationsBySubjectKey(ctx, key)
	if err != nil {
		return 0, false, err
	}
	if !memory.ShouldPromote(group) {
		return 0, false, nil
	}
	if pending := memory.PendingForSubject(mustQueue(ctx, bank), key); len(pending) > 0 {
		return pending[0].ID, false, nil
	}
	if !validMineKind(kind) {
		kind = kindPreferenceOf(group)
	}
	proposal, ok := memory.BuildRuleProposal(group, kind, "", "mined")
	if !ok {
		return 0, false, nil
	}
	// Pairing is best effort: a pairing failure never blocks the promotion,
	// it only leaves the proposal without a relation.
	sameKeyRules, err := bank.RulesBySubjectKey(ctx, key)
	if err != nil {
		sameKeyRules = nil
	}
	if len(sameKeyRules) > 0 {
		candidateAuthor := d.candidateObserverAuthor(ctx, bank, proposal, owner)
		authors := d.ruleObserverAuthors(ctx, bank, sameKeyRules)
		_, different := memory.PartitionSubjectRules(sameKeyRules, key, candidateAuthor, func(r memory.Rule) string {
			return authors[r.RuleID]
		})
		if len(different) > 0 {
			// A newer statement from a different observer is a
			// disagreement: it stays in the queue with the conflicting
			// Rule named, instead of superseding it.
			if related, rel := memory.PairCandidate(proposal.Text, key, different); related != "" {
				proposal.Relation, proposal.RelatedRuleID = rel, related
			} else {
				proposal.Relation, proposal.RelatedRuleID = memory.RelContradicts, different[0].RuleID
			}
		} else if related, rel := memory.PairCandidate(proposal.Text, key, sameKeyRules); related != "" {
			proposal.Relation, proposal.RelatedRuleID = rel, related
		}
	} else if recalled, err := bank.Recall(ctx, proposal.Text, memory.MaxRecall); err == nil && len(recalled) > 0 {
		if related, rel := memory.PairCandidate(proposal.Text, key, recalled); related != "" {
			proposal.Relation, proposal.RelatedRuleID = rel, related
		}
	}
	id, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: memory.EncodeRuleProposal(proposal)})
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// kindPreferenceOf returns the most common valid kind in group. Ties keep the
// earliest leader; an empty group or one with no valid kind defaults to
// preference.
func kindPreferenceOf(group []memory.Observation) string {
	counts := make(map[string]int)
	best, bestN := memory.KindPreference, 0
	for _, o := range group {
		if !validMineKind(o.Kind) {
			continue
		}
		counts[o.Kind]++
		if counts[o.Kind] > bestN {
			best, bestN = o.Kind, counts[o.Kind]
		}
	}
	return best
}

// recordQueueLedger records the queue budget, size and approval latency after
// a rule-queue mutation, so the budget pressure and the latency are stored,
// not just served. It is best effort: a ledger failure never fails the
// decision it follows.
func (d *Daemon) recordQueueLedger(ctx context.Context, owner, stage string) {
	bank, err := d.profileBankFor(owner)
	if err != nil {
		return
	}
	items, err := bank.Queue(ctx)
	if err != nil {
		return
	}
	st := memory.DescribeQueue(items, d.now())
	counts, _ := json.Marshal(map[string]any{
		"pending": st.Pending, "approved": st.Approved, "rejected": st.Rejected,
		"total": st.Total, "budget": st.Budget, "overBudget": st.OverBudget,
		"avgApprovalLatencyHours": st.AvgLatencyHrs, "oldestPendingAgeHours": st.OldestPendingAgeHrs,
	})
	_, _ = bank.RecordLedger(ctx, memory.LedgerRow{
		RunID: queueLedgerRunID, Stage: stage, Status: memory.LedgerSuccess,
		CountsJSON: string(counts),
		Detail:     "rule queue " + stage,
	})
}

// mustQueue lists the bank's queue, swallowing the error for the pending
// check: a listing failure simply skips the dedup and enqueues.
func mustQueue(ctx context.Context, bank memory.Memory) []memory.QueueItem {
	items, err := bank.Queue(ctx)
	if err != nil {
		return nil
	}
	return items
}

// handleRulesQueuePromote serves POST /api/rules/queue/promote
// {"subject_key","kind"?}: run the recurrence check for one subject and
// enqueue its proposal when it recurs. A single mention answers promoted
// false with no item; an already-queued subject answers its pending ID.
func (d *Daemon) handleRulesQueuePromote(w http.ResponseWriter, r *http.Request) {
	user, ok := d.ruleUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SubjectKey string `json:"subject_key"`
		Kind       string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	id, created, err := d.promoteSubject(r.Context(), user.ID, body.SubjectKey, body.Kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !created && id == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"promoted": false})
		return
	}
	if created {
		d.recordQueueLedger(r.Context(), user.ID, "queue_promote")
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "promoted": created})
}

// handleRulesQueueSub routes the rule queue paths: the promote and batch
// actions, the stats view, and POST /api/rules/queue/{id}/decide.
func (d *Daemon) handleRulesQueueSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/rules/queue/")
	switch {
	case rest == "promote":
		d.handleRulesQueuePromote(w, r)
		return
	case rest == "batch":
		d.handleRulesQueueBatch(w, r)
		return
	case rest == "stats":
		d.handleRulesQueueStats(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[1] != "decide" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad queue id", http.StatusBadRequest)
		return
	}
	d.handleRulesQueueDecide(w, r, id)
}
