package memory

import (
	"fmt"
	"strings"
)

// GroundEvent is the Event-lookup result a grounding check runs against:
// whether eventID named a stored Event, that Event's sender ("user" or
// "system") and its full text. The struct keeps the three tied to one
// Event, so a wrong join at the call site cannot pass silently. It carries
// plain strings so the future miner builds it from an event log line
// without importing the Daemon (which would cycle back into this package).
type GroundEvent struct {
	Found  bool
	Sender string
	Text   string
}

// CheckGrounding verifies that quote is grounded in an Event: the Event must
// exist, must have been sent by a User (sender=user), and must contain the
// quote verbatim. Anything else is rejected without a model, per the spec's
// grounding rule. The miner calls this before an observation reaches the
// bank; the caller redacts the quote first (see Redact).
func CheckGrounding(eventID, quote string, ev GroundEvent) error {
	switch {
	case !ev.Found:
		return fmt.Errorf("%w: grounding: event %q not found", ErrNotFound, eventID)
	case ev.Sender != "user":
		return fmt.Errorf("%w: grounding: event %q sender %q, want user", ErrInvalid, eventID, ev.Sender)
	case quote == "":
		return fmt.Errorf("%w: grounding: quote empty", ErrInvalid)
	case !strings.Contains(ev.Text, quote):
		return fmt.Errorf("%w: grounding: quote is not a substring of event %q", ErrInvalid, eventID)
	}
	return nil
}
