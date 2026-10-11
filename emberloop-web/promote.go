package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop-web/memory"
)

// Promotion queue: a Rule proposed as project knowledge. Proposing builds a
// kind=promotion queue item with a diff preview against the nearest AGENTS.md
// (or a new note file); approval writes the file in the Session's worktree
// and leaves it uncommitted, then marks the Rule promoted. The Daemon never
// commits; the Reviewer loop reviews the edit as a normal change.

// promotionCreateBody is POST /api/promotions and POST
// /api/sessions/{id}/promote: which Rule to promote, in which Session's
// worktree, and to which target. Target defaults to agents_md.
type promotionCreateBody struct {
	RuleID    string `json:"rule_id"`
	SessionID string `json:"session_id,omitempty"`
	Target    string `json:"target,omitempty"`
}

// promotionDecideBody is POST /api/promotions/queue/{id}/decide: approve
// writes the file and marks the Rule promoted; reject keeps the item as a
// label and writes nothing. SessionID overrides the worktree the proposal was
// built from; Text overrides the line the approval writes.
type promotionDecideBody struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

// promotionView is the creation and listing view: the queue item plus the
// resolved target and diff preview for approval.
type promotionView struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Target     string `json:"target"`
	TargetPath string `json:"target_path"`
	Diff       string `json:"diff"`
}

// promotionUserOf resolves the signed-in User behind a promotion call. The
// machine token is no User: agents propose Rules through remember, but only a
// human promotes one to project knowledge.
func (d *Daemon) promotionUserOf(w http.ResponseWriter, r *http.Request) (User, bool) {
	who, ok := d.requesterOf(r)
	if !ok || who.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return User{}, false
	}
	return *who.user, true
}

// sessionWorkDir returns the WorkDir of a visible Session, or ErrNotFound.
func (d *Daemon) sessionWorkDir(sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("%w: session required", memory.ErrInvalid)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok || s.meta.Owner == "" {
		return "", fmt.Errorf("%w: session %s", memory.ErrNotFound, sessionID)
	}
	if strings.TrimSpace(s.meta.WorkDir) == "" {
		return "", fmt.Errorf("%w: session %s has no work dir", memory.ErrInvalid, sessionID)
	}
	return s.meta.WorkDir, nil
}

// latestRuleVersion returns the latest version of ruleID in the bank.
func latestRuleVersion(ctx context.Context, bank memory.Memory, ruleID string) (memory.Rule, error) {
	versions, err := bank.RuleVersions(ctx, ruleID)
	if err != nil {
		return memory.Rule{}, err
	}
	if len(versions) == 0 {
		return memory.Rule{}, fmt.Errorf("%w: rule %s", memory.ErrNotFound, ruleID)
	}
	latest := versions[0]
	for _, v := range versions[1:] {
		if v.Version > latest.Version {
			latest = v
		}
	}
	return latest, nil
}

// sessionWorkDirForRule resolves the worktree for a promotion without an
// explicit Session: the work dir of the newest source-observation Session
// that is still known (live or persisted) with a work dir. Rules carry their
// provenance (source observation IDs), so the Memory page can propose from
// its rules list without picking a Session. When no source Session resolves,
// it reports ErrNotFound naming session_id, and the caller passes one.
func (d *Daemon) sessionWorkDirForRule(ctx context.Context, bank memory.Memory, rule memory.Rule) (string, string, error) {
	type candidate struct {
		id string
		at time.Time
	}
	var cands []candidate
	for _, oid := range rule.SourceObservationIDs {
		o, err := bank.ObservationByID(ctx, oid)
		if err != nil {
			continue
		}
		if strings.TrimSpace(o.SessionID) == "" {
			continue
		}
		cands = append(cands, candidate{id: o.SessionID, at: o.CreatedAt})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].at.After(cands[j].at) })
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range cands {
		if s, ok := d.sessions[c.id]; ok && s.meta.Owner != "" && strings.TrimSpace(s.meta.WorkDir) != "" {
			return c.id, s.meta.WorkDir, nil
		}
	}
	return "", "", fmt.Errorf("%w: no live session for rule %s; pass session_id", memory.ErrNotFound, rule.RuleID)
}

// buildPromotionProposal resolves the Rule and the target paths for a
// promotion, returning the proposal (with its diff preview) for the queue.
// An empty sessionID resolves through the Rule's provenance
// (sessionWorkDirForRule); otherwise the named Session's work dir applies.
func (d *Daemon) buildPromotionProposal(ctx context.Context, bank memory.Memory, ruleID, sessionID, target, author string) (memory.PromotionProposal, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return memory.PromotionProposal{}, fmt.Errorf("%w: rule_id is required", memory.ErrInvalid)
	}
	if !memory.ValidPromotionTarget(target) {
		if strings.TrimSpace(target) == "" {
			target = memory.PromotionAgentsMD
		} else {
			return memory.PromotionProposal{}, fmt.Errorf("%w: promotion target %q", memory.ErrInvalid, target)
		}
	}
	rule, err := latestRuleVersion(ctx, bank, ruleID)
	if err != nil {
		return memory.PromotionProposal{}, err
	}
	var workDir string
	if strings.TrimSpace(sessionID) == "" {
		sessionID, workDir, err = d.sessionWorkDirForRule(ctx, bank, rule)
		if err != nil {
			return memory.PromotionProposal{}, err
		}
	} else {
		workDir, err = d.sessionWorkDir(sessionID)
		if err != nil {
			return memory.PromotionProposal{}, err
		}
	}
	text := strings.TrimSpace(rule.Text)
	if text == "" {
		return memory.PromotionProposal{}, fmt.Errorf("%w: rule text empty", memory.ErrInvalid)
	}
	p := memory.PromotionProposal{
		RuleID: rule.RuleID, RuleVersion: rule.Version,
		Text: memory.Redact(text), Kind: rule.Kind, SubjectKey: rule.SubjectKey,
		Anchors:        append([]string(nil), rule.Anchors...),
		ObservedCommit: rule.ObservedCommit, Author: rule.ApprovedBy,
		SessionID: sessionID, Target: target,
		SourceObservationIDs: append([]int64(nil), rule.SourceObservationIDs...),
		ApprovedBy:           rule.ApprovedBy,
	}
	switch target {
	case memory.PromotionNote:
		abs, rel := memory.NotesTargetPath(workDir, rule.RuleID)
		content := memory.FormatNoteFile(p)
		p.TargetPath = rel
		p.ProposedText = content
		p.Diff = memory.PromotionNotePreview(rel, content)
		_ = abs
	default:
		abs, rel, err := memory.NearestAgentsMD(workDir)
		if err != nil {
			return memory.PromotionProposal{}, err
		}
		_, diff, err := memory.AgentsMDDiff(abs, rel, p.Text)
		if err != nil {
			return memory.PromotionProposal{}, err
		}
		p.TargetPath = rel
		p.ProposedText = "- " + p.Text
		p.Diff = diff
	}
	return p, nil
}

// promotionViewOf renders a stored promotion item for the API.
func promotionViewOf(it memory.QueueItem) promotionView {
	p, err := memory.DecodePromotionProposal(it.Payload)
	if err != nil {
		return promotionView{ID: it.ID, Status: it.Status}
	}
	return promotionView{ID: it.ID, Status: it.Status, Target: p.Target, TargetPath: p.TargetPath, Diff: p.Diff}
}

// handlePromotions serves POST /api/promotions: propose a Rule as project
// knowledge. The body names the Rule and optionally the Session worktree to
// propose against; without a session the worktree resolves from the Rule's
// provenance. The answer carries the queue item with its diff preview for
// approval. A pending promotion for the same Rule answers 200 with the
// existing item instead of enqueuing twice.
func (d *Daemon) handlePromotions(w http.ResponseWriter, r *http.Request) {
	user, ok := d.promotionUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body promotionCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	item, created, err := d.proposePromotionResult(r.Context(), user.ID, body.RuleID, body.SessionID, body.Target)
	if err != nil {
		switch {
		case errors.Is(err, memory.ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	view := promotionViewOf(item)
	if created {
		writeJSON(w, http.StatusCreated, view)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// proposePromotionResult enqueues and reports whether the item is new.
func (d *Daemon) proposePromotionResult(ctx context.Context, owner, ruleID, sessionID, target string) (memory.QueueItem, bool, error) {
	bank, err := d.profileBankFor(owner)
	if err != nil {
		return memory.QueueItem{}, false, err
	}
	if target == "" {
		target = memory.PromotionAgentsMD
	}
	if items, err := bank.Queue(ctx); err == nil {
		for _, it := range items {
			if it.Kind != memory.QueuePromotion || it.Status != memory.QueuePending {
				continue
			}
			if p, err := memory.DecodePromotionProposal(it.Payload); err == nil {
				if p.RuleID == strings.TrimSpace(ruleID) {
					return it, false, nil
				}
			}
		}
	}
	proposal, err := d.buildPromotionProposal(ctx, bank, ruleID, sessionID, target, owner)
	if err != nil {
		return memory.QueueItem{}, false, err
	}
	id, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueuePromotion, Payload: memory.EncodePromotionProposal(proposal)})
	if err != nil {
		return memory.QueueItem{}, false, err
	}
	items, err := bank.Queue(ctx)
	if err != nil {
		return memory.QueueItem{}, false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it, true, nil
		}
	}
	return memory.QueueItem{}, false, fmt.Errorf("%w: promotion %d", memory.ErrNotFound, id)
}

// handlePromotionsQueue serves GET /api/promotions/queue: the User's pending
// and decided promotion proposals with their diff previews.
func (d *Daemon) handlePromotionsQueue(w http.ResponseWriter, r *http.Request) {
	user, ok := d.promotionUserOf(w, r)
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
	out := []promotionView{}
	for _, it := range items {
		if it.Kind == memory.QueuePromotion {
			out = append(out, promotionViewOf(it))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// approvePromotionQueueItem approves one pending promotion: it writes the
// target file in the Session's worktree (never committing), marks the Rule
// promoted with its provenance intact, and decides the queue item approved.
// The file edit is idempotent: an AGENTS.md that already holds the line and a
// note file that already holds the same content both succeed without
// duplicating. A file failure leaves the queue pending so approval retries.
func (d *Daemon) approvePromotionQueueItem(ctx context.Context, owner, decidedBy string, itemID int64, body *promotionDecideBody) (map[string]any, error) {
	bank, err := d.profileBankFor(owner)
	if err != nil {
		return nil, err
	}
	items, err := bank.Queue(ctx)
	if err != nil {
		return nil, err
	}
	var item *memory.QueueItem
	for i := range items {
		if items[i].ID == itemID {
			item = &items[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("%w: queue item %d", memory.ErrNotFound, itemID)
	}
	if item.Kind != memory.QueuePromotion {
		return nil, fmt.Errorf("%w: queue item %d is %q, not a promotion", memory.ErrInvalid, itemID, item.Kind)
	}
	if item.Status != memory.QueuePending {
		return nil, fmt.Errorf("%w: queue item %d already decided", memory.ErrInvalid, itemID)
	}
	proposal, err := memory.DecodePromotionProposal(item.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: promotion payload: %v", memory.ErrInvalid, err)
	}
	sessionID := proposal.SessionID
	if body != nil && strings.TrimSpace(body.SessionID) != "" {
		sessionID = strings.TrimSpace(body.SessionID)
	}
	workDir, err := d.sessionWorkDir(sessionID)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(proposal.Text)
	if body != nil && strings.TrimSpace(body.Text) != "" {
		text = strings.TrimSpace(body.Text)
	}
	if text == "" {
		return nil, fmt.Errorf("%w: promotion text empty", memory.ErrInvalid)
	}
	text = memory.Redact(text)
	rule, err := latestRuleVersion(ctx, bank, proposal.RuleID)
	if err != nil {
		return nil, err
	}
	target := proposal.Target
	if target == "" {
		target = memory.PromotionAgentsMD
	}
	var absPath, relPath string
	switch target {
	case memory.PromotionNote:
		absPath, relPath = memory.NotesTargetPath(workDir, rule.RuleID)
		content := memory.FormatNoteFile(memory.PromotionProposal{
			RuleID: rule.RuleID, Text: text, Kind: rule.Kind, SubjectKey: rule.SubjectKey,
			Anchors: rule.Anchors, ObservedCommit: rule.ObservedCommit, Author: rule.ApprovedBy,
		})
		if err := memory.VerifyPromotionTarget(workDir, absPath); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, err
		}
		if raw, err := os.ReadFile(absPath); err == nil {
			if strings.TrimSpace(string(raw)) == strings.TrimSpace(content) {
				break
			}
			return nil, fmt.Errorf("%w: note %s already exists", memory.ErrInvalid, relPath)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			return nil, err
		}
	default:
		var resolveErr error
		absPath, relPath, resolveErr = memory.NearestAgentsMD(workDir)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if err := memory.VerifyPromotionTarget(workDir, absPath); err != nil {
			return nil, err
		}
		newContent, _, err := memory.AgentsMDDiff(absPath, relPath, text)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(absPath, []byte(newContent), 0o644); err != nil {
			return nil, err
		}
	}
	if err := bank.MarkRulePromoted(ctx, rule.RuleID, decidedBy); err != nil {
		return nil, err
	}
	if err := bank.Decide(ctx, itemID, memory.QueueApproved, decidedBy); err != nil {
		return nil, err
	}
	d.recordQueueLedger(ctx, owner, "promote_approve")
	return map[string]any{
		"status": memory.QueueApproved, "rule_id": rule.RuleID,
		"target": target, "target_path": relPath, "abs_path": absPath,
	}, nil
}

// rejectPromotionQueueItem rejects one pending kind=promotion queue item. The
// row stays as a Decider label and nothing is written. A missing ID is
// ErrNotFound; a non-promotion ID is ErrInvalid, so the rule and profile
// queues can never decide a promotion by accident.
func (d *Daemon) rejectPromotionQueueItem(ctx context.Context, bank memory.Memory, id int64, decidedBy string) error {
	items, err := bank.Queue(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.ID != id {
			continue
		}
		if it.Kind != memory.QueuePromotion {
			return fmt.Errorf("%w: queue item %d is %q, not a promotion", memory.ErrInvalid, id, it.Kind)
		}
		return bank.Decide(ctx, id, memory.QueueRejected, decidedBy)
	}
	return fmt.Errorf("%w: queue item %d", memory.ErrNotFound, id)
}

// handlePromotionsQueueDecide serves POST /api/promotions/queue/{id}/decide
// {"status":"approved"|"rejected",...}. Approval writes the worktree file and
// marks the Rule promoted; rejection keeps the item as a label and writes
// nothing.
func (d *Daemon) handlePromotionsQueueDecide(w http.ResponseWriter, r *http.Request, id int64) {
	user, ok := d.promotionUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body promotionDecideBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	switch body.Status {
	case memory.QueueRejected:
		bank, err := d.profileBankFor(user.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := d.rejectPromotionQueueItem(r.Context(), bank, id, user.ID); err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		d.recordQueueLedger(r.Context(), user.ID, "promote_reject")
		writeJSON(w, http.StatusOK, map[string]any{"status": memory.QueueRejected})
	case memory.QueueApproved:
		out, err := d.approvePromotionQueueItem(r.Context(), user.ID, user.ID, id, &body)
		if err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		http.Error(w, "status must be approved or rejected", http.StatusBadRequest)
	}
}

// handlePromotionsQueueSub routes the promotion queue paths: the stats view
// and POST /api/promotions/queue/{id}/decide.
func (d *Daemon) handlePromotionsQueueSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/promotions/queue/")
	if rest == "stats" {
		user, ok := d.promotionUserOf(w, r)
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
	d.handlePromotionsQueueDecide(w, r, id)
}

// handleSessionPromote serves the chat action POST
// /api/sessions/{id}/promote {"rule_id","target"?}: it proposes the Rule
// against that Session's worktree, so the composer can promote a recalled Rule
// without leaving the chat.
func (d *Daemon) handleSessionPromote(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := d.promotionUserOf(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if d.hiddenSession(id) {
		http.NotFound(w, r)
		return
	}
	d.mu.Lock()
	_, exists := d.sessions[id]
	d.mu.Unlock()
	if !exists {
		http.NotFound(w, r)
		return
	}
	var body struct {
		RuleID string `json:"rule_id"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	item, created, err := d.proposePromotionResult(r.Context(), user.ID, body.RuleID, id, body.Target)
	if err != nil {
		switch {
		case errors.Is(err, memory.ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	view := promotionViewOf(item)
	if created {
		writeJSON(w, http.StatusCreated, view)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
