package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	el "example.com/emberloop"
)

// A resumed Run seeds the driver from the spec branch, where a merge writes
// its done mark: the checkout still reads open until the final merge to
// master, so seeding from it re-opens every spec-merged Ticket and the idle
// watchdog parks it.
func TestRunDriverFilesReadsSpecBranchDone(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	wt := filepath.Join(repo, ".el-worktrees", "specdone")
	testGit(t, repo, "worktree", "add", "-b", "spec/demo", wt, master)
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", wt) })

	rel := ".scratch/demo/issues/01-first.md"
	done := "# 01: First\n\n**Blocked by:** first (01)\n\n**Status:** done\n\n- [x] alpha\n"
	if err := os.WriteFile(filepath.Join(wt, rel), []byte(done), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, wt, "add", rel)
	testGit(t, wt, "commit", "-m", "mark 01 done")

	specRun, err := el.ResolveSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	files, err := runDriverFiles(repo, "spec/demo", specRun)
	if err != nil {
		t.Fatal(err)
	}
	raw := files[filepath.Join(repo, rel)]
	if !strings.Contains(raw, "**Status:** done") {
		t.Fatalf("driver files read %q, want the spec-branch done mark", raw)
	}
	// Only the Status is taken from the branch: its Blocked by rendering is
	// not the id form the driver parses, so the checkout's line must stay.
	if strings.Contains(raw, "first (01)") {
		t.Fatalf("driver files took the spec-branch body %q, want only its Status", raw)
	}
	if !strings.Contains(raw, "**Blocked by:** None") {
		t.Fatalf("driver files dropped the checkout Blocked by: %q", raw)
	}
	// The checkout keeps the open status: seeding from it is what re-opened
	// the merged Ticket before this fix.
	checkout, err := os.ReadFile(filepath.Join(repo, rel))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(checkout), "**Status:** done") {
		t.Fatalf("checkout ticket = %q, want it left open", checkout)
	}
}

// A Ticket merged into the spec branch stays merged across a Daemon restart:
// the done mark lives on the branch, not the checkout, and a Resume seeds
// from the branch, so the row comes back merged and its dependent starts.
func TestRunResumeKeepsSpecBranchMerged(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	first := ticketByID(r, "01-first").WorkerSession

	// Merge 01-first into the spec branch the way the driver does: submit
	// done, let the reviewers pass, wait for the row to read merged.
	submitSession(t, srv.URL, first, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged"
	}, "01-first merged")
	checkout, err := os.ReadFile(filepath.Join(repo, ".scratch/demo/issues/01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(checkout), "**Status:** done") {
		t.Fatalf("checkout ticket = %q, want it left open until the master merge", checkout)
	}

	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != 200 {
		t.Fatalf("pause status = %d, want 200", code)
	}
	srv.Close()

	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if r := getRun(t, srv2.URL, run.ID); r.Status != "paused" {
		t.Fatalf("reloaded status = %q, want paused", r.Status)
	}
	resumed, code := postRunAction(t, srv2.URL, run.ID, "resume")
	if code != 200 {
		t.Fatalf("resume status = %d, want 200", code)
	}
	if got := ticketByID(resumed, "01-first").State; got != "merged" {
		t.Fatalf("resumed 01-first state = %q, want merged from the spec-branch mark", got)
	}
	// The dependent 02-second is ready on the resumed merge and starts.
	waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "02-second").WorkerSession != ""
	}, "02-second starts after the resumed merge")
}
