package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No role serves code_review any more: a worker reviews itself by spawning its
// reviewer types, and the tool is gone for every session.
func TestNoRoleServesCodeReview(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"work", "full", "review", "scout", "implement", "plan"} {
		served, err := RoleTools(role)
		if err != nil {
			t.Fatal(err)
		}
		for _, tl := range served {
			if tl.Name() == "code_review" {
				t.Errorf("%s role must not serve code_review", role)
			}
		}
	}
}

// The review roles still reach only read-only tools (execute runs sandboxed
// for them), and implement still does not hold the old review_worker tool.
func TestReviewRolesServeNothingThatWrites(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"review", "scout"} {
		served, err := RoleTools(role)
		if err != nil {
			t.Fatal(err)
		}
		for _, tl := range served {
			if n := tl.Name(); n != "read" && n != "find" && n != "websearch" && n != "ctx_search" && n != "check" && n != "execute" {
				t.Errorf("%s role serves %s", role, n)
			}
		}
	}
	implement, _ := RoleTools("implement")
	for _, tl := range implement {
		if tl.Name() == "review_worker" {
			t.Errorf("implement role must not serve review_worker")
		}
	}
}

// spawnToolFake answers the git and herdr calls a worktree spawn makes for a
// ticket tab: git resolves the base and makes the worktree, tab list is empty,
// tab create opens w1:t1 with root pane w1:p1, and pane run starts the agent.
func spawnToolFake() Exec {
	return func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd == "git" {
			return Result{Stdout: "abc123def\n"}, nil
		}
		switch {
		case len(args) > 1 && args[0] == "tab" && args[1] == "list":
			return Result{Stdout: `{"result":{"tabs":[]}}`}, nil
		case len(args) > 1 && args[0] == "tab" && args[1] == "create":
			return Result{Stdout: `{"result":{"root_pane":{"pane_id":"w1:p1","tab_id":"w1:t1"}}}`}, nil
		}
		return Result{}, nil
	}
}

// runSpawnTool drives a worker entry of the spawn tool against a fake herdr.
func runSpawnTool(t *testing.T) (string, error) {
	t.Helper()
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	spawnExec = spawnToolFake()
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })
	// Hermetic panes: the runner itself sits in a herdr pane, so a leaked
	// parent would send the group's post-spawn delivery check at the real
	// herdr and hang the test waiting for its own pane to go idle.
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("EMBERLOOP_AGENT_ID", "")

	repo := t.TempDir()
	ticket := filepath.Join(repo, "20-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	// Keep this test about the spawn: a report warning here would make "must
	// not warn" fail for a reason unrelated to the tab.
	t.Setenv("EMBERLOOP_HERDR_REPORT", "/fake/herdr-report")
	spawn, _ := spawnToolForRole(t, "implement")
	return spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": ticket},
	}})
}

// A spawn opens the worker in its ticket's tab and reports it without a
// warning when the report extension is present.
// A spawn opens the worker in its ticket's tab and reports it without a
// warning when the report extension is present.
func TestSpawnWorkerOpensTheTicketTab(t *testing.T) {
	out, err := runSpawnTool(t)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.Contains(out, "worker 20-x started") || !strings.Contains(out, "w1:t1") {
		t.Errorf("the result must name the worker and its ticket tab, got %q", out)
	}
	if !strings.Contains(out, "end your turn") {
		t.Errorf("a spawn that starts a non-blocking worker must say to end the turn, got %q", out)
	}
	if strings.Contains(out, "warning") {
		t.Errorf("a healthy spawn must not warn, got %q", out)
	}
}

func TestSpawnWorkerWarnsWhenReportExtensionIsMissing(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	spawnExec = spawnToolFake()
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("EMBERLOOP_AGENT_ID", "")

	// No env var and a fresh HOME: the lookup has nowhere to find the
	// extension, which is the case ticket 19 hit.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EMBERLOOP_HERDR_REPORT", "")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")

	repo := t.TempDir()
	ticket := filepath.Join(repo, "21-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawn, _ := spawnToolForRole(t, "implement")
	out, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": ticket},
	}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.Contains(out, "worker 21-x started") {
		t.Errorf("the spawn must still succeed, got %q", out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "unknown") {
		t.Errorf("a missing report extension must be warned about, got %q", out)
	}
}

// The worker reaches its brief, and through it the emberloop:// skill addresses,
// with read. The brief lives beside the worker store, outside the worktree;
// read opens any absolute path, so the launch needs no read exception.
func TestWorkerLaunchCanReadItsBrief(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("EMBERLOOP_AGENT_ID", "")
	var launched string
	base := spawnToolFake()
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
	t.Setenv("EMBERLOOP_SPEC", "/repo/.scratch/p/spec.md")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EMBERLOOP_HERDR_REPORT", "/fake/herdr-report")
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": ticket},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if launched == "" {
		t.Fatal("the worker was never launched")
	}
	if strings.Contains(launched, "EMBERLOOP_READ_ALLOW") {
		t.Errorf("worker launch must not carry a read exception; read opens any path:\n%s", launched)
	}
	// Spawned from an implement session, the worker records its spec.
	w, err := recall("22-x")
	if err != nil {
		t.Fatal(err)
	}
	if w.Spec != "/repo/.scratch/p/spec.md" {
		t.Errorf("worker spec = %q, want the session's EMBERLOOP_SPEC", w.Spec)
	}
}
