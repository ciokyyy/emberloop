package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// restartInterruptedText marks the status Event of a Session whose turn the
// Daemon cut short at shutdown. The turn did not fail: the Session, its
// worktree and its transcript are intact, so the next Daemon continues it.
const restartInterruptedText = "restart: turn interrupted"

const (
	restartWorkerContinue = "The Daemon restarted while you were working. This session and its worktree are unchanged. If your last tool call shows no result, the restart cut it: run it again. Run git status, then continue the ticket where you left off, and submit done when it is finished."
	restartReviewContinue = "The Daemon restarted while you were reviewing. This session is unchanged. If your last tool call shows no result, the restart cut it: run it again. Continue your review of the current tip and submit your verdict with the submit tool."
)

// interruptedByRestartLocked reports whether the Session's last turn was cut
// short by a Daemon restart: a clean Shutdown marked it, or a hard kill left
// its status at working with no live process. Callers hold d.mu.
func interruptedByRestartLocked(s *session) bool {
	if s == nil || s.meta.Status == "ended" {
		return false
	}
	if s.meta.RestartStopped {
		return true
	}
	if s.live {
		return false
	}
	if s.meta.Status == "working" {
		return true
	}
	for i := len(s.events) - 1; i >= 0; i-- {
		if s.events[i].Kind == "status" {
			return s.events[i].Text == restartInterruptedText
		}
	}
	return false
}

// continueInterruptedRunSessions sends every Session of the Run whose turn a
// restart cut short a continue prompt. It resumes the same Session in the same
// worktree (the resume token and the worktree path are on disk), so no
// progress is lost: a Worker or Reviewer picks up where it stopped instead of
// waiting out the stall nudge. A merged Ticket, a paused Ticket and an ended
// Session are left alone. It returns after every send settled.
func (d *Daemon) continueInterruptedRunSessions(runID string) {
	type target struct {
		id, text string
	}
	var targets []target
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil || rs.Status != runStatusRunning {
		d.mu.Unlock()
		return
	}
	add := func(ticketID, sessionID, text string) {
		if sessionID == "" || rs.Merged[ticketID] != "" || rs.TicketPaused[ticketID] {
			return
		}
		s := d.sessions[sessionID]
		if !interruptedByRestartLocked(s) {
			return
		}
		if s.meta.Status == "working" && !s.live {
			// A hard kill left the turn open: close it in the log first so the
			// Run's Working check and the status pill read idle.
			d.appendModeEventLocked(sessionID, "status", restartInterruptedText, "idle", "")
		}
		if s.meta.Carry != "" {
			text += "\n\nMessages that were queued for you when the Daemon stopped:\n" + strings.TrimSpace(s.meta.Carry)
		}
		s.meta.RestartStopped = false
		s.meta.Carry = ""
		d.persistMeta(s.meta)
		targets = append(targets, target{sessionID, text})
	}
	tickets := make([]string, 0, len(rs.Sessions))
	for ticketID := range rs.Sessions {
		tickets = append(tickets, ticketID)
	}
	for ticketID := range rs.Reviewers {
		if _, ok := rs.Sessions[ticketID]; !ok {
			tickets = append(tickets, ticketID)
		}
	}
	sort.Strings(tickets)
	for _, ticketID := range tickets {
		add(ticketID, rs.Sessions[ticketID], restartWorkerContinue)
		for _, sessionID := range rs.Reviewers[ticketID] {
			add(ticketID, sessionID, restartReviewContinue)
		}
	}
	d.mu.Unlock()
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			if code, _ := d.resumeReleased(t.id); code != 0 {
				return
			}
			_, _ = d.routeLiveOpt(t.id, "steer", t.text, nil, nil, true, systemSender)
		}(t)
	}
	wg.Wait()
	if len(targets) > 0 {
		d.snapshotAndPersist(runID)
	}
}

// markShutdownInterruptedLocked ends the open turn of a live Session the
// Daemon is about to close. The log reads idle with the restart marker, and a
// later turn error from the closing process stays an idle edge (see
// appendEventExtraLocked), so the next Daemon never reads a restart as a
// failure that advances a Chain. Callers hold d.mu.
func (d *Daemon) markShutdownInterruptedLocked(id string) {
	s := d.sessions[id]
	if s == nil || s.meta.Status != "working" {
		return
	}
	d.appendModeEventLocked(id, "status", restartInterruptedText, "idle", "")
}

// armRestartStopsLocked stops the Run Worker and Reviewer turns of every Run a
// restart request paused, without waiting for the turns to end. The Run is
// already paused, so the driver starts nothing and sends no nudge or re-prompt;
// each busy Session stops once its last in-flight tool call finished: now when
// no tool runs, else on that tool's terminal update (maybeStopForRestart). A
// turn cut between tool_use and tool_result could leave the resumed transcript
// invalid, so the stop never fires with a tool open. Subagents and other
// Sessions are not touched: the drain still waits for them. Callers hold d.mu.
func (d *Daemon) armRestartStopsLocked() {
	for _, rs := range d.runs {
		if rs.Status == runStatusDone || rs.Status == runStatusAbandoned {
			continue
		}
		ids := make([]string, 0, len(rs.Sessions))
		for ticketID, id := range rs.Sessions {
			if rs.Merged[ticketID] == "" {
				ids = append(ids, id)
			}
		}
		for ticketID, byType := range rs.Reviewers {
			if rs.Merged[ticketID] != "" {
				continue
			}
			for _, id := range byType {
				ids = append(ids, id)
			}
		}
		for _, id := range ids {
			s := d.sessions[id]
			if s == nil || s.meta.Status == "ended" {
				continue
			}
			if d.stopRestart == nil {
				d.stopRestart = map[string]bool{}
			}
			d.stopRestart[id] = true
			if s.busy && len(s.openTools) == 0 {
				go d.fireRestartStop(id)
			}
		}
	}
}

// maybeStopForRestart fires an armed restart stop when a tool call finishes
// and no other tool call is in flight. Callers hold no lock.
func (d *Daemon) maybeStopForRestart(id string, e Event) {
	if e.Kind != "tool_call_updated" || !toolCallDone(e.Text) {
		return
	}
	d.mu.Lock()
	s := d.sessions[id]
	fire := d.stopRestart[id] && s != nil && s.busy && len(s.openTools) == 0 && !s.meta.RestartStopped
	d.mu.Unlock()
	if fire {
		go d.fireRestartStop(id)
	}
}

// fireRestartStop interrupts one armed Session at a tool-call boundary. It
// marks the Session RestartStopped first (the next Daemon continues it) and
// sets gateSkip, so the submit gate does not remind a turn that ended without
// a submit. A Session that already stopped or ended is left alone.
func (d *Daemon) fireRestartStop(id string) {
	d.mu.Lock()
	s := d.sessions[id]
	if s == nil || !d.stopRestart[id] || !s.busy || s.meta.Status == "ended" || s.meta.RestartStopped || len(s.openTools) > 0 {
		d.mu.Unlock()
		return
	}
	s.meta.RestartStopped = true
	s.gateSkip = true
	d.persistMeta(s.meta)
	ib, _ := d.backendForLocked(s.meta.Harness).(interruptBackend)
	d.mu.Unlock()
	if ib != nil {
		ib.Interrupt(id)
	}
}

// HasRunningRuns reports whether any Run is driving Workers right now.
func (d *Daemon) HasRunningRuns() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, rs := range d.runs {
		if rs.Status == runStatusRunning {
			return true
		}
	}
	return false
}

const (
	// bootLoopWindow and bootLoopMax bound automatic Run resume: a Daemon
	// that boots bootLoopMax times inside bootLoopWindow is crash-looping, so
	// its Runs stay paused (still marked ResumeOnRestart) for a human instead
	// of resuming into the same crash on every boot.
	bootLoopWindow = 5 * time.Minute
	bootLoopMax    = 4
)

// bootLoopTripped records this boot and reports whether the Daemon booted
// bootLoopMax times within bootLoopWindow, this boot included. The record is
// <state-dir>/enderbloom/boots.json; an unreadable file reads as no history.
func (d *Daemon) bootLoopTripped() bool {
	path := filepath.Join(d.stateDir, "enderbloom", "boots.json")
	var boots []time.Time
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &boots)
	}
	now := d.now()
	kept := boots[:0]
	for _, b := range boots {
		if now.Sub(b) < bootLoopWindow {
			kept = append(kept, b)
		}
	}
	kept = append(kept, now)
	if raw, err := json.Marshal(kept); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, raw, 0o644)
	}
	return len(kept) >= bootLoopMax
}

// logBootLoop explains a tripped guard in the restart log.
func (d *Daemon) logBootLoop() {
	d.LogRestart("boot loop: %d boots within %s, not resuming Runs automatically; resume them by hand once the crash is fixed", bootLoopMax, bootLoopWindow)
}
