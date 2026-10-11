package web

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const workerTestChainSettings = `{"chains":{"worker":[{"backend":"pig","model":"worker-primary"},{"backend":"grok","model":"worker-fallback"}]}}`

func waitSessionCond(t *testing.T, base, id string, cond func(sessionBody) bool, what string) sessionBody {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s := getSession(t, base, id)
		if cond(s) {
			return s
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s: %+v", what, getSession(t, base, id))
	return sessionBody{}
}

func pauseTestRun(t *testing.T, base, id string) {
	t.Helper()
	if getRun(t, base, id).Status != "running" {
		return
	}
	if _, code := postRunAction(t, base, id, "pause"); code != 200 {
		t.Errorf("pause Run status = %d, want 200", code)
	}
}

func firstHandoffAndTask(events []Event) (handoff, task Event, ok bool) {
	for _, e := range events {
		if e.Kind != "prompt" {
			continue
		}
		if e.Handoff && handoff.Seq == 0 {
			handoff = e
			continue
		}
		if handoff.Seq > 0 && e.Seq > handoff.Seq && strings.Contains(e.Text, "--- Ticket ") {
			task = e
			return handoff, task, true
		}
	}
	return handoff, task, false
}

func TestSetChainPositionSeedsTicketRowState(t *testing.T) {
	rs := &runState{}
	setChainPosition(rs, "01-first", "worker", 0)
	row := rs.Rows["01-first"]
	if row.State != "queued" {
		t.Fatalf("saved Ticket state = %q, want queued before the first driver snapshot", row.State)
	}
}

func TestLoadRunsDefaultsLegacyChainRowState(t *testing.T) {
	stateDir := tempDir(t)
	runsDir := filepath.Join(stateDir, "enderbloom", "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rs := runState{
		ID: "legacy-run", Status: runStatusPaused,
		Rows: map[string]savedRow{"01-first": {ChainPositions: map[string]int{"worker": 1}}},
	}
	raw, err := json.Marshal(rs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "legacy-run.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(stateDir, NewFakeBackend())
	d.mu.Lock()
	row := d.runs["legacy-run"].Rows["01-first"]
	d.mu.Unlock()
	if row.State != "queued" || row.ChainPositions["worker"] != 1 {
		t.Fatalf("loaded legacy Ticket row = %+v, want queued with its Chain position", row)
	}
}

func TestRunWorkerFallbackChainFailureClasses(t *testing.T) {
	cases := []struct {
		name     string
		error    string
		advances bool
	}{
		{name: "quota", error: "quota exceeded", advances: true},
		{name: "credit", error: "payment required", advances: true},
		{name: "auth", error: "HTTP 401 unauthorized", advances: true},
		{name: "session limit", error: "session limit reached", advances: true},
		{name: "other", error: "worker stopped on an unrelated error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, spec := initRunRepo(t, map[string]string{
				"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
			})
			previousInjection := memoryInjection
			memoryInjection = func(_ SessionMeta, op handleOp) injection {
				if op == handleCreate {
					return injection{texts: []string{"create-time marker"}, profile: 11}
				}
				if op == handleResume {
					return injection{texts: []string{"resume-time marker"}, profile: 12}
				}
				return injection{}
			}
			t.Cleanup(func() { memoryInjection = previousInjection })
			useSettings(t, workerTestChainSettings)
			fake := NewFakeBackend()
			var failOnce sync.Once
			var failureMu sync.Mutex
			failedID := ""
			fake.TurnFailure = func(id, prompt string) (string, bool) {
				if !strings.Contains(prompt, "Implement ticket 01-first") {
					return "", false
				}
				fired := false
				failOnce.Do(func() {
					fired = true
					failureMu.Lock()
					failedID = id
					failureMu.Unlock()
				})
				return tc.error, fired
			}
			readFailedID := func() string {
				failureMu.Lock()
				defer failureMu.Unlock()
				return failedID
			}
			d := NewDaemon(tempDir(t), fake)
			srv := newRunTestServer(t, d)
			defer srv.Close()

			run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
			started := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
				return ticketByID(r, "01-first").WorkerSession != ""
			}, "initial worker")
			before := ticketByID(started, "01-first")
			deadline := time.Now().Add(15 * time.Second)
			oldID := ""
			for time.Now().Before(deadline) && oldID == "" {
				oldID = readFailedID()
				time.Sleep(10 * time.Millisecond)
			}
			if oldID == "" {
				t.Fatal("fake Backend did not script the initial Worker failure")
			}
			oldSession := waitSessionCond(t, srv.URL, oldID, func(s sessionBody) bool {
				return s.Meta.Status == "failed"
			}, "worker failure")

			if !tc.advances {
				r := getRun(t, srv.URL, run.ID)
				if got := ticketByID(r, "01-first").WorkerSession; got != oldID {
					t.Fatalf("other failure replaced Worker %q with %q", oldID, got)
				}
				assertHolds(t, 400*time.Millisecond, "other failure started a fallback Session", func() bool {
					return countSessions(t, srv.URL) == 1
				})
				if len(eventsOfKind(oldSession.Events, "error")) == 0 {
					t.Fatal("failed Session lost its error Event")
				}
				d.mu.Lock()
				pending := d.runs[run.ID].ChainPendingSubmit["01-first"]
				d.mu.Unlock()
				if pending {
					t.Fatal("non-advancing Worker failure set the pending-submit marker")
				}
				pauseTestRun(t, srv.URL, run.ID)
				return
			}

			r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
				tk := ticketByID(r, "01-first")
				return tk.WorkerSession != "" && tk.WorkerSession != oldID && tk.Chain["worker"].Position == 2
			}, "fallback Worker")
			after := ticketByID(r, "01-first")
			if after.Branch != before.Branch {
				t.Fatalf("fallback branch = %q, want same Ticket branch %q", after.Branch, before.Branch)
			}
			fallback := waitSessionCond(t, srv.URL, after.WorkerSession, func(s sessionBody) bool {
				_, _, ok := firstHandoffAndTask(s.Events)
				return ok && s.Meta.Status == "idle"
			}, "Handoff and injected Worker task")
			injections := injectionEventsOf(t, fallback.Events)
			if len(injections) != 1 || injections[0].ProfileVersion != 11 {
				t.Fatalf("fallback task injection = %+v, want one create-time injection", injections)
			}
			d.mu.Lock()
			pending := d.runs[run.ID].ChainPendingSubmit["01-first"]
			d.mu.Unlock()
			if !pending {
				t.Fatal("advancing Worker failure did not set the pending-submit marker")
			}
			found := false
			for _, e := range oldSession.Events {
				found = found || e.Kind == "error" && strings.Contains(e.Text, tc.error)
			}
			if oldSession.Meta.Status != "failed" || !found {
				t.Fatalf("failed Session does not retain its error Event %q: %+v", tc.error, oldSession.Events)
			}
			handoff, task, ok := firstHandoffAndTask(fallback.Events)
			if !ok || !handoff.Handoff || handoff.Seq >= task.Seq || !strings.Contains(handoff.Text, "Previous Session failure") {
				t.Fatalf("fallback prompt order lacks Handoff then task: handoff=%+v task=%+v", handoff, task)
			}
			for _, want := range []string{"Implement ticket 01-first", "--- Ticket ", "--- Spec ", "--- AGENTS.md chain ---"} {
				if !strings.Contains(task.Text, want) {

					t.Fatalf("normal injected task misses %q: %q", want, task.Text)
				}
			}
			if fallback.Meta.WorkDir != oldSession.Meta.WorkDir {
				t.Fatalf("fallback Worker worktree = %q, want same Ticket worktree %q", fallback.Meta.WorkDir, oldSession.Meta.WorkDir)
			}
			if fallback.Meta.Harness != "grok" || fallback.Meta.Model != "worker-fallback" {
				t.Fatalf("fallback entry = %s/%s, want grok/worker-fallback", fallback.Meta.Harness, fallback.Meta.Model)
			}
			if got := after.Chain["worker"]; got.Position != 2 || got.Total != 2 || got.Backend != "grok" || got.Model != "worker-fallback" {
				t.Fatalf("active Chain entry = %+v", got)
			}
			pauseTestRun(t, srv.URL, run.ID)
		})
	}
}

func TestRunWorkerFallbackChainStartFailure(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	fake := NewFakeBackend()
	fake.CreateErrs = []error{errors.New("quota exceeded")}
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.WorkerSession != "" && tk.Chain["worker"].Position == 2
	}, "Worker fallback after start failure")
	tk := ticketByID(r, "01-first")
	fallback := waitSessionCond(t, srv.URL, tk.WorkerSession, func(s sessionBody) bool {
		return s.Meta.Status == "idle"
	}, "started fallback Worker")
	if fallback.Meta.Harness != "grok" || fallback.Meta.Model != "worker-fallback" {
		t.Fatalf("fallback config = %s/%s, want grok/worker-fallback", fallback.Meta.Harness, fallback.Meta.Model)
	}
	worktrees, err := runGit(repo, "worktree", "list", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(worktrees, "worktree "); got != 2 {
		t.Fatalf("worktree count after start fallback = %d, want main plus one Ticket worktree:\n%s", got, worktrees)
	}
	pauseTestRun(t, srv.URL, run.ID)
}

func TestRunFallbackTaskResumeFailureNeedsYou(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	fake := NewFakeBackend()
	fake.ResumeNote = "fallback resume unavailable"
	var failOnce sync.Once
	fake.TurnFailure = func(_, prompt string) (string, bool) {
		if !strings.Contains(prompt, "Implement ticket 01-first") {
			return "", false
		}
		fired := false
		failOnce.Do(func() { fired = true })
		return "quota exceeded", fired
	}
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.State == "needs-you" && tk.Chain["worker"].Position == 2
	}, "Needs-you after fallback task could not resume")
	tk := ticketByID(r, "01-first")
	fallback := getSession(t, srv.URL, tk.WorkerSession)
	if _, _, ok := firstHandoffAndTask(fallback.Events); ok {
		t.Fatal("fallback task was sent after Session resume failed")
	}
	if !strings.Contains(tk.NeedsYou, "fallback resume unavailable") {
		t.Fatalf("Needs-you reason = %q, want resume failure", tk.NeedsYou)
	}
	d.mu.Lock()
	exhausted := d.runs[run.ID].ChainExhausted["01-first"]
	d.mu.Unlock()
	if exhausted {
		t.Fatal("task dispatch failure incorrectly marked the provider Chain exhausted")
	}
	pauseTestRun(t, srv.URL, run.ID)
}

func TestRunFallbackStartOtherFailureNeedsYouNotExhausted(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	fake := NewFakeBackend()
	var failOnce sync.Once
	fake.TurnFailure = func(_, prompt string) (string, bool) {
		if !strings.Contains(prompt, "Implement ticket 01-first") {
			return "", false
		}
		fired := false
		failOnce.Do(func() {
			fired = true
			fake.CreateErrs = []error{errors.New("fallback backend unavailable")}
		})
		return "quota exceeded", fired
	}
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.State == "needs-you" && tk.Chain["worker"].Position == 2
	}, "Needs-you after non-advancing fallback start failure")
	tk := ticketByID(r, "01-first")
	if !strings.Contains(tk.NeedsYou, "fallback backend unavailable") {
		t.Fatalf("Needs-you reason = %q, want fallback start failure", tk.NeedsYou)
	}
	d.mu.Lock()
	exhausted := d.runs[run.ID].ChainExhausted["01-first"]
	d.mu.Unlock()
	if exhausted {
		t.Fatal("non-advancing start failure marked the provider Chain exhausted")
	}
	pauseTestRun(t, srv.URL, run.ID)
}

func TestRunWorkerFallbackResetsReviewerRepromptCount(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	fake := NewFakeBackend()
	fake.Delay = 150 * time.Millisecond
	var failOnce sync.Once
	fake.TurnFailure = func(_, prompt string) (string, bool) {
		if !strings.Contains(prompt, "trigger worker fallback") {
			return "", false
		}
		fired := false
		failOnce.Do(func() { fired = true })
		return "quota exceeded", fired
	}
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "initial Worker")
	initial := ticketByID(r, "01-first").WorkerSession
	submitSession(t, srv.URL, initial, "done", []string{"alpha"})
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.ReviewerSessions[reviewerSpecType] != "" && tk.ReviewerSessions[reviewerStandardsType] != ""
	}, "initial Reviewers")
	oldReviewers := ticketByID(r, "01-first").ReviewerSessions
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "steer", "trigger worker fallback"); code != 200 {
		t.Fatalf("steer Worker failure status = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.WorkerSession != initial && tk.Chain["worker"].Position == 2
	}, "fallback Worker")
	fallbackID := ticketByID(r, "01-first").WorkerSession
	fallback := waitSessionCond(t, srv.URL, fallbackID, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok && s.Meta.Status == "idle"
	}, "fallback Worker task")
	change := filepath.Join(fallback.Meta.WorkDir, "fallback-change.txt")
	if err := os.WriteFile(change, []byte("fallback change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(fallback.Meta.WorkDir, "add", "fallback-change.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(fallback.Meta.WorkDir, "commit", "-m", "fallback worker change"); err != nil {
		t.Fatal(err)
	}
	submitSession(t, srv.URL, fallbackID, "done", []string{"alpha"})
	d.mu.Lock()
	pendingSubmit := d.runs[run.ID].ChainPendingSubmit["01-first"]
	d.mu.Unlock()
	if pendingSubmit {
		t.Fatal("replacement done submit did not clear its pending-submit marker")
	}
	for _, typ := range requiredReviewerTypes {
		id := oldReviewers[typ]
		review := waitSessionCond(t, srv.URL, id, func(s sessionBody) bool {
			count := 0
			for _, e := range s.Events {
				if e.Kind == "prompt" && strings.Contains(e.Text, "Review ticket 01-first") {
					count++
				}
			}
			return count >= 2 && s.Meta.Status == "idle"
		}, typ+" re-review after Worker fallback")
		if review.Meta.Status != "idle" {
			t.Fatalf("%s status = %q, want idle", typ, review.Meta.Status)
		}
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.Verdicts[reviewerSpecType].Verdict == "pass" && tk.Verdicts[reviewerStandardsType].Verdict == "pass"
	}, "re-review verdicts")
	if tk := ticketByID(r, "01-first"); tk.WorkerSession != fallbackID {
		t.Fatalf("Worker after re-review = %q, want fallback %q", tk.WorkerSession, fallbackID)
	}
	pauseTestRun(t, srv.URL, run.ID)
}

func TestRunWorkerChainExhaustionNeedsYou(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	fake := NewFakeBackend()
	fake.TurnFailure = func(_, prompt string) (string, bool) {
		return "quota exceeded", strings.Contains(prompt, "Implement ticket 01-first")
	}
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.State == "needs-you" && tk.WorkerSession != ""
	}, "Needs-you on exhausted Chain")
	tk := ticketByID(r, "01-first")
	if tk.Chain["worker"].Position != 2 || tk.Chain["worker"].Total != 2 {
		t.Fatalf("exhausted Chain position = %+v, want entry 2 of 2", tk.Chain["worker"])
	}
	failed := getSession(t, srv.URL, tk.WorkerSession)
	if failed.Meta.Status != "failed" {
		t.Fatalf("failed Worker status = %q, want failed", failed.Meta.Status)
	}
	pauseTestRun(t, srv.URL, run.ID)
}

func TestRunReviewerStartChainAdvancesOnProviderFailure(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, `{"chains":{"reviewer-spec":[{"backend":"pig","model":"review-primary"},{"backend":"grok","model":"review-fallback"}]}}`)
	fake := NewFakeBackend()
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "Worker")
	worker := ticketByID(r, "01-first").WorkerSession
	fake.CreateFailure = func(meta SessionMeta) error {
		if meta.Reviewer == reviewerSpecType && meta.Harness == "pig" {
			return errors.New("quota exceeded")
		}
		return nil
	}
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		if tk.State == "needs-you" {
			var events []Event
			if id := tk.ReviewerSessions[reviewerSpecType]; id != "" {
				events = getSession(t, srv.URL, id).Events
			}
			t.Fatalf("Reviewer start Chain Needs-you: reason=%q reviewer=%q events=%+v", tk.NeedsYou, tk.ReviewerSessions[reviewerSpecType], events)
		}
		return tk.ReviewerSessions[reviewerSpecType] != "" &&
			tk.Chain[reviewerSpecType].Position == 2 && tk.Verdicts[reviewerSpecType].Verdict == "pass"
	}, "fallback spec Reviewer after start failure")
	tk := ticketByID(r, "01-first")
	reviewer := getSession(t, srv.URL, tk.ReviewerSessions[reviewerSpecType])
	if reviewer.Meta.Harness != "grok" || reviewer.Meta.Model != "review-fallback" {
		t.Fatalf("Reviewer fallback config = %s/%s", reviewer.Meta.Harness, reviewer.Meta.Model)
	}
	if got := tk.Chain[reviewerSpecType]; got.Position != 2 || got.Total != 2 {
		t.Fatalf("Reviewer Chain position = %+v", got)
	}
}

func TestRunReviewerFallbackChain(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, `{"chains":{"reviewer-spec":[{"backend":"pig","model":"review-primary"},{"backend":"grok","model":"review-fallback"}]}}`)
	fake := NewFakeBackend()
	var mu sync.Mutex
	failedID := ""
	failed := false
	fake.TurnFailure = func(id, prompt string) (string, bool) {
		if strings.HasPrefix(prompt, "# Handoff") || !strings.Contains(prompt, "Review ticket 01-first for spec") || !strings.Contains(prompt, "as reviewer-spec") {
			return "", false
		}
		mu.Lock()
		defer mu.Unlock()
		if failed {
			return "", false
		}
		failed, failedID = true, id
		return "HTTP 401 unauthorized", true
	}
	d := NewDaemon(tempDir(t), fake)
	srv := newRunTestServer(t, d)
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "Worker")
	worker := ticketByID(r, "01-first").WorkerSession
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})

	deadline := time.Now().Add(15 * time.Second)
	var tk runTicketJSON
	var oldID string
	for time.Now().Before(deadline) {
		r = getRun(t, srv.URL, run.ID)
		tk = ticketByID(r, "01-first")
		mu.Lock()
		oldID = failedID
		mu.Unlock()
		if oldID != "" && tk.ReviewerSessions[reviewerSpecType] != oldID && tk.Chain[reviewerSpecType].Position == 2 && tk.Verdicts[reviewerSpecType].Verdict == "pass" {
			break
		}
		if tk.State == "needs-you" {
			currentText := ""
			if id := tk.ReviewerSessions[reviewerSpecType]; id != "" {
				currentText = capText(sessTranscript(getSession(t, srv.URL, id)), 2000)
			}
			t.Fatalf("Reviewer Chain reached Needs-you: reason=%q sessions=%v events=%s", tk.NeedsYou, tk.ReviewerSessions, currentText)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if oldID == "" || tk.ReviewerSessions[reviewerSpecType] == oldID || tk.Verdicts[reviewerSpecType].Verdict != "pass" {
		t.Fatalf("fallback spec Reviewer did not pass: ticket=%+v", tk)
	}
	if oldID == "" || oldID == tk.ReviewerSessions[reviewerSpecType] {
		t.Fatalf("reviewer Session was not replaced: old=%q current=%q", oldID, tk.ReviewerSessions[reviewerSpecType])
	}
	old := getSession(t, srv.URL, oldID)
	if old.Meta.Status != "failed" || len(eventsOfKind(old.Events, "error")) == 0 {
		t.Fatalf("failed Reviewer Session did not retain its error log: %+v", old)
	}
	fallback := waitSessionCond(t, srv.URL, tk.ReviewerSessions[reviewerSpecType], func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok && s.Meta.Status == "idle"
	}, "reviewer Handoff and task")
	handoff, task, ok := firstHandoffAndTask(fallback.Events)
	if !ok || handoff.Seq >= task.Seq || !strings.Contains(task.Text, "Review ticket 01-first") || !strings.Contains(task.Text, "--- Ticket ") {
		t.Fatalf("fallback Reviewer prompt sequence = handoff %+v task %+v", handoff, task)
	}
	if fallback.Meta.WorkDir != old.Meta.WorkDir {
		t.Fatalf("fallback Reviewer worktree = %q, want %q", fallback.Meta.WorkDir, old.Meta.WorkDir)
	}
	if fallback.Meta.Harness != "grok" || fallback.Meta.Model != "review-fallback" {
		t.Fatalf("fallback Reviewer config = %s/%s", fallback.Meta.Harness, fallback.Meta.Model)
	}
}

func TestRunReviewerFallbackChainFailureClasses(t *testing.T) {
	cases := []struct {
		name  string
		error string
	}{
		{name: "quota", error: "quota exceeded"},
		{name: "credit", error: "payment required"},
		{name: "session limit", error: "session limit reached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, spec := initRunRepo(t, map[string]string{
				"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
			})
			useSettings(t, `{"chains":{"reviewer-spec":[{"backend":"pig","model":"review-primary"},{"backend":"grok","model":"review-fallback"}]}}`)
			fake := NewFakeBackend()
			var failOnce sync.Once
			var mu sync.Mutex
			failedID := ""
			fake.TurnFailure = func(id, prompt string) (string, bool) {
				if !strings.Contains(prompt, "Review ticket 01-first for spec") || !strings.Contains(prompt, "as reviewer-spec") {
					return "", false
				}
				fired := false
				failOnce.Do(func() {
					fired = true
					mu.Lock()
					failedID = id
					mu.Unlock()
				})
				return tc.error, fired
			}
			d := NewDaemon(tempDir(t), fake)
			srv := newRunTestServer(t, d)
			defer srv.Close()

			run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
			r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
				return ticketByID(r, "01-first").WorkerSession != ""
			}, "Worker")
			submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
			r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
				tk := ticketByID(r, "01-first")
				return tk.Chain[reviewerSpecType].Position == 2 && tk.Verdicts[reviewerSpecType].Verdict == "pass"
			}, "fallback spec Reviewer")
			tk := ticketByID(r, "01-first")
			mu.Lock()
			oldID := failedID
			mu.Unlock()
			if oldID == "" || tk.ReviewerSessions[reviewerSpecType] == oldID {
				t.Fatalf("Reviewer Session was not replaced after %s: old=%q current=%q", tc.name, oldID, tk.ReviewerSessions[reviewerSpecType])
			}
			old := getSession(t, srv.URL, oldID)
			if old.Meta.Status != "failed" || len(eventsOfKind(old.Events, "error")) == 0 {
				t.Fatalf("failed Reviewer did not retain its %s error: %+v", tc.name, old.Events)
			}
			fallback := waitSessionCond(t, srv.URL, tk.ReviewerSessions[reviewerSpecType], func(s sessionBody) bool {
				_, _, ok := firstHandoffAndTask(s.Events)
				return ok && s.Meta.Status == "idle"
			}, "Reviewer Handoff and task")
			handoff, task, ok := firstHandoffAndTask(fallback.Events)
			if !ok || handoff.Seq >= task.Seq {
				t.Fatalf("Reviewer fallback prompt order = handoff %+v, task %+v", handoff, task)
			}
			pauseTestRun(t, srv.URL, run.ID)
		})
	}
}

func TestRunResumeReconcilesFailedChainSession(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := newRunTestServer(t, d)

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "initial Worker")
	worker := ticketByID(r, "01-first").WorkerSession
	waitSessionCond(t, srv.URL, worker, func(s sessionBody) bool { return s.Meta.Status == "idle" }, "initial Worker idle")
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != 200 {
		t.Fatalf("pause status = %d, want 200", code)
	}
	// Persist a failed Session while paused, without the post-turn handler,
	// so Resume exercises reconciliation after reload.
	d.appendEvent(worker, "prompt", "recover after restart", "")
	d.appendEvent(worker, "status", "", "working")
	d.appendEvent(worker, "error", "quota exceeded", "")
	d.appendEvent(worker, "status", "", "failed")
	srv.Close()

	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv2 := newRunTestServer(t, d2)
	defer srv2.Close()
	if _, code := postRunAction(t, srv2.URL, run.ID, "resume"); code != 200 {
		t.Fatalf("resume status = %d, want 200", code)
	}
	r = waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.WorkerSession != worker && tk.Chain["worker"].Position == 2
	}, "reconciled fallback Worker")
	tk := ticketByID(r, "01-first")
	fallback := waitSessionCond(t, srv2.URL, tk.WorkerSession, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok && s.Meta.Status == "idle"
	}, "reconciled Handoff and task")
	if fallback.Meta.WorkDir != getSession(t, srv2.URL, worker).Meta.WorkDir {
		t.Fatal("reconciled fallback changed the Ticket worktree")
	}
	if len(eventsOfKind(getSession(t, srv2.URL, worker).Events, "error")) == 0 {
		t.Fatal("reconciled failed Session lost its error Event")
	}
	pauseTestRun(t, srv2.URL, run.ID)
}

func TestRunWorkerChainPositionSurvivesRestart(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	useSettings(t, workerTestChainSettings)
	stateDir := tempDir(t)
	fake := NewFakeBackend()
	var failOnce sync.Once
	fake.TurnFailure = func(_, prompt string) (string, bool) {
		if !strings.Contains(prompt, "Implement ticket 01-first") {
			return "", false
		}
		fired := false
		failOnce.Do(func() { fired = true })
		return "quota exceeded", fired
	}
	d := NewDaemon(stateDir, fake)
	srv := newRunTestServer(t, d)

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.WorkerSession != "" && tk.Chain["worker"].Position == 2
	}, "fallback Worker position")
	worker := ticketByID(r, "01-first").WorkerSession
	waitSessionCond(t, srv.URL, worker, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok && s.Meta.Status == "idle"
	}, "fallback task completion")
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != 200 {
		t.Fatalf("pause status = %d, want 200", code)
	}
	srv.Close()

	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv2 := newRunTestServer(t, d2)
	defer srv2.Close()
	loaded := getRun(t, srv2.URL, run.ID)
	if got := ticketByID(loaded, "01-first").Chain["worker"]; got.Position != 2 || got.Total != 2 || got.Backend != "grok" || got.Model != "worker-fallback" {
		t.Fatalf("reloaded Chain entry = %+v", got)
	}
	if got := ticketByID(loaded, "01-first").WorkerSession; got != worker {
		t.Fatalf("reloaded Worker = %q, want %q", got, worker)
	}
	if got := ticketByID(loaded, "01-first").State; got == "" {
		t.Fatal("reloaded Ticket row has an empty state")
	}
	d2.mu.Lock()
	pendingSubmit := d2.runs[run.ID].ChainPendingSubmit["01-first"]
	d2.mu.Unlock()
	if !pendingSubmit {
		t.Fatal("reloaded Run lost the replacement Worker's pending-submit marker")
	}
	rows := runFileRows(t, stateDir, run.ID)
	if rows["01-first"] == nil {
		t.Fatal("persisted Run file lost Ticket row")
	}
	positions, ok := rows["01-first"]["chainPositions"].(map[string]any)
	if !ok || positions["worker"] != float64(1) {
		t.Fatalf("persisted Ticket Chain positions = %v, want worker index 1", rows["01-first"]["chainPositions"])
	}
}

func newRunTestServer(t *testing.T, d *Daemon) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	return srv
}
