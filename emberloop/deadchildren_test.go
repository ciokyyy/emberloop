package emberloop

import (
	"strings"
	"testing"
	"time"
)

// A child with no submit and no live pane is not pending: its pane is gone,
// so the parent is not waiting on it.
func TestDeadChildWithGonePaneIsNotPending(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "gone-pane", ParentID: "p", Parent: "pP", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if got := PendingChildren("p"); len(got) != 0 {
		t.Fatalf("a child whose pane is gone must not be pending, got %v", got)
	}
}

// A live child that owes a result is still pending.
func TestLiveChildOwingAResultIsPending(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: liveDeliverReply}
	deliverExec = rec.run
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "live-pane", ParentID: "p", Parent: "pP", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	// pane get returns 0: the pane still exists.
	if got := PendingChildren("p"); len(got) != 1 || got[0] != "c1" {
		t.Fatalf("a live child owing a result must stay pending, got %v", got)
	}
}

// A turn ending in a provider error notifies the parent with the agent id
// and the error, instead of an allow with no message.
func TestErroredTurnNotifiesParentWithError(t *testing.T) {
	rec := spawnAgent(t, true)
	t.Setenv("EMBERLOOP_AGENT_ID", "c1")
	t.Setenv("EMBERLOOP_AGENT_TYPE", "scout")
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "live-pane", ParentID: "p", Parent: "pP", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	d, err := GateStop("c1", true, true, "provider blew up", true, func() string { return "last words" })
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != GateAllow {
		t.Fatalf("an errored turn must allow the stop, got %v", d.Outcome)
	}
	var noticed []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "agent prompt ") {
			noticed = append(noticed, strings.Join(c, " "))
		}
	}
	if len(noticed) != 1 {
		t.Fatalf("an errored turn must deliver one notice, got %v", rec.calls)
	}
	call := noticed[0]
	if !strings.Contains(call, "c1") || !strings.Contains(call, "provider blew up") || !strings.Contains(call, "stopped with error") {
		t.Errorf("the error notice must name the agent and the error, got:\n%s", call)
	}
}

// A dead child outside any group gets the lost notice exactly once, and the
// sweep never reports it twice.
func TestDeadChildYieldsExactlyOneLostNotice(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "gone-pane", ParentID: "p", Parent: "pP", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	sweepDeadChildren("", "p")
	sweepDeadChildren("", "p")
	var noticed []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "lost:") {
			noticed = append(noticed, strings.Join(c, " "))
		}
	}
	if len(noticed) != 1 {
		t.Fatalf("a dead child must yield exactly one lost notice, got %v", rec.calls)
	}
	if !strings.Contains(noticed[0], "c1") {
		t.Errorf("the lost notice must name the child, got:\n%s", noticed[0])
	}
	if got := PendingChildren("p"); len(got) != 0 {
		t.Errorf("a reported dead child must stay non-pending, got %v", got)
	}
}

// A live child still lets its parent stop, as today: pending children allow
// the stop with nothing delivered.
func TestLiveChildStillLetsParentStop(t *testing.T) {
	rec := spawnAgent(t, true)
	t.Setenv("EMBERLOOP_AGENT_ID", "wp")
	t.Setenv("EMBERLOOP_AGENT_TYPE", "worker")
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "live-pane", ParentID: "wp", Parent: "pP", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	d, err := GateStop("wp", true, false, "", true, func() string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != GateAllow {
		t.Fatalf("a live pending child must allow the parent stop, got %v", d.Outcome)
	}
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "agent prompt ") {
			t.Errorf("a pending allow must deliver nothing itself, got %v", rec.calls)
			break
		}
	}
}

// The stop gate runs the sweep before allowing on pending children: a worker
// with only dead children is blocked and nudged to submit, instead of
// stopping text-only with nobody told.
func TestStopGateBlocksAWorkerWithOnlyDeadChildren(t *testing.T) {
	spawnAgent(t, true)
	t.Setenv("EMBERLOOP_AGENT_ID", "wp")
	t.Setenv("EMBERLOOP_AGENT_TYPE", "worker")
	deliverExec = (&recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}}).run
	// Re-point the parent record at the same state dir the sweep reads.
	if err := WriteAgentState(AgentState{ID: "wp", Type: "worker", Pane: "parent-pane", Parent: "topP", Deliver: false}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "gone-pane", ParentID: "wp", Parent: "parent-pane", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	d, err := GateStop("wp", true, false, "", true, func() string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != GateBlock {
		t.Fatalf("a worker with only dead children must be blocked, got %v", d.Outcome)
	}
	if !strings.Contains(d.Reminder, "reminder 1 of 3") || !strings.Contains(d.Reminder, "submit") {
		t.Errorf("the block must nudge the worker to submit, got %q", d.Reminder)
	}
}

// An errored child stops counting as pending, so its parent is nudged instead
// of waiting on it.
func TestErroredChildIsNotPending(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	deliverExec = rec.run
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "live-pane", ParentID: "p", Parent: "pP", Deliver: true, Errored: true, ErrorDetail: "boom"}); err != nil {
		t.Fatal(err)
	}
	if got := PendingChildren("p"); len(got) != 0 {
		t.Fatalf("an errored child must not be pending, got %v", got)
	}
}

// An errored member completes its group with a stopped-with-error line, so
// the group never waits on it.
func TestErroredMemberCompletesGroupWithErrorLine(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	// Two members in one open group, both alive, neither submitted.
	if err := WriteAgentState(AgentState{ID: "e1", Type: "scout", Pane: "pane-e1", ParentID: "parent1", Parent: "wP", GroupID: "grp1", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "e2", Type: "scout", Pane: "pane-e2", ParentID: "parent1", Parent: "wP", GroupID: "grp1", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := writeSpawnGroup(spawnGroup{ID: "grp1", Parent: "wP", ParentID: "parent1", Members: []string{"e1", "e2"}}); err != nil {
		t.Fatal(err)
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, "e1", "answered", "report", "good facts"); err != nil {
		t.Fatal(err)
	}
	if !setChildErrored("e2", "provider blew up") {
		t.Fatal("the errored mark must be owned once")
	}
	if err := deliverGroupIfComplete("grp1"); err != nil {
		t.Fatal(err)
	}
	got := promptedCalls(rec, "wP")
	if len(got) != 1 {
		t.Fatalf("an errored member must complete the group with exactly one notice, got %v", rec.calls)
	}
	if !strings.Contains(got[0], "good facts") {
		t.Errorf("the notice must carry the submitted result, got:\n%s", got[0])
	}
	if !strings.Contains(got[0], "stopped with error") || !strings.Contains(got[0], "e2") || !strings.Contains(got[0], "provider blew up") {
		t.Errorf("the notice must report the errored agent with its error, got:\n%s", got[0])
	}
}

// A dead member whose group already delivered its lost line is not reported
// again: the group notice is the one lost notice.
func TestDeliveredGroupDeadMemberIsNotReportedTwice(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	if err := WriteAgentState(AgentState{ID: "d1", Type: "scout", Pane: "pane-d1", ParentID: "parent1", Parent: "wP", GroupID: "grp9", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "d2", Type: "scout", Pane: "pane-d2-gone", ParentID: "parent1", Parent: "wP", GroupID: "grp9", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := writeSpawnGroup(spawnGroup{ID: "grp9", Parent: "wP", ParentID: "parent1", Members: []string{"d1", "d2"}}); err != nil {
		t.Fatal(err)
	}
	rec.reply = func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" && paneArg(args) == "pane-d2-gone" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, "d1", "answered", "report", "facts"); err != nil {
		t.Fatal(err)
	}
	got := promptedCalls(rec, "wP")
	if len(got) != 1 {
		t.Fatalf("the group must deliver exactly one notice, got %v", rec.calls)
	}
	if !strings.Contains(got[0], "lost:") || !strings.Contains(got[0], "d2") {
		t.Fatalf("the group notice must report the dead member as lost, got:\n%s", got[0])
	}
	sweepDeadChildren("", "parent1")
	sweepDeadChildren("", "parent1")
	got = promptedCalls(rec, "wP")
	if len(got) != 1 {
		t.Fatalf("a delivered group member must never get an individual lost notice, got %v", rec.calls)
	}
	if got2 := PendingChildren("parent1"); len(got2) != 0 {
		t.Errorf("a delivered dead member must stay non-pending, got %v", got2)
	}
}

// A worker whose only children are dead group members is blocked at once, even
// though the group's notice is still on its way: the gate does not wait on the
// dead, and the background sweep delivers the group notice after.
func TestStopGateBlocksWithOnlyDeadGroupedChildren(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	_ = rec
	t.Setenv("EMBERLOOP_AGENT_ID", "wp2")
	t.Setenv("EMBERLOOP_AGENT_TYPE", "worker")
	oldDeliver := deliverExec
	rec2 := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec2.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	if err := WriteAgentState(AgentState{ID: "wp2", Type: "worker", Pane: "parent-pane", Parent: "topP", Deliver: false}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "gd1", Type: "scout", Pane: "gone-gd1", ParentID: "wp2", Parent: "parent-pane", GroupID: "grpb", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := writeSpawnGroup(spawnGroup{ID: "grpb", Parent: "parent-pane", ParentID: "wp2", Members: []string{"gd1"}}); err != nil {
		t.Fatal(err)
	}
	d, err := GateStop("wp2", true, false, "", true, func() string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != GateBlock {
		t.Fatalf("a worker with only dead grouped children must be blocked, got %v", d.Outcome)
	}
}

// A dead child whose launcher recorded its exit names it in the lost notice,
// so the parent sees how it died instead of only that its pane is gone.
func TestLostNoticeCarriesChildExit(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	code := 1
	if err := WriteAgentState(AgentState{ID: "c1", Type: "scout", Pane: "gone-pane", ParentID: "p", Parent: "pP", Deliver: true, ExitCode: &code, ExitStderr: "boom happened"}); err != nil {
		t.Fatal(err)
	}
	sweepDeadChildren("", "p")
	var noticed []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "lost:") {
			noticed = append(noticed, strings.Join(c, " "))
		}
	}
	if len(noticed) != 1 {
		t.Fatalf("a dead child must yield exactly one lost notice, got %v", rec.calls)
	}
	if !strings.Contains(noticed[0], "exit 1") || !strings.Contains(noticed[0], "boom happened") {
		t.Errorf("the lost notice must carry the child's exit, got:\n%s", noticed[0])
	}
}

// A child inside its boot grace still counts as pending when its pane looks
// empty: the shell may still be starting it. Past the grace the same empty
// pane is dead and reported once.
func TestBootingChildIsPendingNotLost(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return barePaneProcs(paneArg(args))
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	now := time.Now().UTC().Format(time.RFC3339)
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if err := WriteAgentState(AgentState{ID: "c-boot", Type: "scout", Pane: "boot-pane", ParentID: "p", Parent: "pP", Deliver: true, LaunchedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "c-old", Type: "scout", Pane: "old-pane", ParentID: "p", Parent: "pP", Deliver: true, LaunchedAt: old}); err != nil {
		t.Fatal(err)
	}
	if got := PendingChildren("p"); len(got) != 1 || got[0] != "c-boot" {
		t.Fatalf("only the booting child must stay pending, got %v", got)
	}
	sweepDeadChildren("", "p")
	var noticed []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "lost:") {
			noticed = append(noticed, strings.Join(c, " "))
		}
	}
	if len(noticed) != 1 || !strings.Contains(noticed[0], "c-old") {
		t.Fatalf("only the past-grace child must be reported, got %v", rec.calls)
	}
	if got := PendingChildren("p"); len(got) != 1 || got[0] != "c-boot" {
		t.Fatalf("the booting child must stay pending after the sweep, got %v", got)
	}
}
