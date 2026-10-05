package wick

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The decision is the one place the block/remind/give-up rules live: a submit,
// a provider error or pending children allow the stop; otherwise it blocks
// with reminder n of 3, pins on the 3rd attempt only where a harness can, and
// gives up after the 3rd block.
func TestGateDecisionBlocksRemindsPinsAndGivesUp(t *testing.T) {
	cases := []struct {
		name string
		in   GateInput
		want GateOutcome
	}{
		{"a submit this turn allows the stop", GateInput{Submitted: true}, GateAllow},
		{"a provider error allows the stop", GateInput{Failed: true}, GateAllow},
		{"children still owing a result allow the stop", GateInput{Pending: true}, GateAllow},
		{"the first stop without a submit blocks", GateInput{}, GateBlock},
		{"the second blocks", GateInput{Blocked: 1}, GateBlock},
		{"the third blocks without a pin where the harness cannot pin", GateInput{Blocked: 2}, GateBlock},
		{"the third pins where the harness can", GateInput{Blocked: 2, CanPin: true}, GatePin},
		{"the fourth gives up", GateInput{Blocked: 3}, GateGiveUp},
		{"the fourth gives up even where the harness can pin", GateInput{Blocked: 3, CanPin: true}, GateGiveUp},
	}
	for _, c := range cases {
		if got := DecideGate(c.in).Outcome; got != c.want {
			t.Errorf("%s: outcome %v, want %v", c.name, got, c.want)
		}
	}
}

// The reminder names the attempt and follows omp's wording: keep working is
// the default, submit only when done, blocked only for a real blocker.
func TestGateReminderNumberingAndWording(t *testing.T) {
	for n, want := range map[int]string{0: "reminder 1 of 3", 1: "reminder 2 of 3", 2: "reminder 3 of 3"} {
		if got := DecideGate(GateInput{Blocked: n}).Reminder; !strings.Contains(got, want) {
			t.Errorf("block %d: reminder must say %q, got %q", n, want, got)
		}
	}
	blocked := DecideGate(GateInput{}).Reminder
	for _, want := range []string{"submit", "tool call", "not an order to stop", "genuinely complete", "nameable blocker", "Never invent a blocker", "text only"} {
		if !strings.Contains(blocked, want) {
			t.Errorf("reminder missing %q:\n%s", want, blocked)
		}
	}
	// claude cannot pin a tool, so its 3rd block is its last.
	if last := DecideGate(GateInput{Blocked: 2}).Reminder; !strings.Contains(last, "last") {
		t.Errorf("the 3rd claude block must say it is the last: %q", last)
	}
}

// gate runs the stop-gate subcommand over one hook JSON and returns stdout.
func gate(t *testing.T, hook string) string {
	t.Helper()
	var out bytes.Buffer
	if err := RunStopGate(strings.NewReader(hook), &out); err != nil {
		t.Fatalf("stop gate: %v", err)
	}
	return out.String()
}

// spawnAgent points the stores at a temp dir and records an agent that delivers
// its result to a parent pane, the way a non-blocking agent is spawned.
func spawnAgent(t *testing.T, deliver bool) *recorder {
	t.Helper()
	deliverTestState(t)
	t.Setenv("WICK_AGENT_ID", "w1")
	t.Setenv("WICK_AGENT_TYPE", "worker")
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", ParentID: "p", Parent: "pP", Deliver: deliver}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec.run
	return rec
}

// gateBlock is the hook's block JSON.
type gateBlock struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func decodeBlock(t *testing.T, out string) gateBlock {
	t.Helper()
	var b gateBlock
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatalf("block output %q is not JSON: %v", out, err)
	}
	return b
}

// Four stops without a submit block with reminders 1, 2 and 3, then give up:
// the 4th writes nothing and delivers the "stopped without submitting" notice
// to the parent, quoting the agent's last message.
func TestStopGateRemindsThenGivesUpWithANotice(t *testing.T) {
	rec := spawnAgent(t, true)
	for n, active := range []bool{false, true, true} {
		out := gate(t, `{"stop_hook_active":`+strconv.FormatBool(active)+`}`)
		got := decodeBlock(t, out)
		if got.Decision != "block" {
			t.Fatalf("stop %d: decision %q, want block", n+1, got.Decision)
		}
		if want := fmt.Sprintf("reminder %d of 3", n+1); !strings.Contains(got.Reason, want) {
			t.Errorf("stop %d: reason must say %q, got %q", n+1, want, got.Reason)
		}
		if len(rec.calls) != 0 {
			t.Fatalf("a blocked stop must not deliver anything, got %v", rec.calls)
		}
	}

	out := gate(t, `{"stop_hook_active":true,"last_assistant_message":"I am stuck on the flaky test"}`)
	if strings.TrimSpace(out) != "" {
		t.Errorf("the 4th stop must write nothing, got %q", out)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("the give-up must deliver one notice, got %v", rec.calls)
	}
	call := strings.Join(rec.calls[0], " ")
	for _, want := range []string{"<system-notice> worker w1 stopped without submitting:", "I am stuck on the flaky test", "</system-notice>"} {
		if !strings.Contains(call, want) {
			t.Errorf("notice missing %q:\n%s", want, call)
		}
	}
}

// A blocking agent's give-up is its result in the waiting spawn, not a pane
// notice.
func TestStopGateGiveUpBecomesTheWaitingSpawnResult(t *testing.T) {
	rec := spawnAgent(t, false)
	for i := 0; i < 4; i++ {
		gate(t, `{"stop_hook_active":`+strconv.FormatBool(i > 0)+`,"last_assistant_message":"out of ideas"}`)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("a blocking agent's give-up must not type into a pane, got %v", rec.calls)
	}
	s, ok, err := ReadAgentSubmit("w1")
	if err != nil || !ok || !s.Stopped {
		t.Fatalf("the give-up must be recorded as a stopped result: %+v ok=%v err=%v", s, ok, err)
	}
	got := renderSpawn([]*spawnedAgent{{ID: "w1", Type: AgentType{Name: "worker", Blocking: true}}}, nil)
	if !strings.Contains(got, "stopped without submitting: out of ideas") {
		t.Errorf("the waiting spawn must return the stopped result, got %q", got)
	}
}

// A submit recorded this turn allows the stop, and is consumed so the next
// turn's first stop blocks again.
func TestStopGateAllowsASubmitAndConsumesIt(t *testing.T) {
	spawnAgent(t, true)
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "w1", Type: "worker", Status: "done", Summary: "shipped", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	if out := gate(t, `{"stop_hook_active":false}`); strings.TrimSpace(out) != "" {
		t.Errorf("a submit this turn must allow the stop, got %q", out)
	}
	out := gate(t, `{"stop_hook_active":false}`)
	if !strings.Contains(out, "reminder 1 of 3") {
		t.Errorf("the next turn's stop must block afresh, got %q", out)
	}
}

// No output and nothing delivered when the turn ended in a provider error.
func TestStopGateLetsAProviderErrorThrough(t *testing.T) {
	rec := spawnAgent(t, true)
	path := filepath.Join(t.TempDir(), "session.jsonl")
	recLine := `{"type":"assistant","isApiErrorMessage":true,"message":{"content":[{"type":"text","text":"provider blew up"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(recLine), 0o644); err != nil {
		t.Fatal(err)
	}
	out := gate(t, `{"stop_hook_active":false,"transcript_path":`+strconv.Quote(path)+`}`)
	if strings.TrimSpace(out) != "" {
		t.Errorf("a provider error must allow the stop, got %q", out)
	}
	if len(rec.calls) != 0 {
		t.Errorf("a provider error must deliver nothing, got %v", rec.calls)
	}
}

// An agent waiting on auto-backgrounded children is allowed to stop: it is
// waiting for their notices.
func TestStopGateLetsAnAgentWaitingOnChildrenThrough(t *testing.T) {
	rec := spawnAgent(t, true)
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", ParentID: "w1", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if out := gate(t, `{"stop_hook_active":false}`); strings.TrimSpace(out) != "" {
		t.Errorf("pending children must allow the stop, got %q", out)
	}
	if len(rec.calls) != 0 {
		t.Errorf("pending children must deliver nothing, got %v", rec.calls)
	}
}

// stop_hook_active false means a fresh turn: the count starts from zero.
func TestStopGateStartsTheCountOnAFreshTurn(t *testing.T) {
	spawnAgent(t, true)
	gate(t, `{"stop_hook_active":false}`)
	if out := gate(t, `{"stop_hook_active":true}`); !strings.Contains(out, "reminder 2 of 3") {
		t.Fatalf("want reminder 2, got %q", out)
	}
	if out := gate(t, `{"stop_hook_active":false}`); !strings.Contains(out, "reminder 1 of 3") {
		t.Errorf("a fresh turn must start the count over, got %q", out)
	}
}

// A real claude session has no agent id: the gate has nothing to hold.
func TestStopGateIgnoresANonAgentSession(t *testing.T) {
	deliverTestState(t)
	t.Setenv("WICK_AGENT_ID", "")
	if out := gate(t, `{"stop_hook_active":false}`); strings.TrimSpace(out) != "" {
		t.Errorf("a top-level session must be left alone, got %q", out)
	}
}
