package web

import (
	"net/http"
	"strings"
	"time"
)

// Stuck-state repairs: three ways a Session or Ticket stayed stuck for good
// after a restart or a dead turn, none of which any later pass could undo.
//
//   - A tip-check claim (TipRedTold) is set before the check runs. A restart
//     mid-check persisted the claim with no cached result, so every pass read
//     the tip as already told and never checked, told or reviewed it.
//   - A Reviewer seat whose Session cannot resume (its first turn died before
//     the harness saved a conversation) swallowed every owed prompt: the seat
//     stayed idle, the round never closed and the Worker never got findings.
//   - A Session left at working by a hard kill, outside any resuming Run,
//     stayed working with no process, with nothing to settle it.

// dropLostTipClaims clears a TipRedTold claim that has no red cached result:
// the check never finished (a restart cut it), so the next pass runs it again.
// A claim backed by a red result stays: that tip was told. Green and infra
// results release their claim at runtime, so a stored one is stale too.
func dropLostTipClaims(rs *runState) {
	for ticketID, tip := range rs.TipRedTold {
		res, ok := rs.TipCheckCache[tip]
		if !ok || res.OK || isCheckInfraError(res.Failing) {
			delete(rs.TipRedTold, ticketID)
		}
	}
}

// tellError is a failed send to a Session. code is the resume status when the
// Session could not be brought back, 0 for a send the live Session refused.
type tellError struct {
	code int
	msg  string
}

func (e *tellError) Error() string { return e.msg }

// seatTellFailLimit is how many repair passes in a row (one a minute) may fail
// to reach a Reviewer seat before the Run replaces the seat.
const seatTellFailLimit = 3

// seatUnreachable reports whether err means the Session is gone for good: it
// could not resume. A single-flight "resuming" conflict is transient.
func seatUnreachable(err error) bool {
	te, ok := err.(*tellError)
	if !ok || te.code == 0 {
		return false
	}
	if te.code == http.StatusConflict && strings.Contains(te.msg, "resuming") {
		return false
	}
	return true
}

// noteSeatTell tracks the outcome of one owed-prompt repair send. A seat
// unreachable seatTellFailLimit passes in a row is replaced.
func (d *Daemon) noteSeatTell(runID, ticketID, typ, sess string, err error) {
	key := ticketID + "/" + typ + "/" + sess
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return
	}
	if !seatUnreachable(err) {
		delete(rs.seatTellFails, key)
		d.mu.Unlock()
		return
	}
	if rs.seatTellFails == nil {
		rs.seatTellFails = map[string]int{}
	}
	rs.seatTellFails[key]++
	replace := rs.seatTellFails[key] >= seatTellFailLimit
	d.mu.Unlock()
	if replace {
		d.replaceReviewerSeat(runID, ticketID, typ, sess)
	}
}

// replaceReviewerSeat drops a Reviewer seat whose Session cannot be reached.
// The next driver pass finds the seat missing and starts a fresh Reviewer on
// the current tip, which records into the open round. The old Session ends.
func (d *Daemon) replaceReviewerSeat(runID, ticketID, typ, sess string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok || rs.Reviewers[ticketID][typ] != sess {
		d.mu.Unlock()
		return
	}
	delete(rs.Reviewers[ticketID], typ)
	delete(rs.ReviewTips[ticketID], typ)
	delete(rs.ReviewPending[ticketID], typ)
	delete(rs.ReviewSeen[ticketID], typ)
	delete(rs.FormatReprompt[ticketID], typ)
	delete(rs.ReviewStarted[ticketID], typ)
	delete(rs.seatTellFails, ticketID+"/"+typ+"/"+sess)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	_, _ = d.endSession(sess)
}

// healStrandedSessions settles every Session a hard kill left at working with
// no process, except the Sessions of Runs that resume on this boot: their own
// restart path continues them (continueInterruptedRunSessions). The settle
// logs the restart marker as an idle edge, so a Run that later reads the
// Session still sees it as cut by a restart. It pushes nothing: the turn did
// not finish, and the User did not wait on it this boot.
func (d *Daemon) healStrandedSessions() {
	d.mu.Lock()
	defer d.mu.Unlock()
	resuming := map[string]bool{}
	for runID, rs := range d.runs {
		if !rs.ResumeOnRestart {
			continue
		}
		resuming[runID] = true
		for _, id := range rs.Sessions {
			resuming[id] = true
		}
		for _, byType := range rs.Reviewers {
			for _, id := range byType {
				resuming[id] = true
			}
		}
		for _, ids := range rs.TicketSessions {
			for _, id := range ids {
				resuming[id] = true
			}
		}
	}
	for id, s := range d.sessions {
		if s.live || s.meta.Status != "working" || resuming[id] || resuming[s.meta.ParentID] {
			continue
		}
		e := Event{
			Version:   1,
			Seq:       int64(len(s.events) + 1),
			SessionID: id,
			Kind:      "status",
			Text:      restartInterruptedText,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
		}
		s.events = append(s.events, e)
		s.meta.Status = "idle"
		s.last = e.CreatedAt
		d.persistEvent(s.meta, e)
		d.persistMeta(s.meta)
	}
}
