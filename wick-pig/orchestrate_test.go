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
	if WaitCap != 4*time.Minute {
		t.Errorf("WaitCap = %s, want 4m", WaitCap)
	}
	if waitCap != WaitCap {
		t.Errorf("waitCap = %s, want the default %s", waitCap, WaitCap)
	}
	if !strings.Contains(waitTool.desc, "240000") {
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
	for _, role := range []string{"work", "full"} {
		if !serves(role) {
			t.Errorf("%s role must serve code_review", role)
		}
	}
	// review and scout may only look; implement delegates and never reviews:
	// each worker reviews its own work; plan reviews nothing, review belongs
	// where code is written.
	for _, role := range []string{"review", "scout", "implement", "plan"} {
		if serves(role) {
			t.Errorf("%s role must not serve code_review", role)
		}
	}
	implement, _ := RoleTools("implement")
	for _, tl := range implement {
		if tl.Name() == "review_worker" {
			t.Errorf("implement role must not serve review_worker")
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

// spawnToolFake answers the git and herdr calls spawn_worker makes, with the
// given layout for the tiling that follows.
func spawnToolFake(layout string) Exec {
	return func(cmd string, args []string, _ ExecOpts) (Result, error) {
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
			return Result{Stdout: layout}, nil
		}
		return Result{}, nil
	}
}

// runSpawnTool drives the spawn_worker tool against a fake herdr with the
// given run-tab layout.
func runSpawnTool(t *testing.T, layout string) (string, error) {
	t.Helper()
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	spawnExec = spawnToolFake(layout)
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })

	repo := t.TempDir()
	ticket := filepath.Join(repo, "20-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TAB_ID", "w1:t1")
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	// Keep these tiling tests about tiling: a report warning here would make
	// "must not warn" fail for a reason unrelated to the tab.
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	args, _ := json.Marshal(map[string]any{"ticket": ticket})
	return spawnTool.Execute(context.Background(), repo, args)
}

// Two panes of different sizes and no divider between them: the tab is uneven
// and nothing can move it. The spawn stands and the result carries a short
// warning instead of failing.
const unevenNoSplit = `{"result":{"layout":{"area":{"x":0,"y":0,"width":100,"height":100},"tab_id":"w1:t1",` +
	`"panes":[{"pane_id":"w1:p1","rect":{"x":0,"y":0,"width":80,"height":100}},{"pane_id":"w1:p2","rect":{"x":80,"y":0,"width":20,"height":100}}],"splits":[]}}}`

// The run tab after the worker joins it: the orchestrator and the worker each
// half. Tiling has nothing to fix, so the result is a plain success.
const evenSplit = `{"result":{"layout":{"area":{"x":0,"y":0,"width":100,"height":100},"tab_id":"w1:t1",` +
	`"panes":[{"pane_id":"w1:p1","rect":{"x":0,"y":0,"width":50,"height":100}},{"pane_id":"w1:p2","rect":{"x":50,"y":0,"width":50,"height":100}}],` +
	`"splits":[{"id":"s","direction":"right","ratio":0.5,"rect":{"x":0,"y":0,"width":100,"height":100}}]}}}`

func TestSpawnWorkerWarnsWhenTilingFails(t *testing.T) {
	out, err := runSpawnTool(t, unevenNoSplit)
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

func TestSpawnWorkerTilesTheRunTab(t *testing.T) {
	out, err := runSpawnTool(t, evenSplit)
	if err != nil {
		t.Fatalf("spawn_worker: %v", err)
	}
	if strings.Contains(out, "warning") {
		t.Errorf("an even tab must not warn, got %q", out)
	}
}

// A pig worker without the report extension never registers with herdr, so
// wait_workers can only ever report it as unknown. The spawn must say so up
// front, and name how to fix it, rather than leave the merger waiting on a
// state that cannot arrive.
func TestSpawnWorkerWarnsWhenReportExtensionIsMissing(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	spawnExec = spawnToolFake(evenSplit)
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })

	// No env var and a fresh HOME: the lookup has nowhere to find the
	// extension, which is the case ticket 19 hit.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WICK_HERDR_REPORT", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", "")

	repo := t.TempDir()
	ticket := filepath.Join(repo, "21-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"ticket": ticket})
	out, err := spawnTool.Execute(context.Background(), repo, args)
	if err != nil {
		t.Fatalf("spawn_worker: %v", err)
	}
	if !strings.Contains(out, "spawned worker") {
		t.Errorf("the spawn must still succeed, got %q", out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "unknown") {
		t.Errorf("a missing report extension must be warned about, got %q", out)
	}
}

// The worker reaches its brief, and through it the wick:// skill addresses,
// with read. The brief lives beside the worker store, outside the worktree;
// read opens any absolute path, so the launch needs no read exception.
func TestWorkerLaunchCanReadItsBrief(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	var launched string
	base := spawnToolFake(evenSplit)
	spawnExec = func(cmd string, args []string, o ExecOpts) (Result, error) {
		if len(args) > 1 && args[0] == "pane" && args[1] == "run" {
			launched = strings.Join(args, " ")
		}
		return base(cmd, args, o)
	}
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })

	repo := t.TempDir()
	ticket := filepath.Join(repo, "22-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TAB_ID", "w1:t1")
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	args, _ := json.Marshal(map[string]any{"ticket": ticket})
	if _, err := spawnTool.Execute(context.Background(), repo, args); err != nil {
		t.Fatalf("spawn_worker: %v", err)
	}
	if launched == "" {
		t.Fatal("the worker was never launched")
	}
	if strings.Contains(launched, "WICK_READ_ALLOW") {
		t.Errorf("worker launch must not carry a read exception; read opens any path:\n%s", launched)
	}
}

// herdr refuses `agent prompt` for a pig worker: pig only reports its state,
// it is not an agent herdr detects. tell_worker must still deliver, by typing
// the message as one line into the pane, and then wait for the turn to settle.
func TestTellWorkerTypesIntoAPigPane(t *testing.T) {
	withWorker(t, "01-x")
	rec := &recorder{reply: func(_ string, args []string) Result {
		if args[0] == "agent" && args[1] == "prompt" {
			return Result{Code: 1, Stdout: `{"error":{"code":"agent_not_ready","message":"agent p1 is not an active named agent"}}`}
		}
		return Result{}
	}}
	old := tellExec
	tellExec = rec.run
	t.Cleanup(func() { tellExec = old })

	args, _ := json.Marshal(map[string]any{"worker": "01-x", "text": "rebase onto master,\nthen resolve"})
	out, err := tellTool.Execute(context.Background(), "", args)
	if err != nil {
		t.Fatalf("tell_worker: %v", err)
	}
	if !strings.Contains(out, "settle") {
		t.Errorf("an agent that settled must be reported as such, got %q", out)
	}
	var typed, entered, waited bool
	for _, c := range rec.calls {
		line := strings.Join(c[1:], " ")
		switch {
		case strings.HasPrefix(line, "pane send-text p1 "):
			typed = true
			if strings.Contains(line, "\n") || !strings.Contains(line, "rebase onto master, then resolve") {
				t.Errorf("typed text must be one line, got %q", line)
			}
		case line == "pane send-keys p1 Enter":
			entered = typed
		case strings.HasPrefix(line, "agent wait p1") && !strings.Contains(line, "working"):
			waited = entered
		}
	}
	if !typed || !entered || !waited {
		t.Errorf("want send-text, then Enter, then a settle wait; calls: %v", rec.calls)
	}
}

// A worker still busy at the tool budget got the message; the reply says so
// instead of failing, and points at wait_workers.
func TestTellWorkerStillWorkingIsNotAnError(t *testing.T) {
	withWorker(t, "01-x")
	rec := &recorder{reply: func(_ string, args []string) Result {
		if args[0] == "agent" && args[1] == "wait" && !strings.Contains(strings.Join(args, " "), "working") {
			return Result{Code: 1, Stdout: `{"error":{"code":"timeout"}}`}
		}
		return Result{}
	}}
	old := tellExec
	tellExec = rec.run
	t.Cleanup(func() { tellExec = old })

	args, _ := json.Marshal(map[string]any{"worker": "01-x", "text": "go on"})
	out, err := tellTool.Execute(context.Background(), "", args)
	if err != nil {
		t.Fatalf("tell_worker: %v", err)
	}
	if !strings.Contains(out, "still working") || !strings.Contains(out, "wait_workers") {
		t.Errorf("got %q", out)
	}
}
