package web

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// twoIdleTickets are two independent tickets, so a block on the first still
// leaves the second free to prove the Run keeps going.
func twoIdleTickets() map[string]string {
	return map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	}
}

// gitOK runs git and reports success without failing the test, for the
// negative probes (branch absent, path no worktree) the reconcile answers.
func gitOK(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// A stale implement branch with no unique commits (the leftover that failed
// Luna runs 5b92408e and 9734f809 with "a branch named ... already exists")
// is dropped and the ticket starts normally.
func TestStartReusesMergedStaleBranch(t *testing.T) {
	repo, spec := initRunRepo(t, singleIdleTicket())
	testGit(t, repo, "branch", "implement/01-first")

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started past the stale branch")
	if r.Status == "failed" {
		t.Fatalf("run status = failed, want running: error=%q", r.Error)
	}
	if got := strings.TrimSpace(testGit(t, repo, "worktree", "list", "--porcelain")); !strings.Contains(got, filepath.Join(repo, ".el-worktrees", "01-first")) {
		t.Fatalf("worktree list = %q, want the 01-first checkout", got)
	}
	pauseTestRun(t, srv.URL, run.ID)
}

// A stale directory at the worktree path that git never registered is
// cleaned and the ticket starts normally.
func TestStartCleansUnregisteredWorktreePath(t *testing.T) {
	repo, spec := initRunRepo(t, singleIdleTicket())
	stale := filepath.Join(repo, ".el-worktrees", "01-first")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "junk.txt"), []byte("stale output"), 0o644); err != nil {
		t.Fatal(err)
	}
	if gitOK(repo, "worktree", "list", "--porcelain") {
		out, _ := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").CombinedOutput()
		if strings.Contains(string(out), stale) {
			t.Fatalf("stale path unexpectedly registered: %s", out)
		}
	}

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started past the stale directory")
	if r.Status == "failed" {
		t.Fatalf("run status = failed, want running: error=%q", r.Error)
	}
	if _, err := os.Stat(filepath.Join(stale, "junk.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale junk still present: %v", err)
	}
	pauseTestRun(t, srv.URL, run.ID)
}

// A stale branch holding unmerged work from an earlier Run is never
// deleted: the ticket goes Needs-you with the remedy while the Run and its
// other tickets keep going.
func TestStartBlockedOnUnmergedBranchNeedsYou(t *testing.T) {
	repo, spec := initRunRepo(t, twoIdleTickets())
	testGit(t, repo, "branch", "implement/01-first")
	testGit(t, repo, "checkout", "implement/01-first")
	writeFile(t, repo, "stale-work.txt", "work an earlier run left behind\n")
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", "stale unmerged work")
	testGit(t, repo, "checkout", "main")

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.State == "needs-you" && ticketByID(r, "02-second").WorkerSession != ""
	}, "blocked first ticket needs-you while the second starts")
	if r.Status == "failed" {
		t.Fatalf("run status = failed, want running: error=%q", r.Error)
	}
	tk := ticketByID(r, "01-first")
	if !strings.Contains(tk.NeedsYou, "implement/01-first") || !strings.Contains(tk.NeedsYou, "unmerged") {
		t.Fatalf("needs-you reason = %q, want the stale branch named with its remedy", tk.NeedsYou)
	}
	if tk.WorkerSession != "" {
		t.Fatalf("blocked ticket holds worker %q, want none", tk.WorkerSession)
	}
	// The branch survives: its commits are a human's to save or drop.
	if !gitOK(repo, "show-ref", "--verify", "--quiet", "refs/heads/implement/01-first") {
		t.Fatalf("stale branch implement/01-first was deleted; want it kept")
	}
	pauseTestRun(t, srv.URL, run.ID)
}

// A registered worktree holding uncommitted changes may be a live worker's
// desk: the start refuses it as Needs-you instead of force-removing it.
func TestStartBlockedOnDirtyWorktreeNeedsYou(t *testing.T) {
	repo, spec := initRunRepo(t, singleIdleTicket())
	testGit(t, repo, "worktree", "add", "-b", "implement/01-first", filepath.Join(repo, ".el-worktrees", "01-first"), "HEAD")
	writeFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "dirty.txt", "uncommitted\n")

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "needs-you"
	}, "dirty worktree needs-you")
	if r.Status == "failed" {
		t.Fatalf("run status = failed, want running: error=%q", r.Error)
	}
	tk := ticketByID(r, "01-first")
	if !strings.Contains(tk.NeedsYou, "uncommitted changes") {
		t.Fatalf("needs-you reason = %q, want the dirty worktree named", tk.NeedsYou)
	}
	if _, err := os.Stat(filepath.Join(repo, ".el-worktrees", "01-first", "dirty.txt")); err != nil {
		t.Fatalf("dirty worktree was removed: %v", err)
	}
	pauseTestRun(t, srv.URL, run.ID)
}
