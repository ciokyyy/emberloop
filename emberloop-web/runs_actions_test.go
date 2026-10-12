package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// actionTicket decodes the ticket fields the per-ticket actions surface:
// state, the needs-you banner text and the worker branch.
type actionTicket struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	NeedsYou    string `json:"needsYou"`
	Branch      string `json:"branch"`
	LastRefusal string `json:"lastRefusal"`
}

type actionRun struct {
	ID      string         `json:"id"`
	Status  string         `json:"status"`
	Tickets []actionTicket `json:"tickets"`
}

func getActionRun(t *testing.T, base, id string) actionRun {
	t.Helper()
	resp, err := http.Get(base + "/api/runs/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Run actionRun `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Run
}

func actionTicketByID(r actionRun, id string) actionTicket {
	for _, tk := range r.Tickets {
		if tk.ID == id {
			return tk
		}
	}
	return actionTicket{}
}

// postTicketAction posts one per-ticket action and decodes the answered
// snapshot when the status is 200.
func postTicketAction(t *testing.T, base, runID, ticketID, action, text string) (int, actionRun) {
	t.Helper()
	var body []byte
	if text == "" {
		body, _ = json.Marshal(map[string]string{})
	} else {
		body, _ = json.Marshal(map[string]string{"text": text})
	}
	resp, err := http.Post(base+"/api/runs/"+runID+"/tickets/"+ticketID+"/"+action, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Run   actionRun `json:"run"`
		Error string    `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Run
}

func waitActionState(t *testing.T, base, runID, ticketID, want, what string) actionRun {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		r := getActionRun(t, base, runID)
		if got := actionTicketByID(r, ticketID); got.State == want {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s: ticket %s state = %q, want %q", what, ticketID, actionTicketByID(getActionRun(t, base, runID), ticketID).State, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// workerPromptTexts reads one worker session's prompt event texts, so tests
// can count the driver's tells and the user's steers.
func workerPromptTexts(t *testing.T, base, sessionID string) []string {
	t.Helper()
	s := getSession(t, base, sessionID)
	var out []string
	for _, e := range s.Events {
		if e.Kind == "prompt" {
			out = append(out, e.Text)
		}
	}
	return out
}

func waitPromptContaining(t *testing.T, base, sessionID, substr string, n int, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		found := 0
		for _, text := range workerPromptTexts(t, base, sessionID) {
			if strings.Contains(text, substr) {
				found++
			}
		}
		if found >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s: %d prompts contain %q, want %d", what, found, substr, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A ticket steer reaches its worker: the worker transcript holds the text
// and the answered snapshot names the run.
func TestTicketSteerReachesWorker(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	_ = repo
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession

	code, arun := postTicketAction(t, srv.URL, run.ID, "01-first", "steer", "focus on the alpha checkbox")
	if code != http.StatusOK {
		t.Fatalf("steer status = %d, want 200", code)
	}
	if arun.ID != run.ID {
		t.Fatalf("steer run = %q, want %q", arun.ID, run.ID)
	}
	waitPromptContaining(t, srv.URL, worker, "focus on the alpha checkbox", 1, "steer text in worker transcript")
}

// Steer validation: empty text is 400, an unknown ticket is 404, and a
// ticket with no worker yet (or a finished run) is 409.
func TestTicketSteerValidation(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")

	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "steer", ""); code != http.StatusBadRequest {
		t.Fatalf("empty steer status = %d, want 400", code)
	}
	if code, _ := postTicketAction(t, srv.URL, run.ID, "no-such-ticket", "steer", "hi"); code != http.StatusNotFound {
		t.Fatalf("unknown ticket steer status = %d, want 404", code)
	}
	if code, _ := postTicketAction(t, srv.URL, "no-such-run", "01-first", "steer", "hi"); code != http.StatusNotFound {
		t.Fatalf("unknown run steer status = %d, want 404", code)
	}
	// 02-second waits on 01-first, so it has no worker: steering it is 409.
	if code, _ := postTicketAction(t, srv.URL, run.ID, "02-second", "steer", "hi"); code != http.StatusConflict {
		t.Fatalf("workerless steer status = %d, want 409", code)
	}
}

// Park shows parked at once (ahead of the driver's next pass); retry
// unparks, bumps the retry generation in the run file and the ticket runs
// again.
func TestTicketParkAndRetry(t *testing.T) {
	t.Parallel()
	// A second independent ticket keeps the Run live while 01 is parked. A
	// Run whose only work is parked ends on the driver's next poll, and the
	// repeat park and the retry below would race that end.
	tickets := demoTickets()
	tickets["03-third.md"] = "# 03: Third\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] gamma\n"
	repo, spec := initRunRepo(t, tickets)
	stateDir := tempDir(t)
	d := runTestDaemon(t, stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")

	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "park", ""); code != http.StatusOK {
		t.Fatalf("park status = %d, want 200", code)
	}
	if got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first"); got.State != "parked" {
		t.Fatalf("parked state = %q, want parked", got.State)
	}
	// Repeats are idempotent.
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "park", ""); code != http.StatusOK {
		t.Fatalf("second park status = %d, want 200", code)
	}

	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "retry", ""); code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", code)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "enderbloom", "runs", run.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		RetryGen map[string]int  `json:"retryGen"`
		Parked   map[string]bool `json:"parked"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RetryGen["01-first"] != 1 {
		t.Fatalf("retryGen = %v, want 01-first at 1", saved.RetryGen)
	}
	if saved.Parked["01-first"] {
		t.Fatalf("parked still set after retry: %v", saved.Parked)
	}
	waitActionState(t, srv.URL, run.ID, "01-first", "running", "unparked ticket running again")
}

// Discard marks the ticket discarded, removes its worktree and branch, and
// refuses further actions on it. The worker session stays for its
// transcript.
func TestTicketDiscardRemovesWorktree(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	arun := getActionRun(t, srv.URL, run.ID)
	branch := actionTicketByID(arun, "01-first").Branch
	if branch == "" {
		t.Fatalf("ticket branch empty in %+v", arun)
	}

	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "discard", ""); code != http.StatusOK {
		t.Fatalf("discard status = %d, want 200", code)
	}
	if got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first"); got.State != "discarded" {
		t.Fatalf("discarded state = %q, want discarded", got.State)
	}
	if _, err := os.Stat(filepath.Join(repo, ".el-worktrees", strings.TrimPrefix(branch, "implement/"))); !os.IsNotExist(err) {
		t.Fatalf("worktree still present: %v", err)
	}
	if out := testGit(t, repo, "branch", "--list", branch); strings.TrimSpace(out) != "" {
		t.Fatalf("branch %q still present", branch)
	}
	// The transcript survives the discard.
	s := getSession(t, srv.URL, worker)
	if len(s.Events) == 0 {
		t.Fatalf("worker transcript empty after discard")
	}
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "steer", "hi"); code != http.StatusConflict {
		t.Fatalf("steer after discard status = %d, want 409", code)
	}
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "retry", ""); code != http.StatusConflict {
		t.Fatalf("retry after discard status = %d, want 409", code)
	}
}

// A finished run refuses every per-ticket action with 409: reviewers pass
// by default, so one done submit merges the single ticket and ends the run.
func TestTicketActionsOnFinishedRunConflict(t *testing.T) {
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
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	waitRunTickets(t, srv.URL, run.ID, map[string]string{"01-first": "merged"})
	for _, action := range []string{"steer", "retry", "park", "discard"} {
		text := ""
		if action == "steer" {
			text = "hi"
		}
		if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", action, text); code != http.StatusConflict {
			t.Fatalf("%s on finished run status = %d, want 409", action, code)
		}
	}
}

// fakeClock is the Daemon clock override for tests: the web driver world and
// idle quiet time read it instead of time.Now, so advancing it trips the idle
// and stall alerts without waiting minutes.
type fakeClock struct {
	now atomic.Int64
}

func newFakeClock() *fakeClock {
	c := &fakeClock{}
	c.now.Store(time.Now().UnixNano())
	return c
}

func (c *fakeClock) get() time.Time { return time.Unix(0, c.now.Load()).UTC() }

func (c *fakeClock) advance(d time.Duration) {
	c.now.Store(c.get().Add(d).UnixNano())
}

// Ten minutes with no worker activity marks the ticket needs-you with the
// idle reason, surfaced on the ticket snapshot the banner reads.
func TestNeedsYouIdleAlertSurfaced(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	clock := newFakeClock()
	d.clock = clock.get
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")

	clock.advance(11 * time.Minute)
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first")
		if got.State == "needs-you" && strings.Contains(got.NeedsYou, "no worker activity for 10 minutes") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle needs-you never surfaced: state=%q reason=%q", got.State, got.NeedsYou)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Three stall nudges with no submit mark the ticket needs-you with the
// stall reason. Commits between nudges date fresh activity, so the idle
// alert never fires first.
func TestStallNudgesEscalateToNeedsYou(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	clock := newFakeClock()
	d.clock = clock.get
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	arun := getActionRun(t, srv.URL, run.ID)
	worktree := filepath.Join(repo, ".el-worktrees", strings.TrimPrefix(actionTicketByID(arun, "01-first").Branch, "implement/"))

	commitEmpty := func() string {
		t.Helper()
		testGit(t, worktree, "commit", "--allow-empty", "-m", "probe activity")
		return strings.TrimSpace(testGit(t, worktree, "rev-parse", "HEAD"))
	}
	// Nudge 1 after 3m idle, nudge 2 five minutes later, nudge 3 five
	// after that. Each commit dates fresh activity on the pass that sees
	// the new tip (polled here), so the idle alert never fires first and
	// only the third nudge escalates.
	clock.advance(3*time.Minute + time.Second)
	waitPromptContaining(t, srv.URL, worker, "you have not submitted", 1, "first stall nudge")
	tip := commitEmpty()
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").Tip == tip
	}, "activity dated at the new tip")
	clock.advance(5*time.Minute + time.Second)
	waitPromptContaining(t, srv.URL, worker, "you have not submitted", 2, "second stall nudge")
	tip = commitEmpty()
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").Tip == tip
	}, "activity dated at the newer tip")
	clock.advance(5*time.Minute + time.Second)

	deadline := time.Now().Add(15 * time.Second)
	for {
		got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first")
		if got.State == "needs-you" && strings.Contains(got.NeedsYou, "after 3 nudges") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stall needs-you never surfaced: state=%q reason=%q", got.State, got.NeedsYou)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The same refusal three times in a row marks the ticket needs-you: a rebase
// conflict refuses every finish identically, the worker submits done three
// times, and each refusal is told back before the next submit. A conflict
// refusal repeats verbatim (unlike a changes verdict, which the submit gate
// refuses as an empty resubmit), so it still exercises the driver's streak.
func TestSameRefusalThreeTimesMarksNeedsYou(t *testing.T) {
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
	// A committed worker change plus a conflicting spec-branch move makes
	// the finish rebase conflict deterministically on every pass. (A dirty
	// worktree never reaches the finish: the submit gate refuses it first.)
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	commitTicketFile(t, worktree, "conflict.txt", "worker\n", "worker change")
	specWT := filepath.Join(repo, ".el-worktrees", "spec-move")
	testGit(t, repo, "worktree", "add", specWT, "spec/demo")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", specWT) })
	writeFile(t, specWT, "conflict.txt", "spec\n")
	testGit(t, specWT, "add", "conflict.txt")
	testGit(t, specWT, "commit", "-m", "spec moves")

	for n := 1; n <= 3; n++ {
		submitSession(t, srv.URL, worker, "done", []string{"alpha"})
		// Each submit's refusal reaches the worker before the next
		// submit, so the streak counts consecutive identical refusals.
		waitPromptContaining(t, srv.URL, worker, "conflicted", n, "refusal tell")
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first")
		if got.State == "needs-you" && strings.Contains(got.NeedsYou, "same refusal 3 times") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("refusal needs-you never surfaced: state=%q reason=%q refusal=%q", got.State, got.NeedsYou, got.LastRefusal)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Retry after the refusal alert clears the banner and the ticket works
// again: the retry generation resets the driver's streak and backoff, so a
// resolved branch from here merges. A conflict refusal repeats verbatim
// (unlike a changes verdict, which the submit gate refuses as an empty
// resubmit).
func TestRetryAfterRefusalAlertMerges(t *testing.T) {
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
	commitTicketFile(t, worktree, "conflict.txt", "worker\n", "worker change")
	specWT := filepath.Join(repo, ".el-worktrees", "spec-move")
	testGit(t, repo, "worktree", "add", specWT, "spec/demo")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", specWT) })
	writeFile(t, specWT, "conflict.txt", "spec\n")
	testGit(t, specWT, "add", "conflict.txt")
	testGit(t, specWT, "commit", "-m", "spec moves")

	for n := 1; n <= 3; n++ {
		submitSession(t, srv.URL, worker, "done", []string{"alpha"})
		waitPromptContaining(t, srv.URL, worker, "conflicted", n, "refusal tell")
	}
	waitActionState(t, srv.URL, run.ID, "01-first", "needs-you", "refusal needs-you")

	code, retryRun := postTicketAction(t, srv.URL, run.ID, "01-first", "retry", "clean up and submit again")
	if code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", code)
	}
	if got := actionTicketByID(retryRun, "01-first"); got.NeedsYou != "" {
		t.Fatalf("needsYou in retry response = %q, want cleared", got.NeedsYou)
	}
	// The retry's steer reached the worker.
	waitPromptContaining(t, srv.URL, worker, "clean up and submit again", 1, "retry steer text")

	// A branch reset onto the moved spec, plus a new commit, merges: the
	// rebase lands and the reviewers judge the new tip.
	testGit(t, worktree, "reset", "--hard", "spec/demo")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	waitRunTickets(t, srv.URL, run.ID, map[string]string{"01-first": "merged"})
}
