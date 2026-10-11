package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// isAncestor reports whether ancestor is an ancestor of ref in repo: git
// merge-base --is-ancestor exits 0 when it holds and 1 when it does not,
// and testGit fatals on any nonzero exit, so this wraps exec directly.
func isAncestor(t *testing.T, repo, ancestor, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, ref)
	cmd.Dir = repo
	if err := cmd.Run(); err == nil {
		return true
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false
	} else {
		t.Fatalf("git merge-base --is-ancestor %s %s: %v", ancestor, ref, err)
		return false
	}
}

// The spec branch is created from the master tip at Start: a fresh Run gets
// spec/<slug> at the master tip, records the branch and the base SHA on the
// row, and forks the first ticket branch from it.
func TestSpecBranchCreatedFromMasterAtStart(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.SpecBranch != "spec/demo" {
		t.Fatalf("specBranch = %q, want spec/demo", run.SpecBranch)
	}
	if run.SpecBase != master {
		t.Fatalf("specBase = %q, want the master tip %q", run.SpecBase, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != master {
		t.Fatalf("spec/demo = %q, want the master tip %q", got, master)
	}
	// The main checkout never moves at Start.
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != master {
		t.Fatalf("main = %q, want the untouched tip %q", got, master)
	}
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	_ = r
	// The ticket branch forks from the spec branch, so it starts at the
	// spec tip.
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first")); got != master {
		t.Fatalf("implement/01-first = %q, want the spec tip %q", got, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "merge-base", "spec/demo", "implement/01-first")); got != master {
		t.Fatalf("merge-base = %q, want %q", got, master)
	}
}

// A spec branch that already exists is reused as it stands: Start records
// the current master tip as the base but never resets the branch to it.
func TestSpecBranchReusedWhenPresent(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	// Pre-create the spec branch with a commit master lacks.
	specWT := filepath.Join(repo, ".el-worktrees", "preexisting")
	testGit(t, repo, "worktree", "add", "-b", "spec/demo", specWT, master)
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", specWT) })
	writeFile(t, specWT, "spec-note.txt", "spec\n")
	testGit(t, specWT, "add", "spec-note.txt")
	testGit(t, specWT, "commit", "-m", "spec work")
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if specTip == master {
		t.Fatalf("spec/demo = %q, want it ahead of master %q", specTip, master)
	}

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.SpecBranch != "spec/demo" {
		t.Fatalf("specBranch = %q, want spec/demo", run.SpecBranch)
	}
	if run.SpecBase != master {
		t.Fatalf("specBase = %q, want the master tip %q", run.SpecBase, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != specTip {
		t.Fatalf("spec/demo = %q, want the reused tip %q", got, specTip)
	}
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	_ = r
	// The ticket sees the reused spec work, not bare master.
	if !isAncestor(t, repo, specTip, "implement/01-first") {
		t.Fatalf("spec tip %q is not an ancestor of implement/01-first, want the ticket forked from it", specTip)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "implement/01-first:spec-note.txt")); got != "spec" {
		t.Fatalf("ticket branch spec-note.txt = %q, want the reused spec work", got)
	}
}

// Tickets merge into the spec branch, never master: after a full chain run
// master is untouched while the spec branch carries every ticket's done
// commit, and each later ticket forked from the earlier merge.
func TestTicketsMergeIntoSpecNotMaster(t *testing.T) {
	repo, spec := initRunRepo(t, chainTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" &&
			ticketByID(r, "02-second").WorkerSession != ""
	}, "01-first merged and 02-second started")
	// The second ticket forked from the spec branch after the first merge,
	// so it sees the first ticket's done commit.
	firstSHA := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if !isAncestor(t, repo, firstSHA, "implement/02-second") {
		t.Fatalf("spec tip %q is not an ancestor of implement/02-second, want the ticket forked from the first merge", firstSHA)
	}
	submitSession(t, srv.URL, ticketByID(r, "02-second").WorkerSession, "done", []string{"beta"})
	passReviewers(t, srv.URL, run.ID, "02-second")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "02-second").State == "merged" &&
			ticketByID(r, "03-third").WorkerSession != ""
	}, "02-second merged and 03-third started")
	submitSession(t, srv.URL, ticketByID(r, "03-third").WorkerSession, "done", []string{"gamma"})
	passReviewers(t, srv.URL, run.ID, "03-third")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	_ = r
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != master {
		t.Fatalf("main = %q, want the untouched tip %q", got, master)
	}
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if specTip == master {
		t.Fatalf("spec/demo = %q, want it ahead of master", specTip)
	}
	for _, id := range []string{"01-first", "02-second", "03-third"} {
		raw := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/"+id+".md"))
		if !strings.Contains(raw, "done") {
			t.Fatalf("spec %s = %q, want Status done", id, raw)
		}
	}
}

// A master that moves mid-run never blocks a ticket and never leaks into
// the spec branch: the finish rebases onto the spec branch as it stands,
// not onto master.
func TestMovingMasterDoesNotBlockTicket(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	// Master moves while the worker is out with an unrelated file: the move
	// must neither stop the spec-branch merge nor leak into the spec branch.
	writeFile(t, repo, "master.txt", "master\n")
	testGit(t, repo, "add", "master.txt")
	testGit(t, repo, "commit", "-m", "master moves")
	moved := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	if moved == master {
		t.Fatalf("main = %q, want it moved past %q", moved, master)
	}
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done despite the master move")
	_ = r
	// The spec branch carries the ticket and not the master move; master
	// carries the move and not the ticket.
	if isAncestor(t, repo, moved, "spec/demo") {
		t.Fatalf("master move %q is an ancestor of spec/demo, want it kept out", moved)
	}
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if isAncestor(t, repo, specTip, "main") {
		t.Fatalf("spec tip %q is an ancestor of main, want master without the ticket", specTip)
	}
}

// The branch name and base SHA persist in the Run file and resume after a
// restart: the reloaded Run merges into the same spec branch.
func TestSpecBranchPersistsAcrossRestart(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.SpecBranch != "spec/demo" || run.SpecBase != master {
		t.Fatalf("run = %+v, want spec/demo at %q", run, master)
	}
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	first := ticketByID(r, "01-first").WorkerSession

	raw, err := os.ReadFile(filepath.Join(stateDir, "enderbloom", "runs", run.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		SpecBranch string `json:"specBranch"`
		SpecBase   string `json:"specBase"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SpecBranch != "spec/demo" || saved.SpecBase != master {
		t.Fatalf("run file = %+v, want spec/demo at %q", saved, master)
	}

	// Pausing freezes the old driver the way a real restart kills it; the
	// reloaded Daemon below is the only driver afterwards.
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != 200 {
		t.Fatalf("pause status = %d, want 200", code)
	}
	srv.Close()

	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	reloaded := getRun(t, srv2.URL, run.ID)
	if reloaded.SpecBranch != "spec/demo" || reloaded.SpecBase != master {
		t.Fatalf("reloaded run = %+v, want spec/demo at %q", reloaded, master)
	}
	if got := ticketByID(reloaded, "01-first").WorkerSession; got != first {
		t.Fatalf("reloaded worker = %q, want the same %q", got, first)
	}
	if _, code := postRunAction(t, srv2.URL, run.ID, "resume"); code != 200 {
		t.Fatalf("resume status = %d, want 200", code)
	}
	submitSession(t, srv2.URL, first, "done", []string{"alpha"})
	passReviewers(t, srv2.URL, run.ID, "01-first")
	r = waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" &&
			ticketByID(r, "02-second").WorkerSession != ""
	}, "01-first merged and 02-second started after resume")
	submitSession(t, srv2.URL, ticketByID(r, "02-second").WorkerSession, "done", []string{"beta"})
	passReviewers(t, srv2.URL, run.ID, "02-second")
	_ = waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != master {
		// The restart changed nothing about the target: master stays put
		// while the spec branch carries the work.
		t.Fatalf("main = %q, want the untouched tip %q", got, master)
	}
	specRaw := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/01-first.md"))
	if !strings.Contains(specRaw, "done") {
		t.Fatalf("spec 01-first = %q, want Status done after the resumed merge", specRaw)
	}
}

// A spec branch checked out in another worktree still merges: the finish
// moves the ref with update-ref instead of pushing, so a User working on
// spec/<slug> by hand never wedges the ticket (their checkout falls behind).
func TestSpecBranchCheckedOutElsewhereStillMerges(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	// The User checks out the spec branch by hand while the ticket runs.
	sideWT := filepath.Join(repo, ".el-worktrees", "side-hand")
	testGit(t, repo, "worktree", "add", sideWT, "spec/demo")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", sideWT) })
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "worker.txt", "worker\n")
	testGit(t, worktree, "add", "worker.txt")
	testGit(t, worktree, "commit", "-m", "worker change")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done with the spec branch checked out elsewhere")
	_ = r
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != master {
		t.Fatalf("main = %q, want the untouched tip %q", got, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:worker.txt")); got != "worker" {
		t.Fatalf("spec worker.txt = %q, want the merged worker change", got)
	}
}

// The fast-forward refuses when the ticket tip does not contain the spec
// tip: the worker stays alive with a clear refusal instead of forcing the
// ref. Equal tips are a no-op success.
func TestFFUpdateSpecBranchRefusesNonFastForward(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	base := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	// Diverge the spec branch from main: neither contains the other.
	sideWT := filepath.Join(repo, ".el-worktrees", "side-diverge")
	testGit(t, repo, "worktree", "add", "-b", "spec/demo", sideWT, base)
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", sideWT) })
	writeFile(t, sideWT, "spec.txt", "spec\n")
	testGit(t, sideWT, "add", "spec.txt")
	testGit(t, sideWT, "commit", "-m", "spec work")
	writeFile(t, repo, "main.txt", "main\n")
	testGit(t, repo, "add", "main.txt")
	testGit(t, repo, "commit", "-m", "main work")
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if _, kept, err := ffUpdateSpecBranch(repo, "main", "spec/demo", "01-first"); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(kept, "moved under the ticket") {
		t.Fatalf("kept = %q, want the moved-under refusal", kept)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != specTip {
		t.Fatalf("spec/demo = %q, want the untouched tip %q", got, specTip)
	}
	if sha, kept, err := ffUpdateSpecBranch(repo, "spec/demo", "spec/demo", "01-first"); err != nil || kept != "" || sha != specTip {
		t.Fatalf("equal-tip update = %q %q %v, want %q with no refusal", sha, kept, err, specTip)
	}
}
