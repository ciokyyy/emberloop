package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	t.Parallel()
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

	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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

// Tickets merge into the spec branch as they finish. Once the whole chain
// completes, the final gate moves master to the checked spec-branch tip.
func TestTicketsMergeIntoSpecThenMasterAtFinalGate(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, chainTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	mainTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if mainTip != specTip || specTip == master {
		t.Fatalf("main = %q, spec/demo = %q, initial master = %q; want master advanced to the completed spec", mainTip, specTip, master)
	}
	for _, id := range []string{"01-first", "02-second", "03-third"} {
		raw := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/"+id+".md"))
		if !strings.Contains(raw, "done") {
			t.Fatalf("spec %s = %q, want Status done", id, raw)
		}
	}
}

// A master move during a Ticket does not block that Ticket. At the final
// merge gate, the completed spec branch is rebased onto the moved master.
func TestMovingMasterDoesNotBlockTicket(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	// Master moves while the Worker runs: it neither blocks the Ticket merge
	// nor leaks into the spec branch until the final-merge rebase.
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
	// Finalization rebases the spec branch onto the moved master and advances
	// master only after the final check succeeds.
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	if !isAncestor(t, repo, moved, specTip) {
		t.Fatalf("master move %q is not an ancestor of spec/demo %q", moved, specTip)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != specTip {
		t.Fatalf("main = %q, want the checked spec tip %q", got, specTip)
	}
}

// The branch name and base SHA persist in the Run file and resume after a
// restart: the reloaded Run merges into the same spec branch.
func TestSpecBranchPersistsAcrossRestart(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	stateDir := tempDir(t)
	d := runTestDaemon(t, stateDir, NewFakeBackend())
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

	// Graceful restart freezes the Run while preserving the Worker Session;
	// the reloaded Daemon below is the only driver afterwards.
	if !d.RequestRestart() {
		t.Fatal("RequestRestart returned false")
	}
	srv.Close()

	d2 := runTestDaemon(t, stateDir, NewFakeBackend())
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
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")) {
		t.Fatalf("main = %q, want the final spec tip", got)
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
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")) {
		t.Fatalf("main = %q, want the final spec tip", got)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:worker.txt")); got != "worker" {
		t.Fatalf("spec worker.txt = %q, want the merged worker change", got)
	}
}

// The fast-forward refuses when the ticket tip does not contain the spec
// tip: the worker stays alive with a clear refusal instead of forcing the
// ref. Equal tips are a no-op success.
func TestFFUpdateSpecBranchRefusesNonFastForward(t *testing.T) {
	t.Parallel()
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

// A checkout sitting on another branch at Start still forks the spec branch
// from the master tip: ensureSpecBranch resolves the master ref explicitly,
// never HEAD.
func TestSpecBranchForksFromMasterWhenCheckoutElsewhere(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	// Move the checkout onto a side branch HEAD does not share with master.
	testGit(t, repo, "checkout", "-b", "side")
	writeFile(t, repo, "side.txt", "side\n")
	testGit(t, repo, "add", "side.txt")
	testGit(t, repo, "commit", "-m", "side work")
	side := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	if side == master {
		t.Fatalf("HEAD = %q, want it off the master tip %q", side, master)
	}
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.SpecBase != master {
		t.Fatalf("specBase = %q, want the master tip %q", run.SpecBase, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != master {
		t.Fatalf("spec/demo = %q, want the master tip %q", got, master)
	}
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	_ = r
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first")); got != master {
		t.Fatalf("implement/01-first = %q, want the spec tip %q", got, master)
	}
}

// Reuse still records the master tip at Start when the checkout sits
// elsewhere: the branch keeps its own tip while the base names master for
// the later final-merge rebase.
func TestSpecBranchReuseRecordsMasterTipWhenCheckoutElsewhere(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	// Pre-create the spec branch with a commit master lacks.
	specWT := filepath.Join(repo, ".el-worktrees", "preexisting")
	testGit(t, repo, "worktree", "add", "-b", "spec/demo", specWT, master)
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", specWT) })
	writeFile(t, specWT, "spec-note.txt", "spec\n")
	testGit(t, specWT, "add", "spec-note.txt")
	testGit(t, specWT, "commit", "-m", "spec work")
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	// Move the checkout onto a side branch, so HEAD names neither master
	// nor the spec branch.
	testGit(t, repo, "checkout", "-b", "side")
	writeFile(t, repo, "side.txt", "side\n")
	testGit(t, repo, "add", "side.txt")
	testGit(t, repo, "commit", "-m", "side work")
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.SpecBase != master {
		t.Fatalf("specBase = %q, want the master tip %q", run.SpecBase, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != specTip {
		t.Fatalf("spec/demo = %q, want the reused tip %q", got, specTip)
	}
}

// A checkpoint rebase updates the spec branch to master, records the new
// base and reports how many master commits the branch was behind.
func TestRebaseSpecBranchOntoMaster(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.SpecBehind != 0 {
		t.Fatalf("specBehind at start = %d, want 0", run.SpecBehind)
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")

	writeFile(t, repo, "master-only.txt", "master\n")
	testGit(t, repo, "add", "master-only.txt")
	testGit(t, repo, "commit", "-m", "master moves")
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	if got := getRun(t, srv.URL, run.ID).SpecBehind; got != 1 {
		t.Fatalf("specBehind after master moves = %d, want 1", got)
	}

	rebased, code := postRunAction(t, srv.URL, run.ID, "rebase")
	if code != http.StatusOK {
		t.Fatalf("rebase status = %d, want 200", code)
	}
	if !isAncestor(t, repo, master, "spec/demo") {
		t.Fatalf("master tip %s is not an ancestor of rebased spec branch", master)
	}
	if rebased.SpecBase != master || rebased.SpecBehind != 0 {
		t.Fatalf("rebased base/distance = %s/%d, want %s/0", rebased.SpecBase, rebased.SpecBehind, master)
	}
}

// A checkpoint conflict leaves the spec ref unchanged and publishes a
// needs-you Run snapshot naming the conflicting files.
func TestRebaseConflictNeedsYouWithFiles(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")

	branchTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	specWorktree := filepath.Join(repo, ".el-worktrees", "manual-spec")
	testGit(t, repo, "worktree", "add", specWorktree, "spec/demo")
	t.Cleanup(func() { _, _ = runGit(repo, "worktree", "remove", "--force", specWorktree) })
	writeFile(t, specWorktree, "shared.txt", "spec version\n")
	testGit(t, specWorktree, "add", "shared.txt")
	testGit(t, specWorktree, "commit", "-m", "spec change")
	branchTip = strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	writeFile(t, repo, "shared.txt", "master version\n")
	testGit(t, repo, "add", "shared.txt")
	testGit(t, repo, "commit", "-m", "master change")

	conflicted, code := postRunAction(t, srv.URL, run.ID, "rebase")
	if code != http.StatusOK {
		t.Fatalf("rebase status = %d, want 200", code)
	}
	if conflicted.Status != runStatusNeedsYou || !strings.Contains(conflicted.Error, "shared.txt") {
		t.Fatalf("rebase conflict snapshot = status %q error %q, want needs-you naming shared.txt", conflicted.Status, conflicted.Error)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != branchTip {
		t.Fatalf("spec branch tip = %s after conflict, want unchanged %s", got, branchTip)
	}
	testGit(t, specWorktree, "reset", "--hard", "main")
	resolved, code := postRunAction(t, srv.URL, run.ID, "rebase")
	if code != http.StatusOK {
		t.Fatalf("resolved rebase status = %d, want 200", code)
	}
	if resolved.Status != runStatusNeedsYou || !strings.Contains(resolved.Error, "succeeded; resume the Run") || resolved.SpecBehind != 0 {
		t.Fatalf("resolved rebase snapshot = status %q error %q behind %d; want needs-you, resume guidance and 0 behind", resolved.Status, resolved.Error, resolved.SpecBehind)
	}
	resumed, code := postRunAction(t, srv.URL, run.ID, "resume")
	if code != http.StatusOK || resumed.Status != runStatusRunning || resumed.Error != "" {
		t.Fatalf("resume after successful rebase = status %d, Run %q error %q", code, resumed.Status, resumed.Error)
	}
	testGit(t, repo, "worktree", "remove", "--force", specWorktree)
	abandoned, code := postRunAction(t, srv.URL, run.ID, "abandon")
	if code != http.StatusOK || abandoned.Status != runStatusAbandoned {
		t.Fatalf("cleanup abandon = status %d, Run %q", code, abandoned.Status)
	}
}

// Abandon ends the Run's Sessions and removes its Ticket and spec worktrees
// and branches, while keeping an abandoned Run row for the UI.
func TestAbandonRunRemovesBranchesAndWorktrees(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	run = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(run, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")

	abandoned, code := postRunAction(t, srv.URL, run.ID, "abandon")
	if code != http.StatusOK || abandoned.Status != runStatusAbandoned {
		t.Fatalf("abandon = status %d, Run %q; want 200 abandoned", code, abandoned.Status)
	}
	for _, branch := range []string{"spec/demo", "implement/01-first"} {
		if exists, err := specBranchExists(repo, branch); err != nil || exists {
			t.Fatalf("branch %s exists after abandon: exists=%v err=%v", branch, exists, err)
		}
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("Ticket worktree still exists after abandon: stat err=%v", err)
	}
	if got := getSession(t, srv.URL, worker).Meta.Status; got != "ended" {
		t.Fatalf("Worker status after abandon = %q, want ended", got)
	}
}
func writeGeneratedTestFiles(root string) error {
	files := map[string]string{
		"emberloop-web/web/dist/index.html":       "generated bundle\n",
		"emberloop-web/web/package-lock.json":     "generated lock\n",
		"emberloop-web/web/e2e/package-lock.json": "generated e2e lock\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestFinalMergeBuildsGeneratedFilesRebasesAndChecksBeforeMovingMaster(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	var builds, checks atomic.Int32
	d.runFinalBuild = func(worktree string) (string, error) {
		builds.Add(1)
		if err := writeGeneratedTestFiles(worktree); err != nil {
			return "", err
		}
		return "generated", nil
	}
	d.runRepoCheck = func(worktree string) (string, error) {
		checks.Add(1)
		for _, file := range []string{"emberloop-web/web/dist/index.html", "emberloop-web/web/package-lock.json", "emberloop-web/web/e2e/package-lock.json"} {
			if _, err := runGit(worktree, "cat-file", "-e", "HEAD:"+file); err != nil {
				return "generated files were not committed before check", err
			}
		}
		return "green", nil
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	initialMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	if builds.Load() != 0 || checks.Load() != 0 {
		t.Fatalf("final build/check ran before all tickets merged: %d/%d", builds.Load(), checks.Load())
	}
	// Master moves independently while the Ticket is active. The final gate
	// must rebase onto this commit before checking the merge result.
	writeFile(t, repo, "master-only.txt", "master change\n")
	testGit(t, repo, "add", "master-only.txt")
	testGit(t, repo, "commit", "-m", "master moves")
	movedMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	if movedMaster == initialMaster {
		t.Fatal("master did not move in the fixture")
	}

	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool { return r.Status == "done" }, "green final merge")
	if builds.Load() != 1 || checks.Load() != 1 {
		t.Fatalf("final build/check calls = %d/%d, want once each", builds.Load(), checks.Load())
	}
	specTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	mainTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	if specTip != mainTip || !isAncestor(t, repo, movedMaster, mainTip) {
		t.Fatalf("main=%s spec=%s moved-master=%s; want green spec rebased and merged", mainTip, specTip, movedMaster)
	}
	for file, want := range map[string]string{
		"emberloop-web/web/dist/index.html":       "generated bundle",
		"emberloop-web/web/package-lock.json":     "generated lock",
		"emberloop-web/web/e2e/package-lock.json": "generated e2e lock",
		"master-only.txt":                         "master change",
	} {
		got := strings.TrimSpace(testGit(t, repo, "show", "main:"+file))
		if got != want {
			t.Errorf("main:%s = %q, want %q", file, got, want)
		}
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD")); got != mainTip {
		t.Fatalf("checked-out HEAD = %s, master ref = %s; fast-forward must update the worktree", got, mainTip)
	}
	if got := strings.TrimSpace(testGit(t, repo, "status", "--porcelain")); got != "" {
		t.Fatalf("checked-out master has post-merge diff: %s", got)
	}
	_ = r
}

func TestFinalMergeRedCheckNeedsYouAndKeepsMaster(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	initialMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	var builds, checks atomic.Int32
	var failCheck atomic.Bool
	failCheck.Store(true)
	d.runFinalBuild = func(worktree string) (string, error) {
		builds.Add(1)
		return "", writeGeneratedTestFiles(worktree)
	}
	d.runRepoCheck = func(string) (string, error) {
		checks.Add(1)
		if failCheck.Load() {
			return "FAIL: scripted repo check", errors.New("check failed")
		}
		return "green", nil
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool { return r.Status == runStatusNeedsYou }, "red final check needs-you")
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != initialMaster {
		t.Fatalf("main = %q after red check, want unchanged %q", got, initialMaster)
	}
	if !strings.Contains(r.Error, "FAIL: scripted repo check") || !containsString(r.FinalMergeFiles, "emberloop-web/web/dist/index.html") {
		t.Fatalf("final gate diagnostics = %q files=%v, want failing output and generated paths", r.Error, r.FinalMergeFiles)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:emberloop-web/web/dist/index.html")); got != "generated bundle" {
		t.Fatalf("generated output on spec branch = %q, want committed output", got)
	}

	failCheck.Store(false)
	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != 200 {
		t.Fatalf("resume after red check = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool { return r.Status == "done" }, "resumed green final check")
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")) {
		t.Fatalf("main = %q, want resumed spec tip", got)
	}
	if builds.Load() != 2 || checks.Load() != 2 {
		t.Fatalf("build/check calls after resume = %d/%d, want 2/2", builds.Load(), checks.Load())
	}
	_ = r
}

func TestFinalMergeConflictNeedsYouListsFilesAndKeepsMaster(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n",
	})
	writeFile(t, repo, "shared.txt", "base\n")
	testGit(t, repo, "add", "shared.txt")
	testGit(t, repo, "commit", "-m", "add shared base")
	initialMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	var builds, checks atomic.Int32
	d.runFinalBuild = func(string) (string, error) { builds.Add(1); return "", nil }
	d.runRepoCheck = func(string) (string, error) { checks.Add(1); return "green", nil }
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "first worker started")
	firstWT := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, firstWT, "shared.txt", "spec change\n")
	testGit(t, firstWT, "add", "shared.txt")
	testGit(t, firstWT, "commit", "-m", "change shared file")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" && ticketByID(r, "02-second").WorkerSession != ""
	}, "first merged and second started")
	writeFile(t, repo, "shared.txt", "master change\n")
	testGit(t, repo, "add", "shared.txt")
	testGit(t, repo, "commit", "-m", "conflicting master change")
	movedMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	if movedMaster == initialMaster {
		t.Fatal("master did not move in the fixture")
	}
	submitSession(t, srv.URL, ticketByID(r, "02-second").WorkerSession, "done", []string{"beta"})
	passReviewers(t, srv.URL, run.ID, "02-second")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool { return r.Status == runStatusNeedsYou }, "final rebase conflict needs-you")
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != movedMaster {
		t.Fatalf("main = %q after conflict, want unchanged %q", got, movedMaster)
	}
	if !containsString(r.FinalMergeFiles, "shared.txt") || !strings.Contains(r.Error, "shared.txt") {
		t.Fatalf("final conflict = %q files=%v, want shared.txt listed", r.Error, r.FinalMergeFiles)
	}
	if builds.Load() != 0 || checks.Load() != 0 {
		t.Fatalf("build/check ran before resolving the conflict: %d/%d", builds.Load(), checks.Load())
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
