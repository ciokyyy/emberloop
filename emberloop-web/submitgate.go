package web

import (
	"strings"

	el "example.com/emberloop"
)

// The Daemon side of the shared stop gate. While el.WebSubmitEnabled holds, a
// spawned child (worker or scout) settles only through submit: after its
// backend turn returns, runTurn calls enforceSubmitGate, which re-prompts a
// child that ended the turn without a submit with the shared decision's
// reminder (el.DecideGate, the same rules `el pig-gate` enforces for the
// herdr pig extension), for the bounded number of attempts the decision
// defines, then records the give-up as a stopped submit so the Group still
// delivers exactly once. With the flag off (the default) the gate is
// skipped entirely — no reminders, no give-up notice — and the child's Group
// result is delivered on settle from its submit text, else its last
// assistant message (see groupResultText). Top-level Sessions and Run
// workers and reviewers (a parent but no Group) are never gated; failed
// turns go through the decision, which allows them, when gated; ended
// Sessions and user-stopped turns (gateSkip) skip it.

// submitPinBackend is the optional harness seam for the gate's pinned
// attempt: forcing submit as the next tool call. No Backend implements it:
// the pig SDK drives turns through Session.Send, which takes only a prompt
// and exposes no tool-choice parameter, and claude and fake have no pin
// either, so a pin degrades to a plain reminder prompt carrying the pinned
// text. When a harness gains the seam, it flips CanPin on with it and the
// decision starts pinning.
type submitPinBackend interface {
	PinSubmitTool(sessionID string) bool
}

// gateBaseline snapshots what the submit gate needs before a turn runs: the
// event sequence submits must come after, the harness for the pin decision,
// and whether this Session is a spawned group member at all.
func (d *Daemon) gateBaseline(id string) (mark int64, harness string, gated bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return 0, "", false
	}
	if len(s.events) > 0 {
		mark = s.events[len(s.events)-1].Seq
	}
	if s.meta.ParentID == "" || (s.meta.GroupID == "" && !d.isRunWorkerLocked(id)) {
		return mark, "", false
	}
	return mark, s.meta.Harness, true
}

// isRunWorkerLocked reports a Run ticket Worker Session (not a Reviewer);
// callers hold d.mu. Run Workers are always gated, flag or not: a Worker
// that ends a turn without submit stalls the whole Run.
func (d *Daemon) isRunWorkerLocked(id string) bool {
	s, ok := d.sessions[id]
	if !ok {
		return false
	}
	rs, ok := d.runs[s.meta.ParentID]
	if !ok {
		return false
	}
	for _, sid := range rs.Sessions {
		if sid == id {
			return true
		}
	}
	return false
}

// gateSubmittedSince reports a submit Event after mark: the child submitted
// this turn, whether through the submit route or straight into the log.
func (d *Daemon) gateSubmittedSince(id string, mark int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return false
	}
	for _, e := range s.events {
		if e.Seq > mark && e.Kind == "submit" {
			return true
		}
	}
	return false
}

// takeGateSkip reports and clears a user stop that landed on this turn. An
// interrupt or cancel is deliberate: the gate must not remind it.
func (d *Daemon) takeGateSkip(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return true
	}
	skip := s.gateSkip
	s.gateSkip = false
	return skip
}

// gateTurnStatus reports the turn's settle status for the shared decision.
func (d *Daemon) gateTurnStatus(id string) (status string, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return "", false
	}
	return s.meta.Status, true
}

// enforceSubmitGate re-prompts a spawned child whose turn ended without a
// submit, with the shared decision's reminder, until it submits, fails, is
// stopped, or the decision gives up. It runs on the turn goroutine while the
// Session stays busy, so queued steers wait for it like any other turn.
// While el.WebSubmitEnabled is false it returns at once: no reminders, no
// give-up notice.
func (d *Daemon) enforceSubmitGate(id string, be Backend, mark int64, harness string) {
	d.mu.Lock()
	runWorker := d.isRunWorkerLocked(id)
	d.mu.Unlock()
	if !el.WebSubmitEnabled && !runWorker {
		return
	}
	// Pig is the harness that can pin (as in core); the SDK seam decides
	// whether the pin actually forces. claude and fake never pin.
	canPin := harness == "pig"
	for blocked := 0; ; {
		if d.takeGateSkip(id) {
			return
		}
		status, ok := d.gateTurnStatus(id)
		if !ok || status == "ended" {
			return
		}
		if d.gateSubmittedSince(id, mark) {
			return
		}
		dec := el.DecideGate(el.GateInput{Blocked: blocked, Failed: status == "failed", CanPin: canPin})
		switch dec.Outcome {
		case el.GateAllow:
			return
		case el.GateGiveUp:
			// A Run Worker records no stopped submit: the driver's
			// busy-aware stall nudge keeps chasing it.
			if !runWorker {
				d.giveUpWithoutSubmit(id)
			}
			return
		case el.GatePin:
			// A pin forces submit as the next tool call where the harness
			// allows it; no Backend pins, so the reminder runs as a plain
			// prompt either way.
			if pb, ok := be.(submitPinBackend); ok {
				pb.PinSubmitTool(id)
			}
			d.remindChild(id, be, dec.Reminder)
			blocked++
		case el.GateBlock:
			d.remindChild(id, be, dec.Reminder)
			blocked++
		default:
			return
		}
	}
}

// remindChild logs one gate reminder as the turn's next prompt and runs it.
func (d *Daemon) remindChild(id string, be Backend, reminder string) {
	d.appendPromptEvent(id, reminder, "", 0, nil)
	promptBackend(be, id, reminder, nil, func(kind, text, status string) {
		d.appendEvent(id, kind, text, status)
	})
}

// giveUpWithoutSubmit records a child that never submitted within the gate's
// bound: a stopped submit, the way the submit route would record one, so the
// Group's single delivery still carries it (marked) and never hangs.
func (d *Daemon) giveUpWithoutSubmit(id string) {
	report := "stopped without submitting: " + d.stoppedExcerpt(id)
	d.mu.Lock()
	rec := d.submits[id]
	if rec == nil {
		rec = &submitRec{}
		d.submits[id] = rec
	}
	rec.Count++
	rec.Status = "stopped"
	rec.Report = report
	d.mu.Unlock()
	d.appendEvent(id, "submit", "stopped", "")
}

// stoppedExcerpt is the child's last reply as one bounded line for the
// give-up notice, "(no message)" when it said nothing. The bound mirrors
// core's stop excerpt.
func (d *Daemon) stoppedExcerpt(id string) string {
	const limit = 200 // core's stopExcerptLimit
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return "(no message)"
	}
	start := 0
	for i := len(s.events) - 1; i >= 0; i-- {
		if s.events[i].Kind == "prompt" {
			start = i + 1
			break
		}
	}
	var b strings.Builder
	for _, e := range s.events[start:] {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	text := strings.Join(strings.Fields(b.String()), " ")
	if text == "" {
		return "(no message)"
	}
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}
