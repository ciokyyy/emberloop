package el

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tellTestState points the agent and submit stores at a temp dir, silences the
// poll interval the blocking wait uses, and restores them after.
func tellTestState(t *testing.T) {
	t.Helper()
	oldState, oldExec, oldPoll := statePath, tellExec, pollEvery
	statePath = filepath.Join(t.TempDir(), "workers.json")
	pollEvery = 5 * time.Millisecond
	t.Cleanup(func() { statePath, tellExec, pollEvery = oldState, oldExec, oldPoll })
}

// toolForRole finds the named tool a role serves, the way a caller gets it over
// tools/list, and returns a caller for it.
func toolForRole(t *testing.T, role, name string) func(string, map[string]any) (string, error) {
	t.Helper()
	served, err := RoleTools(role)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range served {
		if tool.Name() != name {
			continue
		}
		return func(root string, params map[string]any) (string, error) {
			raw, _ := json.Marshal(params)
			return tool.Execute(context.Background(), root, raw)
		}
	}
	t.Fatalf("%s does not serve %s", role, name)
	return nil
}

// A tell to a non-blocking agent issues the delivery and returns at once: the
// agent's answer arrives later as a submit notice, so the caller never waits.
func TestTellToANonBlockingAgentReturnsAtOnce(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "p1", Parent: "wP", ParentID: "p", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	tellExec = rec.run

	tell := toolForRole(t, "implement", "tell")
	out, err := tell("/repo", map[string]any{"agent": "w1", "text": "do the thing"})
	if err != nil {
		t.Fatalf("tell: %v", err)
	}
	if !strings.Contains(out, "end your turn") {
		t.Errorf("a non-blocking tell must say to end the turn, got %q", out)
	}
	var delivered bool
	for _, c := range rec.calls {
		line := strings.Join(c, " ")
		if strings.HasPrefix(line, herdrBin()+" agent prompt p1 ") {
			delivered = true
		}
		if strings.Contains(line, "agent wait") {
			t.Errorf("tell must not wait for an agent, got %q", line)
		}
	}
	if !delivered {
		t.Errorf("tell must deliver to the agent's pane, calls: %v", rec.calls)
	}
}

// A tell to a blocking agent returns that agent's next submit, not the one the
// parent already read at spawn.
func TestTellToABlockingAgentReturnsItsNextSubmit(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2", Parent: "wP", ParentID: "p"}); err != nil {
		t.Fatal(err)
	}
	// The submit from the spawn the parent already read: tell must not return it.
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "a1", Type: "scout", Status: "answered", Report: "old facts", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	rec := &recorder{reply: func(_ string, args []string) Result {
		switch {
		case args[0] == "agent" && args[1] == "prompt":
			return Result{Code: 1} // a pig pane refuses; Nudge types instead
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
	if !strings.Contains(out, "the new facts") || strings.Contains(out, "old facts") {
		t.Errorf("tell must return the agent's next submit, got %q", out)
	}
	if strings.Contains(out, "end your turn") {
		t.Errorf("a blocking tell that returned a submit must not say to end the turn, got %q", out)
	}
}

// A tell to a blocking agent that outlasts the tool budget auto-backgrounds it:
// the result arrives later as a notice, so the caller is told to end its turn.
func TestTellToABlockingAgentAutoBackgroundsPastTheBudget(t *testing.T) {
	tellTestState(t)
	oldBudget := toolBudget
	toolBudget = 40 * time.Millisecond
	t.Cleanup(func() { toolBudget = oldBudget })
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2", Parent: "wP", ParentID: "p"}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	tellExec = rec.run

	tell := toolForRole(t, "implement", "tell")
	out, err := tell("/repo", map[string]any{"agent": "a1", "text": "again"})
	if err != nil {
		t.Fatalf("tell: %v", err)
	}
	if !strings.Contains(out, "still running") || !strings.Contains(out, "end your turn") {
		t.Errorf("a tell past the budget must background the agent and say to end the turn, got %q", out)
	}
	if st, _, _ := ReadAgentState("a1"); !st.Deliver {
		t.Error("a backgrounded blocking agent must have Deliver set so its submit becomes a notice")
	}
}

// finish works only for an agent whose type has a worktree of its own: a scout
// has nothing to merge or discard.
func TestFinishWorksOnlyForAWorktreeAgent(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2"}); err != nil {
		t.Fatal(err)
	}
	finish := toolForRole(t, "implement", "finish")
	_, err := finish("/repo", map[string]any{"agent": "a1", "merge": false})
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("finish on a scout must fail naming worktree, got %v", err)
	}
}

// finish discards a worktree agent through the tool.
func TestFinishDiscardsAWorktreeAgent(t *testing.T) {
	reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	old := finishExec
	finishExec = f.run
	t.Cleanup(func() { finishExec = old })
	if err := WriteAgentState(AgentState{ID: "07-x", Type: "worker", Pane: "root"}); err != nil {
		t.Fatal(err)
	}
	finish := toolForRole(t, "implement", "finish")
	out, err := finish("/repo", map[string]any{"agent": "07-x", "merge": false})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !strings.Contains(out, "discarded") {
		t.Errorf("finish must discard, got %q", out)
	}
}

// finish merges a worktree agent whose reviewers passed at its tip.
func TestFinishMergesAWorktreeAgent(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	old := finishExec
	finishExec = f.run
	t.Cleanup(func() { finishExec = old })
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	if err := WriteAgentState(AgentState{ID: "07-x", Type: "worker", Pane: "root"}); err != nil {
		t.Fatal(err)
	}
	finish := toolForRole(t, "implement", "finish")
	out, err := finish("/repo", map[string]any{"agent": "07-x"})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !strings.Contains(out, "merged") {
		t.Errorf("finish must merge once the reviewers passed, got %q", out)
	}
}

// A tell whose delivery fails puts the child back on the notice path, so a
// result the tell never got to wait for is still delivered rather than lost.
func TestTellRestoresTheChildWhenDeliveryFails(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2", Parent: "wP", ParentID: "p", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "a1", Type: "scout", Status: "answered", Report: "facts", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(_ string, args []string) Result {
		if args[0] == "pane" {
			return Result{Code: 1, Stderr: "pane is gone"}
		}
		return Result{Code: 1} // herdr refuses `agent prompt` for a pig pane
	}}
	tellExec = rec.run

	tell := toolForRole(t, "implement", "tell")
	if _, err := tell("/repo", map[string]any{"agent": "a1", "text": "again"}); err == nil {
		t.Fatal("a failed delivery must be reported")
	}
	if st, _, _ := ReadAgentState("a1"); !st.Deliver {
		t.Error("a failed tell must leave the child on the notice path")
	}
	if sub, _, _ := ReadAgentSubmit("a1"); !sub.ThisTurn {
		t.Error("a failed tell must leave the child's submit as this turn's")
	}
}

// An agent whose type definition is gone from the repo is still finished from
// its worktree record, rather than stranded.
func TestFinishFallsBackToTheWorktreeRecordForAnUnknownType(t *testing.T) {
	reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	old := finishExec
	finishExec = f.run
	t.Cleanup(func() { finishExec = old })
	if err := WriteAgentState(AgentState{ID: "07-x", Type: "custom", Pane: "root"}); err != nil {
		t.Fatal(err)
	}
	finish := toolForRole(t, "implement", "finish")
	out, err := finish("/repo", map[string]any{"agent": "07-x", "merge": false})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !strings.Contains(out, "discarded") {
		t.Errorf("finish must discard a worktree agent whose type is unknown, got %q", out)
	}
}

// A blocking tell returns the stop gate's give-up the way it returns a submit,
// rather than leaving it for a wait that no longer exists.
func TestTellToABlockingAgentReturnsAGiveUp(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2", Parent: "wP", ParentID: "p"}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(_ string, args []string) Result {
		switch {
		case args[0] == "agent" && args[1] == "prompt":
			return Result{Code: 1} // a pig pane refuses; Nudge types instead
		case args[0] == "pane" && args[1] == "send-text":
			_ = WriteAgentSubmit(AgentSubmit{AgentID: "a1", Type: "scout", Status: "stopped", Report: "stopped without submitting: I am stuck", Stopped: true})
		}
		return Result{}
	}}
	tellExec = rec.run

	tell := toolForRole(t, "implement", "tell")
	out, err := tell("/repo", map[string]any{"agent": "a1", "text": "again"})
	if err != nil {
		t.Fatalf("tell: %v", err)
	}
	if !strings.Contains(out, "stopped without submitting: I am stuck") {
		t.Errorf("a tell must return the stop gate's give-up, got %q", out)
	}
}

// A give-up the parent already read is not the answer to a later tell: the
// claim clears it, so only the new turn's submit comes back.
func TestTellDoesNotReturnAStaleGiveUp(t *testing.T) {
	tellTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Pane: "p2", Parent: "wP", ParentID: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "a1", Type: "scout", Status: "stopped", Report: "stopped without submitting: old", Stopped: true}); err != nil {
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
	if !strings.Contains(out, "the new facts") || strings.Contains(out, "stopped without submitting: old") {
		t.Errorf("tell must return the new turn's submit, got %q", out)
	}
}
