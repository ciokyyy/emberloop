package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func postRunJSON(t *testing.T, base, path string, body any) (int, runJSON, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Run   runJSON `json:"run"`
		Error string  `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Run, out.Error
}

func TestTicketPauseSwitchResumeKeepsBranchAndWIP(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "Ticket Worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	branch := ticketByID(r, "01-first").Branch
	worktree := getSession(t, srv.URL, worker).Meta.WorkDir
	tip, err := runGit(repo, "rev-parse", branch)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.runStartReviewer(run.ID, "01-first", reviewerSpecType, strings.TrimSpace(tip)); err != nil {
		t.Fatalf("start Ticket Reviewer: %v", err)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").ReviewerSessions[reviewerSpecType] != ""
	}, "Ticket Reviewer started")
	oldReviewer := ticketByID(r, "01-first").ReviewerSessions[reviewerSpecType]

	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/switch", map[string]string{"backend": "fake", "model": "switched-model"}); code != http.StatusConflict {
		t.Fatalf("Switch while running = %d, want 409", code)
	}
	writeFile(t, worktree, "pause-checkpoint.txt", "preserve this uncommitted work\n")
	if code, paused, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/pause", map[string]string{}); code != http.StatusOK {
		t.Fatalf("Ticket pause = %d (%s), want 200", code, errText)
	} else if ticketByID(paused, "01-first").State != runStatusPaused {
		t.Fatalf("paused Ticket state = %q, want paused", ticketByID(paused, "01-first").State)
	}
	if got := getSession(t, srv.URL, worker).Meta.Status; got != "ended" {
		t.Fatalf("paused Worker status = %q, want ended", got)
	}
	if got := getSession(t, srv.URL, oldReviewer).Meta.Status; got != "ended" {
		t.Fatalf("paused Reviewer status = %q, want ended", got)
	}
	if subject := testGit(t, worktree, "log", "-1", "--format=%s"); !bytes.Contains([]byte(subject), []byte("WIP: paused Run "+run.ID+" Ticket 01-first")) {
		t.Fatalf("checkpoint commit subject = %q, want labelled WIP", subject)
	}
	if got := testGit(t, worktree, "show", "HEAD:pause-checkpoint.txt"); got != "preserve this uncommitted work\n" {
		t.Fatalf("checkpoint file = %q, want committed content", got)
	}

	code, switched, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/switch", map[string]string{"backend": "fake", "model": "switched-model"})
	if code != http.StatusOK {
		t.Fatalf("Ticket Switch = %d (%s), want 200", code, errText)
	}
	for _, role := range []string{"worker", reviewerSpecType, reviewerStandardsType} {
		if got := ticketByID(switched, "01-first").Chain[role].Model; got != "switched-model" {
			t.Fatalf("active %s model = %q, want switched-model", role, got)
		}
	}
	if got := ticketByID(getRun(t, srv.URL, run.ID), "01-first").WorkerSession; got != worker {
		t.Fatalf("Switch restarted Worker automatically: got %q, want ended %q", got, worker)
	}

	code, resumed, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/resume", map[string]string{})
	if code != http.StatusOK {
		t.Fatalf("Ticket Resume = %d (%s), want 200", code, errText)
	}
	next := ticketByID(resumed, "01-first")
	if next.WorkerSession == "" || next.WorkerSession == worker {
		t.Fatalf("resumed Worker = %q, want a new Session replacing %q", next.WorkerSession, worker)
	}
	workerSession := getSession(t, srv.URL, next.WorkerSession)
	if workerSession.Meta.Harness != "fake" || workerSession.Meta.Model != "switched-model" {
		t.Fatalf("resumed Worker config = %s/%s, want fake/switched-model", workerSession.Meta.Harness, workerSession.Meta.Model)
	}
	newReviewer := next.ReviewerSessions[reviewerSpecType]
	if newReviewer == "" || newReviewer == oldReviewer {
		t.Fatalf("resumed Reviewer = %q, want a new Session replacing %q", newReviewer, oldReviewer)
	}
	reviewerSession := getSession(t, srv.URL, newReviewer)
	if reviewerSession.Meta.Harness != "fake" || reviewerSession.Meta.Model != "switched-model" {
		t.Fatalf("resumed Reviewer config = %s/%s, want fake/switched-model", reviewerSession.Meta.Harness, reviewerSession.Meta.Model)
	}
	waitSessionCond(t, srv.URL, newReviewer, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok
	}, "Ticket Reviewer Resume Handoff and task")
	if next.Branch != branch {
		t.Fatalf("resumed branch = %q, want same branch %q", next.Branch, branch)
	}
	waitSessionCond(t, srv.URL, next.WorkerSession, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok
	}, "Ticket Resume Handoff and task")
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/switch", map[string]string{"backend": "fake", "model": "late"}); code != http.StatusConflict {
		t.Fatalf("Switch after Resume = %d, want 409", code)
	}
}

func TestRunPauseSwitchResumeReplacesWorkersManually(t *testing.T) {
	t.Parallel()
	tickets := map[string]string{
		"01-first.md":  "# 01: First\n\n**Type:** design\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	}
	repo, spec := initRunRepo(t, tickets)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != "" && ticketByID(r, "02-second").WorkerSession != ""
	}, "both Workers started")
	old := map[string]string{}
	branches := map[string]string{}
	for _, id := range []string{"01-first", "02-second"} {
		tk := ticketByID(r, id)
		old[id], branches[id] = tk.WorkerSession, tk.Branch
		writeFile(t, getSession(t, srv.URL, tk.WorkerSession).Meta.WorkDir, "pause.txt", id+" work\n")
	}

	_, code := postRunAction(t, srv.URL, run.ID, "pause")
	if code != http.StatusOK {
		t.Fatalf("Run pause = %d, want 200", code)
	}
	for _, id := range []string{"01-first", "02-second"} {
		if got := getSession(t, srv.URL, old[id]).Meta.Status; got != "ended" {
			t.Fatalf("%s Worker status = %q, want ended", id, got)
		}
		worktree := filepath.Join(repo, ".el-worktrees", id)
		if got := testGit(t, worktree, "show", "HEAD:pause.txt"); got != id+" work\n" {
			t.Fatalf("%s WIP content = %q", id, got)
		}
	}
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "run-switched"}); code != http.StatusOK {
		t.Fatalf("Run Switch = %d, want 200", code)
	}
	if got := getRun(t, srv.URL, run.ID).Model; got != "run-switched" {
		t.Fatalf("Run model = %q, want run-switched", got)
	}
	if got := ticketByID(getRun(t, srv.URL, run.ID), "01-first").Chain["worker-design"].Model; got != "run-switched" {
		t.Fatalf("active Designer model = %q, want run-switched", got)
	}
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "late"}); code != http.StatusOK {
		t.Fatalf("repeated paused Run Switch = %d, want 200", code)
	}
	for _, id := range []string{"01-first", "02-second"} {
		if got := ticketByID(getRun(t, srv.URL, run.ID), id).WorkerSession; got != old[id] {
			t.Fatalf("Switch restarted %s automatically: got %q, want %q", id, got, old[id])
		}
	}

	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusOK {
		t.Fatalf("Run Resume = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != old["01-first"] && ticketByID(r, "02-second").WorkerSession != old["02-second"]
	}, "replacement Workers after manual Run Resume")
	for _, id := range []string{"01-first", "02-second"} {
		tk := ticketByID(r, id)
		if tk.Branch != branches[id] {
			t.Fatalf("%s branch changed from %q to %q", id, branches[id], tk.Branch)
		}
		waitSessionCond(t, srv.URL, tk.WorkerSession, func(s sessionBody) bool {
			_, _, ok := firstHandoffAndTask(s.Events)
			return ok
		}, id+" Resume Handoff and task")
	}
}

func TestRunSwitchRequiresPause(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "new-model"}); code != http.StatusConflict {
		t.Fatalf("unpaused Run Switch = %d, want 409", code)
	}
}

func TestRunSwitchChangesReviewerModelOnResume(t *testing.T) {
	// Serial: a fast Run driver can race a pause/switch/resume sequence into a concluded run.
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "Worker started before Reviewer setup")
	ticket := ticketByID(r, "01-first")
	oldWorker := ticket.WorkerSession
	waitInitialTurn := func(sessionID, role string) {
		initial := waitSessionCond(t, srv.URL, sessionID, func(s sessionBody) bool {
			for _, event := range s.Events {
				if event.Kind == "prompt" {
					return true
				}
			}
			return false
		}, "initial "+role+" task")
		var promptSeq int64
		for _, event := range initial.Events {
			if event.Kind == "prompt" {
				promptSeq = event.Seq
				break
			}
		}
		waitTurnEnd(t, srv.URL, sessionID, promptSeq)
		if !d.waitRunSessionIdle(sessionID, 10*time.Second) {
			t.Fatalf("initial %s did not settle before Run pause", role)
		}
	}
	waitInitialTurn(oldWorker, "Worker")
	tip, err := runGit(repo, "rev-parse", ticket.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.runStartReviewer(run.ID, "01-first", reviewerSpecType, strings.TrimSpace(tip)); err != nil {
		t.Fatalf("start Reviewer: %v", err)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").ReviewerSessions[reviewerSpecType] != ""
	}, "Reviewer started before Run pause")
	oldReviewer := ticketByID(r, "01-first").ReviewerSessions[reviewerSpecType]
	waitInitialTurn(oldReviewer, "Reviewer")

	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("Run pause = %d, want 200", code)
	}
	if code, _, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "run-switched"}); code != http.StatusOK {
		t.Fatalf("Run Switch = %d (%s), want 200", code, errText)
	}
	paused := getRun(t, srv.URL, run.ID)
	for _, role := range []string{"worker", reviewerSpecType, reviewerStandardsType} {
		if got := ticketByID(paused, "01-first").Chain[role].Model; got != "run-switched" {
			t.Fatalf("paused %s model = %q, want run-switched", role, got)
		}
	}

	resumed, code := postRunAction(t, srv.URL, run.ID, "resume")
	if code != http.StatusOK {
		t.Fatalf("Run Resume = %d, want 200", code)
	}
	resumedTicket := ticketByID(resumed, "01-first")
	if resumedTicket.State == runStatusPaused {
		t.Fatal("Run Resume returned before clearing its Ticket pause marker")
	}
	if resumedTicket.WorkerSession == oldWorker || resumedTicket.WorkerSession == "" {
		t.Fatalf("resumed Worker = %q, want replacement for %q", resumedTicket.WorkerSession, oldWorker)
	}
	if got := getSession(t, srv.URL, resumedTicket.WorkerSession).Meta.Model; got != "run-switched" {
		t.Fatalf("resumed Worker model = %q, want run-switched", got)
	}
	newReviewer := resumedTicket.ReviewerSessions[reviewerSpecType]
	if newReviewer == "" || newReviewer == oldReviewer {
		t.Fatalf("resumed Reviewer = %q, want replacement for %q", newReviewer, oldReviewer)
	}
	deadline := time.Now().Add(15 * time.Second)
	var reviewerSession sessionBody
	for {
		reviewerSession = getSession(t, srv.URL, newReviewer)
		if _, _, ok := firstHandoffAndTask(reviewerSession.Events); ok {
			break
		}
		if time.Now().After(deadline) {
			current := getActionRun(t, srv.URL, run.ID)
			d.mu.Lock()
			rs := d.runs[run.ID]
			status, driving, currentReviewer, needsYou := "", false, "", ""
			var pausedRoles map[string]bool
			if rs != nil {
				status, driving = rs.Status, rs.driving
				currentReviewer = rs.Reviewers["01-first"][reviewerSpecType]
				needsYou = rs.NeedsYouReason["01-first"]
				pausedRoles = make(map[string]bool, len(rs.PausedRoles["01-first"]))
				for role, paused := range rs.PausedRoles["01-first"] {
					pausedRoles[role] = paused
				}
			}
			d.mu.Unlock()
			t.Fatalf("timed out waiting for Run-switched Reviewer Handoff and task: run=%+v ticket=%+v status=%q driving=%v currentReviewer=%q needsYou=%q pausedRoles=%v session=%+v", current, actionTicketByID(current, "01-first"), status, driving, currentReviewer, needsYou, pausedRoles, reviewerSession)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if reviewerSession.Meta.Harness != "fake" || reviewerSession.Meta.Model != "run-switched" {
		t.Fatalf("resumed Reviewer config = %s/%s, want fake/run-switched", reviewerSession.Meta.Harness, reviewerSession.Meta.Model)
	}
}

func TestRunResumePreservesStartedAndUnstartedTicketPauses(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
		"03-third.md":  "# 03: Third\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] gamma\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != "" &&
			ticketByID(r, "02-second").WorkerSession != "" &&
			ticketByID(r, "03-third").WorkerSession == ""
	}, "two Workers started and blocked Ticket remains unstarted")
	first, second := ticketByID(r, "01-first"), ticketByID(r, "02-second")
	for _, id := range []string{"02-second", "03-third"} {
		if code, _, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/"+id+"/pause", map[string]any{}); code != http.StatusOK {
			t.Fatalf("pause Ticket %s = %d (%s), want 200", id, code, errText)
		}
	}
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("Run pause = %d, want 200", code)
	}
	resumed, code := postRunAction(t, srv.URL, run.ID, "resume")
	if code != http.StatusOK {
		t.Fatalf("Run Resume = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != first.WorkerSession
	}, "Run-paused Worker replacement")
	if got := ticketByID(r, "02-second").WorkerSession; got != second.WorkerSession {
		t.Fatalf("Run Resume replaced individually paused Worker: got %q, want %q", got, second.WorkerSession)
	}
	if got := ticketByID(r, "02-second").State; got != runStatusPaused {
		t.Fatalf("individually paused Ticket state = %q, want paused", got)
	}
	third := ticketByID(r, "03-third")
	if third.WorkerSession != "" || third.State != runStatusPaused {
		t.Fatalf("unstarted individually paused Ticket after Run Resume = session %q, state %q; want no Session and paused", third.WorkerSession, third.State)
	}
	if got := ticketByID(resumed, "03-third").State; got != runStatusPaused {
		t.Fatalf("Run Resume response unstarted Ticket state = %q, want paused", got)
	}
	if got := ticketByID(resumed, "02-second").WorkerSession; got != second.WorkerSession {
		t.Fatalf("Run Resume response Worker = %q, want still-paused Session %q", got, second.WorkerSession)
	}
	code, secondResumed, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/02-second/resume", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("started Ticket Resume = %d (%s), want 200", code, errText)
	}
	newSecond := ticketByID(secondResumed, "02-second")
	if newSecond.WorkerSession == second.WorkerSession || newSecond.WorkerSession == "" {
		t.Fatalf("started Ticket Resume Worker = %q, want replacement for %q", newSecond.WorkerSession, second.WorkerSession)
	}
	if newSecond.Branch != second.Branch {
		t.Fatalf("started Ticket Resume branch = %q, want %q", newSecond.Branch, second.Branch)
	}
	waitSessionCond(t, srv.URL, newSecond.WorkerSession, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok
	}, "Ticket Resume after Run Resume")

	code, ticketResumed, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/03-third/resume", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("unstarted Ticket Resume = %d (%s), want 200", code, errText)
	}
	if got := ticketByID(ticketResumed, "03-third").State; got == runStatusPaused {
		t.Fatalf("explicit Ticket Resume kept unstarted Ticket paused: state %q", got)
	}
}

func TestResumeSkipsMergedTicketPausedRoles(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	rs := &runState{
		ID:             "run1",
		Status:         runStatusPaused,
		Merged:         map[string]string{"01-done": "abc"},
		Repo:           tempDir(t),
		Branches:       map[string]string{"01-done": "implement/01-done"},
		Worktrees:      map[string]string{"01-done": filepath.Join(tempDir(t), "gone")},
		PausedRoles:    map[string]map[string]bool{"01-done": {reviewerSpecType: true}},
		RunPausedRoles: map[string]map[string]bool{"01-done": {reviewerSpecType: true}},
		TicketPaused:   map[string]bool{"01-done": true},
	}
	d.mu.Lock()
	d.runs["run1"] = rs
	d.markRunRolePausedByRunLocked(rs, "01-done", reviewerStandardsType)
	d.mu.Unlock()
	if err := d.resumePausedRunRoles("run1", ""); err != nil {
		t.Fatalf("resumePausedRunRoles with merged Ticket = %v, want nil", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(rs.PausedRoles) != 0 || len(rs.RunPausedRoles) != 0 || len(rs.TicketPaused) != 0 {
		t.Fatalf("stale marks left: %v %v %v", rs.PausedRoles, rs.RunPausedRoles, rs.TicketPaused)
	}
}

func TestFinishWorkerAnswersMergedTicketWithoutWorktree(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	gone := filepath.Join(tempDir(t), "gone")
	rs := &runState{
		ID:          "run1",
		Status:      runStatusRunning,
		Repo:        repo,
		SpecBranch:  "spec/x",
		Merged:      map[string]string{"01-first": "abc"},
		Branches:    map[string]string{"01-first": "implement/01-first"},
		Worktrees:   map[string]string{"01-first": gone},
		Sessions:    map[string]string{},
		PausedRoles: map[string]map[string]bool{},
		notify:      make(chan struct{}),
	}
	d.mu.Lock()
	d.runs["run1"] = rs
	d.mu.Unlock()
	fin, err := d.runFinishWorker("run1", "01-first")
	if err != nil || !fin.Merged || fin.SHA != "abc" {
		t.Fatalf("Finish of merged Ticket = %+v, %v; want merged abc", fin, err)
	}

	// Merge landed on the spec branch but the record never saved.
	head, err := runGit(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	head = strings.TrimSpace(head)
	for _, b := range []string{"implement/02-second", "spec/x"} {
		if _, err := runGit(repo, "branch", b, head); err != nil {
			t.Fatal(err)
		}
	}
	d.mu.Lock()
	rs.Merged = map[string]string{}
	rs.Branches["02-second"] = "implement/02-second"
	rs.Worktrees["02-second"] = gone
	d.mu.Unlock()
	fin, err = d.runFinishWorker("run1", "02-second")
	if err != nil || !fin.Merged || fin.SHA != head {
		t.Fatalf("Finish with gone worktree and tip in spec = %+v, %v; want merged %s", fin, err, head)
	}
}

func TestCleanTipSHATrimsAppendedProse(t *testing.T) {
	t.Parallel()
	sha := "9f23f230118efc0c55039ab616c66d21ec6be429"
	cases := map[string]string{
		sha:                              sha,
		" " + sha + "\n":                 sha,
		sha + ". Commit scope clean: ok": sha,
		sha + " (docs only)":             sha,
		strings.ToUpper(sha):             sha,
		"abc123":                         "abc123",
		sha + "ff":                       sha + "ff",
		"not a sha at all":               "not a sha at all",
	}
	for in, want := range cases {
		if got := cleanTipSHA(in); got != want {
			t.Errorf("cleanTipSHA(%q) = %q, want %q", in, got, want)
		}
	}
	rs := &runState{
		Reviews:    map[string]map[string]storedReview{"t": {"reviewer-spec": {Verdict: "pass", At: sha + ". Commit scope clean"}}},
		ReviewTips: map[string]map[string]string{"t": {"reviewer-spec": sha + ". x"}},
	}
	repairStoredReviewTips(rs)
	if got := rs.Reviews["t"]["reviewer-spec"].At; got != sha {
		t.Fatalf("repaired At = %q, want %q", got, sha)
	}
	if got := rs.ReviewTips["t"]["reviewer-spec"]; got != sha {
		t.Fatalf("repaired ReviewTips = %q, want %q", got, sha)
	}
}
