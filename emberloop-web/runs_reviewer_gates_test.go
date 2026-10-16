package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Reviewer-flow gates from the Luna Run retro: the Worker submit gate (no
// commits ahead or uncommitted work refused), the pre-review tip check, the
// selective re-review carry, and the scope warning with the round-4+ rule.

// commitTicketFile writes one file in a Ticket worktree and commits it, so a
// done submit passes the uncommitted-work gate.
func commitTicketFile(t *testing.T, worktree, name, content, msg string) {
	t.Helper()
	writeFile(t, worktree, name, content)
	testGit(t, worktree, "add", name)
	testGit(t, worktree, "commit", "-m", msg)
}

// postSubmit posts one submit payload to a Session and returns the status
// and decoded body, so gate refusals assert the tool error directly.
func postSubmit(t *testing.T, base, sessionID string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	resp, err := http.Post(base+"/api/sessions/"+sessionID+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out == nil {
		out = map[string]any{}
	}
	return resp.StatusCode, out
}

func postDone(t *testing.T, base, sessionID string) (int, map[string]any) {
	t.Helper()
	return postSubmit(t, base, sessionID, map[string]any{"status": "done", "completed": []string{"alpha"}})
}

// countPromptTells counts prompt Events carrying substr in one Session. The
// fake Backend echoes prompts in its default reply, so a raw transcript
// substring count would double-count every tell.
func countPromptTells(t *testing.T, base, sessionID, substr string) int {
	t.Helper()
	n := 0
	for _, e := range getSession(t, base, sessionID).Events {
		if e.Kind == "prompt" && strings.Contains(e.Text, substr) {
			n++
		}
	}
	return n
}

// A done submit with no commits ahead of the base is refused with a tool
// error telling the Worker to commit; committing first is accepted and the
// Reviewers spawn.
func TestWorkerDoneRefusedWithNoCommitsAhead(t *testing.T) {
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

	if code, out := postDone(t, srv.URL, worker); code != http.StatusConflict {
		t.Fatalf("done without commits status = %d (%v), want 409", code, out)
	} else if msg, _ := out["error"].(string); !strings.Contains(strings.ToLower(msg), "no commits ahead") || !strings.Contains(strings.ToLower(msg), "commit") {
		t.Fatalf("done without commits error = %q, want it to name no commits ahead and commit", msg)
	}

	commitTicketFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "work.go", "package main\n", "work")
	if code, out := postDone(t, srv.URL, worker); code != http.StatusAccepted {
		t.Fatalf("done with a commit status = %d (%v), want 202", code, out)
	}
	waitReviewers(t, srv.URL, run.ID, "01-first")
}

// A done submit with uncommitted changes in the worktree is refused with a
// tool error telling the Worker to commit; committing first is accepted.
func TestWorkerDoneRefusedWithUncommittedChanges(t *testing.T) {
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
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")

	commitTicketFile(t, worktree, "work.go", "package main\n", "work")
	writeFile(t, worktree, "dirty.go", "package main\n")
	if code, out := postDone(t, srv.URL, worker); code != http.StatusConflict {
		t.Fatalf("done with uncommitted changes status = %d (%v), want 409", code, out)
	} else if msg, _ := out["error"].(string); !strings.Contains(strings.ToLower(msg), "uncommitted") || !strings.Contains(strings.ToLower(msg), "commit") {
		t.Fatalf("done with uncommitted changes error = %q, want it to name uncommitted changes and commit", msg)
	}

	testGit(t, worktree, "add", "dirty.go")
	testGit(t, worktree, "commit", "-m", "dirty")
	if code, out := postDone(t, srv.URL, worker); code != http.StatusAccepted {
		t.Fatalf("done after committing status = %d (%v), want 202", code, out)
	}
	waitReviewers(t, srv.URL, run.ID, "01-first")
}

// A red tip check returns the tip to the Worker with the failing output and
// spends no Reviewer turn; the tell goes out once per tip, the check runs
// once per tip, and a fixed new tip reviews and merges normally.
func TestTipCheckRedReturnsToWorkerWithoutReviewerTurn(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var mu sync.Mutex
	calls := map[string]int{}
	var red atomic.Bool
	red.Store(true)
	d := runTestDaemon(t, tempDir(t), fb)
	d.tipCheck = func(_, _ /* branch */, tip string) (bool, []string) {
		mu.Lock()
		calls[tip]++
		mu.Unlock()
		if red.Load() {
			return false, []string{"TestTipRed"}
		}
		return true, nil
	}
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		return ReviewerSubmit{
			Verdict: "pass", Tip: tip,
			Ran: []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
		}, true
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	commitTicketFile(t, worktree, "work.go", "package main\n", "work")
	if code, out := postDone(t, srv.URL, worker); code != http.StatusAccepted {
		t.Fatalf("done status = %d (%v), want 202", code, out)
	}
	// The branch tip straight from git: the driver's row tip can lag the
	// branch before its first snapshot.
	t1 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))

	var transcript string
	transcriptDeadline := time.Now().Add(5 * time.Second)
	for {
		transcript = sessTranscript(getSession(t, srv.URL, worker))
		if strings.Contains(transcript, "repo check is red") && strings.Contains(transcript, "TestTipRed") {
			break
		}
		if time.Now().After(transcriptDeadline) {
			t.Fatalf("worker transcript lacks the red-tip return after 5s:\n%s", transcript)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// No Reviewer turn is spent on the red tip.
	time.Sleep(2 * time.Second)
	r = getRun(t, srv.URL, run.ID)
	if tk := ticketByID(r, "01-first"); len(tk.ReviewerSessions) != 0 {
		t.Fatalf("reviewer sessions = %v, want none on a red tip", tk.ReviewerSessions)
	} else if len(tk.Verdicts) != 0 {
		t.Fatalf("verdicts = %v, want none on a red tip", tk.Verdicts)
	}

	// A second done at the same red tip is accepted but tells nothing new.
	if code, out := postDone(t, srv.URL, worker); code != http.StatusAccepted {
		t.Fatalf("second done at the red tip status = %d (%v), want 202", code, out)
	}
	if n := countPromptTells(t, srv.URL, worker, "repo check is red"); n != 1 {
		t.Fatalf("red-tip tells = %d, want one per tip", n)
	}
	mu.Lock()
	if len(calls) != 1 || calls[t1] != 1 {
		t.Fatalf("tip checks = %v, want one check for the one tip", calls)
	}
	mu.Unlock()

	// A fixed new tip reviews and merges: the check runs once for it, and
	// the Reviewer prompt carries the green evidence.
	red.Store(false)
	commitTicketFile(t, worktree, "fix.go", "package main\n", "fix")
	if code, out := postDone(t, srv.URL, worker); code != http.StatusAccepted {
		t.Fatalf("done on the fixed tip status = %d (%v), want 202", code, out)
	}
	waitReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after the fixed tip passes")
	mu.Lock()
	if len(calls) != 2 {
		t.Fatalf("tip checks = %v, want one check per tip", calls)
	}
	mu.Unlock()
	for _, sess := range ticketByID(r, "01-first").ReviewerSessions {
		if transcript := sessTranscript(getSession(t, srv.URL, sess)); !strings.Contains(transcript, "Driver check at") || !strings.Contains(transcript, ": green") {
			t.Fatalf("reviewer transcript lacks the green driver-check evidence:\n%s", transcript)
		}
	}
}

// A User retry forgets a red tip check: the idle Worker's unchanged tip is
// checked again and, once green, reviews and merges.
func TestTipCheckRetryRechecksTheSameTip(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var calls atomic.Int32
	var red atomic.Bool
	red.Store(true)
	d := runTestDaemon(t, tempDir(t), fb)
	d.tipCheck = func(_, _, _ string) (bool, []string) {
		calls.Add(1)
		if red.Load() {
			return false, []string{"TestTipRed"}
		}
		return true, nil
	}
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		return ReviewerSubmit{
			Verdict: "pass", Tip: tip,
			Ran: []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
		}, true
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	commitTicketFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "work.go", "package main\n", "work")
	if code, out := postDone(t, srv.URL, worker); code != http.StatusAccepted {
		t.Fatalf("done status = %d (%v), want 202", code, out)
	}
	deadline := time.Now().Add(5 * time.Second)
	for countPromptTells(t, srv.URL, worker, "repo check is red") != 1 {
		if time.Now().After(deadline) {
			t.Fatal("worker never told the tip is red")
		}
		time.Sleep(10 * time.Millisecond)
	}

	red.Store(false)
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "retry", ""); code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", code)
	}
	waitReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after the retry rechecks the tip")
	if n := calls.Load(); n != 2 {
		t.Fatalf("tip checks = %d, want 2: red, then the retry's recheck", n)
	}
}

// A green tip check is cached per tip: both Reviewer prompts on one tip
// share one check, and the prompts carry it as evidence.
func TestTipCheckGreenCachedPerTip(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var calls atomic.Int32
	d := runTestDaemon(t, tempDir(t), fb)
	d.tipCheck = func(_, _, _ string) (bool, []string) {
		calls.Add(1)
		return true, nil
	}
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		return ReviewerSubmit{
			Verdict: "pass", Tip: tip,
			Ran: []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
		}, true
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "work.go", "package main\n", "work")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	waitReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	if got := calls.Load(); got != 1 {
		t.Fatalf("tip checks = %d, want one for the shared tip", got)
	}
	for _, sess := range ticketByID(r, "01-first").ReviewerSessions {
		if transcript := sessTranscript(getSession(t, srv.URL, sess)); !strings.Contains(transcript, "Driver check at") {
			t.Fatalf("reviewer transcript lacks the green evidence:\n%s", transcript)
		}
	}
}

// Selective re-review: the seat that passed at T1 while the other said
// changes keeps its pass at T2 without a rerun when the T1..T2 diff touches
// no cited file, stays small and inside the previous scope. The carried pass
// is recorded at T2 with its note, and the merge still needs both seats.
func TestSelectiveCarrySkipsPassedSeatOnSafeDiff(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var mu sync.Mutex
	calls := map[string]int{}
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		ran := []ReviewRan{{Command: "go vet ./...", ExitCode: 0}}
		key := "standards"
		if strings.Contains(prompt, "reviewer-spec") {
			key = "spec"
		}
		mu.Lock()
		calls[key]++
		n := calls[key]
		mu.Unlock()
		if key == "spec" && n == 1 {
			return ReviewerSubmit{
				Verdict: "changes", Tip: tip, Ran: ran,
				Findings: []ReviewFinding{{File: "work.go", Line: 1, Severity: "major", Problem: "handle nil"}},
			}, true
		}
		return ReviewerSubmit{Verdict: "pass", Tip: tip, Ran: ran}, true
	}
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	commitTicketFile(t, worktree, "work.go", "package main\n", "work v1")
	commitTicketFile(t, worktree, "notes.txt", "notes\n", "notes")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "changes refusal told")
	// Tips straight from git: the driver's row tip is empty once merged.
	t1 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))

	// Rework touches only notes.txt: uncited, one line, inside the T1 scope.
	commitTicketFile(t, worktree, "notes.txt", "notes\nmore\n", "tweak notes")
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "fixed"}}); code != http.StatusAccepted {
		t.Fatalf("done after safe rework = %d (%v), want 202", code, out)
	}
	// T2 straight from git right after the submit: the merge rebase moves the
	// branch, and the row tip is empty once merged.
	t2 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if t1 == "" || t2 == "" || t1 == t2 {
		t.Fatalf("tips = %q and %q, want two different SHAs", t1, t2)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done on the carried pass plus the spec re-review")

	// The passed seat got no second turn; the changes seat did.
	if n := strings.Count(sessTranscript(getSession(t, srv.URL, revs["reviewer-standards"])), "Review ticket"); n != 1 {
		t.Fatalf("standards prompts = %d, want one (carried, not rerun)", n)
	}
	if n := strings.Count(sessTranscript(getSession(t, srv.URL, revs["reviewer-spec"])), "Review ticket"); n != 2 {
		t.Fatalf("spec prompts = %d, want two (re-reviewed)", n)
	}

	// The carry is recorded at T2 with its note on the round and the review.
	ext := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
	if len(ext.Rounds) < 2 {
		t.Fatalf("rounds = %+v, want a second round", ext.Rounds)
	}
	rd := ext.Rounds[1]
	if rd.Tip != t2 {
		t.Fatalf("round 2 tip = %q, want %q", rd.Tip, t2)
	}
	rv, ok := rd.Reviews["reviewer-standards"]
	if !ok || rv.Verdict != "pass" || rv.HeldFromRound == nil || *rv.HeldFromRound != 1 || rv.At != t2 {
		t.Fatalf("round 2 standards = %+v, want pass held from round 1 at %q", rv, t2)
	}
	if !strings.HasPrefix(rv.Note, "carried from ") {
		t.Fatalf("round 2 standards note = %q, want carried from T1", rv.Note)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["standards"] != 1 {
		t.Fatalf("standards reviewer calls = %v, want one", calls)
	}
	if calls["spec"] != 2 {
		t.Fatalf("spec reviewer calls = %v, want two", calls)
	}
}

// Selective re-review reruns the passed seat when the new diff touches a
// file the changes verdict cited.
func TestSelectiveCarryRerunsPassedSeatOnCitedFile(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var mu sync.Mutex
	calls := map[string]int{}
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		ran := []ReviewRan{{Command: "go vet ./...", ExitCode: 0}}
		key := "standards"
		if strings.Contains(prompt, "reviewer-spec") {
			key = "spec"
		}
		mu.Lock()
		calls[key]++
		n := calls[key]
		mu.Unlock()
		if key == "spec" && n == 1 {
			return ReviewerSubmit{
				Verdict: "changes", Tip: tip, Ran: ran,
				Findings: []ReviewFinding{{File: "work.go", Line: 1, Severity: "major", Problem: "handle nil"}},
			}, true
		}
		return ReviewerSubmit{Verdict: "pass", Tip: tip, Ran: ran}, true
	}
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	commitTicketFile(t, worktree, "work.go", "package main\n", "work v1")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "changes refusal told")

	// Rework touches the cited file, so the passed seat is re-prompted too.
	commitTicketFile(t, worktree, "work.go", "package main\n\nfunc f() {}\n", "work v2")
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "fixed"}}); code != http.StatusAccepted {
		t.Fatalf("done after cited rework = %d (%v), want 202", code, out)
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after both seats re-review")
	for typ, sess := range revs {
		if n := strings.Count(sessTranscript(getSession(t, srv.URL, sess)), "Review ticket"); n != 2 {
			t.Fatalf("reviewer %s prompts = %d, want two (cited file changed)", typ, n)
		}
	}
}

// Selective re-review reruns the passed seat when the new diff adds more
// than the line threshold, even inside the previous scope.
func TestSelectiveCarryRerunsPassedSeatOnLargeDiff(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var mu sync.Mutex
	calls := map[string]int{}
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		ran := []ReviewRan{{Command: "go vet ./...", ExitCode: 0}}
		key := "standards"
		if strings.Contains(prompt, "reviewer-spec") {
			key = "spec"
		}
		mu.Lock()
		calls[key]++
		n := calls[key]
		mu.Unlock()
		if key == "spec" && n == 1 {
			return ReviewerSubmit{
				Verdict: "changes", Tip: tip, Ran: ran,
				Findings: []ReviewFinding{{File: "work.go", Line: 1, Severity: "major", Problem: "handle nil"}},
			}, true
		}
		return ReviewerSubmit{Verdict: "pass", Tip: tip, Ran: ran}, true
	}
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	commitTicketFile(t, worktree, "work.go", "package main\n", "work v1")
	commitTicketFile(t, worktree, "notes.txt", "notes\n", "notes")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "changes refusal told")

	// Rework adds over the threshold to an uncited in-scope file.
	var b strings.Builder
	b.WriteString("notes\n")
	for i := 0; i < selectiveRereviewLineThreshold+10; i++ {
		b.WriteString("line\n")
	}
	commitTicketFile(t, worktree, "notes.txt", b.String(), "big notes")
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "fixed"}}); code != http.StatusAccepted {
		t.Fatalf("done after large rework = %d (%v), want 202", code, out)
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after both seats re-review")
	for typ, sess := range revs {
		if n := strings.Count(sessTranscript(getSession(t, srv.URL, sess)), "Review ticket"); n != 2 {
			t.Fatalf("reviewer %s prompts = %d, want two (large diff)", typ, n)
		}
	}
}

// The scope check warns Reviewers about changed files outside the ticket's
// stated scope without blocking, and the Worker first prompt carries the
// scope rule.
func TestScopeWarningInReviewerPrompt(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\nWork in work.go only.\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		return ReviewerSubmit{
			Verdict: "pass", Tip: tip,
			Ran: []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
		}, true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	firstPromptWhere(t, srv.URL, worker, func(e Event) bool {
		return strings.Contains(e.Text, "Keep the diff to the ticket's stated scope")
	})
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	commitTicketFile(t, worktree, "work.go", "package main\n", "work")
	commitTicketFile(t, worktree, "surprise.go", "package main\n", "surprise")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	for typ, sess := range revs {
		prompt := firstPromptWhere(t, srv.URL, sess, func(e Event) bool {
			return strings.Contains(e.Text, "Scope note (non-blocking)") && strings.Contains(e.Text, "surprise.go")
		})
		start := strings.Index(prompt.Text, "Scope note (non-blocking)")
		warning := prompt.Text[start:]
		if end := strings.IndexByte(warning, '\n'); end >= 0 {
			warning = warning[:end]
		}
		if !strings.Contains(warning, "surprise.go") || strings.Contains(warning, "work.go") {
			t.Fatalf("reviewer %s scope note = %q, want surprise.go only", typ, warning)
		}
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done: the scope note never blocks")
}

// Scope matching: the ticket path, a named parent directory, or the basename
// keeps a file in scope; anything else warns. Unknown bodies or diffs warn
// on nothing.
func TestFileInTicketScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		body  string
		file  string
		scope bool
	}{
		{"mentioned file", "Work in work.go only.", "work.go", true},
		{"unmentioned file", "Work in work.go only.", "other.go", false},
		{"named directory", "Touch files under emberloop-web/ as needed.", "emberloop-web/x.go", true},
		{"other directory", "Touch files under emberloop-web/ as needed.", "emberloop/y.go", false},
		{"basename mention", "Fix the checkout helper.", "pkg/checkout.go", false},
		{"nested path mention", "See docs/agent/webui.md for the flow.", "docs/agent/webui.md", true},
		{"empty body", "", "work.go", false},
		{"empty file", "Work in work.go.", "", false},
	} {
		if got := fileInTicketScope(tc.body, tc.file); got != tc.scope {
			t.Errorf("%s: fileInTicketScope(%q, %q) = %v, want %v", tc.name, tc.body, tc.file, got, tc.scope)
		}
	}
	if got := scopeWarningFiles("Work in work.go only.", []string{"work.go", "b.go", "a.go"}); len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("scopeWarningFiles = %q, want the sorted out-of-scope files", got)
	}
	if got := scopeWarningFiles("", []string{"work.go"}); len(got) != 0 {
		t.Fatalf("scopeWarningFiles with an empty body = %q, want none", got)
	}
}

// The round-4+ rule is prompt-only: no note before round 4, and from round 4
// on the prompt says restated majors do not block while blockers still do.
func TestLateRoundReviewNote(t *testing.T) {
	t.Parallel()
	if got := lateRoundReviewNote(nil); got != "" {
		t.Fatalf("no rounds note = %q, want empty", got)
	}
	if got := lateRoundReviewNote(make([]ticketRound, 2)); got != "" {
		t.Fatalf("two rounds note = %q, want empty", got)
	}
	got := lateRoundReviewNote(make([]ticketRound, 3))
	if !strings.Contains(got, "review round 4") || !strings.Contains(got, "only `blocker` blocks") {
		t.Fatalf("three rounds note = %q, want the round-4 rule", got)
	}
	if got := lateRoundReviewNote(make([]ticketRound, 5)); !strings.Contains(got, "review round 6") {
		t.Fatalf("five rounds note = %q, want round 6", got)
	}
}

// The selective re-review line threshold is a small documented constant.
func TestSelectiveRereviewLineThresholdValue(t *testing.T) {
	t.Parallel()
	if selectiveRereviewLineThreshold != 40 {
		t.Fatalf("selectiveRereviewLineThreshold = %d, want 40", selectiveRereviewLineThreshold)
	}
}

// Persistent design Reviewer re-prompts keep the visual brief separate from
// the standards axis instead of reverting to the code-review default.
func TestDesignReviewerRereviewUsesDesignBrief(t *testing.T) {
	t.Parallel()
	prompt := reviewerPromptWithExtras("10-design", "spec.md", reviewerDesignType, "branch", "tip", nil, "", "", "")
	if !strings.Contains(prompt, designBrief) || strings.Contains(prompt, standardsBrief) {
		t.Fatalf("design re-review prompt has the wrong brief: %s", prompt)
	}
}
