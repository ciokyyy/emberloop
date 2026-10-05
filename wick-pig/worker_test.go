package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ticketHerdr is a fake herdr whose one project workspace holds a tab per
// ticket, so a spawn can be shown to open a tab, reuse it, and close it. Git
// and anything else go to base, which may be a fake or the real osExec.
type ticketHerdr struct {
	base  Exec
	mu    sync.Mutex
	tabs  map[string]string   // label -> tab id
	panes map[string][]string // tab id -> pane ids, in creation order
	next  int
	calls [][]string
	// runErr makes pane run fail, for the undo path. noRootPane makes tab
	// create answer with a tab id but no pane, so the undo must close it.
	runErr     bool
	noRootPane bool
}

func newTicketHerdr(base Exec) *ticketHerdr {
	return &ticketHerdr{base: base, tabs: map[string]string{}, panes: map[string][]string{}}
}

// tab is the tab id the fake holds for a label, or "" when it has none.
func (h *ticketHerdr) tab(label string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tabs[label]
}

// paneRun is the pane the last pane run targeted, or "".
func (h *ticketHerdr) paneRun() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.calls {
		if len(c) > 2 && c[0] == "pane" && c[1] == "run" {
			return c[2]
		}
	}
	return ""
}

// called reports whether any herdr call starts with the given words.
func (h *ticketHerdr) called(words ...string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	want := strings.Join(words, " ")
	for _, c := range h.calls {
		if len(c) >= len(words) && strings.Join(c[:len(words)], " ") == want {
			return true
		}
	}
	return false
}

func (h *ticketHerdr) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd != herdrBin() {
		if h.base != nil {
			return h.base(cmd, args, opts)
		}
		return Result{}, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, args)
	if len(args) < 2 {
		return Result{Stdout: "{}"}, nil
	}
	reply := func(result map[string]any) Result {
		b, _ := json.Marshal(map[string]any{"result": result})
		return Result{Stdout: string(b)}
	}
	switch args[0] + " " + args[1] {
	case "tab list":
		tabs := []any{}
		for label, id := range h.tabs {
			tabs = append(tabs, map[string]any{"tab_id": id, "label": label, "workspace_id": "w1"})
		}
		return reply(map[string]any{"tabs": tabs}), nil
	case "tab create":
		label, _ := flag(args, "--label")
		h.next++
		id := fmt.Sprintf("w1:t%d", h.next)
		pane := fmt.Sprintf("w1:p%d", h.next)
		h.tabs[label] = id
		h.panes[id] = []string{pane}
		root := map[string]any{"pane_id": pane, "tab_id": id}
		if h.noRootPane {
			delete(root, "pane_id")
		}
		return reply(map[string]any{
			"tab":       map[string]any{"tab_id": id, "label": label, "workspace_id": "w1"},
			"root_pane": root,
		}), nil
	case "tab close":
		for label, id := range h.tabs {
			if id == args[2] {
				delete(h.tabs, label)
				delete(h.panes, id)
			}
		}
		return Result{Stdout: `{"result":{}}`}, nil
	case "pane list":
		panes := []any{}
		for id, ps := range h.panes {
			for _, p := range ps {
				panes = append(panes, map[string]any{"pane_id": p, "tab_id": id, "workspace_id": "w1"})
			}
		}
		return reply(map[string]any{"panes": panes}), nil
	case "pane run":
		if h.runErr {
			return Result{Code: 1, Stderr: `{"error":{"message":"launch failed"}}`}, nil
		}
		return Result{}, nil
	case "agent list":
		agents := []any{}
		for _, ps := range h.panes {
			for _, p := range ps {
				agents = append(agents, map[string]any{"pane_id": p, "agent_status": "idle", "name": "a"})
			}
		}
		return reply(map[string]any{"agents": agents}), nil
	}
	return Result{Stdout: "{}"}, nil
}

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

// The whole worker life on a git worktree in the project workspace: spawn
// makes the worktree with plain git and the ticket tab with herdr tabs, wait
// reads the agent, and finish merges and drops both.
func TestSpawnWaitFinish(t *testing.T) {
	// Isolated state so the test never touches a real worker store.
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")

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

	// Git is real; herdr is the fake, so the test never opens a real tab in
	// the user's session.
	h := newTicketHerdr(osExec)

	// --- spawn: worktree isolation is the whole point --------------------
	worker, err := Spawn(h.run, SpawnInput{
		Ticket:  ticket,
		Repo:    repo,
		Base:    "main",
		Harness: "stub",
		Launch:  []string{"true"},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

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
	// The worker lives in the ticket's tab in the project workspace.
	if worker.Workspace != "w1" || worker.Tab != "w1:t1" || worker.Pane != "w1:p1" {
		t.Errorf("workspace/tab/pane = %s/%s/%s, want w1/w1:t1/w1:p1", worker.Workspace, worker.Tab, worker.Pane)
	}
	if got := h.paneRun(); got != worker.Pane {
		t.Errorf("implementor launched in %q, want the ticket tab's root pane %q", got, worker.Pane)
	}

	// --- wait: the worker settles into idle ------------------------------
	statuses, pending, err := Wait(h.run, nil, 30*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if !contains(settled, statuses[0].State) {
		t.Errorf("state = %q, want a settled state; herdr must match the worker by pane, not by its reported name", statuses[0].State)
	}
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

	result, err := Finish(h.run, worker.ID, true)
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
	if h.tab("07-example") != "" {
		t.Error("the ticket tab is still open after finish")
	}

	if _, _, err := Wait(h.run, nil, 0); err != nil {
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
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()

	// The SHA comes from the stubbed git, not from recomputing what the code
	// under test does.
	const sha = "9f2c1a4b7e6d5c8a9f0e1d2c3b4a5968778695a4"

	var gitCalls [][]string
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" {
			gitCalls = append(gitCalls, args)
			return Result{Stdout: sha + "\n"}, nil
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)

	worker, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/17-thing.md", Repo: repo})
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

	// The worktree comes from plain git, at the resolved SHA: herdr is never
	// asked to make one.
	revIdx, addIdx := -1, -1
	var add []string
	for i, args := range gitCalls {
		if revIdx < 0 && args[0] == "rev-parse" {
			revIdx = i
		}
		if args[0] == "worktree" && len(args) > 1 && args[1] == "add" {
			addIdx, add = i, args
		}
	}
	if revIdx < 0 {
		t.Error("spawn never ran `git rev-parse` in the repo")
	}
	if addIdx < 0 {
		t.Fatal("spawn never ran `git worktree add`")
	}
	if got := add[len(add)-1]; got != sha {
		t.Errorf("git worktree add base = %q, want the resolved sha %q", got, sha)
	}
	if v, _ := flag(add, "-b"); v != "implement/17-thing" {
		t.Errorf("git worktree add -b = %q, want implement/17-thing", v)
	}
	// The ref must be resolved before the worktree exists, or it would resolve
	// to the worker's own tip.
	if revIdx > addIdx {
		t.Errorf("git rev-parse ran at call %d, after worktree add at %d", revIdx, addIdx)
	}
	if h.called("worktree", "create") || h.called("worktree", "remove") {
		t.Error("spawn used a herdr worktree command; herdr owns tabs and panes only")
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
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
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

	// Real git, stubbed herdr: the SHA is git's, no tab is opened.
	h := newTicketHerdr(osExec)

	worker, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/18-thing.md", Repo: repo, Base: "main"})

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
			Pane: "%1", Tab: "w1:t1", Workspace: "%2",
		},
	}); err != nil {
		t.Fatal(err)
	}

	var tabClosed bool
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		if len(args) > 1 && args[0] == "tab" && args[1] == "close" && args[2] == "w1:t1" {
			tabClosed = true
		}
		return Result{Stdout: `{"result":{}}`}, nil
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
	if !tabClosed {
		t.Error("finish did not close the ticket tab")
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Error("finish left the git worktree on disk")
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

// Without the project workspace there is nowhere to put the ticket's tab, so
// spawn refuses before it creates anything.
func TestSpawnNeedsTheProjectWorkspace(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	repo := t.TempDir()
	var calls int
	run := func(string, []string, ExecOpts) (Result, error) {
		calls++
		return Result{}, nil
	}
	if _, err := Spawn(run, SpawnInput{Ticket: "repo/.scratch/01-x.md", Repo: repo}); err == nil || !strings.Contains(err.Error(), "HERDR_WORKSPACE_ID") {
		t.Fatalf("err = %v, want the unset workspace named", err)
	}
	if calls != 0 {
		t.Errorf("spawn ran %d commands before refusing: it must create nothing", calls)
	}
	if _, err := os.Stat(worktreePath(repo, "01-x")); !os.IsNotExist(err) {
		t.Error("spawn created a worktree without a workspace")
	}
}

// Spawn finds the tab labelled with the ticket id in the project workspace, or
// creates it there with the worktree as cwd, and runs the implementor in its
// root pane.
func TestSpawnOpensTheTicketTabInTheProjectWorkspace(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" && args[0] == "rev-parse" {
			return Result{Stdout: "abc123\n"}, nil
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)

	w, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/23-thing.md", Repo: repo, Launch: []string{"agent"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	var create []string
	for _, c := range h.calls {
		if len(c) > 1 && c[0] == "tab" && c[1] == "create" {
			create = c
		}
	}
	if create == nil {
		t.Fatal("spawn never created a ticket tab")
	}
	if v, _ := flag(create, "--workspace"); v != "w1" {
		t.Errorf("tab create --workspace = %q, want the project workspace w1", v)
	}
	if v, _ := flag(create, "--label"); v != "23-thing" {
		t.Errorf("tab create --label = %q, want the ticket id 23-thing", v)
	}
	if v, _ := flag(create, "--cwd"); v != worktreePath(repo, "23-thing") {
		t.Errorf("tab create --cwd = %q, want the worktree", v)
	}
	if h.called("worktree", "create") {
		t.Error("spawn created a herdr worktree; the worktree is git's")
	}
	if got := h.paneRun(); got != w.Pane {
		t.Errorf("implementor launched in %q, want the tab's root pane %q", got, w.Pane)
	}
}

// A respawn reuses the ticket's existing tab and replaces the implementor in
// its root pane: a ticket never ends up with two tabs.
func TestSpawnReusesTheTicketTab(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" && args[0] == "rev-parse" {
			return Result{Stdout: "abc123\n"}, nil
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)
	// A tab from an earlier spawn of the same ticket: same label, existing
	// root pane.
	h.tabs["24-thing"] = "w1:t9"
	h.panes["w1:t9"] = []string{"w1:p9"}

	w, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/24-thing.md", Repo: repo})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if w.Tab != "w1:t9" || w.Pane != "w1:p9" {
		t.Errorf("tab/pane = %s/%s, want the existing w1:t9/w1:p9", w.Tab, w.Pane)
	}
	if h.called("tab", "create") {
		t.Error("spawn created a second tab for a ticket that already had one")
	}
	if got := h.paneRun(); got != "w1:p9" {
		t.Errorf("pane run in %q, want the existing implementor pane w1:p9", got)
	}
}

// A failure after the worktree exists must not leave the branch and checkout
// behind, or the ticket can never be spawned again. The tab this spawn created
// is closed with it.
func TestSpawnUndoesTheWorktreeWhenTheLaunchFails(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	var removed, deleted bool
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		switch {
		case args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case args[0] == "worktree" && len(args) > 1 && args[1] == "remove":
			removed = true
		case args[0] == "branch" && len(args) > 1 && args[1] == "-D":
			deleted = true
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)
	h.runErr = true

	if _, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/20-thing.md", Repo: repo}); err == nil {
		t.Fatal("spawn returned nil after a failed launch")
	}
	if !removed {
		t.Error("the git worktree was left behind after a failed launch")
	}
	if !deleted {
		t.Error("the implement branch was left behind after a failed launch")
	}
	if h.tab("20-thing") != "" {
		t.Error("the tab this spawn created was left open")
	}
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store["20-thing"]; ok {
		t.Error("a failed spawn left a worker record, so the ticket cannot spawn again")
	}
}

// A failure after the worktree exists closes only the tab this spawn created:
// an existing one may hold a previous implementor.
func TestSpawnKeepsATabItDidNotCreate(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if args[0] == "rev-parse" {
			return Result{Stdout: "abc123\n"}, nil
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)
	h.tabs["21-thing"] = "w1:t9"
	h.panes["w1:t9"] = []string{"w1:p9"}
	h.runErr = true

	if _, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/21-thing.md", Repo: repo}); err == nil {
		t.Fatal("spawn returned nil after a failed launch")
	}
	if h.tab("21-thing") != "w1:t9" {
		t.Error("the undo closed a tab this spawn did not create")
	}
}

// A tab create that answers with a tab but no root pane has still created the
// tab, so the undo closes it rather than leaking a pane-less tab.
func TestSpawnClosesATabItCreatedWithoutAPane(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if args[0] == "rev-parse" {
			return Result{Stdout: "abc123\n"}, nil
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)
	h.noRootPane = true

	if _, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/25-thing.md", Repo: repo}); err == nil {
		t.Fatal("spawn returned nil when tab create gave no pane")
	}
	if !h.called("tab", "close") {
		t.Error("spawn left open the pane-less tab it had just created")
	}
}
