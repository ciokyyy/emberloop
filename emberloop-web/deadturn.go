package web

import (
	"fmt"
	"strings"
	"time"
)

// Dead turns: a gated turn (a spawned child or Run Worker turn under the
// submit gate) that ends fast with no assistant text, no tool call and no
// submit. Seen in the field when the context window is full: the provider
// returns nothing, the turn settles idle in about a second, and the gate
// answers with "ended without calling submit" reminders that each die the
// same way, burning turns until the gate gives up. A dead turn gets no
// reminder: usage near or over the window arms an automatic compress instead,
// and anything else is recorded as a turn failure so the Run Chain classifier
// (runs_chain.go, failures.go) sees it and either advances or surfaces it.

// deadTurnMaxDuration bounds a dead turn: working to settle. A longer turn
// with no output is not dead: it may hold a long tool call (which always
// logs tool_call_started first) or a stalled stream the pig watchdog already
// owns, so those keep today's gate behavior.
const deadTurnMaxDuration = 30 * time.Second

// deadTurnSince reports whether the turn after mark was dead: a working to
// idle settle with no submit, no tool call started, no non-blank assistant
// text and no error, all within deadTurnMaxDuration. The turn's prompt sits
// at or before mark (senders append it before the turn launches), so the
// working edge opens the window, not the prompt. It reads the Event log
// only, so tests build Events directly and the gate path needs no clock.
func deadTurnSince(events []Event, mark int64) bool {
	var working *Event
	lastStatus := ""
	for i := range events {
		e := &events[i]
		if e.Seq <= mark {
			continue
		}
		switch e.Kind {
		case "submit":
			return false
		case "tool_call_started":
			return false
		case "assistant_chunk":
			if strings.TrimSpace(e.Text) != "" {
				return false
			}
		case "error":
			return false
		case "status":
			if e.Status == "working" && working == nil {
				working = e
			}
			if e.Status != "" {
				lastStatus = e.Status
			}
		}
	}
	if working == nil || lastStatus != "idle" {
		return false
	}
	var last *Event
	for i := range events {
		if events[i].Seq > mark {
			last = &events[i]
		}
	}
	if last == nil {
		return false
	}
	return last.CreatedAt.Sub(working.CreatedAt) <= deadTurnMaxDuration
}

// deadTurnErrorText is the synthetic turn failure a dead gated turn records
// when usage is below the compress threshold. The wording stays clear of the
// Chain-advancing classes (failures.go): a dead turn is not quota, credit,
// auth or session-limit evidence, so the Chain surfaces it as today instead
// of falling over to the next entry.
func deadTurnErrorText(used, size int64, haveUsage bool) string {
	if haveUsage && size > 0 {
		return fmt.Sprintf("dead turn: no assistant text, tool call or submit before the turn ended (usage %d of %d)", used, size)
	}
	return "dead turn: no assistant text, tool call or submit before the turn ended"
}

// failDeadTurn records a dead gated turn as a turn failure so the Chain
// classifier sees it, and reports whether it did. It returns false when the
// turn after mark was not dead, leaving the gate to remind as today.
func (d *Daemon) failDeadTurn(id string, mark int64) bool {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return false
	}
	events := append([]Event(nil), s.events...)
	d.mu.Unlock()
	if !deadTurnSince(events, mark) {
		return false
	}
	used, size, haveUsage := usageOf(events)
	d.appendEvent(id, "error", deadTurnErrorText(used, size, haveUsage), "")
	d.appendEvent(id, "status", "", "failed")
	return true
}
