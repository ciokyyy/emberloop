package wick

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Seams under test (ticket 01, per the spec's testing decisions): the spawn,
// tell and finish tools through their public contract (tool name, arguments
// and returned text, as the MCP server calls them), driven against a fake
// herdr grid. No test reaches inside the delivery internals.

// A spawn reply names every started agent's id and its pane id, so the caller
// can address it either way.
func TestSpawnOutputListsAgentAndPaneIDs(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("WICK_AGENT_ID", "parent1")
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run
	oldDeliver := deliverExec
	rec := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	deliverExec = rec.run
	t.Cleanup(func() { deliverExec = oldDeliver })

	spawn, _ := spawnToolForRole(t, "implement")
	out, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
	}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("launched %d scouts, want 2", len(ids))
	}
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Errorf("spawn reply must list agent id %q, got:\n%s", id, out)
		}
		st, ok, err := ReadAgentState(id)
		if err != nil || !ok || st.Pane == "" {
			t.Fatalf("agent %q has no pane recorded: %+v ok=%v err=%v", id, st, ok, err)
		}
		if !strings.Contains(out, st.Pane) {
			t.Errorf("spawn reply must list pane id %q for agent %q, got:\n%s", st.Pane, id, out)
		}
	}
}

// After a scout's grouped result is delivered, its pane is closed and its
// launch file is gone. Workers own worktrees and stay for finish; only the
// scout closes here.
func TestScoutPaneClosesAfterGroupDelivery(t *testing.T) {
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
	id := ids[0]
	st, ok, err := ReadAgentState(id)
	if err != nil || !ok || st.Pane == "" {
		t.Fatalf("scout %q has no pane: %+v ok=%v err=%v", id, st, ok, err)
	}
	pane := st.Pane
	launch := launchPath(Session{ID: id})
	if _, err := os.Stat(launch); err != nil {
		t.Fatalf("scout launch file must exist before delivery: %v", err)
	}

	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, id, "answered", "report", "facts here"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got := waitForPrompts(t, rec, "wP", 1, 2*time.Second); len(got) != 1 {
		t.Fatalf("the grouped notice must arrive, got %v", rec.calls)
	}
	pendingDeliveries.Wait()
	closed := false
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "pane close "+pane) {
			closed = true
		}
	}
	if !closed {
		t.Errorf("the scout pane %q must be closed after its result is delivered, calls: %v", pane, rec.calls)
	}
	if _, err := os.Stat(launch); !os.IsNotExist(err) {
		t.Errorf("the scout launch file must be cleaned up after delivery: %v", err)
	}
}

// A tell addressed by pane reaches the same agent as a tell by id.
func TestTellResolvesPaneID(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "p1", Parent: "wP", ParentID: "p", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	tellExec = rec.run

	tell := toolForRole(t, "implement", "tell")
	out, err := tell("/repo", map[string]any{"pane": "p1", "text": "do the thing"})
	if err != nil {
		t.Fatalf("tell by pane: %v", err)
	}
	if !strings.Contains(out, "end your turn") {
		t.Errorf("a non-blocking tell by pane must say to end the turn, got %q", out)
	}
	delivered := false
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "p1") {
			delivered = true
		}
	}
	if !delivered {
		t.Errorf("tell by pane must deliver to the agent's pane, calls: %v", rec.calls)
	}
}

// Tell needs exactly one of agent or pane: neither or both is an error that
// acts on nothing.
func TestTellNeedsExactlyOneOfAgentOrPane(t *testing.T) {
	tellTestState(t)
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	tellExec = rec.run
	tell := toolForRole(t, "implement", "tell")

	if _, err := tell("/repo", map[string]any{"text": "hi"}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("tell with neither agent nor pane must fail naming exactly one, got %v", err)
	}
	if _, err := tell("/repo", map[string]any{"agent": "w1", "pane": "p1", "text": "hi"}); err == nil || !strings.Contains(err.Error(), "exclusive") {
		t.Errorf("tell with both agent and pane must fail as exclusive, got %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("a rejected tell must act on nothing, calls: %v", rec.calls)
	}
}

// An unknown pane is a clear error that acts on nothing.
func TestTellUnknownPaneIsAClearError(t *testing.T) {
	tellTestState(t)
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	tellExec = rec.run
	tell := toolForRole(t, "implement", "tell")

	_, err := tell("/repo", map[string]any{"pane": "nope", "text": "hi"})
	if err == nil || !strings.Contains(err.Error(), "unknown pane") {
		t.Fatalf("tell with an unknown pane must fail naming it, got %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("an unknown pane must act on nothing, calls: %v", rec.calls)
	}
}

// A pane whose agent is gone is not delivered to: it matches no live agent.
func TestTellPaneWithNoLiveAgentIsAClearError(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p-gone", Parent: "wP", ParentID: "p"}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(_ string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{}
	}}
	tellExec = rec.run
	tell := toolForRole(t, "implement", "tell")

	_, err := tell("/repo", map[string]any{"pane": "p-gone", "text": "hi"})
	if err == nil || !strings.Contains(err.Error(), "no live agent") {
		t.Fatalf("tell to a gone pane must fail naming no live agent, got %v", err)
	}
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "agent prompt p-gone ") {
			t.Errorf("a gone pane must never be delivered to, calls: %v", rec.calls)
		}
	}
}

// A blocking tell that returns a scout's submit closes its pane: the result
// was delivered through the tell, so no idle pane is left behind.
func TestTellReturnClosesTheScoutPane(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2", Parent: "wP", ParentID: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "a1", Type: "scout", Status: "answered", Report: "old facts", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	rec := &recorder{reply: func(_ string, args []string) Result {
		switch {
		case args[0] == "agent" && args[1] == "prompt":
			return Result{Code: 1}
		case args[0] == "pane" && args[1] == "send-text":
			if _, err := submitOnce(t, scout, "a1", "answered", "report", "the new facts"); err != nil {
				t.Errorf("submit: %v", err)
			}
		}
		return Result{}
	}}
	tellExec = rec.run

	tell := toolForRole(t, "implement", "tell")
	out, err := tell("/repo", map[string]any{"agent": "a1", "text": "again"})
	if err != nil {
		t.Fatalf("tell: %v", err)
	}
	if !strings.Contains(out, "the new facts") {
		t.Fatalf("tell must return the new submit, got %q", out)
	}
	closed := false
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "pane close p2") {
			closed = true
		}
	}
	if !closed {
		t.Errorf("a scout whose submit returned through tell must have its pane closed, calls: %v", rec.calls)
	}
}

// Finish addressed by pane discards the same worker as finish by id.
func TestFinishResolvesPaneID(t *testing.T) {
	reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	old := finishExec
	finishExec = f.run
	t.Cleanup(func() { finishExec = old })
	if err := WriteAgentState(AgentState{ID: "07-x", Type: "worker", Pane: "root"}); err != nil {
		t.Fatal(err)
	}
	finish := toolForRole(t, "implement", "finish")
	out, err := finish("/repo", map[string]any{"pane": "root", "merge": false})
	if err != nil {
		t.Fatalf("finish by pane: %v", err)
	}
	if !strings.Contains(out, "discarded") {
		t.Errorf("finish by pane must discard, got %q", out)
	}
}

// Finish needs exactly one of agent or pane, and an unknown pane is a clear
// error.
func TestFinishNeedsExactlyOneOfAgentOrPane(t *testing.T) {
	tellTestState(t)
	finish := toolForRole(t, "implement", "finish")
	if _, err := finish("/repo", map[string]any{"merge": false}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("finish with neither must fail naming exactly one, got %v", err)
	}
	if _, err := finish("/repo", map[string]any{"agent": "07-x", "pane": "p1", "merge": false}); err == nil || !strings.Contains(err.Error(), "exclusive") {
		t.Errorf("finish with both must fail as exclusive, got %v", err)
	}
	if _, err := finish("/repo", map[string]any{"pane": "nope", "merge": false}); err == nil || !strings.Contains(err.Error(), "unknown pane") {
		t.Errorf("finish with an unknown pane must fail naming it, got %v", err)
	}
}

// A second submit after its group delivered arrives alone and still closes a
// scout's pane: the follow-up path leaves nothing behind either.
func TestScoutSecondSubmitAfterDeliveredStillCloses(t *testing.T) {
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
	id := ids[0]
	st, _, _ := ReadAgentState(id)
	pane := st.Pane
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, id, "answered", "report", "first facts"); err != nil {
		t.Fatal(err)
	}
	if got := waitForPrompts(t, rec, "wP", 1, 2*time.Second); len(got) != 1 {
		t.Fatalf("the first submit must arrive as the grouped notice, got %v", rec.calls)
	}
	if _, err := submitOnce(t, scout, id, "answered", "report", "second facts"); err != nil {
		t.Fatal(err)
	}
	got := waitForPrompts(t, rec, "wP", 2, 2*time.Second)
	if len(got) != 2 || !strings.Contains(got[1], "second facts") {
		t.Fatalf("the second submit must arrive as its own notice, got %v", rec.calls)
	}
	closed := false
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "pane close "+pane) {
			closed = true
		}
	}
	if !closed {
		t.Errorf("a scout's pane must be closed after its result is delivered, calls: %v", rec.calls)
	}
}

// A scout whose pane vanished still has its launch file cleaned up when its
// group completes with it reported as lost: a vanished pane leaves nothing
// behind.
func TestScoutLostPaneCleansLaunchFile(t *testing.T) {
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
	var lostPane, lostID string
	if st, ok, _ := ReadAgentState(ids[1]); ok {
		lostPane, lostID = st.Pane, ids[1]
	}
	if lostPane == "" {
		t.Fatal("the second scout has no pane to lose")
	}
	launch := launchPath(Session{ID: lostID})
	if _, err := os.Stat(launch); err != nil {
		t.Fatalf("scout launch file must exist before delivery: %v", err)
	}
	rec.reply = func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" && paneArg(args) == lostPane {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
		}
		return Result{Stdout: "{}"}
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, ids[0], "answered", "report", "only facts"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	got := waitForPrompts(t, rec, "wP", 1, 2*time.Second)
	if len(got) != 1 {
		t.Fatalf("a lost member must complete the group with exactly one notice, got %v", rec.calls)
	}
	if !strings.Contains(got[0], "lost:") || !strings.Contains(got[0], lostID) {
		t.Fatalf("the notice must report the vanished agent as lost, got:\n%s", got[0])
	}
	if _, err := os.Stat(launch); !os.IsNotExist(err) {
		t.Errorf("a lost scout's launch file must still be cleaned up: %v", err)
	}
}
