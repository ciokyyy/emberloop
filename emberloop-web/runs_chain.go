package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// runRoles are the Chain slots shown on each Ticket row.
var runRoles = []string{"worker", "worker-design", reviewerSpecType, reviewerStandardsType, "reviewer-design"}

// configuredRunChains snapshots the Run's configured chains so a settings
// edit or Daemon restart cannot change a Ticket's fallback order mid-run.
func configuredRunChains(harness, model, effort string) map[string][]chainEntry {
	chains := make(map[string][]chainEntry, len(runRoles))
	for _, role := range runRoles {
		entries := chainFor(role)
		if role == "worker-design" && len(entries) == 0 {
			entries = chainFor("worker")
		}
		if role == reviewerDesignType && len(entries) == 0 {
			entries = chainFor(reviewerStandardsType)
		}
		if len(entries) == 0 {
			entries = []chainEntry{{Backend: harness, Model: model, Effort: effort}}
		}
		chains[role] = append([]chainEntry(nil), entries...)
	}
	return chains
}

// daemonConfiguredRunChains snapshots the configured Run chains.
func (d *Daemon) daemonConfiguredRunChains(harness, model, effort string) map[string][]chainEntry {
	chains := make(map[string][]chainEntry, len(runRoles))
	for _, role := range runRoles {
		entries := d.daemonChainFor(role)
		if role == "worker-design" && len(entries) == 0 {
			entries = d.daemonChainFor("worker")
		}
		if role == reviewerDesignType && len(entries) == 0 {
			entries = d.daemonChainFor(reviewerStandardsType)
		}
		if len(entries) == 0 {
			entries = []chainEntry{{Backend: harness, Model: model, Effort: effort}}
		}
		chains[role] = append([]chainEntry(nil), entries...)
	}
	return chains
}

// runChain returns a copy of a Run's snapshotted Chain. Old Run files without
// runChain returns a copy of a Run's snapshotted Chain. Old Run files without
// chains fall back to current settings, then to the Run's Start configuration.
func runChain(rs *runState, role string) []chainEntry {
	if rs != nil {
		if entries := rs.Chains[role]; len(entries) > 0 {
			return append([]chainEntry(nil), entries...)
		}
	}
	if entries := chainFor(role); len(entries) > 0 {
		return entries
	}
	if rs == nil || rs.Harness == "" || rs.Model == "" {
		return nil
	}
	return []chainEntry{{Backend: rs.Harness, Model: rs.Model, Effort: rs.Effort}}
}

// workerRunChain applies a paused Ticket's manual Switch to its active Chain
// slot. The remaining fallback entries stay intact; callers clear the
// override when a quota failure advances to the next slot.
func workerRunChain(rs *runState, ticketID, role string) []chainEntry {
	if role == "" {
		role = "worker"
	}
	entries := runChain(rs, role)
	if len(entries) == 0 {
		return nil
	}
	position := clampChainPosition(chainPosition(rs, ticketID, role), len(entries))
	if override, ok := rs.WorkerOverrides[ticketID]; ok {
		if override.Backend == "grok" {
			delete(rs.WorkerOverrides, ticketID)
		} else {
			entries[position] = override
		}
	}
	return entries
}

func runWorkerRole(rs *runState, ticketID string) string {
	for _, ticket := range rs.Tickets {
		if ticket.ID == ticketID && effectiveRunTicketType(rs, ticket) == "design" {
			return "worker-design"
		}
	}
	return "worker"
}

// runRoleChain applies a per-Ticket override to the active slot of any Run
// role; Worker and Reviewer overrides use separate maps.
func runRoleChain(rs *runState, ticketID, role string) []chainEntry {
	if role == "worker" || role == "worker-design" {
		return workerRunChain(rs, ticketID, role)
	}
	entries := runChain(rs, role)
	if len(entries) == 0 {
		return entries
	}
	if override, ok := rs.ReviewerOverrides[ticketID][role]; ok {
		if override.Backend == "grok" {
			delete(rs.ReviewerOverrides[ticketID], role)
		} else {
			position := clampChainPosition(chainPosition(rs, ticketID, role), len(entries))
			entries[position] = override
		}
	}
	return entries
}
func clampChainPosition(position, length int) int {
	if length == 0 || position < 0 {
		return 0
	}
	if position >= length {
		return length - 1
	}
	return position
}

// chainPosition returns the zero-based active entry for one Ticket role.
// Missing positions are the first entry, which keeps old Run files compatible.
func chainPosition(rs *runState, ticketID, role string) int {
	if rs == nil {
		return 0
	}
	row, ok := rs.Rows[ticketID]
	if !ok || row.ChainPositions == nil {
		return 0
	}
	return row.ChainPositions[role]
}

// setChainPosition records a role's active entry on its persisted Ticket row.
// Callers hold d.mu.
func setChainPosition(rs *runState, ticketID, role string, position int) {
	if rs.Rows == nil {
		rs.Rows = map[string]savedRow{}
	}
	row := rs.Rows[ticketID]
	if row.ChainPositions == nil {
		row.ChainPositions = map[string]int{}
	}
	row.ChainPositions[role] = position
	rs.Rows[ticketID] = row
}

// ticketChainEntries returns the active Chain entry for every configured Run
// role. Positions are one-based for the Ticket API; the persisted index is
// zero-based for direct slice lookup.
func ticketChainEntries(rs *runState, ticketID string) map[string]ticketChainEntry {
	out := map[string]ticketChainEntry{}
	for _, role := range runRoles {
		entries := runRoleChain(rs, ticketID, role)
		if len(entries) == 0 {
			continue
		}
		pos := clampChainPosition(chainPosition(rs, ticketID, role), len(entries))
		entry := entries[pos]
		out[role] = ticketChainEntry{Position: pos + 1, Total: len(entries), Backend: entry.Backend, Model: entry.Model}
	}
	return out
}

// lastTurnFailure returns the newest error Event after the most recent prompt.
// Provider error Events from earlier turns never classify a later turn.
func lastTurnFailure(events []Event) string {
	promptSeq := int64(0)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "prompt" {
			promptSeq = events[i].Seq
			break
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Seq <= promptSeq {
			break
		}
		if e.Kind == "error" && strings.TrimSpace(e.Text) != "" {
			return strings.TrimSpace(e.Text)
		}
	}
	return ""
}

// handleRunChainFailure advances a failed Run Worker or Reviewer to its next
// configured entry. The old Session and Event log are deliberately retained.
// It returns true when the failure belongs to an advancing Chain class.
func (d *Daemon) handleRunChainFailure(id string) bool {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok || s.meta.Status != "failed" {
		d.mu.Unlock()
		return false
	}
	runID, ticketID, role := d.findReviewerLocked(id)
	if role == "" {
		runID, ticketID = d.findWorkerLocked(id)
		if runID != "" {
			role = "worker"
			if rs := d.runs[runID]; rs != nil {
				for _, t := range rs.Tickets {
					if t.ID == ticketID && effectiveRunTicketType(rs, staticTicket{ID: t.ID, Type: t.Type}) == "design" {
						role = "worker-design"
						break
					}
				}
			}
		}
	}
	rs := d.runs[runID]
	if rs == nil || role == "" {
		d.mu.Unlock()
		return false
	}
	detail := lastTurnFailure(s.events)
	if detail == "" || !chainAdvances(errors.New(detail)) {
		d.mu.Unlock()
		return false
	}
	if rs.chainHandling == nil {
		rs.chainHandling = map[string]bool{}
	}
	if rs.chainHandling[id] {
		d.mu.Unlock()
		return true
	}
	rs.chainHandling[id] = true
	paused := rs.Status == runStatusPaused || rs.TicketPaused[ticketID]
	if !paused && (role == "worker" || role == "worker-design") {
		if rs.ChainPendingSubmit == nil {
			rs.ChainPendingSubmit = map[string]bool{}
		}
		rs.ChainPendingSubmit[ticketID] = true
		d.persistRun(rs)
	}
	chain := append([]chainEntry(nil), runRoleChain(rs, ticketID, role)...)
	position := clampChainPosition(chainPosition(rs, ticketID, role), len(chain))
	d.mu.Unlock()

	if paused {
		d.clearChainHandling(runID, id)
		return true
	}
	if position+1 >= len(chain) {
		d.recordRunChainPosition(runID, ticketID, role, position)
		d.markRunChainNeedsYou(runID, ticketID, role, detail, true)
		d.clearChainHandling(runID, id)
		return true
	}
	d.startRunChainFallback(runID, ticketID, role, id, detail, chain, position)
	d.clearChainHandling(runID, id)
	return true
}

func (d *Daemon) clearChainHandling(runID, sessionID string) {
	d.mu.Lock()
	if rs := d.runs[runID]; rs != nil && rs.chainHandling != nil {
		delete(rs.chainHandling, sessionID)
	}
	d.mu.Unlock()
}

// startRunChainFallback creates the next Session on the existing Ticket
// worktree. A failed create only advances again when its own error is one of
// the four Chain classes; other start failures make the Ticket Needs-you.
// startRunChainFallback creates the next Session on the existing Ticket
// worktree. A failed create only advances again for a classified Chain error.
func (d *Daemon) startRunChainFallback(runID, ticketID, role, failedID, detail string, chain []chainEntry, position int) {
	d.mu.Lock()
	rs := d.runs[runID]
	old, oldOK := d.sessions[failedID]
	if rs == nil || !oldOK || rs.Status != runStatusRunning || rs.TicketPaused[ticketID] || !d.runRoleSessionLocked(rs, ticketID, role, failedID) {
		d.mu.Unlock()
		return
	}
	oldEvents := append([]Event(nil), old.events...)
	oldMeta := old.meta
	branch, worktree, spec, repo := rs.Branches[ticketID], rs.Worktrees[ticketID], rs.Spec, rs.Repo
	base := rs.Bases[ticketID]
	ticketFile := ticketPath(rs, ticketID)
	tip := rs.ReviewTips[ticketID][role]
	d.mu.Unlock()
	if branch == "" || worktree == "" {
		d.markRunChainNeedsYou(runID, ticketID, role, "the failed Session has no Ticket branch", false)
		return
	}
	prompt := d.runFallbackTaskPrompt(runID, ticketID, role, branch, worktree, repo, spec, ticketFile, base, tip)
	if prompt == "" {
		d.markRunChainNeedsYou(runID, ticketID, role, "could not build the fallback task prompt", false)
		return
	}

	for next := position + 1; next < len(chain); next++ {
		d.mu.Lock()
		rs = d.runs[runID]
		if rs == nil || rs.Status != runStatusRunning || rs.TicketPaused[ticketID] || !d.runRoleSessionLocked(rs, ticketID, role, failedID) {
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()

		entry := chain[next]
		meta := oldMeta
		meta.ID = newID()
		meta.Harness, meta.Model, meta.Effort = entry.Backend, entry.Model, entry.Effort
		meta.WorkDir, meta.Status = worktree, "idle"
		meta.CreatedAt = time.Now().UTC()
		meta.ResumeToken, meta.CompressBlock, meta.Dirs = "", "", ownerDirs{}
		be, serr := d.startSession(meta)
		if serr != nil {
			detail = serr.msg
			advances := chainAdvances(errors.New(detail))
			if advances && next+1 < len(chain) {
				continue
			}
			d.recordRunChainPosition(runID, ticketID, role, next)
			d.markRunChainNeedsYou(runID, ticketID, role, "could not start fallback Session: "+detail, advances)
			return
		}

		d.mu.Lock()
		rs = d.runs[runID]
		if rs == nil || rs.Status != runStatusRunning || rs.TicketPaused[ticketID] || !d.runRoleSessionLocked(rs, ticketID, role, failedID) {
			d.mu.Unlock()
			if be != nil {
				be.CloseSession(meta.ID)
			}
			d.removeSession(meta.ID)
			return
		}
		setChainPosition(rs, ticketID, role, next)
		if role == "worker" || role == "worker-design" {
			delete(rs.WorkerOverrides, ticketID)
			rs.Sessions[ticketID] = meta.ID
			if rs.SubmitRecs != nil {
				delete(rs.SubmitRecs, ticketID)
			}
			if rs.Reprompted != nil {
				delete(rs.Reprompted, ticketID)
			}
		} else {
			if rs.ReviewerOverrides[ticketID] != nil {
				delete(rs.ReviewerOverrides[ticketID], role)
				if len(rs.ReviewerOverrides[ticketID]) == 0 {
					delete(rs.ReviewerOverrides, ticketID)
				}
			}
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
			if rs.ReviewStarted[ticketID] == nil {
				rs.ReviewStarted[ticketID] = map[string]time.Time{}
			}
			rs.ReviewStarted[ticketID][role] = time.Now().UTC()
		}
		d.indexRunTicketSessionLocked(runID, ticketID, meta.ID)
		if rs.ChainExhausted != nil {
			delete(rs.ChainExhausted, ticketID)
		}
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
		d.mu.Unlock()

		d.persistResumeToken(be, meta.ID)
		d.appendEvent(meta.ID, "status", "", "idle")
		handoff := chainHandoff(oldEvents, detail)
		d.launchRunChainHandoff(runID, ticketID, role, meta.ID, be, handoff, prompt)
		return
	}
	d.recordRunChainPosition(runID, ticketID, role, clampChainPosition(position, len(chain)))
	d.markRunChainNeedsYou(runID, ticketID, role, "provider Chain exhausted while starting the fallback Session: "+detail, true)
}

func (d *Daemon) recordRunChainPosition(runID, ticketID, role string, position int) {
	d.mu.Lock()
	if rs := d.runs[runID]; rs != nil {
		setChainPosition(rs, ticketID, role, position)
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	}
	d.mu.Unlock()
}

// runRoleSessionLocked reports whether sessionID is still the current Session
// for this Ticket role. Callers hold d.mu.
func (d *Daemon) runRoleSessionLocked(rs *runState, ticketID, role, sessionID string) bool {
	if role == "worker" || role == "worker-design" {
		return rs.Sessions[ticketID] == sessionID
	}
	return rs.Reviewers[ticketID][role] == sessionID
}

// runFallbackTaskPrompt rebuilds the normal injected role task for a fallback
// Session, after its Handoff turn has established the prior context.
func (d *Daemon) runFallbackTaskPrompt(runID, ticketID, role, branch, worktree, repo, spec, ticketFile, base, tip string) string {
	if role == "worker" || role == "worker-design" {
		seeded := false
		if out, err := runGit(worktree, "status", "--porcelain", "--", "AGENTS.md"); err == nil {
			seeded = strings.HasPrefix(out, "??")
		}
		prompt := buildWorkerFirstPrompt(ticketID, spec, ticketFile, branch, seeded, worktree, role)
		if role == "worker-design" {
			prompt = appendDesignerPrompt(prompt)
		}
		return prompt
	}
	if tip == "" {
		tip, _ = runGit(repo, "rev-parse", branch)
	}
	return buildReviewerFirstPrompt(ticketID, spec, ticketFile, role, branch, tip, worktree, repo, base)
}

// launchRunChainHandoff puts the failed Session's Handoff first, then sends
// the normal role task with create-time injection on the new Session.
// launchRunChainHandoff puts a Handoff first, then sends the normal role task
// with create-time injection on the replacement Session.
func (d *Daemon) launchRunChainHandoff(runID, ticketID, role, sessionID string, be Backend, handoff, task string) {
	go func() {
		d.pauseGate(runID)
		d.mu.Lock()
		rs := d.runs[runID]
		s, ok := d.sessions[sessionID]
		if rs == nil || !ok || rs.Status != runStatusRunning || !d.runRoleSessionLocked(rs, ticketID, role, sessionID) {
			d.mu.Unlock()
			d.cleanupUnclaimedRunChainSession(runID, ticketID, role, sessionID, be)
			return
		}
		s.inject = injection{}
		s.injectOp = handleResume
		s.busy = true
		s.active++
		d.appendEventExtraLocked(sessionID, "prompt", handoff, "", "", 0, nil, func(e *Event) {
			e.Handoff = true
			systemSender.stamp(e)
		})
		d.mu.Unlock()

		d.runTurnWith(sessionID, be, handoff, nil, true, nil)
		d.pauseGate(runID)
		d.mu.Lock()
		rs = d.runs[runID]
		s, ok = d.sessions[sessionID]
		if rs == nil || !ok || rs.Status != runStatusRunning || s.meta.Status != "idle" || !d.runRoleSessionLocked(rs, ticketID, role, sessionID) {
			d.mu.Unlock()
			d.cleanupUnclaimedRunChainSession(runID, ticketID, role, sessionID, be)
			return
		}
		meta := s.meta
		d.mu.Unlock()

		if code, msg := d.resumeReleased(sessionID); code != 0 {
			d.markRunChainNeedsYou(runID, ticketID, role, "could not resume fallback Session: "+msg, false)
			return
		}
		h := startHandle(meta, handleCreate)
		if pb, pv := d.profileBlockFor(meta); len(pb) > 0 {
			h.inject.prepend(pb, pv)
		}
		d.mu.Lock()
		rs = d.runs[runID]
		s, ok = d.sessions[sessionID]
		if rs == nil || !ok || rs.Status != runStatusRunning || s.meta.Status != "idle" || !d.runRoleSessionLocked(rs, ticketID, role, sessionID) {
			d.mu.Unlock()
			d.cleanupUnclaimedRunChainSession(runID, ticketID, role, sessionID, be)
			return
		}
		s.inject, s.injectOp = h.inject, handleCreate
		d.mu.Unlock()
		if _, code := d.routeLive(sessionID, "steer", task); code != http.StatusAccepted {
			d.markRunChainNeedsYou(runID, ticketID, role, fmt.Sprintf("could not send fallback task (HTTP %d)", code), false)
			return
		}
		if role == "worker" {
			d.mu.Lock()
			if rs := d.runs[runID]; rs != nil && rs.Sessions[ticketID] == sessionID && !rs.PausedRoles[ticketID]["worker"] && !rs.TicketPauseHolds[ticketID] && len(rs.RunPausedRoles[ticketID]) == 0 {
				delete(rs.TicketPaused, ticketID)
				d.persistRun(rs)
				d.broadcastRunLocked(rs)
			}
			d.mu.Unlock()
		}
	}()
}

func (d *Daemon) cleanupUnclaimedRunChainSession(runID, ticketID, role, sessionID string, be Backend) {
	d.mu.Lock()
	rs := d.runs[runID]
	s := d.sessions[sessionID]
	claimed := rs != nil && d.runRoleSessionLocked(rs, ticketID, role, sessionID)
	failed := s != nil && s.meta.Status == "failed"
	d.mu.Unlock()
	if claimed || failed {
		return
	}
	if be != nil {
		be.CloseSession(sessionID)
	}
	d.removeSession(sessionID)
}

// chainHandoff adds the provider failure detail that buildHandoff intentionally
// omits from ordinary summaries.
func chainHandoff(events []Event, failure string) string {
	body := strings.TrimRight(buildHandoff(events), "\n")
	if body == "" {
		body = handoffHeader
	}
	return body + "\n\n## Previous Session failure\n" + capText(failure, 1000) + "\n"
}

// markRunChainNeedsYou records a terminal Chain failure on the Ticket and
// publishes the usual Run notification. Callers hold no lock.
func (d *Daemon) markRunChainNeedsYou(runID, ticketID, role, detail string, exhausted bool) {
	reason := fmt.Sprintf("%s Chain stopped after failure: %s", role, detail)
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		return
	}
	if rs.NeedsYouReason == nil {
		rs.NeedsYouReason = map[string]string{}
	}
	rs.NeedsYouReason[ticketID] = reason
	if exhausted {
		if rs.ChainExhausted == nil {
			rs.ChainExhausted = map[string]bool{}
		}
		rs.ChainExhausted[ticketID] = true
	}
	push := d.noteNeedsYouPushLocked(rs, ticketID, reason)
	num, title := ticketNumTitleLocked(rs, ticketID)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	if push {
		d.pushRunNeedsYou(runID, ticketID, num, title, reason)
	}
}

// reconcileRunChainFailures handles a failed active Session restored from disk
// before the resumed driver can re-prompt it or start another reviewer.
func (d *Daemon) reconcileRunChainFailures(runID string) {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil || rs.Status != runStatusRunning {
		d.mu.Unlock()
		return
	}
	var failed []string
	for _, id := range rs.Sessions {
		if s := d.sessions[id]; s != nil && s.meta.Status == "failed" {
			failed = append(failed, id)
		}
	}
	for _, byType := range rs.Reviewers {
		for _, id := range byType {
			if s := d.sessions[id]; s != nil && s.meta.Status == "failed" {
				failed = append(failed, id)
			}
		}
	}
	d.mu.Unlock()
	for _, id := range failed {
		d.handleRunChainFailure(id)
	}
}
