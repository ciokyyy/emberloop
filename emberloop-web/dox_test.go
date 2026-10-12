package web

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	el "example.com/emberloop"
)

// ensureAgentsMD creates a root AGENTS.md only when the directory holds none,
// and never touches an existing file.
func TestEnsureAgentsMD(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	created, err := ensureAgentsMD(dir)
	if err != nil || !created {
		t.Fatalf("ensure empty = %v, %v; want created, nil", created, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# AGENTS.md") || !strings.Contains(string(raw), "Child DOX Index") {
		t.Fatalf("template lacks the DOX shape: %q", raw)
	}
	again, err := ensureAgentsMD(dir)
	if err != nil || again {
		t.Fatalf("ensure again = %v, %v; want false, nil", again, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := ensureAgentsMD(dir)
	if err != nil || third {
		t.Fatalf("ensure over mine = %v, %v; want false, nil", third, err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(raw) != "mine\n" {
		t.Fatalf("existing file touched: %q", raw)
	}
	override := tempDir(t)
	if err := os.WriteFile(filepath.Join(override, "AGENTS.override.md"), []byte("override\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if created, err := ensureAgentsMD(override); err != nil || created {
		t.Fatalf("ensure over override = %v, %v; want false, nil", created, err)
	}
	if _, err := ensureAgentsMD(filepath.Join(dir, "missing")); err == nil {
		t.Fatalf("ensure in a missing directory = nil, want an error")
	}
}

// A Run-launched Worker in a real git repo without a root AGENTS.md gets the
// seed inside its own worktree, while the main checkout stays clean. The
// fixture commits a root, so drop it first to exercise the seed path.
func TestRunWorkerWorktreeSeedsAgentsMD(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	testGit(t, repo, "rm", "-q", "AGENTS.md")
	testGit(t, repo, "commit", "-qm", "drop root")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return len(r.Tickets) > 0 && r.Tickets[0].WorkerSession != ""
	}, "worker for the first ticket")

	wt, err := os.ReadDir(filepath.Join(repo, ".el-worktrees"))
	if err != nil || len(wt) != 1 {
		t.Fatalf("worktrees = %+v, %v; want the one worker checkout", wt, err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".el-worktrees", wt[0].Name(), "AGENTS.md"))
	if err != nil {
		t.Fatalf("no AGENTS.md in the worker checkout: %v", err)
	}
	if !strings.Contains(string(raw), "Child DOX Index") {
		t.Fatalf("seeded file lacks the DOX shape: %q", raw)
	}
	if _, err := os.Stat(filepath.Join(repo, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("main checkout gained AGENTS.md")
	}
}

// A seeding failure stops the worker like any other start error and leaves
// no worktree or branch behind.
func TestRunStartWorkerEnsureError(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	prev := ensureWorktreeDOX
	ensureWorktreeDOX = func(string) (bool, error) { return false, errors.New("no space") }
	t.Cleanup(func() { ensureWorktreeDOX = prev })

	runID := newID()
	d.mu.Lock()
	d.runs[runID] = &runState{
		ID: runID, Repo: repo, Status: runStatusRunning,
		Harness: "fake", Model: "fake-model", Owner: "u1",
		Sessions: map[string]string{}, Branches: map[string]string{},
		Worktrees: map[string]string{}, Targets: map[string]string{},
	}
	d.mu.Unlock()
	tk := el.DriverTicket{ID: "01-first", Path: filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")}
	err := d.runStartWorker(runID, tk)
	if err == nil || !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("runStartWorker err = %v, want the seeding failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".el-worktrees", "01-first")); !os.IsNotExist(statErr) {
		t.Fatalf("failed start left its worktree behind")
	}
}
