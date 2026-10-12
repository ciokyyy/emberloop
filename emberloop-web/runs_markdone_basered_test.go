package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Ticket status and boxes stay untouched while the Worker is under review;
// the spec-branch done commit writes them only after the Ticket merge. The
// spec's own status remains open until a later master-merge stage owns it.
func TestMarkDoneOrderingWaitsForSpecBranchMerge(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	writeFile(t, repo, ".scratch/demo/spec.md", "# Demo spec\n\n**Status:** open\n")
	testGit(t, repo, "add", ".scratch/demo/spec.md")
	testGit(t, repo, "commit", "-m", "add spec status")
	masterBefore := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	mainTicket := filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")
	mainBefore, err := os.ReadFile(mainTicket)
	if err != nil {
		t.Fatal(err)
	}

	fake := NewFakeBackend()
	fake.Delay = 700 * time.Millisecond
	d := runTestDaemon(t, tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	waitReviewers(t, srv.URL, run.ID, "01-first")

	// The submit is not a merge: neither checkout may show speculative progress.
	if got, err := os.ReadFile(mainTicket); err != nil || string(got) != string(mainBefore) {
		t.Fatalf("main ticket before merge = %q, %v; want the untouched open file", got, err)
	}
	specTicketBefore := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/01-first.md"))
	if specTicketBefore != strings.TrimSpace(string(mainBefore)) {
		t.Fatalf("spec ticket before merge = %q, want the open file %q", specTicketBefore, strings.TrimSpace(string(mainBefore)))
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != masterBefore {
		t.Fatalf("main tip before merge = %q, want %q", got, masterBefore)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/spec.md")); !strings.Contains(got, "**Status:** open") {
		t.Fatalf("spec status before master merge = %q, want open", got)
	}

	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	if got := ticketByID(r, "01-first"); got.State != "merged" {
		t.Fatalf("ticket state = %q, want merged", got.State)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/01-first.md")); !strings.Contains(got, "**Status:** done") || !strings.Contains(got, "- [x] alpha") {
		t.Fatalf("merged ticket = %q, want done with alpha ticked", got)
	}
	// The Run is done only after the gated final merge, so master now carries
	// the spec branch and its done-marked Ticket.
	if got, err := os.ReadFile(mainTicket); err != nil || string(got) == string(mainBefore) || !strings.Contains(string(got), "**Status:** done") || !strings.Contains(string(got), "- [x] alpha") {
		t.Fatalf("main ticket after final merge = %q, %v; want the done ticket with alpha ticked", got, err)
	}
	if got := strings.TrimSpace(testGit(t, repo, "status", "--porcelain")); got != "" {
		t.Fatalf("main checkout status = %q, want clean", got)
	}
	if got, want := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")), strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got == masterBefore || got != want {
		t.Fatalf("main tip after final merge = %q, want the spec tip %q (not %q)", got, want, masterBefore)
	}
	if got := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/spec.md")); !strings.Contains(got, "**Status:** open") {
		t.Fatalf("spec status after final merge = %q, want open (no spec status write)", got)
	}
}

// Statusless tickets still get their boxes committed, but only after the
// Ticket branch has been merged into the spec branch.
func TestStatuslessTicketBoxesTickAfterMerge(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n- [ ] alpha\n- [ ] beta\n",
	})
	mainTicket := filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")
	mainBefore, err := os.ReadFile(mainTicket)
	if err != nil {
		t.Fatal(err)
	}
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "statusless ticket worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	_ = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "statusless ticket merged")

	if got, err := os.ReadFile(mainTicket); err != nil || string(got) == string(mainBefore) || !strings.Contains(string(got), "- [x] beta") {
		t.Fatalf("main ticket = %q, %v; want the ticked file after the final merge", got, err)
	}
	specTicket := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/01-first.md"))
	if !strings.Contains(specTicket, "- [x] alpha") || !strings.Contains(specTicket, "- [x] beta") {
		t.Fatalf("spec ticket = %q, want both boxes ticked after merge", specTicket)
	}
}

// A red base blocks starts, lists failures in Needs-you, and shares one check
// result across every Ticket whose base has the same tip.
func TestBaseRedNeedsYouAndCachesPerTip(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	var checks atomic.Int32
	d.baseCheck = func(_, _, _ string) (bool, []string) {
		checks.Add(1)
		return false, []string{"TestFoo", "TestBar"}
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "needs-you" && ticketByID(r, "01-first").State == "needs-you" && ticketByID(r, "02-second").State == "needs-you"
	}, "both tickets blocked by the red base")
	for _, id := range []string{"01-first", "02-second"} {
		if got := ticketByID(r, id); got.WorkerSession != "" {
			t.Fatalf("ticket %s worker = %q, want no Worker on a red base", id, got.WorkerSession)
		}
		needsYou := actionTicketByID(getActionRun(t, srv.URL, run.ID), id).NeedsYou
		if !strings.Contains(needsYou, "TestFoo") || !strings.Contains(needsYou, "TestBar") {
			t.Fatalf("ticket %s needsYou = %q, want both failing tests", id, needsYou)
		}
	}
	if got := checks.Load(); got != 1 {
		t.Fatalf("base checks = %d, want one for the shared spec tip", got)
	}
	_ = getRun(t, srv.URL, run.ID)
	if got := checks.Load(); got != 1 {
		t.Fatalf("base checks after another poll = %d, want cached result", got)
	}
}

// A new spec-branch tip gets a new check; an already-started Worker is not
// rejected by a later red base, while the next Ticket is blocked on that tip.
func TestBaseRedRechecksNewTipWithoutGatingStartedWorker(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n",
	})
	initialTip := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	var callsMu sync.Mutex
	tipCalls := map[string]int{}
	d.baseCheck = func(_, _, tip string) (bool, []string) {
		callsMu.Lock()
		tipCalls[tip]++
		callsMu.Unlock()
		if tip == initialTip {
			return true, nil
		}
		return false, []string{"TestRed"}
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "first ticket started on green base")
	worker := ticketByID(r, "01-first").WorkerSession

	redWorktree := filepath.Join(repo, ".el-worktrees", "red-base")
	testGit(t, repo, "worktree", "add", redWorktree, "spec/demo")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", redWorktree) })
	writeFile(t, redWorktree, "red-base.txt", "red base\n")
	testGit(t, redWorktree, "add", "red-base.txt")
	testGit(t, redWorktree, "commit", "-m", "advance spec branch to a red tip")

	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "needs-you" && ticketByID(r, "01-first").State == "merged" && ticketByID(r, "02-second").State == "needs-you"
	}, "first ticket merged and second blocked on the new red tip")
	if got := ticketByID(r, "01-first").WorkerSession; got == "" {
		t.Fatal("first ticket lost its Worker Session")
	}
	if got := ticketByID(r, "02-second").WorkerSession; got != "" {
		t.Fatalf("second ticket worker = %q, want no Worker", got)
	}
	if got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "02-second").NeedsYou; !strings.Contains(got, "TestRed") {
		t.Fatalf("second ticket needsYou = %q, want TestRed", got)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	if len(tipCalls) < 2 {
		t.Fatalf("checked tips = %v, want checks for the original and changed tip", tipCalls)
	}
	for tip, count := range tipCalls {
		if count != 1 {
			t.Errorf("tip %s checked %d times, want one cached check per tip", tip, count)
		}
	}
}

func TestRunRepoCheckAtTipParsesFailingTests(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	writeFile(t, repo, ".emberloop/check", "#!/bin/sh\nprintf '%s\\n' '--- FAIL: TestPinnedFailure (0.00s)' 'FAIL example.test/pkg 0.01s'\nexit 1\n")
	testGit(t, repo, "add", ".emberloop/check")
	testGit(t, repo, "commit", "-m", "add failing check")
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))

	green, failing := runRepoCheckAtTip(repo, tip)
	if green {
		t.Fatal("repo check returned green, want the scripted failure")
	}
	if len(failing) != 2 || failing[0] != "TestPinnedFailure" || !strings.HasPrefix(failing[1], "FAIL") {
		t.Fatalf("failing tests = %q, want the test and package failures", failing)
	}
}
