package web

import (
	"encoding/json"
	"strings"
)

// Stop-after-submit ends a Run Worker or Reviewer turn as soon as its
// terminal submit is accepted, so the agent does not stream a summary
// afterwards. The submit Event itself is the trigger: every accepted submit
// (Worker done/blocked and Reviewer pass/changes through the submit route)
// is stored as a submit Event, while every refusal (gate refusal, empty ran,
// missing Disposition and friends) answers 4xx and stores nothing, so a
// refused submit never stops its turn. Non-terminal Worker statuses and
// Sessions that are no Run Worker's or Reviewer's never arm it.
//
// The interrupt waits for the submit tool's terminal update on real
// harnesses: the Daemon arms the Session when the submit Event lands and
// interrupts when the submit tool_call_updated completed/failed Event lands,
// i.e. after the tool result reached the agent. Cancelling between tool_use
// and tool_result could leave the resumed transcript invalid; arming after
// the submit and firing on the terminal update keeps that window closed by
// construction. The fake Backend emits no tool_call Events, so its submits
// interrupt at once: its tool result is already in the log.
//
// The stop keeps the turn's queue and sets no gateSkip: the submit Event
// already satisfies the submit gate, and queued follow-ups and re-prompts
// still drain through finishTurn. The turn ends idle, never failed, so no
// Chain advance, no retry and no stall nudge follows. It is a Run setting,
// on unless the Run opts out with NoStopAfterSubmit.

// runStopsAfterSubmit reports whether one Run stops Worker and Reviewer
// turns after an accepted terminal submit. Old run files without the opt-out
// read as enabled.
func runStopsAfterSubmit(rs *runState) bool {
	return rs != nil && !rs.NoStopAfterSubmit
}

// terminalWorkerSubmit reports whether a Worker submit Event text is a
// terminal status: done or blocked. Anything else keeps working.
func terminalWorkerSubmit(text string) bool {
	switch strings.TrimSpace(text) {
	case "done", "blocked":
		return true
	}
	return false
}

// acceptedReviewerSubmit parses one reviewer submit Event payload and reports
// whether it is an accepted terminal submit: a pass or changes verdict that
// passes the tool rules. Refused payloads (gate refusal, empty ran, missing
// Disposition and friends) return false: the agent must keep working.
func acceptedReviewerSubmit(text string) bool {
	var sub ReviewerSubmit
	if err := json.Unmarshal([]byte(text), &sub); err != nil {
		return false
	}
	if v := strings.ToLower(strings.TrimSpace(sub.Verdict)); v != "pass" && v != "changes" {
		return false
	}
	return ValidateReviewerSubmit(sub) == nil
}

// stopSubmitToolDone reports whether a tool_call_updated Event text carries
// the submit tool's terminal update. ToolCall.Text splits an
// mcp__el__submit name, so both harnesses match on submit.
func stopSubmitToolDone(text string) bool {
	var tc ToolCall
	if err := json.Unmarshal([]byte(text), &tc); err != nil {
		return false
	}
	if !strings.EqualFold(tc.Name, "submit") {
		return false
	}
	return tc.Status == "completed" || tc.Status == "failed"
}

// maybeStopAfterSubmit reacts to one freshly stored Event: it arms or fires
// the stop on submit Events, fires an armed stop on the submit tool's
// terminal update, and disarms when the turn settles. Callers hold no lock;
// the interrupt itself always runs without it.
func (d *Daemon) maybeStopAfterSubmit(id string, e Event) {
	var be Backend
	fire := false
	switch e.Kind {
	case "submit":
		d.mu.Lock()
		be, fire = d.armStopSubmitLocked(id, e.Text)
		d.mu.Unlock()
	case "tool_call_updated":
		if !toolCallDone(e.Text) || !stopSubmitToolDone(e.Text) {
			return
		}
		d.mu.Lock()
		be, fire = d.fireStopSubmitLocked(id)
		d.mu.Unlock()
	case "status":
		if e.Status != "idle" && e.Status != "failed" {
			return
		}
		d.mu.Lock()
		if d.stopSubmit != nil {
			delete(d.stopSubmit, id)
		}
		d.mu.Unlock()
		return
	default:
		return
	}
	if !fire {
		return
	}
	if ib, ok := be.(interruptBackend); ok {
		ib.Interrupt(id)
	}
}

// armStopSubmitLocked records one submit Event for a busy Run Worker or
// Reviewer and reports the backend plus whether to interrupt now. The fake
// harness fires at once; any other harness arms until the submit tool's
// terminal update lands. Anything that is no accepted terminal submit, no
// busy Run turn, or an opted-out Run reports false. Callers hold d.mu.
func (d *Daemon) armStopSubmitLocked(id, text string) (be Backend, fire bool) {
	s, ok := d.sessions[id]
	if !ok || !s.busy || s.meta.Status == "ended" {
		return nil, false
	}
	runID, _ := d.findWorkerLocked(id)
	terminal := terminalWorkerSubmit(text)
	if runID == "" {
		var typ string
		runID, _, typ = d.findReviewerLocked(id)
		if typ == "" {
			// Not a Run Worker or Reviewer (web/spec Session): never stop.
			return nil, false
		}
		terminal = acceptedReviewerSubmit(text)
	}
	if !terminal {
		return nil, false
	}
	if !runStopsAfterSubmit(d.runs[runID]) {
		return nil, false
	}
	be = d.backendForLocked(s.meta.Harness)
	if s.meta.Harness == "fake" {
		return be, true
	}
	if d.stopSubmit == nil {
		d.stopSubmit = map[string]bool{}
	}
	d.stopSubmit[id] = true
	return be, false
}

// fireStopSubmitLocked consumes one armed stop and reports the backend when
// the Session still runs. Callers hold d.mu.
func (d *Daemon) fireStopSubmitLocked(id string) (be Backend, fire bool) {
	if d.stopSubmit == nil || !d.stopSubmit[id] {
		return nil, false
	}
	delete(d.stopSubmit, id)
	s, ok := d.sessions[id]
	if !ok || !s.busy || s.meta.Status == "ended" {
		return nil, false
	}
	return d.backendForLocked(s.meta.Harness), true
}
