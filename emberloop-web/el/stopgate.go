package el

import (
	"fmt"
)

// The stop gate is one decision shared by every harness: given what a turn
// did, it says whether an agent may stop. The web Daemon applies it to its
// sessions; only the way a block is enforced differs per harness, never the
// rules.
const (
	// gateMaxBlocks is the number of blocks before the gate lets an agent stop
	// and reports it as stopped without submitting.
	gateMaxBlocks = 3
)

// GateOutcome is what the decision says about one attempted stop.
type GateOutcome int

const (
	// GateAllow lets the stop through: a submit was recorded, the turn ended
	// in a provider error, or the agent is waiting on children's notices.
	GateAllow GateOutcome = iota
	// GateBlock refuses the stop with the reminder.
	GateBlock
	// GatePin refuses the stop and forces the submit tool, where the harness
	// can pin a tool. It is the 3rd attempt, never claude's.
	GatePin
	// GateGiveUp lets the stop through after the last block; the caller sends
	// the "stopped without submitting" notice.
	GateGiveUp
)

// GateInput is everything the decision reads.
type GateInput struct {
	// Submitted is true when a submit was recorded this turn.
	Submitted bool
	// Blocked is how many times this turn has already been blocked.
	Blocked int
	// Failed is true when the turn ended in a provider error.
	Failed bool
	// Pending is true when the agent has auto-backgrounded children that still
	// owe a result; the agent is allowed to wait for their notices.
	Pending bool
	// CanPin is true for a harness that can pin the submit tool.
	CanPin bool
}

// GateDecision is the outcome and, for a block, the reminder to continue with.
type GateDecision struct {
	Outcome  GateOutcome
	Reminder string
}

// DecideGate applies the rules: allow on a submit, a provider error or pending
// children; otherwise block with reminder n of 3, pin on the 3rd attempt only
// where the harness can, and give up after the 3rd block.
func DecideGate(in GateInput) GateDecision {
	switch {
	case in.Submitted, in.Failed, in.Pending:
		return GateDecision{Outcome: GateAllow}
	case in.Blocked >= gateMaxBlocks:
		return GateDecision{Outcome: GateGiveUp}
	case in.Blocked == gateMaxBlocks-1:
		if in.CanPin {
			return GateDecision{Outcome: GatePin, Reminder: gateReminder(gateMaxBlocks) + gatePinned}
		}
		return GateDecision{Outcome: GateBlock, Reminder: gateReminder(gateMaxBlocks) + gateLastBlock}
	default:
		return GateDecision{Outcome: GateBlock, Reminder: gateReminder(in.Blocked + 1)}
	}
}

// gateLastBlock is claude's 3rd block. claude cannot pin a tool, so the 3rd
// block only says it is the last.
const gateLastBlock = " This is the last reminder; call submit now or your turn will be allowed to end."

// gatePinned is the 3rd block where the harness pins the submit tool: the turn
// will not be allowed to end until submit is called.
const gatePinned = " Submit now: submit is pinned as your next tool call."

// gateReminder is the yield reminder: the attempt, why every turn ends with a
// tool call, and the choices in order, first applicable first. The caller adds
// the 3rd attempt's suffix, which differs by what the harness can enforce.
func gateReminder(n int) string {
	return fmt.Sprintf("Your previous turn ended without calling submit (reminder %d of %d). "+
		"Every turn ends with a tool call. Keep working — that is the default, not an order to stop. "+
		"Call submit with a final status only when the work is genuinely complete, "+
		"or with blocked only for a real, nameable blocker. "+
		"Never invent a blocker. Never end a turn with text only.", n, gateMaxBlocks)
}
