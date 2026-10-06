package wick

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// groupTestState points the stores at a temp dir, silences launch spacing,
// and uses a fake herdr for notices that is idle and alive by default.
func groupTestState(t *testing.T, g *fakeGrid) *recorder {
	t.Helper()
	spawnTestState(t)
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("WICK_AGENT_ID", "parent1")
	oldDeliver := deliverExec
	rec := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	deliverExec = rec.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	return rec
}

// waitForPrompts polls the recorder until pane has want prompts or the
// timeout passes: spawn-side and sweep deliveries are dispatched in the
// background so the triggering call returns at once.
func waitForPrompts(t *testing.T, rec *recorder, pane string, want int, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := promptedCalls(rec, pane); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return promptedCalls(rec, pane)
}

func promptedCalls(rec *recorder, pane string) []string {
	var out []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "agent prompt "+pane+" ") {
			out = append(out, strings.Join(c, " "))
		}
	}
	return out
}

// An agent whose pane vanishes is reported as lost in the group notice, so
// the group always completes: the first submit plus the lost line arrive as
// exactly one notice.
func TestSpawnGroupReportsLostPaneInNotice(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("launched %d scouts, want 2", len(ids))
	}
	// The second member's pane is gone: herdr no longer knows it.
	var lostPane string
	if st, ok, _ := ReadAgentState(ids[1]); ok {
		lostPane = st.Pane
	}
	if lostPane == "" {
		t.Fatal("the second scout has no pane to lose")
	}
	rec.reply = func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "get" && args[2] == lostPane {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}

	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, ids[0], "answered", "report", "only facts"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	got := promptedCalls(rec, "wP")
	if len(got) != 1 {
		t.Fatalf("a lost member must complete the group with exactly one notice, got %v", rec.calls)
	}
	if !strings.Contains(got[0], "only facts") {
		t.Errorf("the notice must carry the submitted result, got:\n%s", got[0])
	}
	if !strings.Contains(got[0], "lost:") || !strings.Contains(got[0], ids[1]) {
		t.Errorf("the notice must report the vanished agent as lost, got:\n%s", got[0])
	}
}

// A busy parent still gets the notice at once: herdr queues typed text as
// steering, so the submit never waits for the parent to go idle and the notice
// carries no warning.
func TestSpawnGroupNoticeDeliveredAtOnceToBusyParent(t *testing.T) {
	g := newFakeGrid()
	spawnTestState(t)
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("WICK_AGENT_ID", "parent1")
	oldDeliver := deliverExec
	t.Cleanup(func() { deliverExec = oldDeliver })
	var mu sync.Mutex
	var prompts []string
	deliverExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if len(args) > 1 && args[0] == "agent" && args[1] == "wait" {
			// The parent is working: any wait for idle times out.
			return Result{Code: 1, Stdout: `{"error":{"code":"timeout"}}`}, nil
		}
		if len(args) > 2 && args[0] == "agent" && args[1] == "prompt" {
			mu.Lock()
			prompts = append(prompts, strings.Join(args, " "))
			mu.Unlock()
		}
		return Result{Stdout: "{}"}, nil
	}

	ex := &noneScoutExec{g: g}
	spawnExec = ex.run
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "only"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 1 {
		t.Fatalf("launched %d scouts, want 1", len(ids))
	}
	done := make(chan error, 1)
	go func() {
		_, err := submitOnce(t, AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}, ids[0], "answered", "report", "late facts")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the submit waited on a busy parent")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(prompts) != 1 || !strings.Contains(prompts[0], "late facts") {
		t.Fatalf("a busy parent must get exactly one notice with the result, got %v", prompts)
	}
	if strings.Contains(prompts[0], "warning") {
		t.Errorf("the notice must carry no busy warning, got:\n%s", prompts[0])
	}
}

// A cancelled spawn call strands nothing: the group is already recorded, so
// the members' submits still arrive as the one group notice.
func TestCancelledSpawnStrandsNothing(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run

	types, err := LoadAgentTypes(cwd())
	if err != nil {
		t.Fatal(err)
	}
	tool := spawnToolFor(types, []string{"scout"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, _ := json.Marshal(map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
	}})
	if _, err := tool.Execute(ctx, "/repo", raw); err != nil {
		t.Fatalf("a cancelled spawn must still record its group, got: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("launched %d scouts, want 2", len(ids))
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, ids[0], "answered", "report", "one"); err != nil {
		t.Fatal(err)
	}
	if got := promptedCalls(rec, "wP"); len(got) != 0 {
		t.Fatalf("the first of two submits must deliver nothing, got %v", rec.calls)
	}
	if _, err := submitOnce(t, scout, ids[1], "answered", "report", "two"); err != nil {
		t.Fatal(err)
	}
	got := promptedCalls(rec, "wP")
	if len(got) != 1 || !strings.Contains(got[0], "one") || !strings.Contains(got[0], "two") {
		t.Errorf("the cancelled call's results must still arrive as one grouped notice, got %v", rec.calls)
	}
}

// The spawn tool description and both briefs say once that spawn returns at
// once and its results arrive as a notice, so no caller waits or concludes
// the tool is missing.
func TestSpawnDescriptionAndBriefsNameTheNotice(t *testing.T) {
	_, desc := spawnToolForRole(t, "implement")
	for _, want := range []string{"Returns at once", "One notice", "end your turn"} {
		if !strings.Contains(desc, want) {
			t.Errorf("spawn description missing %q:\n%s", want, desc)
		}
	}
	if strings.Contains(desc, "still running:") {
		t.Errorf("the still-running reply is removed from the description:\n%s", desc)
	}
	brief, err := writeBrief("11-x", "/t/11-x.md", "/t/spec.md", "spec/t", "pig")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(brief)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "After spawn, end your turn; the result arrives as a system notice.") {
		t.Errorf("worker brief must say once that the result arrives as a notice, got:\n%s", raw)
	}
	got := implementPrompt(testRun)
	for _, want := range []string{"After spawn, end your turn", "single notice"} {
		if !strings.Contains(got, want) {
			t.Errorf("orchestrator brief missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Scouts are blocking, so one spawn returns their reports together") {
		t.Errorf("the orchestrator brief must not describe the old blocking wait:\n%s", got)
	}
	if !strings.Contains(systemPrompt, "end your turn") || !strings.Contains(strings.ToLower(systemPrompt), "one notice") {
		t.Errorf("system prompt must say to end the turn and wait for the notice, got:\n%s", systemPrompt)
	}
}

// A member's second submit, after its group already delivered, arrives alone
// as an individual notice: the worker a finish sent back, or a stop-gate
// give-up, is never silently dropped.
func TestGroupMemberSecondSubmitAfterDeliveredArrivesAlone(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "only"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 1 {
		t.Fatalf("launched %d scouts, want 1", len(ids))
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, ids[0], "answered", "report", "first facts"); err != nil {
		t.Fatal(err)
	}
	if got := promptedCalls(rec, "wP"); len(got) != 1 || !strings.Contains(got[0], "first facts") {
		t.Fatalf("the first submit must arrive as the grouped notice, got %v", rec.calls)
	}
	// The same member submits again, the way a worker sent back after finish
	// does: its group is delivered, so the result goes alone, not into the
	// old group.
	if _, err := submitOnce(t, scout, ids[0], "answered", "report", "second facts"); err != nil {
		t.Fatal(err)
	}
	got := promptedCalls(rec, "wP")
	if len(got) != 2 {
		t.Fatalf("the second submit must arrive as its own notice, got %v", rec.calls)
	}
	if !strings.Contains(got[1], "second facts") {
		t.Errorf("the second notice must carry the new result, got:\n%s", got[1])
	}
	if strings.Contains(got[1], "spawn complete") {
		t.Errorf("a post-delivery submit must not reuse the grouped wrapper, got:\n%s", got[1])
	}
}

// A tell claims one member out of a waiting group: the claimed member leaves
// the group and its answer goes to the waiting tell, while the group
// completes with the rest and delivers exactly one notice without it.
func TestTellClaimRemovesMemberFromWaitingGroup(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("launched %d scouts, want 2", len(ids))
	}
	// The parent tells the first member: claimNextSubmit takes it out of the
	// group, the way tellAgentCtx does before waiting.
	undo, err := claimNextSubmit(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = undo
	if st, ok, _ := ReadAgentState(ids[0]); !ok || st.GroupID != "" {
		t.Fatalf("a claimed member must leave its group: %+v ok=%v", st, ok)
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, ids[1], "answered", "report", "other facts"); err != nil {
		t.Fatal(err)
	}
	got := promptedCalls(rec, "wP")
	if len(got) != 1 {
		t.Fatalf("the group must complete without the claimed member, with one notice, got %v", rec.calls)
	}
	if !strings.Contains(got[0], "other facts") {
		t.Errorf("the notice must carry the remaining member, got:\n%s", got[0])
	}
}

// Two reviewers started by one call deliver exactly one notice, after the
// second submit, carrying both verdicts.
func TestReviewerGroupDeliversOneNoticeWithBothVerdicts(t *testing.T) {
	_, tree := reviewRepo(t)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("WICK_AGENT_ID", "parent1")
	oldDeliver := deliverExec
	rec := &recorder{reply: func(cmd string, args []string) Result {
		// Reviewer panes live in the fake, not the recorder: every pane the
		// group checks is alive, and the parent is idle.
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	f := newFakeReviewers(func(typ string, _ int, _ string) (string, string) {
		if typ == reviewerSpec {
			return verdictChanges, "spec findings here"
		}
		return verdictPass, "standards findings here"
	})
	oldExec := spawnExec
	spawnExec = f.run
	t.Cleanup(func() { spawnExec = oldExec })
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tool := spawnToolFor(types, []string{reviewerStandards, reviewerSpec})
	raw, _ := json.Marshal(map[string]any{"agents": []any{
		map[string]any{"type": reviewerStandards},
		map[string]any{"type": reviewerSpec},
	}})
	if _, err := tool.Execute(context.Background(), tree, raw); err != nil {
		t.Fatal(err)
	}
	got := waitForPrompts(t, rec, "wP", 1, 2*time.Second)
	if len(got) != 1 {
		t.Fatalf("two reviewers must deliver exactly one grouped notice, got %v", rec.calls)
	}
	for _, want := range []string{"standards findings here", "spec findings here", "submitted (pass)", "submitted (changes)"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the grouped notice must carry %q, got:\n%s", want, got[0])
		}
	}
}

// A stranded group whose last pending member vanished is finalized the next
// time its parent acts: the sweep in the next spawn delivers the lost report
// without waiting for a submit that never comes.
func TestSpawnSweepFinalizesStrandedLostGroup(t *testing.T) {
	g := newFakeGrid()
	rec := groupTestState(t, g)
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "doomed"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 1 {
		t.Fatalf("launched %d scouts, want 1", len(ids))
	}
	var doomedPane string
	if st, ok, _ := ReadAgentState(ids[0]); ok {
		doomedPane = st.Pane
	}
	if doomedPane == "" {
		t.Fatal("the scout has no pane to lose")
	}
	rec.reply = func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "get" && args[2] == doomedPane {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}
	if got := promptedCalls(rec, "wP"); len(got) != 0 {
		t.Fatalf("with nothing submitted the group must wait, got %v", rec.calls)
	}
	// The parent acts again: the sweep dispatches in the background and
	// delivers after this call ends.
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "next"},
	}}); err != nil {
		t.Fatalf("second spawn: %v", err)
	}
	got := waitForPrompts(t, rec, "wP", 1, 2*time.Second)
	if len(got) != 1 {
		t.Fatalf("the sweep must deliver the stranded group, got %v", rec.calls)
	}
	if !strings.Contains(got[0], "lost:") || !strings.Contains(got[0], ids[0]) {
		t.Errorf("the swept notice must report the vanished member as lost, got:\n%s", got[0])
	}
}
