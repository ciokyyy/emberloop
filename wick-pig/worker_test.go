package wick

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubAgent stands in for a real harness. It is the important half of this test:
// it self-reports through herdr exactly the way pig's integration will, so the
// pane-matching in Wait is exercised against the real herdr, not a mock.
const stubAgent = `#!/usr/bin/env bash
herdr pane report-agent "$HERDR_PANE_ID" --source wick --agent stub --state working --seq 1
sleep 2
herdr pane report-agent "$HERDR_PANE_ID" --source wick --agent stub --state idle --seq 2
while true; do sleep 60; done
`

func TestExecSeamReachesHerdr(t *testing.T) {
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr not on PATH")
	}
	// It splits and closes real panes, and from inside a herdr session it can
	// take a running worker's own pane down with it.
	if os.Getenv("WICK_LIVE_HERDR") == "" {
		t.Skip("set WICK_LIVE_HERDR=1 to run against a real herdr")
	}
	res, err := osExec("herdr", []string{"--version"}, ExecOpts{})
	t.Logf("code=%d err=%v stdout=%q stderr=%q", res.Code, err, res.Stdout, res.Stderr)
	if err != nil {
		t.Fatalf("osExec errored: %v", err)
	}
	if res.Stdout == "" {
		t.Fatal("herdr --version produced no stdout through the exec seam")
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSpawnWaitFinish(t *testing.T) {
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr not on PATH")
	}

	// Isolated state so the test never touches a real worker store.
	statePath = filepath.Join(t.TempDir(), "workers.json")

	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")

	ticket := filepath.Join(repo, "07-example.md")
	if err := os.WriteFile(ticket, []byte("# Example\n\nBlocked by: \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stub := filepath.Join(t.TempDir(), "stub-agent")
	if err := os.WriteFile(stub, []byte(stubAgent), 0o755); err != nil {
		t.Fatal(err)
	}

	run := osExec

	// --- spawn: worktree isolation is the whole point --------------------
	worker, err := Spawn(run, SpawnInput{
		Ticket:  ticket,
		Repo:    repo,
		Base:    "main",
		Harness: "stub",
		Launch:  []string{stub},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Teardown is registered before the first assertion on purpose: a failed
	// assertion must never be the reason a workspace is left behind in the
	// user's herdr. herdr opens a workspace for the worktree *and* one for the
	// source checkout, so sweep every workspace whose pane sits under this
	// test's repo rather than closing one by id.
	t.Cleanup(func() {
		_, _ = Finish(run, worker.ID, false)
		reply, err := herdrJSON(run, []string{"pane", "list"}, 30*time.Second)
		if err != nil {
			return
		}
		for _, pane := range reply.Result.Panes {
			if strings.HasPrefix(pane.Cwd, repo) && pane.Workspace != "" {
				_, _ = herdrJSON(run, []string{"workspace", "close", pane.Workspace}, 30*time.Second)
			}
		}
	})

	if worker.ID != "07-example" {
		t.Errorf("id = %q, want 07-example", worker.ID)
	}
	if worker.Branch != "implement/07-example" {
		t.Errorf("branch = %q, want implement/07-example", worker.Branch)
	}
	if _, err := os.Stat(worker.Worktree); err != nil {
		t.Errorf("worktree missing: %v", err)
	}
	if got := gitOut(t, repo, "branch", "--list", worker.Branch); got == "" {
		t.Errorf("branch %s not created", worker.Branch)
	}
	if got := gitOut(t, worker.Worktree, "rev-parse", "--abbrev-ref", "HEAD"); got != worker.Branch {
		t.Errorf("worktree HEAD = %q, want %q", got, worker.Branch)
	}

	// --- wait: the worker settles into idle ------------------------------
	statuses, pending, err := Wait(run, nil, 30*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if !contains(settled, statuses[0].State) {
		t.Errorf("state = %q, want a settled state; herdr must match the worker by pane, not by its reported name", statuses[0].State)
	}
	t.Logf("worker settled as %q (detail %q)", statuses[0].State, statuses[0].Detail)
	if len(pending) != 0 {
		t.Errorf("pending = %v, want none", pending)
	}

	// --- finish: rebase, fast-forward, drop the worktree -----------------
	if err := os.WriteFile(filepath.Join(worker.Worktree, "feature.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, worker.Worktree, "add", ".")
	gitOut(t, worker.Worktree, "commit", "-qm", "worker work")
	passReviews(t, worker.Worktree, gitOut(t, worker.Worktree, "rev-parse", "HEAD"))

	result, err := Finish(run, worker.ID, true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !result.Merged {
		t.Fatalf("merged = false, detail = %q", result.Detail)
	}
	if result.SHA == "" {
		t.Error("finish returned no sha")
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Errorf("work not present on main: %v", err)
	}
	if _, err := os.Stat(worker.Worktree); !os.IsNotExist(err) {
		t.Errorf("worktree still on disk after finish")
	}

	if _, _, err := Wait(run, nil, 0); err != nil {
		t.Errorf("store should be empty and readable: %v", err)
	}
}

func TestHerdrNameIsLegalForHerdr(t *testing.T) {
	// herdr rejects a name that does not start with a lowercase letter, and
	// to-tickets numbers its tickets, so 01-probe is not a legal name.
	for _, in := range []string{"01-probe", "probe", "07-a_b", "Fix The Thing", "", "../etc/passwd",
		"this-is-a-very-long-ticket-name-that-overflows-the-limit"} {
		got := herdrName(in)
		if got == "" {
			t.Fatalf("herdrName(%q) returned nothing", in)
		}
		if got[0] < 'a' || got[0] > 'z' {
			t.Errorf("herdrName(%q) = %q must start with a lowercase letter", in, got)
		}
		if len(got) > 32 {
			t.Errorf("herdrName(%q) = %q is %d chars; herdr allows 32", in, got, len(got))
		}
		for _, r := range got {
			legal := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_'
			if !legal {
				t.Errorf("herdrName(%q) = %q contains %q, which herdr rejects", in, got, r)
			}
		}
	}
	if got := herdrName("01-probe"); got != "w01-probe" {
		t.Errorf("a numbered ticket needs a letter prefix, got %q", got)
	}
	if got := herdrName("probe"); got != "probe" {
		t.Errorf("an already-legal name must be left alone, got %q", got)
	}
}

// TestSpawnResolvesBaseToASha pins the behaviour that makes later diffs
// meaningful: the base is a commit, not the ref "HEAD". Inside a worktree HEAD
// is the worker's own branch, so a stored "HEAD" makes `git diff <base>` empty
// the moment the worker commits.
func TestSpawnResolvesBaseToASha(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()

	// The SHA comes from the stubbed git, not from recomputing what the code
	// under test does.
	const sha = "9f2c1a4b7e6d5c8a9f0e1d2c3b4a5968778695a4"

	type call struct {
		cmd  string
		args []string
	}
	var calls []call
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		calls = append(calls, call{cmd, args})
		switch {
		case cmd == "git":
			return Result{Stdout: sha + "\n"}, nil
		case len(args) > 0 && args[0] == "worktree":
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"%1"},"workspace":{"workspace_id":"%2"}}}`}, nil
		default:
			return Result{}, nil
		}
	}

	worker, err := Spawn(run, SpawnInput{Ticket: "repo/.scratch/17-thing.md", Repo: repo})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if worker.Base != sha {
		t.Errorf("worker.Base = %q, want the resolved sha %q", worker.Base, sha)
	}

	store, err := load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := store[worker.ID].Base; got != sha {
		t.Errorf("stored base = %q, want the resolved sha %q", got, sha)
	}

	revIdx, wtIdx := -1, -1
	for i, c := range calls {
		if revIdx < 0 && c.cmd == "git" && len(c.args) > 1 && c.args[0] == "rev-parse" {
			revIdx = i
		}
		if wtIdx < 0 && len(c.args) > 0 && c.args[0] == "worktree" {
			wtIdx = i
			if v, ok := flag(c.args, "--base"); !ok || v != sha {
				t.Errorf("herdr worktree create --base = %q (ok=%v), want %q", v, ok, sha)
			}
		}
	}
	if revIdx < 0 {
		t.Error("spawn never ran `git rev-parse` in the repo")
	}
	if wtIdx < 0 {
		t.Fatal("spawn never ran `herdr worktree create`")
	}
	// The ref must be resolved before the worktree exists, or it would resolve
	// to the worker's own tip.
	if revIdx > wtIdx {
		t.Errorf("git rev-parse ran at call %d, after worktree create at %d", revIdx, wtIdx)
	}
}

// flag returns the value that follows name in args.
func flag(args []string, name string) (string, bool) {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// TestSpawnAcceptsABranchName keeps the existing caller contract: a branch name
// is a legal base, and the stored value is still a commit. git is real here, so
// the expected SHA comes from git itself.
func TestSpawnAcceptsABranchName(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	want := gitOut(t, repo, "rev-parse", "main")

	// Real git, stubbed herdr: the SHA is git's, the workspace is not opened.
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		if len(args) > 0 && args[0] == "worktree" {
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"%1"},"workspace":{"workspace_id":"%2"}}}`}, nil
		}
		return Result{}, nil
	}

	worker, err := Spawn(run, SpawnInput{Ticket: "repo/.scratch/18-thing.md", Repo: repo, Base: "main"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if worker.Base != want {
		t.Errorf("base = %q, want main's sha %q", worker.Base, want)
	}
	if worker.Target != "main" {
		t.Errorf("target = %q, want the branch it merges back into, main", worker.Target)
	}
}

// TestFinishRebasesOntoARefBase covers a store written before bases were
// resolved: "main" is not a SHA, and finish still has to work with it.
func TestFinishRebasesOntoARefBase(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")

	worktree := filepath.Join(repo, ".wick-worktrees", "19-thing")
	gitOut(t, repo, "worktree", "add", "-q", "-b", "implement/19-thing", worktree, "main")
	if err := os.WriteFile(filepath.Join(worktree, "feature.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, worktree, "add", ".")
	gitOut(t, worktree, "commit", "-qm", "worker work")

	if err := save(map[string]Worker{
		"19-thing": {
			ID: "19-thing", Ticket: "repo/.scratch/19-thing.md", Repo: repo,
			Base: "main", Branch: "implement/19-thing", Worktree: worktree,
			Pane: "%1", Workspace: "%2",
		},
	}); err != nil {
		t.Fatal(err)
	}

	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		if len(args) > 0 && args[0] == "worktree" {
			return Result{Stdout: `{"result":{}}`}, nil
		}
		return Result{}, nil
	}
	passReviews(t, worktree, gitOut(t, worktree, "rev-parse", "HEAD"))
	res, err := Finish(run, "19-thing", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !res.Merged {
		t.Fatalf("merged = false, detail = %q; a stored ref base must still rebase", res.Detail)
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Errorf("work not present on main: %v", err)
	}
}

// Two workers spawned from the same commit: the first merge moves the target
// branch, and the second must rebase onto where the target is *now*, not onto
// the commit both were spawned from, or its fast-forward fails as "diverging".
func TestFinishRebasesOntoTheTargetsCurrentTip(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	base := gitOut(t, repo, "rev-parse", "main")

	store := map[string]Worker{}
	for _, id := range []string{"30-a", "31-b"} {
		worktree := filepath.Join(repo, ".wick-worktrees", id)
		gitOut(t, repo, "worktree", "add", "-q", "-b", "implement/"+id, worktree, "main")
		if err := os.WriteFile(filepath.Join(worktree, id+".txt"), []byte(id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitOut(t, worktree, "add", ".")
		gitOut(t, worktree, "commit", "-qm", id)
		store[id] = Worker{ID: id, Repo: repo, Base: base, Target: "main", Branch: "implement/" + id,
			Worktree: worktree, Pane: "%1", Workspace: "%2"}
		passReviews(t, worktree, gitOut(t, worktree, "rev-parse", "HEAD"))
	}
	if err := save(store); err != nil {
		t.Fatal(err)
	}
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		if len(args) > 0 && args[0] == "worktree" {
			return Result{Stdout: `{"result":{}}`}, nil
		}
		return Result{}, nil
	}
	for _, id := range []string{"30-a", "31-b"} {
		res, err := Finish(run, id, true)
		if err != nil {
			t.Fatalf("finish %s: %v", id, err)
		}
		if !res.Merged {
			t.Fatalf("%s not merged: %q", id, res.Detail)
		}
	}
	for _, f := range []string{"30-a.txt", "31-b.txt"} {
		if _, err := os.Stat(filepath.Join(repo, f)); err != nil {
			t.Errorf("%s missing on main: %v", f, err)
		}
	}
}

// A worker merges into its target branch only; a repo that has since been
// switched to another branch is refused rather than merged into by accident.
func TestFinishRefusesWhenTheRepoIsOnAnotherBranch(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	worktree := filepath.Join(repo, ".wick-worktrees", "32-c")
	gitOut(t, repo, "worktree", "add", "-q", "-b", "implement/32-c", worktree, "main")
	gitOut(t, repo, "switch", "-q", "-c", "elsewhere")
	if err := save(map[string]Worker{"32-c": {ID: "32-c", Repo: repo, Base: "main", Target: "main",
		Branch: "implement/32-c", Worktree: worktree, Pane: "%1", Workspace: "%2"}}); err != nil {
		t.Fatal(err)
	}
	passReviews(t, worktree, gitOut(t, worktree, "rev-parse", "HEAD"))
	res, err := Finish(func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}, "32-c", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged || !res.Kept || !strings.Contains(res.Detail, "elsewhere") {
		t.Errorf("want kept with the branch named, got %+v", res)
	}
}

// A worker of a run is split into the run's tab, not left alone in the tab its
// worktree opened: that is what lets the run's workers tile together. The split
// carries the worktree as its cwd, so the agent starts where its checkout is.
func TestSpawnSplitsTheRunTabForTheWorker(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()

	type call struct {
		cmd  string
		args []string
	}
	var calls []call
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		calls = append(calls, call{cmd, args})
		if cmd == "git" {
			return Result{Stdout: "9f2c1a4b7e6d5c8a9f0e1d2c3b4a5968778695a4\n"}, nil
		}
		switch {
		case len(args) > 0 && args[0] == "worktree":
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"w9:p1","tab_id":"w9:t1"},"workspace":{"workspace_id":"w9"}}}`}, nil
		case len(args) > 1 && args[0] == "pane" && args[1] == "split":
			return Result{Stdout: `{"result":{"pane":{"pane_id":"w1:p3","tab_id":"w1:t1"}}}`}, nil
		}
		return Result{}, nil
	}

	w, err := Spawn(run, SpawnInput{
		Ticket: "repo/.scratch/20-thing.md", Repo: repo,
		RunTab: "w1:t1", Anchor: "w1:p1",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if w.Pane != "w1:p3" {
		t.Errorf("worker pane = %q, want the pane split into the run tab", w.Pane)
	}
	if w.Tab != "w1:t1" {
		t.Errorf("worker tab = %q, want the run tab", w.Tab)
	}

	var split []string
	for _, c := range calls {
		if len(c.args) > 1 && c.args[0] == "pane" && c.args[1] == "split" {
			split = c.args
		}
	}
	if split == nil {
		t.Fatal("spawn never split the run tab")
	}
	if v, _ := flag(split, "--pane"); v != "w1:p1" {
		t.Errorf("split --pane = %q, want the run tab's anchor w1:p1", v)
	}
	if v, _ := flag(split, "--cwd"); v != worktreePath(repo, "20-thing") {
		t.Errorf("split --cwd = %q, want the worker's worktree", v)
	}
}

// herdr's split reply may carry only the pane id. The run tab is then the one
// the caller named, not an empty string that tiles nothing.
func TestSpawnDefaultsWorkerTabToTheRunTab(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" {
			return Result{Stdout: "9f2c1a4b7e6d5c8a9f0e1d2c3b4a5968778695a4\n"}, nil
		}
		switch {
		case len(args) > 0 && args[0] == "worktree":
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"w9:p1","tab_id":"w9:t1"},"workspace":{"workspace_id":"w9"}}}`}, nil
		case len(args) > 1 && args[0] == "pane" && args[1] == "split":
			return Result{Stdout: `{"result":{"pane":{"pane_id":"w1:p3"}}}`}, nil
		}
		return Result{}, nil
	}

	w, err := Spawn(run, SpawnInput{
		Ticket: "repo/.scratch/20-thing.md", Repo: repo,
		RunTab: "w1:t1", Anchor: "w1:p1",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if w.Tab != "w1:t1" {
		t.Errorf("tab = %q, want the named run tab when herdr omits one", w.Tab)
	}
}

// A split that fails after the worktree was made must not leave the branch and
// checkout behind, or the ticket can never be spawned again.
func TestSpawnRemovesTheWorktreeWhenTheSplitFails(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	repo := t.TempDir()
	var worktreeRemoved bool
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" {
			return Result{Stdout: "9f2c1a4b7e6d5c8a9f0e1d2c3b4a5968778695a4\n"}, nil
		}
		switch {
		case len(args) > 0 && args[0] == "worktree" && args[1] == "create":
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"w9:p1","tab_id":"w9:t1"},"workspace":{"workspace_id":"w9"}}}`}, nil
		case len(args) > 0 && args[0] == "worktree" && args[1] == "remove":
			worktreeRemoved = true
			return Result{Stdout: `{"result":{}}`}, nil
		case len(args) > 1 && args[0] == "pane" && args[1] == "split":
			return Result{Code: 1, Stderr: `{"error":{"message":"split_not_allowed"}}`}, nil
		}
		return Result{}, nil
	}

	if _, err := Spawn(run, SpawnInput{
		Ticket: "repo/.scratch/20-thing.md", Repo: repo,
		RunTab: "w1:t1", Anchor: "w1:p1",
	}); err == nil {
		t.Fatal("spawn returned nil after a failed split")
	}
	if !worktreeRemoved {
		t.Error("the worktree was left behind after a failed split")
	}
}
