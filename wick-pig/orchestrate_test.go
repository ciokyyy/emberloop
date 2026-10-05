package wick

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAgents answers `herdr agent list` with one worker in each named state.
// The pane is p1, the pane the store below puts the worker in.
func fakeAgents(state string) Result {
	return Result{Stdout: `{"result":{"agents":[{"pane_id":"p1","agent_status":"` + state +
		`","name":"w01x","terminal_title_stripped":"working"}]}}`}
}

func withWorker(t *testing.T, id string) {
	t.Helper()
	old := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = old })
	if err := save(map[string]Worker{id: {ID: id, Pane: "p1", Branch: "implement/" + id}}); err != nil {
		t.Fatal(err)
	}
}

// The harness can cut a tool call at MCP_TOOL_TIMEOUT, which is shorter than a
// long wait: a request of an hour must return inside the cap and say that it
// was clamped, so the model knows to call again rather than to treat the wait
// as broken.
func TestWaitToolClampsTimeoutToTheCap(t *testing.T) {
	withWorker(t, "01-x")

	oldCap, oldExec := waitCap, waitExec
	waitCap = 50 * time.Millisecond
	waitExec = (&recorder{reply: func(string, []string) Result { return fakeAgents("working") }}).run
	t.Cleanup(func() { waitCap, waitExec = oldCap, oldExec })

	args, _ := json.Marshal(map[string]any{"timeout_ms": float64(3600000)})
	start := time.Now()
	out, err := waitTool.Execute(context.Background(), "", args)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("wait_workers: %v", err)
	}

	if elapsed > 5*time.Second {
		t.Errorf("an hour-long request blocked for %s: it must return within the cap", elapsed)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("returned in %s: the clamp was not applied, the request was ignored", elapsed)
	}
	if !strings.Contains(out, "clamped") {
		t.Errorf("a clamped wait must say so, got %q", out)
	}
	if !strings.Contains(out, "wait_workers") {
		t.Errorf("a clamped wait must tell the caller what to do next, got %q", out)
	}
}

func TestWaitToolUnderTheCapIsNotClamped(t *testing.T) {
	withWorker(t, "01-x")

	oldExec := waitExec
	waitExec = (&recorder{reply: func(string, []string) Result { return fakeAgents("working") }}).run
	t.Cleanup(func() { waitExec = oldExec })

	// timeout_ms 0 returns now, and a request below the cap is honoured as-is,
	// so neither may claim to have been clamped.
	args, _ := json.Marshal(map[string]any{"timeout_ms": float64(0)})
	out, err := waitTool.Execute(context.Background(), "", args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "clamped") {
		t.Errorf("a wait under the cap must not claim a clamp, got %q", out)
	}
	if !strings.Contains(out, "still running") {
		t.Errorf("the worker is still working, got %q", out)
	}
}

func TestWaitToolDescriptionStatesTheCap(t *testing.T) {
	if WaitCap != 50*time.Second {
		t.Errorf("WaitCap = %s, want 50s", WaitCap)
	}
	if waitCap != WaitCap {
		t.Errorf("waitCap = %s, want the default %s", waitCap, WaitCap)
	}
	if !strings.Contains(waitTool.desc, "50000") {
		t.Errorf("the description must state the cap the tool enforces, got %q", waitTool.desc)
	}
}

// A worker has no tool that can review its own work, so the implement skill
// ends in a step it cannot take. code_review is that tool: it judges this
// worktree's diff, committed and uncommitted, and hands back the review.
func TestCodeReviewReturnsTheReviewersVerdict(t *testing.T) {
	old := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = old })
	// Base is a SHA, resolved at spawn (see resolveRef): it names the exact
	// commit the worktree started from and cannot drift while the branch moves.
	const base = "a1b2c3d"
	if err := save(map[string]Worker{"15-x": {
		ID: "15-x", Base: base, Branch: "implement/15-x", Worktree: "/repo/wt",
	}}); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{reply: func(cmd string, args []string) Result {
		return fakeHerdr(cmd, args, "f:1 major: bug\n", Result{Stdout: "diff --git a/f b/f\n+bug\n"})
	}}
	oldExec := reviewExec
	reviewExec = rec.run
	t.Cleanup(func() { reviewExec = oldExec })

	out, err := codeReviewTool.Execute(context.Background(), "/repo/wt", nil)
	if err != nil {
		t.Fatalf("code_review: %v", err)
	}
	if !strings.Contains(out, "f:1 major: bug") {
		t.Errorf("the reviewer's reply must be the result, got %q", out)
	}
	// Diffing the base against the working tree, not against HEAD: a worker
	// reviews mid-branch, and only this covers commits *and* uncommitted work.
	if strings.Join(rec.calls[0], " ") != "git diff "+base {
		t.Errorf("diff call = %v", rec.calls[0])
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(statePath), "reviews", "wt.diff")); err != nil {
		t.Errorf("diff file not written: %v", err)
	}
}

func TestCodeReviewIsServedWhereWorkIs(t *testing.T) {
	serves := func(role string) bool {
		served, _ := RoleTools(role)
		for _, tl := range served {
			if tl.Name() == "code_review" {
				return true
			}
		}
		return false
	}
	for _, role := range []string{"work", "plan", "full"} {
		if !serves(role) {
			t.Errorf("%s role must serve code_review", role)
		}
	}
	// review and scout may only look; implement delegates and lets the merger
	// review through review_worker.
	for _, role := range []string{"review", "scout", "implement"} {
		if serves(role) {
			t.Errorf("%s role must not serve code_review", role)
		}
	}
	for _, role := range []string{"review", "scout"} {
		served, _ := RoleTools(role)
		for _, tl := range served {
			if n := tl.Name(); n != "read" && n != "find" {
				t.Errorf("%s role serves %s", role, n)
			}
		}
	}
}

// A tab that cannot be evened out is still a working tab, so the spawn stands
// and the result carries a short warning instead of failing.
func TestSpawnWorkerWarnsWhenTilingFails(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })
	repo := t.TempDir()
	ticket := filepath.Join(repo, "20-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TAB_ID", "w1:t1")
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	spawnExec = func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" {
			return Result{Stdout: "abc123def\n"}, nil
		}
		switch {
		case len(args) > 0 && args[0] == "worktree":
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"w9:p1","tab_id":"w9:t1"},"workspace":{"workspace_id":"w9"}}}`}, nil
		case len(args) > 1 && args[0] == "pane" && args[1] == "split":
			return Result{Stdout: `{"result":{"pane":{"pane_id":"w1:p2","tab_id":"w1:t1"}}}`}, nil
		case len(args) > 1 && args[0] == "pane" && args[1] == "list":
			return Result{Stdout: `{"result":{"panes":[{"pane_id":"w1:p1","tab_id":"w1:t1"},{"pane_id":"w1:p2","tab_id":"w1:t1"}]}}`}, nil
		case len(args) > 1 && args[0] == "pane" && args[1] == "layout":
			// Two panes of different sizes and no divider between them: the
			// tab is uneven and nothing can move it.
			return Result{Stdout: `{"result":{"layout":{"area":{"x":0,"y":0,"width":100,"height":100},"tab_id":"w1:t1",` +
				`"panes":[{"pane_id":"w1:p1","rect":{"x":0,"y":0,"width":80,"height":100}},{"pane_id":"w1:p2","rect":{"x":80,"y":0,"width":20,"height":100}}],"splits":[]}}}`}, nil
		}
		return Result{}, nil
	}
	args, _ := json.Marshal(map[string]any{"ticket": ticket})
	out, err := spawnTool.Execute(context.Background(), repo, args)
	if err != nil {
		t.Fatalf("spawn_worker: %v", err)
	}
	if !strings.Contains(out, "spawned worker") {
		t.Errorf("the spawn must still succeed, got %q", out)
	}
	if !strings.Contains(out, "warning") {
		t.Errorf("a failed tile must be reported as a warning, got %q", out)
	}
}
