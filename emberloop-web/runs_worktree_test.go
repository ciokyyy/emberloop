package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	el "example.com/emberloop"
)

// Ticket 03: every Ticket worktree carries a local git identity and a
// read-only mirror of the spec directory, so Workers hit no stat errors or
// aborted rebases. The Run seam here is the same fake-Backend plus real-git
// seam runs_driver_test.go drives: start a Run and assert on the worktree it
// creates.
func TestRunWorktreeHasIdentityAndReadOnlySpec(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	// A new spec file created after the base commit must still be readable
	// from the worktree: a fresh worktree add only carries committed files,
	// and the mirror fills the gap.
	newTicket := filepath.Join(repo, ".scratch", "demo", "issues", "09-uncommitted.md")
	if err := os.WriteFile(newTicket, []byte("# 09: Uncommitted\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	_ = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker for the first ticket")

	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	if st, err := os.Stat(worktree); err != nil || !st.IsDir() {
		t.Fatalf("worktree = %v, %v; want the 01-first checkout", worktree, err)
	}

	// Local git identity: commits and the finish rebase need it.
	if got := strings.TrimSpace(testGit(t, worktree, "config", "user.name")); got != worktreeGitUserName {
		t.Fatalf("worktree user.name = %q, want %q", got, worktreeGitUserName)
	}
	if got := strings.TrimSpace(testGit(t, worktree, "config", "user.email")); got != worktreeGitUserEmail {
		t.Fatalf("worktree user.email = %q, want %q", got, worktreeGitUserEmail)
	}

	// Spec mirror: the same repo-relative directory, readable in the
	// worktree, including the uncommitted gap file.
	specDir := filepath.Dir(spec)
	rel, err := filepath.Rel(repo, specDir)
	if err != nil {
		t.Fatal(err)
	}
	mirrorSpec := filepath.Join(worktree, rel, "spec.md")
	if mraw, err := os.ReadFile(mirrorSpec); err != nil || !strings.Contains(string(mraw), "Demo spec") {
		t.Fatalf("mirror spec = %q, %v; want the committed spec", mraw, err)
	}
	mirrorGap := filepath.Join(worktree, rel, "issues", "09-uncommitted.md")
	if graw, err := os.ReadFile(mirrorGap); err != nil || !strings.Contains(string(graw), "extra") {
		t.Fatalf("mirror gap file = %q, %v; want the uncommitted file readable", graw, err)
	}
	mirrorTicket := filepath.Join(worktree, rel, "issues", "01-first.md")
	if traw, err := os.ReadFile(mirrorTicket); err != nil || !strings.Contains(string(traw), "alpha") {
		t.Fatalf("mirror ticket = %q, %v; want the alpha checkbox", traw, err)
	}

	// Read-only: no write bit anywhere under the mirror, so a Worker cannot
	// tick its own copy. The main checkout stays writable: only the driver
	// ticks boxes there.
	assertNoWriteBits(t, filepath.Join(worktree, rel))
	if st, err := os.Stat(spec); err != nil || st.Mode().Perm()&0o200 == 0 {
		t.Fatalf("main spec mode = %v, %v; want owner-writable for the driver", st.Mode(), err)
	}
	if f, err := os.OpenFile(mirrorTicket, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		t.Fatalf("mirror ticket opened for writing; want read-only")
	}
}

// Ticket 03: the driver ticks the main checkout, never the worktree mirror.
// Ticking the main ticket file succeeds while the mirror stays unticked.
func TestRunWorktreeMirrorStaysUntickedWhenDriverTicks(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	_ = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker for the first ticket")

	mainTicket := filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")
	mirrorTicket := filepath.Join(repo, ".el-worktrees", "01-first", ".scratch", "demo", "issues", "01-first.md")
	before, err := os.ReadFile(mirrorTicket)
	if err != nil {
		t.Fatalf("mirror ticket not readable: %v", err)
	}
	d.mu.Lock()
	path := ticketPath(d.runs[run.ID], "01-first")
	d.mu.Unlock()
	if path != mainTicket {
		t.Fatalf("ticket path = %q, want the main checkout %q", path, mainTicket)
	}
	miss, err := el.TickDriverTicketFile(path, []string{"alpha"})
	if err != nil || len(miss) != 0 {
		t.Fatalf("driver tick = %v, %v; want the alpha box ticked", miss, err)
	}
	after, err := os.ReadFile(mirrorTicket)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("mirror ticket changed after the driver tick; want the driver to tick only the main checkout")
	}
	raw, err := os.ReadFile(mainTicket)
	if err != nil || !strings.Contains(string(raw), "[x] alpha") {
		t.Fatalf("main ticket = %q, %v; want the ticked alpha box", raw, err)
	}
}

func assertNoWriteBits(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Directories stay writable so `git worktree remove` can delete the
		// mirror; the read-only guarantee lives on the files.
		if e.IsDir() {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("%s mode = %o; want no write bits (read-only)", path, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
