package el

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
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
	// procs is the foreground process group of a pane that still runs
	// something. A pane absent from it is at its shell, which is what a stop
	// leaves behind. shell is the pid process-info reports for every pane.
	procs map[string]int
	shell int
	// runErr makes pane run fail, for the undo path. noRootPane makes tab
	// create answer with a tab id but no pane, so the undo must close it.
	runErr     bool
	noRootPane bool
}

func newTicketHerdr(base Exec) *ticketHerdr {
	return &ticketHerdr{
		base: base, tabs: map[string]string{}, panes: map[string][]string{},
		procs: map[string]int{}, shell: 4000,
	}
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

// stopCall is the index of the kill that ended a pane's process, or -1.
func (h *ticketHerdr) stopCall() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, c := range h.calls {
		if len(c) > 0 && c[0] == "kill" {
			return i
		}
	}
	return -1
}

// launchCall is the index of the pane run that launched the implementor, or -1.
func (h *ticketHerdr) launchCall() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, c := range h.calls {
		if len(c) > 2 && c[0] == "pane" && c[1] == "run" {
			return i
		}
	}
	return -1
}

func (h *ticketHerdr) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == "kill" {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.calls = append(h.calls, append([]string{"kill"}, args...))
		if len(args) >= 3 {
			pgid := strings.TrimPrefix(args[2], "-")
			for pane, group := range h.procs {
				if strconv.Itoa(group) == pgid {
					delete(h.procs, pane)
				}
			}
		}
		return Result{}, nil
	}
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
	case "pane process-info":
		pane := paneArg(args)
		known := false
		for _, ps := range h.panes {
			for _, p := range ps {
				if p == pane {
					known = true
				}
			}
		}
		if !known {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
		}
		if group := h.procs[pane]; group != 0 {
			return reply(map[string]any{"process_info": map[string]any{
				"pane_id": pane, "shell_pid": h.shell,
				"foreground_process_group_id": group,
				"foreground_processes":        []any{map[string]any{"pid": group}},
			}}), nil
		}
		return reply(map[string]any{"process_info": map[string]any{
			"pane_id": pane, "shell_pid": h.shell,
			"foreground_process_group_id": h.shell,
			"foreground_processes":        []any{map[string]any{"pid": h.shell}},
		}}), nil
	case "pane run":
		if h.runErr {
			return Result{Code: 1, Stderr: `{"error":{"message":"launch failed"}}`}, nil
		}
		// A launch starts a process past the shell, the way pane run
		// starts the agent: liveness reads it until a stop ends it.
		if len(args) > 2 {
			h.procs[args[2]] = h.shell + 1
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
	if os.Getenv("EL_LIVE_HERDR") == "" {
		t.Skip("set EL_LIVE_HERDR=1 to run against a real herdr")
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
// makes the worktree with plain git and the ticket tab with herdr tabs, and
// finish merges and drops both.
func TestSpawnFinishOnAWorktree(t *testing.T) {
	// Isolated state so the test never touches a real worker store.
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")

	repo := templatedReadmeRepo(t)

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

}

// After finish keeps the implement branch, the ticket can be picked up again:
// spawn reuses the kept branch instead of failing on `git worktree add -b`.
func TestSpawnAfterFinishReusesTheKeptBranch(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := templatedReadmeRepo(t)
	ticket := filepath.Join(repo, "29-thing.md")
	if err := os.WriteFile(ticket, []byte("# Thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Real git, stubbed herdr.
	h := newTicketHerdr(osExec)
	first, err := Spawn(h.run, SpawnInput{Ticket: ticket, Repo: repo, Base: "main", Launch: []string{"true"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Discard keeps the branch, the way finish does.
	if _, err := Finish(h.run, first.ID, false); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if got := gitOut(t, repo, "branch", "--list", first.Branch); got == "" {
		t.Fatalf("finish dropped the branch %s, so there is nothing to reuse", first.Branch)
	}

	second, err := Spawn(h.run, SpawnInput{Ticket: ticket, Repo: repo, Base: "main", Launch: []string{"true"}})
	if err != nil {
		t.Fatalf("respawn after finish: %v", err)
	}
	if second.Branch != first.Branch {
		t.Errorf("respawn branch = %q, want the kept %q", second.Branch, first.Branch)
	}
	if got := gitOut(t, second.Worktree, "rev-parse", "--abbrev-ref", "HEAD"); got != first.Branch {
		t.Errorf("respawn worktree HEAD = %q, want the kept branch %q", got, first.Branch)
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
			if args[0] == "show-ref" {
				// The stub resolves refs; it holds no implement branch.
				return Result{Code: 1}, nil
			}
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
	repo := templatedReadmeRepo(t)
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
	repo := templatedReadmeRepo(t)

	worktree := filepath.Join(repo, ".emberloop-worktrees", "19-thing")
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
	repo := templatedReadmeRepo(t)
	base := gitOut(t, repo, "rev-parse", "main")

	store := map[string]Worker{}
	for _, id := range []string{"30-a", "31-b"} {
		worktree := filepath.Join(repo, ".emberloop-worktrees", id)
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
	repo := templatedReadmeRepo(t)
	worktree := filepath.Join(repo, ".emberloop-worktrees", "32-c")
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
		switch {
		case cmd == "git" && args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case cmd == "git" && args[0] == "show-ref":
			return Result{Code: 1}, nil // no implement branch yet
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
		switch {
		case cmd == "git" && args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case cmd == "git" && args[0] == "show-ref":
			return Result{Code: 1}, nil // no implement branch yet
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

// A respawn finds the implementor's pane by its grid slot, not by whichever
// pane herdr lists first: a reviewer that replaced a pane is listed before the
// tab's root. Whatever still runs in that pane is stopped before the launch, so
// the launch line is never typed into a live agent.
func TestSpawnRespawnsIntoTheTabsRootSlot(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		switch {
		case cmd == "git" && args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case cmd == "git" && args[0] == "show-ref":
			return Result{Code: 1}, nil // no implement branch yet
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)
	// The tab's root is the implementor (slot 1); a reviewer replaced slot 2,
	// and herdr happens to list the reviewer's pane first. Something is still
	// running in the root: the old implementor's process group.
	resetGrid()
	recordGridSlot("w1:t9", 1, "w1:p9")
	h.tabs["28-thing"] = "w1:t9"
	h.panes["w1:t9"] = []string{"w1:p7", "w1:p9"}
	h.procs["w1:p9"] = 4242

	w, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/28-thing.md", Repo: repo})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if w.Pane != "w1:p9" {
		t.Errorf("pane = %q, want the slot-1 root pane w1:p9, not the first pane herdr lists", w.Pane)
	}
	if got := h.paneRun(); got != "w1:p9" {
		t.Errorf("pane run in %q, want the slot-1 root pane w1:p9", got)
	}
	if h.procs["w1:p9"] == 4242 {
		t.Error("the old implementor's process group was left running in the pane")
	}
	stop, launch := h.stopCall(), h.launchCall()
	if stop < 0 {
		t.Fatal("respawn launched without ending the process in the pane")
	}
	if launch < 0 {
		t.Fatal("respawn never launched in the slot-1 pane")
	}
	if stop > launch {
		t.Errorf("the stop at call %d came after the launch at call %d", stop, launch)
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
		case args[0] == "show-ref":
			return Result{Code: 1}, nil // no implement branch yet
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
		switch {
		case args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case args[0] == "show-ref":
			return Result{Code: 1}, nil // no implement branch yet
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
		switch {
		case args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case args[0] == "show-ref":
			return Result{Code: 1}, nil // no implement branch yet
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

// A respawn after finish reuses the branch finish kept: git worktree add
// checks the existing branch out with no -b, because -b fails on a name that
// already exists.
func TestSpawnReusesABranchAtOrBehindTheBase(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	var add []string
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		switch {
		case args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case args[0] == "show-ref":
			return Result{}, nil // the branch exists
		case args[0] == "merge-base":
			return Result{}, nil // the base still contains it
		case args[0] == "worktree" && len(args) > 1 && args[1] == "add":
			add = args
		}
		return Result{}, nil
	}
	h := newTicketHerdr(git)

	if _, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/26-thing.md", Repo: repo}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	want := []string{"worktree", "add", worktreePath(repo, "26-thing"), "implement/26-thing"}
	if strings.Join(add, " ") != strings.Join(want, " ") {
		t.Errorf("git worktree add = %v, want %v: an existing branch is reused, never re-created", add, want)
	}
}

// A branch finish kept that carries commits the base does not have is refused
// by name before spawn creates anything: reusing it would drop the work, and
// re-creating it would silently start from the base instead.
func TestSpawnRefusesABranchAheadOfTheBase(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := t.TempDir()
	var created []string
	git := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		switch {
		case args[0] == "rev-parse":
			return Result{Stdout: "abc123\n"}, nil
		case args[0] == "show-ref":
			return Result{}, nil // the branch exists
		case args[0] == "merge-base":
			return Result{Code: 1}, nil // it is not an ancestor of the base
		}
		created = append(created, strings.Join(append([]string{cmd}, args...), " "))
		return Result{}, nil
	}
	h := newTicketHerdr(git)

	_, err := Spawn(h.run, SpawnInput{Ticket: "repo/.scratch/27-thing.md", Repo: repo})
	if err == nil || !strings.Contains(err.Error(), "implement/27-thing") {
		t.Fatalf("err = %v, want an error naming the branch", err)
	}
	for _, c := range created {
		if strings.HasPrefix(c, "git worktree add") || strings.HasPrefix(c, "git branch") {
			t.Errorf("spawn created something after refusing the branch: %s", c)
		}
	}
	if h.tab("27-thing") != "" {
		t.Error("spawn opened a tab for a ticket whose branch it refused")
	}
	if _, err := os.Stat(worktreePath(repo, "27-thing")); !os.IsNotExist(err) {
		t.Error("spawn left a worktree behind after refusing the branch")
	}
}

// A dirty file in the integration checkout that the incoming change also
// touches blocks the merge up front: finish names it, says to commit or stash
// it, and never starts the merge.
func TestFinishRefusesWhenIntegrationCheckoutDirtyOverlap(t *testing.T) {
	repo, tree := reviewRepo(t)
	// The worker changes a.go; the integration checkout dirties the same file.
	writeFile(t, filepath.Join(repo, "a.go"), "package a\n\n// orchestrator dirt\n")
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	var merged bool
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			if len(args) > 0 && args[0] == "merge" {
				merged = true
			}
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || !res.Kept {
		t.Fatalf("overlapping dirt must keep the worker: %+v", res)
	}
	if !strings.Contains(res.Detail, "a.go") {
		t.Errorf("detail must name the overlapping file, got %q", res.Detail)
	}
	if !strings.Contains(strings.ToLower(res.Detail), "commit") || !strings.Contains(strings.ToLower(res.Detail), "stash") {
		t.Errorf("detail must say to commit or stash, got %q", res.Detail)
	}
	if merged {
		t.Error("finish ran the merge after refusing: it must refuse before it starts")
	}
	if _, err := os.Stat(tree); err != nil {
		t.Errorf("refused merge dropped the worktree: %v", err)
	}
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store["07-x"]; !ok {
		t.Error("refused merge dropped the worker entry")
	}
}

// A dirty file that the incoming change does not touch leaves the merge alone:
// the orchestrator's notes and ticket edits never block work.
func TestFinishMergesWhenIntegrationCheckoutDirtyElsewhere(t *testing.T) {
	repo, tree := reviewRepo(t)
	// Untracked note the worker never touched.
	writeFile(t, filepath.Join(repo, "notes.md"), "orchestrator note\n")
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !res.Merged {
		t.Fatalf("non-overlapping dirt must not block the merge: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(repo, "notes.md")); err != nil {
		t.Errorf("non-overlapping dirt was lost by the merge: %v", err)
	}
	if got := gitOut(t, repo, "show", "HEAD:a.go"); !strings.Contains(got, "func A()") {
		t.Errorf("worker change missing on main after merge: %q", got)
	}
}

// Whatever git reports is passed through whole: a multi-line failure keeps
// every line in the reply, never just the first.
func TestFinishPassesThroughFullGitError(t *testing.T) {
	_, tree := reviewRepo(t)
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	const failure = "error: line one\nline two\nline three"
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" && len(args) > 0 && args[0] == "merge" {
			return Result{Code: 1, Stderr: failure + "\n"}, nil
		}
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || !res.Kept {
		t.Fatalf("a failed merge must keep the worker: %+v", res)
	}
	for _, line := range []string{"line one", "line two", "line three"} {
		if !strings.Contains(res.Detail, line) {
			t.Errorf("detail missing %q, got %q: git text must pass through whole", line, res.Detail)
		}
	}
}

// An untracked file inside a wholly-untracked directory still blocks when the
// incoming change adds the same path: status collapses the directory, so the
// check must list untracked files individually to see the overlap.
func TestFinishRefusesWhenUntrackedDirOverlapsIncomingFile(t *testing.T) {
	repo, tree := reviewRepo(t)
	// The worker adds newdir/file.txt; the checkout holds an untracked file at
	// the same path.
	if err := os.MkdirAll(filepath.Join(tree, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tree, "newdir", "file.txt"), "worker\n")
	gitOut(t, tree, "add", ".")
	gitOut(t, tree, "commit", "-qm", "add newdir/file.txt")
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	if err := os.MkdirAll(filepath.Join(repo, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "newdir", "file.txt"), "orchestrator dirt\n")
	var merged bool
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			if len(args) > 0 && args[0] == "merge" {
				merged = true
			}
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || !res.Kept {
		t.Fatalf("overlapping untracked dirt must keep the worker: %+v", res)
	}
	if !strings.Contains(res.Detail, filepath.Join("newdir", "file.txt")) {
		t.Errorf("detail must name the overlapping file, got %q", res.Detail)
	}
	if merged {
		t.Error("finish ran the merge after refusing: it must refuse before it starts")
	}
}

// A merged worker's ticket is marked done by finish itself: the status line
// reads done, every checkbox is ticked, and that lands in one commit on the
// target branch beside the merged work.
func TestFinishMarksTicketDoneAndTicksBoxesInOneCommit(t *testing.T) {
	repo, tree := reviewRepo(t)
	ticket := filepath.Join(repo, "07-x.md")
	writeFile(t, ticket, "# 07-x: example\n\n**Status:** ready-for-agent\n\n- [ ] first box\n- [ ] second box\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "track the ticket")
	pointWorkerAtTicket(t, ticket)
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	before := gitOut(t, repo, "rev-list", "--count", "main")
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !res.Merged {
		t.Fatalf("want merged, got %+v", res)
	}
	body, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "**Status:** done") {
		t.Errorf("ticket status must read done:\n%s", body)
	}
	if strings.Contains(string(body), "[ ]") {
		t.Errorf("every checkbox must be ticked:\n%s", body)
	}
	if n := strings.Count(string(body), "[x]"); n != 2 {
		t.Errorf("want 2 ticked boxes, got %d:\n%s", n, body)
	}
	// The merge fast-forwards the worker's own commit onto main, so the count
	// grows by two: the rebased work, then the ticket's own commit at the tip.
	if got := gitOut(t, repo, "rev-list", "--count", "main"); got != strconv.Itoa(mustAtoi(t, before)+2) {
		t.Errorf("main commit count = %s, want two more than %s: the rebased work and the ticket commit", got, before)
	}
	if got := gitOut(t, repo, "log", "--format=%s", "-1", "HEAD"); got != "mark 07-x done" {
		t.Errorf("tip commit message = %q, want the ticket's own commit", got)
	}
	if got := gitOut(t, repo, "show", "--name-only", "--format=", "HEAD"); strings.TrimSpace(got) != "07-x.md" {
		t.Errorf("the tip commit must hold only the ticket, got %q", got)
	}
	if got := gitOut(t, repo, "show", "HEAD:a.go"); !strings.Contains(got, "func A()") {
		t.Errorf("worker change missing on main after merge: %q", got)
	}
}

// A refused merge leaves the ticket exactly as it was: overlapping dirt keeps
// the worker, and the ticket file is untouched.
func TestFinishLeavesTicketUntouchedWhenRefused(t *testing.T) {
	repo, tree := reviewRepo(t)
	ticket := filepath.Join(repo, "07-x.md")
	writeFile(t, ticket, "# 07-x: example\n\n**Status:** ready-for-agent\n\n- [ ] first box\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "track the ticket")
	pointWorkerAtTicket(t, ticket)
	want, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "a.go"), "package b\n")
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || !res.Kept {
		t.Fatalf("overlapping dirt must keep the worker: %+v", res)
	}
	if got, err := os.ReadFile(ticket); err != nil || string(got) != string(want) {
		t.Errorf("a refused merge must leave the ticket untouched, got:\n%s", got)
	}
}

// A conflicting rebase keeps the worker and leaves the ticket exactly as it
// was: the branch is aborted back to the worker's own change.
func TestFinishLeavesTicketUntouchedOnConflict(t *testing.T) {
	repo, tree := reviewRepo(t)
	ticket := filepath.Join(repo, "07-x.md")
	writeFile(t, ticket, "# 07-x: example\n\n**Status:** ready-for-agent\n\n- [ ] first box\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "track the ticket")
	pointWorkerAtTicket(t, ticket)
	want, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	writeFile(t, filepath.Join(repo, "a.go"), "package conflict\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "another ticket merged first")
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || !res.Kept {
		t.Fatalf("a conflicting rebase must keep the worker: %+v", res)
	}
	if got, err := os.ReadFile(ticket); err != nil || string(got) != string(want) {
		t.Errorf("a conflicting merge must leave the ticket untouched, got:\n%s", got)
	}
}

// pointWorkerAtTicket re-records the reviewRepo worker with a real ticket file
// in the repo, the way a spawned worker records the ticket it was spawned from.
func pointWorkerAtTicket(t *testing.T, ticket string) {
	t.Helper()
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	w := store["07-x"]
	w.Ticket = ticket
	store["07-x"] = w
	if err := save(store); err != nil {
		t.Fatal(err)
	}
}

// mustAtoi reads a rev-list --count.
func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A discarded worker was not merged, so its ticket stays exactly as it was.
func TestDiscardLeavesTicketUntouched(t *testing.T) {
	repo, _ := reviewRepo(t)
	ticket := filepath.Join(repo, "07-x.md")
	writeFile(t, ticket, "# 07-x: example\n\n**Status:** ready-for-agent\n\n- [ ] first box\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "track the ticket")
	pointWorkerAtTicket(t, ticket)
	want, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	before := gitOut(t, repo, "rev-list", "--count", "main")
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	res, err := Finish(run, "07-x", false)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || res.Kept {
		t.Fatalf("discard: %+v", res)
	}
	if got, err := os.ReadFile(ticket); err != nil || string(got) != string(want) {
		t.Errorf("a discard must leave the ticket untouched, got:\n%s", got)
	}
	if got := gitOut(t, repo, "rev-list", "--count", "main"); got != before {
		t.Errorf("a discard must commit nothing, count %s want %s", got, before)
	}
}

// tickTicket only completes the header field: a file with no status line is
// left alone, an already-done ticket comes back identical so the caller skips
// the commit, and a "Status:" mentioned in the body is not a status line.
func TestTickTicketTouchesOnlyTheHeaderField(t *testing.T) {
	if _, ok := tickTicket("# x\n\n- [ ] box\n"); ok {
		t.Error("a file with no status line must not count as a ticket")
	}
	done, ok := tickTicket("# 07-x\n\n**Status:** done\n\n- [x] box\n")
	if !ok || strings.Contains(done, "[ ]") || !strings.Contains(done, "**Status:** done") {
		t.Errorf("an already-done ticket must come back as-is: %q ok=%v", done, ok)
	}
	raw := "# 07-x\n\n**Status:** ready-for-agent\n\nThe old Status: line read pending.\n\n- [ ] box\n"
	got, ok := tickTicket(raw)
	if !ok {
		t.Fatal("a ticket with a header status must complete")
	}
	if !strings.Contains(got, "**Status:** done") {
		t.Errorf("the header status must read done:\n%s", got)
	}
	if !strings.Contains(got, "The old Status: line read pending.") {
		t.Errorf("a body mention is not a status line and must be left alone:\n%s", got)
	}
	if strings.Contains(got, "[ ]") || !strings.Contains(got, "- [x] box") {
		t.Errorf("the checkbox must still be ticked:\n%s", got)
	}
}

// A ticket commit that fails is reported, with the worker kept: the merge
// already moved the target, so finishing again retries the ticket commit
// rather than merging twice.
func TestFinishReportsATicketCommitFailureAndKeepsTheWorker(t *testing.T) {
	repo, tree := reviewRepo(t)
	ticket := filepath.Join(repo, "07-x.md")
	writeFile(t, ticket, "# 07-x: example\n\n**Status:** ready-for-agent\n\n- [ ] first box\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "track the ticket")
	pointWorkerAtTicket(t, ticket)
	passReviews(t, tree, gitOut(t, tree, "rev-parse", "HEAD"))
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" {
			if len(args) > 0 && args[0] == "commit" {
				return Result{Code: 1, Stderr: "error: commit failed\n"}, nil
			}
			return osExec(cmd, args, opts)
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
	if _, err := Finish(run, "07-x", true); err == nil {
		t.Fatal("a failed ticket commit must be reported")
	}
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store["07-x"]; !ok {
		t.Error("the worker must be kept when its ticket commit fails")
	}
}
