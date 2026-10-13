package el

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spawns are uncapped: the ticket cap limits concurrent tickets in the
// driver, never the workers a spawn starts. Even with the global ticket cap
// at one and the spec's max_tickets at one, one spawn call starts every
// worker and reports none as queued.
func TestSpawnLaunchesEveryWorkerUncapped(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, `{"max_tickets":1}`)
	spec := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(spec, []byte("---\nmax_tickets: 1\n---\n\n# Spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SPEC", spec)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
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
	if ex.launched != 3 {
		t.Fatalf("launched %d workers, want all 3: spawns are uncapped", ex.launched)
	}
	if strings.Contains(out, "queued") {
		t.Errorf("the result must report no queued worker, got:\n%s", out)
	}
	store, err := loadWorkersLocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(store) != 3 {
		t.Fatalf("store holds %d workers, want 3", len(store))
	}
}

// The ticket cap defaults to three when nothing is stored.
func TestTicketCapDefaultsToThree(t *testing.T) {
	resolverSettings(t, "")
	if got := effectiveTicketCapForSpec(""); got != 3 {
		t.Fatalf("effective cap = %d, want 3", got)
	}
	resolverSettings(t, `{"max_tickets":5}`)
	if got := effectiveTicketCapForSpec(""); got != 5 {
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

// Each worker of one spawn call is its own group, so its result reaches the
// parent when it submits and the parent can merge it at once, instead of every
// worker waiting for the slowest of the call.
func TestEachWorkerOfOneSpawnIsItsOwnGroup(t *testing.T) {
	spawnTestState(t)
	resolverSettings(t, `{"max_tickets":1}`)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
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
	// The groups outlive the call, and the state directory is shared with the
	// tests that run after this one: remove what this test created so their
	// dead-child sweeps never see it.
	t.Cleanup(func() {
		entries, _ := os.ReadDir(groupDir())
		for _, e := range entries {
			g, ok, _ := readSpawnGroup(strings.TrimSuffix(e.Name(), ".json"))
			if ok && len(g.Members) == 1 && strings.HasPrefix(g.Members[0], "grp-") {
				os.Remove(filepath.Join(groupDir(), e.Name()))
			}
		}
	})
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
