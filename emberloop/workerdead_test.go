package el

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Seam (pre-agreed by the ticket): spawn through a
// fake herdr, asserting launches, the worker store, history and parent
// notices. A pane herdr answers pane_not_found for is dead; any other herdr
// error counts the worker as alive.

// deadPaneExec is a worktreeExec whose pane liveness the test controls: a
// pane in dead answers pane_not_found, a pane in errPane fails another way,
// every other pane is alive.
type deadPaneExec struct {
	g        *fakeGrid
	mu       sync.Mutex
	launched int
	gitCalls []string
	dead     map[string]bool
	errPane  map[string]bool
}

func (d *deadPaneExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == "git" {
		d.mu.Lock()
		d.gitCalls = append(d.gitCalls, strings.Join(args, " "))
		d.mu.Unlock()
		return Result{Stdout: "abc123def\n"}, nil
	}
	if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
		pane := paneArg(args)
		if d.errPane[pane] {
			return Result{Code: 1, Stderr: "herdr blew up"}, nil
		}
		if d.dead[pane] {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
		}
		return livePaneProcs(pane), nil
	}
	if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
		d.mu.Lock()
		d.launched++
		d.mu.Unlock()
	}
	return d.g.run(cmd, args, opts)
}

// Spawns are uncapped, and the pre-launch prune cancels a worker whose
// pane herdr no longer knows: with a dead first worker, the next spawn
// still starts, and the dead record is gone.
func TestNextSpawnStartsDespiteADeadPane(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, `{"max_tickets":1}`)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
	ex := &deadPaneExec{g: g, dead: map[string]bool{}, errPane: map[string]bool{}}
	spawnExec = ex.run

	repo := t.TempDir()
	first := filepath.Join(repo, "dead-a.md")
	second := filepath.Join(repo, "dead-b.md")
	for _, ticket := range []string{first, second} {
		if err := os.WriteFile(ticket, []byte("# "+ticket+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": first},
	}}); err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	w, err := recall("dead-a")
	if err != nil {
		t.Fatalf("recall dead-a: %v", err)
	}
	// The user closed the pane by hand: herdr no longer knows it.
	ex.dead[w.Pane] = true

	out, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": second},
	}})
	if err != nil {
		t.Fatalf("spawn second: %v", err)
	}
	if strings.Contains(out, "queued") {
		t.Fatalf("spawns are uncapped, got:\n%s", out)
	}
	ex.mu.Lock()
	launched := ex.launched
	ex.mu.Unlock()
	if launched != 2 {
		t.Fatalf("launched %d workers, want 2: the second must start", launched)
	}
	if _, err := recall("dead-b"); err != nil {
		t.Errorf("dead-b must be running: %v", err)
	}
	if _, err := recall("dead-a"); err == nil {
		t.Error("the pruned worker must leave workers.json")
	}
}

// Live workers never block new spawns, whatever herdr reports for their
// panes: an errored pane (anything but pane_not_found) still counts as alive
// and keeps its record, yet the next spawn starts anyway.
func TestLiveWorkersDoNotBlockNewSpawns(t *testing.T) {
	for _, name := range []string{"live", "errored"} {
		t.Run(name, func(t *testing.T) {
			spawnTestState(t)
			resolverSettings(t, `{"max_tickets":1}`)
			g := newFakeGrid()
			t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
			t.Setenv("HERDR_WORKSPACE_ID", "w1")
			t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
			ex := &deadPaneExec{g: g, dead: map[string]bool{}, errPane: map[string]bool{}}
			spawnExec = ex.run
			repo := t.TempDir()
			first := filepath.Join(repo, "cap-a.md")
			second := filepath.Join(repo, "cap-b.md")
			for _, ticket := range []string{first, second} {
				if err := os.WriteFile(ticket, []byte("# "+ticket+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			spawn, _ := spawnToolForRole(t, "implement")
			if _, err := spawn(repo, map[string]any{"agents": []any{
				map[string]any{"type": "worker", "ticket": first},
			}}); err != nil {
				t.Fatalf("spawn first: %v", err)
			}
			w, err := recall("cap-a")
			if err != nil {
				t.Fatalf("recall cap-a: %v", err)
			}
			if name == "errored" {
				ex.errPane[w.Pane] = true
			}
			out, err := spawn(repo, map[string]any{"agents": []any{
				map[string]any{"type": "worker", "ticket": second},
			}})
			if err != nil {
				t.Fatalf("spawn second: %v", err)
			}
			if strings.Contains(out, "queued") {
				t.Fatalf("spawns are uncapped, got:\n%s", out)
			}
			ex.mu.Lock()
			launched := ex.launched
			ex.mu.Unlock()
			if launched != 2 {
				t.Fatalf("launched %d workers, want 2: the second must start", launched)
			}
			if _, err := recall("cap-b"); err != nil {
				t.Errorf("cap-b must be running: %v", err)
			}
			if name == "errored" {
				if _, err := recall("cap-a"); err != nil {
					t.Errorf("an errored pane still counts as alive, but cap-a is gone: %v", err)
				}
			}
		})
	}
}

// A worker whose pane was closed by hand and who never submitted is
// cancelled by the prune: its workers.json entry is gone, its worktree and
// branch stay, and the run history records the cancellation.
func TestDeadPaneWorkerIsCancelledByThePrune(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := templatedReadmeRepo(t)
	ticket := filepath.Join(repo, "dead-c.md")
	if err := os.WriteFile(ticket, []byte("# dead-c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	dead := false
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if dead && cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
		}
		return h.run(cmd, args, opts)
	}
	worker, err := Spawn(run, SpawnInput{
		Ticket: ticket, Spec: "/t/spec.md", Repo: repo, Base: "main",
		Harness: "stub", Launch: []string{"true"},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	dead = true
	pruneDeadWorkers(run)
	if _, err := recall(worker.ID); err == nil {
		t.Fatal("a cancelled worker must leave workers.json")
	}
	if _, err := os.Stat(worker.Worktree); err != nil {
		t.Errorf("a cancelled worker keeps its worktree: %v", err)
	}
	if got := gitOut(t, repo, "branch", "--list", worker.Branch); got == "" {
		t.Errorf("a cancelled worker keeps its branch %s", worker.Branch)
	}
	for _, c := range h.calls {
		if len(c) > 1 && c[0] == "worktree" && c[1] == "remove" {
			t.Errorf("the prune must not drop the worktree: %v", h.calls)
			break
		}
	}
	got, err := ListTicketHistory("t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Outcome != "cancelled" {
		t.Fatalf("history = %+v, want one cancelled entry", got)
	}
	if got[0].Ticket != ticket || got[0].Worker != worker.ID || got[0].Branch != worker.Branch {
		t.Errorf("the cancelled entry must name the ticket, worker and branch: %+v", got[0])
	}
	if !strings.Contains(got[0].Detail, "pane closed manually") {
		t.Errorf("the cancelled entry must say the pane closed manually: %+v", got[0])
	}
}

// The parent of a cancelled worker gets the existing lost notice, worded
// "pane closed manually", through the dead-child sweep, exactly once.
func TestCancelledWorkerNotifiesParentOnce(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, "")
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("EL_AGENT_ID", "parent1")
	ex := &deadPaneExec{g: g, dead: map[string]bool{}, errPane: map[string]bool{}}
	spawnExec = ex.run
	oldDeliver := deliverExec
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			if ex.dead[paneArg(args)] {
				return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
			}
			return livePaneProcs(paneArg(args))
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	repo := t.TempDir()
	ticket := filepath.Join(repo, "dead-d.md")
	if err := os.WriteFile(ticket, []byte("# dead-d\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": ticket},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	w, err := recall("dead-d")
	if err != nil {
		t.Fatalf("recall dead-d: %v", err)
	}
	// Let the spawn's own background dispatch finish while the pane is alive,
	// so only the prune and the sweep below can report the worker.
	waitForDeliveries(3 * time.Second)
	ex.dead[w.Pane] = true
	pruneDeadWorkers(spawnExec)
	sweepDeadChildren("", "parent1")
	sweepDeadChildren("", "parent1")
	waitForDeliveries(3 * time.Second)
	var noticed []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "agent prompt wP ") {
			noticed = append(noticed, strings.Join(c, " "))
		}
	}
	if len(noticed) != 1 {
		t.Fatalf("a cancelled worker must yield exactly one parent notice, got %v", rec.calls)
	}
	if !strings.Contains(noticed[0], "dead-d") || !strings.Contains(noticed[0], "pane closed manually") {
		t.Errorf("the notice must name the worker and say the pane closed manually, got:\n%s", noticed[0])
	}
}

// Spawning a ticket whose worktree and branch survive from a cancelled
// worker reuses both instead of failing with "already exists".
func TestRespawnReusesCancelledWorktreeAndBranch(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := templatedReadmeRepo(t)
	ticket := filepath.Join(repo, "dead-e.md")
	if err := os.WriteFile(ticket, []byte("# dead-e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	dead := false
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if dead && cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
		}
		return h.run(cmd, args, opts)
	}
	first, err := Spawn(run, SpawnInput{
		Ticket: ticket, Spec: "/t/spec.md", Repo: repo, Base: "main",
		Harness: "stub", Launch: []string{"true"},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	dead = true
	pruneDeadWorkers(run)
	second, err := Spawn(run, SpawnInput{
		Ticket: ticket, Spec: "/t/spec.md", Repo: repo, Base: "main",
		Harness: "stub", Launch: []string{"true"},
	})
	if err != nil {
		t.Fatalf("a respawn must reuse the cancelled worktree, got: %v", err)
	}
	if second.Worktree != first.Worktree || second.Branch != first.Branch {
		t.Errorf("respawn = %s on %s, want reuse of %s on %s",
			second.Worktree, second.Branch, first.Worktree, first.Branch)
	}
	if _, err := os.Stat(second.Worktree); err != nil {
		t.Errorf("the reused worktree is missing: %v", err)
	}
	if got := gitOut(t, repo, "branch", "--list", second.Branch); got == "" {
		t.Errorf("the reused branch %s is missing", second.Branch)
	}
}

// The pre-launch prune runs before every spawn: a respawn of a ticket whose
// pane was closed by hand starts fresh, reusing the worktree, while the
// cancelled record is gone.
func TestRespawnOfPrunedTicketStartsFresh(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, `{"max_tickets":1}`)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
	ex := &deadPaneExec{g: g, dead: map[string]bool{}, errPane: map[string]bool{}}
	spawnExec = ex.run
	repo := t.TempDir()
	first := filepath.Join(repo, "q-a.md")
	if err := os.WriteFile(first, []byte("# "+first+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": first},
	}}); err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	w, err := recall("q-a")
	if err != nil {
		t.Fatalf("recall q-a: %v", err)
	}
	ex.dead[w.Pane] = true
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": first},
	}}); err != nil {
		t.Fatalf("spawn respawn: %v", err)
	}
	ex.mu.Lock()
	launched := ex.launched
	ex.mu.Unlock()
	if launched != 2 {
		t.Fatalf("launched %d workers, want 2: the respawn must start", launched)
	}
	if _, err := recall("q-a"); err != nil {
		t.Errorf("q-a must be running after the respawn: %v", err)
	}
}

// An orchestrator has no agent id, so the dead-child sweep matches its pane:
// the next spawn prunes the hand-closed worker and reports it, without any
// direct sweep call.
func TestOrchestratorSpawnReportsCancelledWorker(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, "")
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("EL_AGENT_ID", "")
	ex := &deadPaneExec{g: g, dead: map[string]bool{}, errPane: map[string]bool{}}
	spawnExec = ex.run
	oldDeliver := deliverExec
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if len(args) > 2 && args[0] == "pane" && args[1] == "process-info" {
			if ex.dead[paneArg(args)] {
				return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}
			}
			return livePaneProcs(paneArg(args))
		}
		return Result{Stdout: "{}"}
	}}
	deliverExec = rec.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	repo := t.TempDir()
	first := filepath.Join(repo, "orch-a.md")
	second := filepath.Join(repo, "orch-b.md")
	for _, ticket := range []string{first, second} {
		if err := os.WriteFile(ticket, []byte("# "+ticket+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": first},
	}}); err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	w, err := recall("orch-a")
	if err != nil {
		t.Fatalf("recall orch-a: %v", err)
	}
	waitForDeliveries(3 * time.Second)
	ex.dead[w.Pane] = true
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": second},
	}}); err != nil {
		t.Fatalf("spawn second: %v", err)
	}
	waitForDeliveries(3 * time.Second)
	var noticed []string
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c, " "), "agent prompt wP ") {
			noticed = append(noticed, strings.Join(c, " "))
		}
	}
	if len(noticed) != 1 {
		t.Fatalf("the orchestrator must get exactly one notice, got %v", rec.calls)
	}
	if !strings.Contains(noticed[0], "orch-a") || !strings.Contains(noticed[0], "pane closed manually") {
		t.Errorf("the notice must name the worker and say the pane closed manually, got:\n%s", noticed[0])
	}
	if _, err := recall("orch-a"); err == nil {
		t.Error("the cancelled worker must leave workers.json")
	}
}

// A stray directory at the ticket's worktree path is refused, not adopted:
// only a worktree git lists for the repo is reused.
func TestSpawnRefusesAStrayWorktreePath(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := templatedReadmeRepo(t)
	ticket := filepath.Join(repo, "stray-x.md")
	if err := os.WriteFile(ticket, []byte("# stray-x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktreePath(repo, "stray-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	_, err := Spawn(h.run, SpawnInput{
		Ticket: ticket, Spec: "/t/spec.md", Repo: repo, Base: "main",
		Harness: "stub", Launch: []string{"true"},
	})
	if err == nil || !strings.Contains(err.Error(), "not a git worktree") {
		t.Fatalf("a stray directory must fail loudly, got: %v", err)
	}
	if _, err := recall("stray-x"); err == nil {
		t.Error("a refused spawn must record no worker")
	}
}

// A cancelled worker that committed before its pane closed keeps its work:
// the respawn reuses the branch with its commits.
func TestRespawnReusesCancelledBranchWithCommits(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := templatedReadmeRepo(t)
	ticket := filepath.Join(repo, "dead-f.md")
	if err := os.WriteFile(ticket, []byte("# dead-f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	dead := false
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if dead && cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
		}
		return h.run(cmd, args, opts)
	}
	first, err := Spawn(run, SpawnInput{
		Ticket: ticket, Spec: "/t/spec.md", Repo: repo, Base: "main",
		Harness: "stub", Launch: []string{"true"},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := os.WriteFile(filepath.Join(first.Worktree, "feature.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, first.Worktree, "add", ".")
	gitOut(t, first.Worktree, "commit", "-qm", "worker work")
	dead = true
	pruneDeadWorkers(run)
	second, err := Spawn(run, SpawnInput{
		Ticket: ticket, Spec: "/t/spec.md", Repo: repo, Base: "main",
		Harness: "stub", Launch: []string{"true"},
	})
	if err != nil {
		t.Fatalf("a respawn must reuse the committed branch, got: %v", err)
	}
	if second.Branch != first.Branch || second.Worktree != first.Worktree {
		t.Errorf("respawn = %s on %s, want %s on %s",
			second.Worktree, second.Branch, first.Worktree, first.Branch)
	}
	if _, err := os.Stat(filepath.Join(second.Worktree, "feature.txt")); err != nil {
		t.Errorf("the cancelled worker's commit must survive the respawn: %v", err)
	}
}

// Git lists resolved real paths, so a repo reached through a symlink still
// matches its registered worktree.
func TestWorktreeRegisteredThroughSymlink(t *testing.T) {
	repo := templatedReadmeRepo(t)
	tree := worktreePath(repo, "sym-x")
	gitOut(t, repo, "worktree", "add", "-q", "-b", "implement/sym-x", tree, "main")
	linked := filepath.Join(t.TempDir(), "linked-repo")
	if err := os.Symlink(repo, linked); err != nil {
		t.Fatal(err)
	}
	ok, err := worktreeRegistered(osExec, linked, worktreePath(linked, "sym-x"))
	if err != nil {
		t.Fatalf("worktreeRegistered: %v", err)
	}
	if !ok {
		t.Error("a registered worktree reached through a symlink must match")
	}
}
