package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A spawn of more workers than the cap starts the cap, reports the rest as
// queued, and starts them as earlier ones finish (spec stories 36 and 44).
func TestWorkerCapQueuesBeyondCapAndDrainsOnFinish(t *testing.T) {
	spawnTestState(t)
	// Cap of two: the third worker of one call must queue.
	resolverSettings(t, `{"max_workers":2}`)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	ex := &worktreeExec{g: g}
	spawnExec = ex.run

	repo := t.TempDir()
	tickets := make([]string, 3)
	for i, name := range []string{"cap-a.md", "cap-b.md", "cap-c.md"} {
		// Worker ids come from the ticket file name; keep them distinct.
		tickets[i] = filepath.Join(repo, name)
		if err := os.WriteFile(tickets[i], []byte("# "+tickets[i]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	spawn, _ := spawnToolForRole(t, "implement")
	agents := make([]any, 3)
	for i, ticket := range tickets {
		agents[i] = map[string]any{"type": "worker", "ticket": ticket}
	}
	out, err := spawn(repo, map[string]any{"agents": agents})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if ex.launched != 2 {
		t.Fatalf("launched %d workers, want the cap of 2", ex.launched)
	}
	if !strings.Contains(out, "cap-c queued") {
		t.Errorf("the result must report the queued worker, got:\n%s", out)
	}
	if got := queuedIDs(); len(got) != 1 || got[0] != "cap-c" {
		t.Fatalf("queued = %v, want [cap-c]", got)
	}
	store, err := loadWorkersLocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(store) != 2 {
		t.Fatalf("store holds %d workers, want the cap of 2", len(store))
	}

	// Finishing one starts the queued worker in its place.
	if _, err := Finish(ex.run, "cap-a", false); err != nil {
		t.Fatalf("finish cap-a: %v", err)
	}
	if ex.launched != 3 {
		t.Fatalf("launched %d workers after a finish, want 3: the queued worker must start", ex.launched)
	}
	if got := queuedIDs(); len(got) != 0 {
		t.Fatalf("queued = %v, want empty after the drain", got)
	}
	store, err = loadWorkersLocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(store) != 2 {
		t.Fatalf("store holds %d workers after drain, want 2", len(store))
	}
	if _, ok := store["cap-c"]; !ok {
		t.Errorf("cap-c must be running after the drain, store holds %v", keys(store))
	}
}

// The cap defaults to three when nothing is stored.
func TestWorkerCapDefaultsToThree(t *testing.T) {
	resolverSettings(t, "")
	if got := effectiveWorkerCap(); got != 3 {
		t.Fatalf("effective cap = %d, want 3", got)
	}
	resolverSettings(t, `{"max_workers":5}`)
	if got := effectiveWorkerCap(); got != 5 {
		t.Fatalf("effective cap = %d, want 5", got)
	}
}

func keys(store map[string]Worker) []string {
	var out []string
	for id := range store {
		out = append(out, id)
	}
	return out
}

// A concurrent finish may drain an entry this call queued and start it before
// the call fails: rollback then tears the started worker down instead of
// leaving it running orphaned with an id the parent never learned.
func TestFailedSpawnTearsDownAConcurrentlyDrainedWorker(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, `{"max_workers":1}`)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	ex := &worktreeExec{g: g}
	spawnExec = ex.run
	repo := t.TempDir()
	ticket := filepath.Join(repo, "race-c.md")
	if err := os.WriteFile(ticket, []byte("# race-c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	types := mustLoadAgentTypes(repo)
	// The call queued race-c; before it failed, a concurrent finish drained
	// the entry and started the worker, so the store holds it but the queue
	// is already empty.
	if err := queueWorker(queuedWorker{ID: "race-c", Type: "worker", Ticket: ticket, Repo: repo, Harness: "pig"}); err != nil {
		t.Fatal(err)
	}
	store, err := loadWorkersLocked()
	if err != nil {
		t.Fatal(err)
	}
	store["race-c"] = Worker{ID: "race-c", Ticket: ticket, Repo: repo, Branch: "implement/race-c", Harness: "pig"}
	if err := withFileLock("workers", func() error { return save(store) }); err != nil {
		t.Fatal(err)
	}
	dequeueWorkers([]string{"race-c"})
	// Rollback of the failed call must not leave the started worker behind.
	closeSpawned(ex.run, []*spawnedAgent{{ID: "race-c", Type: types["worker"], Worktree: true, Queued: true}})
	rest, err := loadWorkersLocked()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rest["race-c"]; ok {
		t.Fatalf("race-c is still running after rollback, store holds %v", keys(rest))
	}
	if got := queuedIDs(); len(got) != 0 {
		t.Fatalf("queued = %v, want empty", got)
	}
}

// Each worker of one spawn call is its own group, so its result reaches the
// parent when it submits and the parent can merge it at once, instead of every
// worker waiting for the slowest of the call. A queued worker's group is its
// own too, written with the others.
func TestEachWorkerOfOneSpawnIsItsOwnGroup(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, `{"max_workers":2}`)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	ex := &worktreeExec{g: g}
	spawnExec = ex.run

	repo := t.TempDir()
	var agents []any
	for _, name := range []string{"grp-a.md", "grp-b.md", "grp-c.md"} {
		ticket := filepath.Join(repo, name)
		if err := os.WriteFile(ticket, []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		agents = append(agents, map[string]any{"type": "worker", "ticket": ticket})
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": agents}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	entries, err := os.ReadDir(groupDir())
	if err != nil {
		t.Fatal(err)
	}
	// The state directory can hold groups other tests left; only this call's
	// workers count.
	seen := map[string]bool{}
	for _, e := range entries {
		g, ok, err := readSpawnGroup(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || !ok {
			t.Fatalf("read group %s: %v", e.Name(), err)
		}
		for _, m := range g.Members {
			if !strings.HasPrefix(m, "grp-") {
				continue
			}
			if len(g.Members) != 1 {
				t.Fatalf("group %s has members %v, want exactly one", g.ID, g.Members)
			}
			seen[m] = true
		}
	}
	for _, id := range []string{"grp-a", "grp-b", "grp-c"} {
		if !seen[id] {
			t.Errorf("no group holds %s, groups cover %v", id, seen)
		}
	}
}
