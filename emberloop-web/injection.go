package web

import "encoding/json"

// InjectedRule names one Rule the memory block carried: its ID and version.
// The injection Event lists these so a Session replay can resolve exactly
// what the agent saw.
type InjectedRule struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// InjectionEvent is the JSON Text of an "injection" Event. The Daemon
// appends one on every injection (Profile, recall or both), with
// sender=system. ProfileVersion is the injected Profile version, 0 when the
// block carried no Profile. Rules lists the injected Rule versions, empty
// when the block carried none.
type InjectionEvent struct {
	ProfileVersion int            `json:"profileVersion,omitempty"`
	Rules          []InjectedRule `json:"rules"`
}

func (e InjectionEvent) text() string {
	if e.Rules == nil {
		e.Rules = []InjectedRule{}
	}
	b, _ := json.Marshal(e)
	return string(b)
}

// parseInjectionEvent decodes the Text of an injection Event.
func parseInjectionEvent(text string) (InjectionEvent, bool) {
	var e InjectionEvent
	if err := json.Unmarshal([]byte(text), &e); err != nil {
		return InjectionEvent{}, false
	}
	if e.Rules == nil {
		e.Rules = []InjectedRule{}
	}
	return e, true
}

// MinerReadable reports whether the miner may read e: only a prompt a
// signed-in User sent (plain, steer or follow-up by Mode). Injection Events,
// Handoffs, grouped Subagent results, sudo results and every other
// Daemon-made prompt carry sender=system and are never mined. Nothing
// injected is ever recorded as a user message.
func MinerReadable(e Event) bool {
	return e.Kind == "prompt" && e.Sender == senderUser
}

// FilterMinerEvents returns the Events of a Session the miner may read:
// only prompts a signed-in User sent. Injection Events, Handoffs, grouped
// results, sudo results and every non-prompt Event are dropped. The ticket-19
// miner reads Sessions through this filter, never the raw log.
func FilterMinerEvents(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if MinerReadable(e) {
			out = append(out, e)
		}
	}
	return out
}

// appendInjectionEvent stores one injection Event for the Session's latest
// injection. It is sender=system, never a user message.
func (d *Daemon) appendInjectionEvent(id string, profile int, rules []InjectedRule) Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.appendEventExtraLocked(id, "injection", InjectionEvent{ProfileVersion: profile, Rules: rules}.text(), "", "", 0, nil, func(e *Event) {
		e.Sender = senderSystem
	})
}
