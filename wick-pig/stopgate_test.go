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
	// claude cannot pin a tool, so its 3rd block is its last and says so.
	if last := DecideGate(GateInput{Blocked: 2}).Reminder; !strings.Contains(last, "last") {
		t.Errorf("the 3rd claude block must say it is the last: %q", last)
	}
	// A harness that can pin does not claim the turn will be allowed to end: it
	// forces submit instead.
	pinned := DecideGate(GateInput{Blocked: 2, CanPin: true}).Reminder
	if !strings.Contains(pinned, "pinned") || strings.Contains(pinned, "allowed to end") {
		t.Errorf("the pinned reminder must reflect the pin, got %q", pinned)
	}
}

// The harness reports whether it can pin the submit tool, which is what decides
// the pin on the 3rd attempt.
func TestHarnessGateCapability(t *testing.T) {
	if g := Registered("pig").Gate(); g == nil || !g.CanPin() {
		t.Error("pig pins tool_choice at the API level")
	}
	if g := Registered("claude").Gate(); g == nil || g.CanPin() {
		t.Error("claude cannot pin a tool")
	}
}

// The excerpt from the hook is preferred; a hook that omits it falls back to
// the transcript, and a long message is bounded to one line.
func TestStopExcerptPrefersTheHookFallsBackAndTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	recLine := `{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"the last word"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(recLine), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := stopExcerpt(StopHook{TranscriptPath: path}); got != "the last word" {
		t.Errorf("with no hook message the excerpt must come from the transcript, got %q", got)
	}
	if got := stopExcerpt(StopHook{LastAssistantMessage: "from the hook", TranscriptPath: path}); got != "from the hook" {
		t.Errorf("the hook message must win, got %q", got)
	}
	long := strings.Repeat("x", stopExcerptLimit+50)
	got := stopExcerpt(StopHook{LastAssistantMessage: long})
	if len([]rune(got)) != stopExcerptLimit+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("a long message must be cut to %d runes and elided, got %d runes", stopExcerptLimit, len([]rune(got)))
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

// askPigGate runs the pig-gate subcommand over one settle event and returns the
// decision the extension would enforce.
func askPigGate(t *testing.T, event string) PigGateDecision {
	t.Helper()
	var out bytes.Buffer
	if err := RunPigGate(event, &out); err != nil {
		t.Fatalf("pig gate: %v", err)
	}
	var d PigGateDecision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("pig gate output %q is not JSON: %v", out.String(), err)
	}
	return d
}

// The pig gate is the same decision over one JSON event per settle: four
// settles without a submit block with reminders 1, 2 and 3 — the third pinning
// submit, which pig can do — and the fourth lets the agent settle and delivers
// the "stopped without submitting" notice.
func TestPigGateRemindsPinsThenGivesUpWithANotice(t *testing.T) {
	rec := spawnAgent(t, true)
	for n, fresh := range []bool{true, false, false} {
		event := fmt.Sprintf(`{"fresh":%v,"last_message":"I am stuck on the flaky test"}`, fresh)
		got := askPigGate(t, event)
		want := pigGateBlock
		if n == 2 {
			want = pigGatePin
		}
		if got.Outcome != want {
			t.Fatalf("settle %d: outcome %q, want %q", n+1, got.Outcome, want)
		}
		if say := fmt.Sprintf("reminder %d of 3", n+1); !strings.Contains(got.Reminder, say) {
			t.Errorf("settle %d: reminder must say %q, got %q", n+1, say, got.Reminder)
		}
		if n == 2 && !strings.Contains(got.Reminder, "pinned") {
			t.Errorf("pig can pin submit, so the 3rd reminder must say so: %q", got.Reminder)
		}
		if len(rec.calls) != 0 {
			t.Fatalf("a blocked settle must not deliver anything, got %v", rec.calls)
		}
	}

	got := askPigGate(t, `{"fresh":false,"last_message":"I am stuck on the flaky test"}`)
	if got.Outcome != pigGateGiveUp {
		t.Fatalf("the 4th settle: outcome %q, want %q", got.Outcome, pigGateGiveUp)
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

// The gate is asked once per settle attempt, and a submit recorded this turn
// or a turn that ended in a provider error settles at once, with nothing
// delivered.
func TestPigGateLetsASubmitAndAProviderErrorThrough(t *testing.T) {
	rec := spawnAgent(t, true)
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "w1", Type: "worker", Status: "done", Summary: "shipped", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	if got := askPigGate(t, `{"fresh":true}`); got.Outcome != pigGateAllow {
		t.Errorf("a submit this turn must allow the settle, got %q", got.Outcome)
	}
	if got := askPigGate(t, `{"fresh":true,"failed":true}`); got.Outcome != pigGateAllow {
		t.Errorf("a provider error must allow the settle, got %q", got.Outcome)
	}
	if len(rec.calls) != 0 {
		t.Errorf("an allowed settle must deliver nothing, got %v", rec.calls)
	}
}

// A fresh turn starts the attempt count over: a settle after one is reminder 1
// again, the way a tell follow-up or a delivered notice restarts the ladder.
func TestPigGateStartsTheCountOnAFreshTurn(t *testing.T) {
	spawnAgent(t, true)
	askPigGate(t, `{"fresh":true}`)
	if got := askPigGate(t, `{"fresh":false}`); !strings.Contains(got.Reminder, "reminder 2 of 3") {
		t.Fatalf("want reminder 2, got %q", got.Reminder)
	}
	if got := askPigGate(t, `{"fresh":true}`); !strings.Contains(got.Reminder, "reminder 1 of 3") {
		t.Errorf("a fresh turn must start the count over, got %q", got.Reminder)
	}
}

// The extension loads into a top-level pig session too, which has no agent
// type and no submit tool: the gate has nothing to hold. Every session now
// carries an agent id, so the test sets one the way main.go does.
func TestPigGateIgnoresANonAgentSession(t *testing.T) {
	deliverTestState(t)
	t.Setenv("WICK_AGENT_ID", "top-1")
	t.Setenv("WICK_AGENT_TYPE", "")
	if got := askPigGate(t, `{"fresh":true}`); got.Outcome != pigGateAllow {
		t.Errorf("a top-level session must be left alone, got %q", got.Outcome)
	}
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

// A blocking agent's give-up is recorded as its result and never typed into a
// pane; the waiting spawn is what returns it.
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
	if s.Report != "stopped without submitting: out of ideas" {
		t.Errorf("the stopped result must carry the excerpt, got %q", s.Report)
	}
}

// A blocking agent that gives up is returned by the waiting spawn as its
// result, never typed into a pane.
func TestBlockingGiveUpIsReturnedByTheWaitingSpawn(t *testing.T) {
	spawnTestState(t)
	t.Setenv("WICK_AGENT_TYPE", "scout")
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	oldDeliver := deliverExec
	deliverExec = rec.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	spawnExec = giveUpExec{g: g}.run

	spawn, _ := spawnToolForRole(t, "implement")
	out, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "answer"},
	}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.Contains(out, "stopped without submitting: gave up") {
		t.Errorf("the waiting spawn must return the stopped result, got:\n%s", out)
	}
	if len(rec.calls) != 0 {
		t.Errorf("a blocking agent's give-up must not be typed, got %v", rec.calls)
	}
}

// giveUpExec plays a scout that never submits: the moment its pane starts it
// writes the stopped result the stop gate gives up with, so the waiting spawn
// must return that instead of a report.
type giveUpExec struct{ g *fakeGrid }

func (e giveUpExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
		if err := giveUpWithoutSubmit(envIn(args[3], "WICK_AGENT_ID"), "gave up"); err != nil {
			return Result{}, err
		}
		return Result{Stdout: "{}"}, nil
	}
	return e.g.run(cmd, args, opts)
}

// The give-up and a spawn's auto-background decide ownership under the child's
// lock: whichever runs first owns the result, so the parent always hears.
func TestGiveUpOwnershipIsDecidedUnderTheLock(t *testing.T) {
	deliverTestState(t)
	t.Setenv("WICK_AGENT_TYPE", "scout")
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec.run

	// The spawn backgrounded the agent first: its give-up goes out as a notice.
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", ParentID: "p", Parent: "pP"}); err != nil {
		t.Fatal(err)
	}
	if bg, err := backgroundAgent("a1"); err != nil || !bg {
		t.Fatalf("background: %v bg=%v", err, bg)
	}
	if err := giveUpWithoutSubmit("a1", "lost"); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("a backgrounded agent's give-up must be delivered as a notice, got %v", rec.calls)
	}

	// The give-up wrote first: the waiting spawn owns it, nothing is typed.
	rec2 := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec2.run
	if err := WriteAgentState(AgentState{ID: "a2", Type: "scout", ParentID: "p", Parent: "pP"}); err != nil {
		t.Fatal(err)
	}
	if err := giveUpWithoutSubmit("a2", "lost"); err != nil {
		t.Fatal(err)
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("a waiting spawn's give-up must not be typed, got %v", rec2.calls)
	}
	if s, ok, _ := ReadAgentSubmit("a2"); !ok || !s.Stopped {
		t.Errorf("the waiting spawn must find the stopped result: %+v ok=%v", s, ok)
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

// A top-level claude session has no agent type and no submit tool: the gate
// has nothing to hold. Every session now carries an agent id, so the test
// sets one the way main.go does.
func TestStopGateIgnoresANonAgentSession(t *testing.T) {
	deliverTestState(t)
	t.Setenv("WICK_AGENT_ID", "top-1")
	t.Setenv("WICK_AGENT_TYPE", "")
	if out := gate(t, `{"stop_hook_active":false}`); strings.TrimSpace(out) != "" {
		t.Errorf("a top-level session must be left alone, got %q", out)
	}
}
