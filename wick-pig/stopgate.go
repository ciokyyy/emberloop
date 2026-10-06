package wick

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The stop gate is one decision shared by every harness: given what a turn did,
// it says whether an agent may stop. claude drives it through a `Stop` hook
// that runs the stop-gate subcommand; pig drives it from its extension (ticket
// 04). Only the way a block is enforced differs, never the rules.
const (
	// gateMaxBlocks is the number of blocks before the gate lets an agent stop
	// and reports it as stopped without submitting.
	gateMaxBlocks = 3
	// stopExcerptLimit bounds the last-message excerpt in a stopped notice.
	stopExcerptLimit = 200
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

// gateReminder is omp's yield reminder: the attempt, why every turn ends with a
// tool call, and the choices in order, first applicable first. The caller adds
// the 3rd attempt's suffix, which differs by what the harness can enforce.
func gateReminder(n int) string {
	return fmt.Sprintf("Your previous turn ended without calling submit (reminder %d of %d). "+
		"Every turn ends with a tool call. Keep working — that is the default, not an order to stop. "+
		"Call submit with a final status only when the work is genuinely complete, "+
		"or with blocked only for a real, nameable blocker. "+
		"Never invent a blocker. Never end a turn with text only.", n, gateMaxBlocks)
}

// StopHook is the subset of claude's `Stop` hook JSON the gate reads.
type StopHook struct {
	// StopHookActive is false on a turn's first stop and true on a stop that
	// follows a block; a false stop starts the attempt count over.
	StopHookActive bool `json:"stop_hook_active"`
	// TranscriptPath is the session transcript, read for a provider error and,
	// failing last_assistant_message, the last message.
	TranscriptPath string `json:"transcript_path"`
	// LastAssistantMessage is the text of the message the turn ended with.
	LastAssistantMessage string `json:"last_assistant_message"`
}

// hookBlock is the block JSON a Stop hook writes to refuse a stop.
type hookBlock struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// RunStopGate serves `wick-pig stop-gate`: claude's Stop hook. It reads the
// hook JSON on in, asks the shared decision, and either writes the block JSON
// to out or lets the stop through. A stop it gives up on writes nothing and
// sends the parent a "stopped without submitting" notice the way that agent's
// submit would go.
func RunStopGate(in io.Reader, out io.Writer) error {
	var hook StopHook
	if err := json.NewDecoder(in).Decode(&hook); err != nil && err != io.EOF {
		return fmt.Errorf("stop-gate: %w", err)
	}
	// Only a spawned agent has a submit tool to gate: a top-level or one-shot
	// read-only session has no agent type, so nothing to gate. Every session
	// now carries an agent id, so the id alone no longer marks one.
	if os.Getenv("WICK_AGENT_TYPE") == "" {
		return nil
	}
	id := os.Getenv("WICK_AGENT_ID")
	if id == "" {
		return nil
	}
	failed := false
	if hook.TranscriptPath != "" {
		state, _ := (claudeHarness{}).TurnState(hook.TranscriptPath)
		failed = state == TurnFailed
	}
	d, err := GateStop(id, !hook.StopHookActive, failed, (claudeHarness{}).Gate().CanPin(),
		func() string { return stopExcerpt(hook) })
	if err != nil {
		return err
	}
	switch d.Outcome {
	case GateAllow, GateGiveUp:
		// An allowed stop and a give-up both let the turn end: nothing is
		// written, and a give-up has already sent its notice.
		return nil
	default:
		return json.NewEncoder(out).Encode(hookBlock{Decision: "block", Reason: d.Reminder})
	}
}

// PigGateInput is one settle attempt as the pig extension reports it: whether a
// new turn began since the last attempt, whether the turn ended in a provider
// error, and the last message of the turn, which a give-up quotes.
type PigGateInput struct {
	Fresh       bool   `json:"fresh"`
	Failed      bool   `json:"failed"`
	LastMessage string `json:"last_message"`
}

// PigGateDecision is the decision as the pig extension enforces it: what to do,
// and for a block or a pin the reminder to continue the agent with.
type PigGateDecision struct {
	Outcome  string `json:"outcome"`
	Reminder string `json:"reminder,omitempty"`
}

// The outcomes the pig extension knows. A pin also blocks: the reminder and
// the forced submit tool are one attempt.
const (
	pigGateAllow  = "allow"
	pigGateBlock  = "block"
	pigGatePin    = "pin"
	pigGateGiveUp = "giveup"
)

// RunPigGate serves `wick-pig pig-gate <event>`: the pig extension's half of
// the shared stop gate. The event is one attempted settle as JSON; the answer
// comes back as JSON for the extension to enforce, because only the extension
// can hold a pig agent at its settle. A give-up is delivered from here, the
// same way the claude hook delivers it.
func RunPigGate(event string, out io.Writer) error {
	var in PigGateInput
	if err := json.Unmarshal([]byte(event), &in); err != nil {
		return fmt.Errorf("pig-gate: %w", err)
	}
	// Only a spawned agent has a submit tool to gate: a top-level or one-shot
	// read-only session has no agent type, so nothing to gate. Every session
	// now carries an agent id, so the id alone no longer marks one.
	if os.Getenv("WICK_AGENT_TYPE") == "" {
		return json.NewEncoder(out).Encode(PigGateDecision{Outcome: pigGateAllow})
	}
	id := os.Getenv("WICK_AGENT_ID")
	if id == "" {
		return json.NewEncoder(out).Encode(PigGateDecision{Outcome: pigGateAllow})
	}
	d, err := GateStop(id, in.Fresh, in.Failed, (pigHarness{}).Gate().CanPin(),
		func() string { return excerpt(in.LastMessage, nil) })
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(pigDecision(d))
}

// pigDecision names the decision for the extension. A pin keeps its reminder:
// the extension continues the agent and forces the submit tool at once.
func pigDecision(d GateDecision) PigGateDecision {
	switch d.Outcome {
	case GateBlock:
		return PigGateDecision{Outcome: pigGateBlock, Reminder: d.Reminder}
	case GatePin:
		return PigGateDecision{Outcome: pigGatePin, Reminder: d.Reminder}
	case GateGiveUp:
		return PigGateDecision{Outcome: pigGateGiveUp}
	default:
		return PigGateDecision{Outcome: pigGateAllow}
	}
}

// GateStop asks the shared decision for one agent at one attempted stop and
// applies the shared bookkeeping: a fresh turn starts the count over, the
// attempt count is stored, and a give-up is recorded and, for a non-blocking
// agent, delivered as a notice. lastMessage is read only on a give-up. The
// caller enforces what the decision leaves: claude writes block JSON, pig
// continues the agent.
func GateStop(id string, fresh, failed, canPin bool, lastMessage func() string) (GateDecision, error) {
	// stop_hook_active false is a fresh turn (a tell, a delivered notice), so
	// the attempt count starts over.
	if fresh {
		if err := writeGateBlocked(id, 0); err != nil {
			return GateDecision{}, err
		}
	}
	submitted := false
	if s, ok, err := ReadAgentSubmit(id); err != nil {
		return GateDecision{}, err
	} else if ok {
		submitted = s.ThisTurn
	}
	blocked := readGateBlocked(id)
	d := DecideGate(GateInput{
		Submitted: submitted,
		Blocked:   blocked,
		Failed:    failed,
		Pending:   len(PendingChildren(id)) > 0,
		CanPin:    canPin,
	})
	switch d.Outcome {
	case GateAllow:
		// The submit that allowed this stop belongs to this turn only; clear it
		// so the next turn's first stop blocks afresh.
		if err := ClearAgentSubmitThisTurn(id); err != nil {
			return d, err
		}
		return d, writeGateBlocked(id, 0)
	case GateGiveUp:
		if err := writeGateBlocked(id, 0); err != nil {
			return d, err
		}
		return d, giveUpWithoutSubmit(id, lastMessage())
	default:
		return d, writeGateBlocked(id, blocked+1)
	}
}

// stopExcerpt is the agent's last message, one flattened line and bounded, for
// the "stopped without submitting" notice. The hook field is preferred; a hook
// that omits it falls back to the transcript.
func stopExcerpt(hook StopHook) string {
	return excerpt(hook.LastAssistantMessage, func() string {
		if hook.TranscriptPath == "" {
			return ""
		}
		text, _ := (claudeHarness{}).LastReply(hook.TranscriptPath)
		return text
	})
}

// excerpt flattens one last message to a single bounded line. fallback is read
// only when the message itself is empty, and may be nil.
func excerpt(text string, fallback func() string) string {
	if strings.TrimSpace(text) == "" && fallback != nil {
		text = fallback()
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "(no message)"
	}
	runes := []rune(text)
	if len(runes) > stopExcerptLimit {
		return string(runes[:stopExcerptLimit]) + "…"
	}
	return text
}

// giveUpWithoutSubmit lets the stop through after the last block and reports
// the agent to its parent the way a submit would: a member of an open group
// joins its group's single notice, a member whose group already delivered or
// any other agent with Deliver set is delivered as an individual notice.
func giveUpWithoutSubmit(id, excerpt string) error {
	typ := os.Getenv("WICK_AGENT_TYPE")
	if typ == "" {
		st, _, err := ReadAgentState(id)
		if err != nil {
			return err
		}
		typ = st.Type
	}
	s := AgentSubmit{
		AgentID: id,
		Type:    typ,
		Status:  "stopped",
		Report:  "stopped without submitting: " + excerpt,
		Stopped: true,
		At:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteAgentSubmit(s); err != nil {
		return err
	}
	if st, ok, err := ReadAgentState(id); err != nil {
		return err
	} else if ok && st.GroupID != "" {
		if delivered, err := spawnGroupDelivered(st.GroupID); err != nil {
			return err
		} else if !delivered {
			if st.Deliver {
				return maybeDeliverGroup(id)
			}
			return nil
		}
	}
	// Who owns the result is decided under the child's lock, after the submit
	// exists, exactly as submitRun does for an agent outside any open group.
	st, deliver, err := submitDelivers(id)
	if err != nil {
		return err
	}
	if deliver {
		if err := deliverToParent(st, stoppedNotice(typ, id, excerpt)); err != nil {
			return err
		}
		closeDeliveredScout(deliverExec, id)
	}
	return nil
}

// stoppedNotice is the one-line system notice a give-up sends, the shape of a
// submit notice but naming what happened.
func stoppedNotice(typ, id, excerpt string) string {
	return fmt.Sprintf("<system-notice> %s %s stopped without submitting: %s</system-notice>", typ, id, excerpt)
}

// gateStateDir holds one file per agent, its stop-gate attempt count for the
// current turn.
func gateStateDir() string { return filepath.Join(filepath.Dir(statePath), "gates") }

func gateStatePath(id string) string { return filepath.Join(gateStateDir(), id+".json") }

type gateState struct {
	Blocked int `json:"blocked"`
}

func readGateBlocked(id string) int {
	raw, err := os.ReadFile(gateStatePath(id))
	if err != nil {
		return 0
	}
	var g gateState
	if json.Unmarshal(raw, &g) != nil {
		return 0
	}
	return g.Blocked
}

func writeGateBlocked(id string, n int) error {
	if err := os.MkdirAll(gateStateDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(gateState{Blocked: n})
	if err != nil {
		return err
	}
	return os.WriteFile(gateStatePath(id), append(raw, '\n'), 0o644)
}
