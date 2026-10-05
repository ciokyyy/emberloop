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
		outcome := GateBlock
		if in.CanPin {
			outcome = GatePin
		}
		return GateDecision{Outcome: outcome, Reminder: gateReminder(gateMaxBlocks)}
	default:
		return GateDecision{Outcome: GateBlock, Reminder: gateReminder(in.Blocked + 1)}
	}
}

// gateReminder is omp's yield reminder: the attempt, why every turn ends with a
// tool call, and the choices in order, first applicable first. The last one
// says it is the last, which is what a harness that cannot pin relies on.
func gateReminder(n int) string {
	last := ""
	if n >= gateMaxBlocks {
		last = " This is the last reminder; call submit now or your turn will be allowed to end."
	}
	return fmt.Sprintf("Your previous turn ended without calling submit (reminder %d of %d). "+
		"Every turn ends with a tool call. Keep working — that is the default, not an order to stop. "+
		"Call submit with a final status only when the work is genuinely complete, "+
		"or with blocked only for a real, nameable blocker. "+
		"Never invent a blocker. Never end a turn with text only.%s", n, gateMaxBlocks, last)
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
	// A top-level session has no id and no submit tool: nothing to gate.
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

// GateStop asks the shared decision for one agent at one attempted stop and
// applies the shared bookkeeping: a fresh turn starts the count over, the
// attempt count is stored, and a give-up is recorded and, for a non-blocking
// agent, delivered as a notice. lastMessage is read only on a give-up. The
// caller enforces what the decision leaves: claude writes block JSON, pig
// continues the agent.
func GateStop(id string, fresh, failed, canPin bool, lastMessage func() string) (GateDecision, error) {
	st, _, err := ReadAgentState(id)
	if err != nil {
		return GateDecision{}, err
	}
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
		return d, giveUpWithoutSubmit(id, st, lastMessage())
	default:
		return d, writeGateBlocked(id, blocked+1)
	}
}

// stopExcerpt is the agent's last message, one flattened line and bounded, for
// the "stopped without submitting" notice. The hook field is preferred; a hook
// that omits it falls back to the transcript.
func stopExcerpt(hook StopHook) string {
	text := strings.TrimSpace(hook.LastAssistantMessage)
	if text == "" && hook.TranscriptPath != "" {
		if t, ok := (claudeHarness{}).LastReply(hook.TranscriptPath); ok {
			text = t
		}
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
// the agent to its parent the way a submit would: delivered as a notice for a
// non-blocking agent, returned by the waiting spawn for a blocking one.
func giveUpWithoutSubmit(id string, st AgentState, excerpt string) error {
	typ := os.Getenv("WICK_AGENT_TYPE")
	if typ == "" {
		typ = st.Type
	}
	s := AgentSubmit{
		AgentID:  id,
		Type:     typ,
		Status:   "stopped",
		Report:   "stopped without submitting: " + excerpt,
		Stopped:  true,
		At:       time.Now().UTC().Format(time.RFC3339),
		ThisTurn: false,
	}
	if err := WriteAgentSubmit(s); err != nil {
		return err
	}
	if st.Deliver {
		return deliverToParent(st, stoppedNotice(typ, id, excerpt))
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
