package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const pauseSessionWait = 20 * time.Second

// pauseRunSessions pauses the Run driver, stops its Sessions, and commits
// every unmerged Worker's outstanding work before returning.
func (d *Daemon) pauseRunSessions(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	rs := d.runs[id]
	if rs == nil {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusRunning {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is not running"})
		return
	}
	rs.Status = runStatusPaused
	rs.ResumeOnRestart = false
	repo := rs.Repo
	workers := map[string]string{}
	branches := map[string]string{}
	for _, ticket := range rs.Tickets {
		if rs.TicketPaused[ticket.ID] && !rs.TicketPauseHolds[ticket.ID] && len(rs.RunPausedRoles[ticket.ID]) == 0 {
			if rs.TicketPauseHolds == nil {
				rs.TicketPauseHolds = map[string]bool{}
			}
			rs.TicketPauseHolds[ticket.ID] = true
		}
		if !rs.TicketPauseHolds[ticket.ID] {
			if rs.Sessions[ticket.ID] != "" && rs.Merged[ticket.ID] == "" && !rs.Discarded[ticket.ID] {
				rs.TicketPaused[ticket.ID] = true
				d.markRunRolePausedByRunLocked(rs, ticket.ID, "worker")
				if rs.ChainPendingSubmit == nil {
					rs.ChainPendingSubmit = map[string]bool{}
				}
				rs.ChainPendingSubmit[ticket.ID] = true
			}
			for role, sessionID := range rs.Reviewers[ticket.ID] {
				if sessionID != "" {
					rs.TicketPaused[ticket.ID] = true
					d.markRunRolePausedByRunLocked(rs, ticket.ID, role)
				}
			}
		}
		if worktree := rs.Worktrees[ticket.ID]; worktree != "" && rs.Merged[ticket.ID] == "" && !rs.Discarded[ticket.ID] {
			workers[ticket.ID] = worktree
			branches[ticket.ID] = rs.Branches[ticket.ID]
		}
	}
	owned := runSessionIDsLocked(d, rs, "")
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()

	settled, err := d.settleRunSessions(owned)
	for ticketID, worktree := range workers {
		if commitErr := commitPausedWorkerWorktree(repo, branches[ticketID], worktree, id, ticketID); commitErr != nil {
			err = errorsJoin(err, fmt.Errorf("ticket %s: %w", ticketID, commitErr))
		}
	}
	err = errorsJoin(err, d.endRunSessions(settled))
	d.writePauseRunResult(w, id, err)
}

// pauseTicket freezes one Ticket without stopping other work in the Run.
func (d *Daemon) pauseTicket(w http.ResponseWriter, r *http.Request, runID, ticketID string) {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if _, ok := runTicketStatic(rs, ticketID); !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusRunning && rs.Status != runStatusPaused {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is finished"})
		return
	}
	if rs.Merged[ticketID] != "" || rs.Discarded[ticketID] {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "ticket is finished"})
		return
	}
	if rs.TicketPauseHolds == nil {
		rs.TicketPauseHolds = map[string]bool{}
	}
	rs.TicketPauseHolds[ticketID] = true
	rs.TicketPaused[ticketID] = true
	if rs.Sessions[ticketID] != "" {
		d.markRunRolePausedLocked(rs, ticketID, "worker")
		if rs.ChainPendingSubmit == nil {
			rs.ChainPendingSubmit = map[string]bool{}
		}
		rs.ChainPendingSubmit[ticketID] = true
	}
	for role, sessionID := range rs.Reviewers[ticketID] {
		if sessionID != "" {
			d.markRunRolePausedLocked(rs, ticketID, role)
		}
	}
	worktree, branch, repo := rs.Worktrees[ticketID], rs.Branches[ticketID], rs.Repo
	owned := runSessionIDsLocked(d, rs, ticketID)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()

	settled, err := d.settleRunSessions(owned)
	if worktree != "" {
		if commitErr := commitPausedWorkerWorktree(repo, branch, worktree, runID, ticketID); commitErr != nil {
			err = errorsJoin(err, commitErr)
		}
	}
	err = errorsJoin(err, d.endRunSessions(settled))
	d.writePauseRunResult(w, runID, err)
}

func (d *Daemon) writePauseRunResult(w http.ResponseWriter, runID string, pauseErr error) {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs != nil {
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	}
	d.mu.Unlock()
	if rs == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "run not found"})
		return
	}
	out := d.runJSON(rs)
	if pauseErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": pauseErr.Error(), "run": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": out})
}

func errorsJoin(current, next error) error {
	if current == nil {
		return next
	}
	return fmt.Errorf("%v; %w", current, next)
}

func (d *Daemon) markRunRolePausedLocked(rs *runState, ticketID, role string) {
	if rs.PausedRoles == nil {
		rs.PausedRoles = map[string]map[string]bool{}
	}
	if rs.PausedRoles[ticketID] == nil {
		rs.PausedRoles[ticketID] = map[string]bool{}
	}
	rs.PausedRoles[ticketID][role] = true
}

func (d *Daemon) markRunRolePausedByRunLocked(rs *runState, ticketID, role string) {
	d.markRunRolePausedLocked(rs, ticketID, role)
	if rs.RunPausedRoles == nil {
		rs.RunPausedRoles = map[string]map[string]bool{}
	}
	if rs.RunPausedRoles[ticketID] == nil {
		rs.RunPausedRoles[ticketID] = map[string]bool{}
	}
	rs.RunPausedRoles[ticketID][role] = true
}

func clearPausedRunRoleLocked(rs *runState, ticketID, role string) {
	delete(rs.PausedRoles[ticketID], role)
	if len(rs.PausedRoles[ticketID]) == 0 {
		delete(rs.PausedRoles, ticketID)
	}
	delete(rs.RunPausedRoles[ticketID], role)
	if len(rs.RunPausedRoles[ticketID]) == 0 {
		delete(rs.RunPausedRoles, ticketID)
	}
	if len(rs.PausedRoles[ticketID]) == 0 && len(rs.RunPausedRoles[ticketID]) == 0 && !rs.TicketPauseHolds[ticketID] {
		delete(rs.TicketPaused, ticketID)
	}
}

// runSessionIDsLocked returns the Run-owned Sessions in scope, including
// descendants such as scouts. Callers hold d.mu.
func runSessionIDsLocked(d *Daemon, rs *runState, onlyTicket string) []string {
	owned := map[string]bool{}
	for ticketID, id := range rs.Sessions {
		if onlyTicket == "" || ticketID == onlyTicket {
			if id != "" {
				owned[id] = true
			}
		}
	}
	for ticketID, roles := range rs.Reviewers {
		if onlyTicket != "" && ticketID != onlyTicket {
			continue
		}
		for _, id := range roles {
			if id != "" {
				owned[id] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for id, sess := range d.sessions {
			if owned[sess.meta.ParentID] && !owned[id] {
				owned[id] = true
				changed = true
			}
		}
	}
	ids := make([]string, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (d *Daemon) settleRunSessions(ids []string) ([]string, error) {
	var result error
	stopped := map[string]bool{}
	all := append([]string(nil), ids...)
	for _, id := range ids {
		if err := d.settleRunSession(id); err != nil {
			result = errorsJoin(result, fmt.Errorf("settle session %s: %w", id, err))
		}
		stopped[id] = true
	}
	// A Worker can create a child while its turn is settling. Re-scan after
	// its parent settles so no agent remains active in the WIP worktree.
	for {
		d.mu.Lock()
		var more []string
		for id, sess := range d.sessions {
			if !stopped[id] && stopped[sess.meta.ParentID] {
				more = append(more, id)
			}
		}
		d.mu.Unlock()
		if len(more) == 0 {
			break
		}
		sort.Strings(more)
		for _, id := range more {
			if err := d.settleRunSession(id); err != nil {
				result = errorsJoin(result, fmt.Errorf("settle child session %s: %w", id, err))
			}
			stopped[id] = true
			all = append(all, id)
		}
	}
	return all, result
}

func (d *Daemon) endRunSessions(ids []string) error {
	var result error
	for _, id := range ids {
		if _, code := d.endSession(id); code != http.StatusOK && code != http.StatusNotFound {
			result = errorsJoin(result, fmt.Errorf("end session %s returned HTTP %d", id, code))
		}
	}
	return result
}

func (d *Daemon) settleRunSession(id string) error {
	_, code := d.interrupt(id)
	if code != http.StatusOK && code != http.StatusNotFound && code != http.StatusGone {
		return fmt.Errorf("interrupt returned HTTP %d", code)
	}
	if d.waitRunSessionIdle(id, pauseSessionWait) {
		return nil
	}
	// Closing the harness process is the final stop for a Backend that cannot
	// interrupt a turn; wait for its callback before touching the worktree.
	_, _ = d.endSession(id)
	if !d.waitRunSessionIdle(id, pauseSessionWait) {
		return fmt.Errorf("session did not settle within %s", pauseSessionWait)
	}
	return nil
}

func (d *Daemon) waitRunSessionIdle(id string, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		d.mu.Lock()
		s := d.sessions[id]
		if s == nil || (!s.busy && s.active == 0) {
			d.mu.Unlock()
			return true
		}
		ch := s.notify
		d.mu.Unlock()
		select {
		case <-ch:
		case <-timer.C:
			return false
		}
	}
}

func commitPausedWorkerWorktree(repo, branch, worktree, runID, ticketID string) error {
	if !runWorktreePath(repo, worktree) {
		return fmt.Errorf("refused worktree outside .el-worktrees: %s", worktree)
	}
	current, err := runGit(worktree, "branch", "--show-current")
	if err != nil || strings.TrimSpace(current) != branch {
		return fmt.Errorf("worktree branch is %q, want %q: %v", strings.TrimSpace(current), branch, err)
	}
	status, err := runGit(worktree, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect worktree: %w", err)
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if _, err := runGit(worktree, "add", "--all"); err != nil {
		return fmt.Errorf("stage worktree changes: %w", err)
	}
	message := fmt.Sprintf("WIP: paused Run %s Ticket %s", runID, ticketID)
	if _, err := runGit(worktree, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit WIP checkpoint: %w", err)
	}
	return nil
}

func (d *Daemon) resumeTicket(w http.ResponseWriter, r *http.Request, runID, ticketID string) {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if _, ok := runTicketStatic(rs, ticketID); !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusRunning {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "resume the Run first"})
		return
	}
	if !rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "ticket is not paused"})
		return
	}
	if rs.Merged[ticketID] != "" || rs.Discarded[ticketID] {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "ticket is finished"})
		return
	}
	d.mu.Unlock()
	if err := d.resumePausedRunRoles(runID, ticketID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	d.mu.Lock()
	if rs := d.runs[runID]; rs != nil {
		delete(rs.TicketPauseHolds, ticketID)
		if len(rs.PausedRoles[ticketID]) == 0 && len(rs.RunPausedRoles[ticketID]) == 0 {
			delete(rs.TicketPaused, ticketID)
		}
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	}
	d.mu.Unlock()
	d.writeCurrentRun(w, runID)
}

func (d *Daemon) writeCurrentRun(w http.ResponseWriter, runID string) {
	d.mu.Lock()
	rs := d.runs[runID]
	d.mu.Unlock()
	if rs == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "run not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": d.runJSON(rs)})
}

// resumePausedRunRoles replaces ended Run roles on their existing Ticket
// branch. It is called only by a manual Resume action.
func (d *Daemon) resumePausedRunRoles(runID, onlyTicket string) error {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		return fmt.Errorf("unknown run %q", runID)
	}
	tickets := make([]string, 0, len(rs.PausedRoles))
	for ticketID := range rs.PausedRoles {
		if onlyTicket != "" && onlyTicket == ticketID {
			tickets = append(tickets, ticketID)
		} else if onlyTicket == "" && !rs.TicketPauseHolds[ticketID] && len(rs.RunPausedRoles[ticketID]) > 0 {
			tickets = append(tickets, ticketID)
		}
	}
	sort.Strings(tickets)
	d.mu.Unlock()
	for _, ticketID := range tickets {
		roles := []string{"worker", reviewerSpecType, reviewerStandardsType}
		for _, role := range roles {
			d.mu.Lock()
			rs = d.runs[runID]
			needed := rs != nil && rs.PausedRoles[ticketID][role]
			if needed && onlyTicket == "" {
				needed = rs.RunPausedRoles[ticketID][role] && !rs.TicketPauseHolds[ticketID]
			}
			d.mu.Unlock()
			if !needed {
				continue
			}
			if err := d.resumePausedRunRole(runID, ticketID, role); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Daemon) resumePausedRunRole(runID, ticketID, role string) error {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil || rs.Status != runStatusRunning || !rs.PausedRoles[ticketID][role] {
		d.mu.Unlock()
		return nil
	}
	oldID := rs.Sessions[ticketID]
	if role != "worker" {
		oldID = rs.Reviewers[ticketID][role]
	}
	oldSession := d.sessions[oldID]
	var oldMeta SessionMeta
	var oldEvents []Event
	if oldSession != nil {
		oldMeta = oldSession.meta
		oldEvents = append([]Event(nil), oldSession.events...)
	}
	branch, worktree, repo := rs.Branches[ticketID], rs.Worktrees[ticketID], rs.Repo
	spec, ticketFile, base := rs.Spec, ticketPath(rs, ticketID), rs.Bases[ticketID]
	owner, noRecall := rs.Owner, rs.RecallOff[ticketID]
	chain := runRoleChain(rs, ticketID, role)
	if len(chain) == 0 {
		chain = []chainEntry{{Backend: rs.Harness, Model: rs.Model, Effort: rs.Effort}}
	}
	position := clampChainPosition(chainPosition(rs, ticketID, role), len(chain))
	entry := chain[position]
	if role == "worker" && branch == "" {
		// An unstarted Ticket has no branch or Session to hand off; clear its
		// pause marker and let the normal driver start it after Resume.
		clearPausedRunRoleLocked(rs, ticketID, role)
		d.mu.Unlock()
		return nil
	}
	if branch == "" || worktree == "" {
		d.mu.Unlock()
		return fmt.Errorf("ticket %s has no branch to resume", ticketID)
	}
	if oldID != "" && oldMeta.ID == "" {
		oldMeta = SessionMeta{ID: oldID, Harness: entry.Backend, Model: entry.Model, WorkDir: worktree, ParentID: runID, Owner: owner}
	}
	meta := oldMeta
	meta.ID = newID()
	meta.Harness, meta.Model, meta.Effort = entry.Backend, entry.Model, entry.Effort
	meta.WorkDir, meta.Status, meta.CreatedAt = worktree, "idle", time.Now().UTC()
	meta.ParentID, meta.Owner = runID, owner
	meta.ResumeToken, meta.CompressBlock, meta.Dirs = "", "", ownerDirs{}
	if role == "worker" {
		meta.RunWorker, meta.Reviewer, meta.NoRecall = true, "", noRecall
	} else {
		meta.RunWorker, meta.Reviewer = false, role
	}
	d.mu.Unlock()

	be, serr := d.startSession(meta)
	if serr != nil {
		return fmt.Errorf("could not start resumed %s for %s: %s", role, ticketID, serr.msg)
	}
	tip := ""
	if role != "worker" {
		tip, _ = runGit(repo, "rev-parse", branch)
	}
	task := d.runFallbackTaskPrompt(runID, ticketID, role, branch, worktree, repo, spec, ticketFile, base, tip)
	if task == "" {
		be.CloseSession(meta.ID)
		d.removeSession(meta.ID)
		return fmt.Errorf("could not build resumed %s task for %s", role, ticketID)
	}
	handoff := pauseHandoff(oldEvents, role, branch)

	d.mu.Lock()
	rs = d.runs[runID]
	if rs == nil || rs.Status != runStatusRunning || !rs.PausedRoles[ticketID][role] {
		d.mu.Unlock()
		be.CloseSession(meta.ID)
		d.removeSession(meta.ID)
		return fmt.Errorf("ticket %s is no longer paused", ticketID)
	}
	if role == "worker" && rs.Sessions[ticketID] != oldID {
		d.mu.Unlock()
		be.CloseSession(meta.ID)
		d.removeSession(meta.ID)
		return fmt.Errorf("ticket %s Worker changed during Resume", ticketID)
	}
	if role != "worker" && rs.Reviewers[ticketID][role] != oldID {
		d.mu.Unlock()
		be.CloseSession(meta.ID)
		d.removeSession(meta.ID)
		return fmt.Errorf("ticket %s Reviewer changed during Resume", ticketID)
	}
	setChainPosition(rs, ticketID, role, position)
	if role == "worker" {
		rs.Sessions[ticketID] = meta.ID
		if rs.ChainPendingSubmit == nil {
			rs.ChainPendingSubmit = map[string]bool{}
		}
		rs.ChainPendingSubmit[ticketID] = true
		if rs.SubmitRecs != nil {
			delete(rs.SubmitRecs, ticketID)
		}
	} else {
		if rs.Reviewers[ticketID] == nil {
			rs.Reviewers[ticketID] = map[string]string{}
		}
		rs.Reviewers[ticketID][role] = meta.ID
		if rs.Reviews[ticketID] != nil {
			delete(rs.Reviews[ticketID], role)
		}
		if rs.ReviewSeen[ticketID] != nil {
			delete(rs.ReviewSeen[ticketID], role)
		}
		if rs.FormatReprompt[ticketID] != nil {
			delete(rs.FormatReprompt[ticketID], role)
		}
		if rs.ReviewTips[ticketID] == nil {
			rs.ReviewTips[ticketID] = map[string]string{}
		}
		rs.ReviewTips[ticketID][role] = tip
		if rs.ReviewPending[ticketID] == nil {
			rs.ReviewPending[ticketID] = map[string][]string{}
		}
		rs.ReviewPending[ticketID][role] = nil
		appendReviewPending(rs, ticketID, role, tip)
		if rs.ReviewStarted[ticketID] == nil {
			rs.ReviewStarted[ticketID] = map[string]time.Time{}
		}
		rs.ReviewStarted[ticketID][role] = time.Now().UTC()
	}
	clearPausedRunRoleLocked(rs, ticketID, role)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()

	d.persistResumeToken(be, meta.ID)
	d.appendEvent(meta.ID, "status", "", "idle")
	d.launchRunChainHandoff(runID, ticketID, role, meta.ID, be, handoff, task)
	return nil
}

func pauseHandoff(events []Event, role, branch string) string {
	body := strings.TrimRight(buildHandoff(events), "\n")
	if body == "" {
		body = handoffHeader
	}
	return body + fmt.Sprintf("\n\n## Pause checkpoint\nThe previous %s Session was paused and its work preserved on branch %s. Continue from this committed branch; do not duplicate existing commits.\n", role, branch)
}

type runSwitchRequest struct {
	Backend string `json:"backend"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
}

func (d *Daemon) decodeRunSwitch(w http.ResponseWriter, r *http.Request) (chainEntry, bool) {
	var req runSwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return chainEntry{}, false
	}
	backend := strings.TrimSpace(req.Backend)
	if backend == "" {
		backend = strings.TrimSpace(req.Harness)
	}
	model := strings.TrimSpace(req.Model)
	if backend == "" || model == "" {
		http.Error(w, "backend and model are required", http.StatusBadRequest)
		return chainEntry{}, false
	}
	d.mu.Lock()
	_, registered := d.backends[backend]
	available := registered || (backend == "fake" && d.backend != nil) || (backend == "claude" && d.claude != nil)
	d.mu.Unlock()
	if !available {
		http.Error(w, "backend is not available", http.StatusBadRequest)
		return chainEntry{}, false
	}
	if backend == "claude" && !validClaudeModel(model) {
		http.Error(w, "unknown claude model", http.StatusBadRequest)
		return chainEntry{}, false
	}
	return chainEntry{Backend: backend, Model: model}, true
}

func setReviewerOverridesLocked(rs *runState, ticketID string, requested chainEntry) {
	if rs.ReviewerOverrides == nil {
		rs.ReviewerOverrides = map[string]map[string]chainEntry{}
	}
	for _, role := range []string{reviewerSpecType, reviewerStandardsType} {
		chain := runRoleChain(rs, ticketID, role)
		if len(chain) == 0 {
			continue
		}
		position := clampChainPosition(chainPosition(rs, ticketID, role), len(chain))
		selected := requested
		selected.Effort = chain[position].Effort
		if rs.ReviewerOverrides[ticketID] == nil {
			rs.ReviewerOverrides[ticketID] = map[string]chainEntry{}
		}
		rs.ReviewerOverrides[ticketID][role] = selected
	}
}

func (d *Daemon) switchTicketBackend(w http.ResponseWriter, r *http.Request, runID, ticketID string) {
	entry, ok := d.decodeRunSwitch(w, r)
	if !ok {
		return
	}
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if _, ok := runTicketStatic(rs, ticketID); !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusPaused && !rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "ticket or Run must be paused before switching"})
		return
	}
	if rs.Merged[ticketID] != "" || rs.Discarded[ticketID] {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "ticket is finished"})
		return
	}
	chain := workerRunChain(rs, ticketID)
	if len(chain) > 0 {
		pos := clampChainPosition(chainPosition(rs, ticketID, "worker"), len(chain))
		entry.Effort = chain[pos].Effort
	} else {
		entry.Effort = rs.Effort
	}
	rs.WorkerOverrides[ticketID] = entry
	setReviewerOverridesLocked(rs, ticketID, entry)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.writeCurrentRun(w, runID)
}

func (d *Daemon) switchRunBackend(w http.ResponseWriter, r *http.Request, runID string) {
	entry, ok := d.decodeRunSwitch(w, r)
	if !ok {
		return
	}
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusPaused {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "Run must be paused before switching"})
		return
	}
	if rs.WorkerOverrides == nil {
		rs.WorkerOverrides = map[string]chainEntry{}
	}
	for _, ticket := range rs.Tickets {
		if rs.Merged[ticket.ID] != "" || rs.Discarded[ticket.ID] {
			continue
		}
		selected := entry
		chain := workerRunChain(rs, ticket.ID)
		if len(chain) > 0 {
			pos := clampChainPosition(chainPosition(rs, ticket.ID, "worker"), len(chain))
			selected.Effort = chain[pos].Effort
		} else {
			selected.Effort = rs.Effort
		}
		rs.WorkerOverrides[ticket.ID] = selected
	}
	if rs.ReviewerOverrides == nil {
		rs.ReviewerOverrides = map[string]map[string]chainEntry{}
	}
	for _, ticket := range rs.Tickets {
		if rs.Merged[ticket.ID] != "" || rs.Discarded[ticket.ID] {
			continue
		}
		for _, role := range []string{reviewerSpecType, reviewerStandardsType} {
			chain := runRoleChain(rs, ticket.ID, role)
			if len(chain) == 0 {
				continue
			}
			position := clampChainPosition(chainPosition(rs, ticket.ID, role), len(chain))
			selected := entry
			selected.Effort = chain[position].Effort
			if rs.ReviewerOverrides[ticket.ID] == nil {
				rs.ReviewerOverrides[ticket.ID] = map[string]chainEntry{}
			}
			rs.ReviewerOverrides[ticket.ID][role] = selected
		}
	}
	rs.Harness, rs.Model = entry.Backend, entry.Model
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.writeCurrentRun(w, runID)
}

// preparePausedRunChainFailuresLocked lets a failed quota/auth/etc. Session
// take its configured Chain step on manual Resume instead of restarting the
// same entry as an ordinary pause. Callers hold d.mu.
func preparePausedRunChainFailuresLocked(d *Daemon, rs *runState) {
	clear := func(ticketID, role, sessionID string) {
		if rs.TicketPauseHolds[ticketID] || !rs.RunPausedRoles[ticketID][role] {
			return
		}
		s := d.sessions[sessionID]
		if s == nil || s.meta.Status != "failed" {
			return
		}
		detail := lastTurnFailure(s.events)
		if detail == "" || !chainAdvances(fmt.Errorf("%s", detail)) {
			return
		}
		if role == "worker" && rs.WorkerOverrides[ticketID].Backend != "" {
			return
		}
		if role != "worker" && rs.ReviewerOverrides[ticketID][role].Backend != "" {
			return
		}
		clearPausedRunRoleLocked(rs, ticketID, role)
		if !rs.TicketPauseHolds[ticketID] {
			delete(rs.TicketPaused, ticketID)
		}
	}
	for ticketID, sessionID := range rs.Sessions {
		clear(ticketID, "worker", sessionID)
	}
	for ticketID, reviewers := range rs.Reviewers {
		for role, sessionID := range reviewers {
			clear(ticketID, role, sessionID)
		}
	}
}
