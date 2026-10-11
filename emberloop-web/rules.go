package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

// ruleProposalOf decodes a kind=rule queue payload in either the current or
// the older remember shape.
func ruleProposalOf(payload string) (memory.RuleProposal, error) {
	return memory.DecodeRememberProposal(payload)
}

// approveRuleQueueItem approves (or edits then approves) one rule queue item:
// it creates the Rule and marks the item approved. The queue payload is never
// rewritten, so an edit keeps both the proposal (in the queue row) and the
// final text (in the Rule). Same key, same author supersedes deterministically:
// live Rules with the same normalized key and the same author are marked
// superseded. A different author is a disagreement: both Rules stay live for
// a human to resolve. Text over MaxRuleChars is refused, never clipped.
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
	proposal, err := ruleProposalOf(item.Payload)
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
	author := proposal.Author
	if author == "" {
		author = decidedBy
	}
	// Provenance: the source observations stay on the Rule, the quotes stay
	// in the queue row. Redact the final text: approval is an egress from the
	// Daemon's store to future prompts.
	rule, err := bank.AddRule(ctx, memory.Rule{
		Text: memory.Redact(text), Kind: kind, SubjectKey: key,
		ObservedCommit: observedCommitFor(item.Payload),
		Status:         memory.StatusActive,
		SourceObservationIDs: proposal.SourceObservationIDs,
		ApprovedBy:           decidedBy,
	})
	if err != nil {
		return memory.Rule{}, err
	}
	// Deterministic supersession: same key, same author, newer wins. Only the
	// new Rule's author supersedes; a different author's Rules stay live as
	// the disagreement the approver resolves by hand.
	if key != "" {
		if existing, err := bank.RulesBySubjectKey(ctx, key); err == nil {
			for _, old := range memory.FindSuperseded(existing, key, author) {
				if old.RuleID == rule.RuleID {
					continue
				}
				_ = bank.SetRuleStatus(ctx, old.RuleID, memory.StatusSuperseded)
			}
		}
	}
	if err := bank.Decide(ctx, itemID, memory.QueueApproved, decidedBy); err != nil {
		return memory.Rule{}, err
	}
	return rule, nil
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
// creates the Rule (superseding the same key and author) and keeps the
// proposal in the queue row; a rejection keeps the item as a label and writes
// nothing.
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
		if err := bank.Decide(r.Context(), id, memory.QueueRejected, user.ID); err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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
		writeJSON(w, http.StatusOK, rule)
	default:
		http.Error(w, "status must be approved or rejected", http.StatusBadRequest)
	}
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
	for _, id := range ids {
		res := ruleBatchResult{ID: id, Status: body.Status}
		switch body.Status {
		case memory.QueueRejected:
			if err := bank.Decide(r.Context(), id, memory.QueueRejected, user.ID); err != nil {
				res.Error = err.Error()
			}
		case memory.QueueApproved:
			rule, err := d.approveRuleQueueItem(r.Context(), user.ID, user.ID, id, nil)
			if err != nil {
				res.Error = err.Error()
			} else {
				res.RuleID = rule.RuleID
			}
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, results)
}

// promoteSubject enqueues the recurrence proposal for subjectKey in the
// owner's bank when the key recurs across Sessions. A single mention never
// reaches the queue; the same subject key in two Sessions on different tasks
// does, unless situational language counts against it. A pending item for the
// same key is returned as-is instead of enqueued twice. Contradiction pairing
// runs before the enqueue: the FTS5 relation to a stored Rule with the same
// key or high overlap goes on the proposal for the approver.
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
		kind = KindPreferenceOf(group)
	}
	proposal, ok := memory.BuildRuleProposal(group, kind, "", "mined")
	if !ok {
		return 0, false, nil
	}
	// Contradiction pairing: the same subject key or high textual overlap
	// with a stored Rule goes on the proposal as its relation.
	if rules, err := bank.RulesBySubjectKey(ctx, key); err == nil && len(rules) > 0 {
		if related, rel := memory.PairCandidate(proposal.Text, key, rules); related != "" {
			proposal.Relation, proposal.RelatedRuleID = rel, related
		}
	} else if err == nil {
		if recalled, err := bank.Recall(ctx, proposal.Text, memory.MaxRecall); err == nil && len(recalled) > 0 {
			if related, rel := memory.PairCandidate(proposal.Text, key, recalled); related != "" {
				proposal.Relation, proposal.RelatedRuleID = rel, related
			}
		}
	}
	id, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: memory.EncodeRuleProposal(proposal)})
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// KindPreferenceOf returns the most common kind in group, defaulting to
// preference when the group is empty or tied.
func KindPreferenceOf(group []memory.Observation) string {
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
