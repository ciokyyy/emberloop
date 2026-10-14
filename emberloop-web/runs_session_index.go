package web

import "sort"

type runTicketRef struct {
	runID    string
	ticketID string
}

// indexRunTicketSessionLocked records one Run-owned Session and every
// descendant under its Ticket. Callers hold d.mu; the owning Run persists
// TicketSessions with its other state.
func (d *Daemon) indexRunTicketSessionLocked(runID, ticketID, sessionID string) bool {
	if runID == "" || ticketID == "" || sessionID == "" {
		return false
	}
	rs := d.runs[runID]
	if rs == nil {
		return false
	}
	if rs.TicketSessions == nil {
		rs.TicketSessions = map[string][]string{}
	}
	if d.runSessionIndex == nil {
		d.runSessionIndex = map[runTicketRef]map[string]struct{}{}
	}
	if d.runSessionOwners == nil {
		d.runSessionOwners = map[string]runTicketRef{}
	}
	ref := runTicketRef{runID: runID, ticketID: ticketID}
	if previous, ok := d.runSessionOwners[sessionID]; ok {
		if previous == ref {
			return false
		}
		delete(d.runSessionIndex[previous], sessionID)
		if oldRun := d.runs[previous.runID]; oldRun != nil {
			if oldRun.TicketSessions == nil {
				oldRun.TicketSessions = map[string][]string{}
			}
			oldRun.TicketSessions[previous.ticketID] = removeSessionID(oldRun.TicketSessions[previous.ticketID], sessionID)
		}
	}
	if d.runSessionIndex[ref] == nil {
		d.runSessionIndex[ref] = map[string]struct{}{}
	}
	d.runSessionIndex[ref][sessionID] = struct{}{}
	d.runSessionOwners[sessionID] = ref
	if !containsSessionID(rs.TicketSessions[ticketID], sessionID) {
		rs.TicketSessions[ticketID] = append(rs.TicketSessions[ticketID], sessionID)
	}
	return true
}

func removeSessionID(ids []string, id string) []string {
	for i, candidate := range ids {
		if candidate == id {
			return append(ids[:i:i], ids[i+1:]...)
		}
	}
	return ids
}

func containsSessionID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// indexRunChildSessionLocked associates a newly created child with the same
// Ticket as its parent and persists the updated index. Callers hold d.mu.
func (d *Daemon) indexRunChildSessionLocked(meta SessionMeta) {
	ref, ok := d.runSessionOwners[meta.ParentID]
	if !ok || !d.indexRunTicketSessionLocked(ref.runID, ref.ticketID, meta.ID) {
		return
	}
	d.persistRun(d.runs[ref.runID])
}

// runTicketSessionIDsLocked returns one Ticket's indexed Session ids. The
// direct Run fields backfill manually constructed and pre-index Run states.
// Callers hold d.mu.
func (d *Daemon) runTicketSessionIDsLocked(rs *runState, ticketID string) []string {
	if rs == nil || ticketID == "" {
		return nil
	}
	if id := rs.Sessions[ticketID]; id != "" {
		d.indexRunTicketSessionLocked(rs.ID, ticketID, id)
	}
	for _, id := range rs.Reviewers[ticketID] {
		if id != "" {
			d.indexRunTicketSessionLocked(rs.ID, ticketID, id)
		}
	}
	ids := append([]string(nil), rs.TicketSessions[ticketID]...)
	sort.Strings(ids)
	return ids
}

// runTicketOwnedSessionIDsLocked returns only the current Worker/Reviewers
// and their descendants. The persistent index also retains older attempts,
// but a pause must not disturb Sessions from a superseded Chain entry.
// Callers hold d.mu.
func runTicketOwnedSessionIDsLocked(d *Daemon, rs *runState, ticketID string) []string {
	ids := d.runTicketSessionIDsLocked(rs, ticketID)
	children := map[string][]string{}
	for _, id := range ids {
		if s := d.sessions[id]; s != nil && s.meta.ParentID != "" {
			children[s.meta.ParentID] = append(children[s.meta.ParentID], id)
		}
	}
	owned := map[string]struct{}{}
	queue := []string{}
	if worker := rs.Sessions[ticketID]; worker != "" {
		owned[worker] = struct{}{}
		queue = append(queue, worker)
	}
	for _, reviewer := range rs.Reviewers[ticketID] {
		if reviewer != "" {
			if _, exists := owned[reviewer]; !exists {
				owned[reviewer] = struct{}{}
				queue = append(queue, reviewer)
			}
		}
	}
	for i := 0; i < len(queue); i++ {
		for _, childID := range children[queue[i]] {
			if _, exists := owned[childID]; !exists {
				owned[childID] = struct{}{}
				queue = append(queue, childID)
			}
		}
	}
	result := make([]string, 0, len(owned))
	for id := range owned {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// rebuildRunSessionIndexLocked restores the persisted index and discovers
// child Sessions from their persisted ParentID metadata. This is a one-time
// startup pass; Run requests use the index instead of scanning d.sessions.
// Callers hold d.mu.
func (d *Daemon) rebuildRunSessionIndexLocked() {
	d.runSessionIndex = map[runTicketRef]map[string]struct{}{}
	d.runSessionOwners = map[string]runTicketRef{}
	dirty := map[string]bool{}
	var queue []string

	for runID, rs := range d.runs {
		if rs.TicketSessions == nil {
			rs.TicketSessions = map[string][]string{}
		}
		for ticketID, id := range rs.Sessions {
			if id == "" {
				continue
			}
			if !containsSessionID(rs.TicketSessions[ticketID], id) {
				dirty[runID] = true
			}
			d.indexRunTicketSessionLocked(runID, ticketID, id)
		}
		for ticketID, reviewers := range rs.Reviewers {
			for _, id := range reviewers {
				if id == "" {
					continue
				}
				if !containsSessionID(rs.TicketSessions[ticketID], id) {
					dirty[runID] = true
				}
				d.indexRunTicketSessionLocked(runID, ticketID, id)
			}
		}
		for ticketID, ids := range rs.TicketSessions {
			for _, id := range ids {
				d.indexRunTicketSessionLocked(runID, ticketID, id)
				queue = append(queue, id)
			}
		}
	}

	children := map[string][]string{}
	for id, sess := range d.sessions {
		if sess.meta.ParentID != "" {
			children[sess.meta.ParentID] = append(children[sess.meta.ParentID], id)
		}
	}
	for parentID, ids := range children {
		sort.Strings(ids)
		children[parentID] = ids
	}
	for i := 0; i < len(queue); i++ {
		parentID := queue[i]
		ref, ok := d.runSessionOwners[parentID]
		if !ok {
			continue
		}
		for _, childID := range children[parentID] {
			if !containsSessionID(d.runs[ref.runID].TicketSessions[ref.ticketID], childID) {
				dirty[ref.runID] = true
			}
			if d.indexRunTicketSessionLocked(ref.runID, ref.ticketID, childID) {
				queue = append(queue, childID)
			}
		}
	}
	for runID := range dirty {
		if rs := d.runs[runID]; rs != nil {
			d.persistRun(rs)
		}
	}
}

// removeRunSessionIndexLocked drops a Session that failed to start. Callers
// hold d.mu.
func (d *Daemon) removeRunSessionIndexLocked(sessionID string) {
	ref, ok := d.runSessionOwners[sessionID]
	if !ok {
		return
	}
	delete(d.runSessionOwners, sessionID)
	delete(d.runSessionIndex[ref], sessionID)
	if rs := d.runs[ref.runID]; rs != nil {
		rs.TicketSessions[ref.ticketID] = removeSessionID(rs.TicketSessions[ref.ticketID], sessionID)
		d.persistRun(rs)
	}
}
