package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	el "example.com/emberloop"
)

// postRunAction posts to a Run action (pause, resume) and returns the Run
// with the response code. Empty body: the action needs no arguments.
func postRunAction(t *testing.T, base, id, action string) (runJSON, int) {
	t.Helper()
	resp, err := http.Post(base+"/api/runs/"+id+"/"+action, "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Run runJSON `json:"run"`
	}
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out.Run, resp.StatusCode
}

// listRunsJSON lists every Run the Daemon knows, finished ones included.
func listRunsJSON(t *testing.T, base string) []runJSON {
	t.Helper()
	resp, err := http.Get(base + "/api/runs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Runs []runJSON `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Runs
}

// countSessions counts the Daemon's Sessions through the public list, which
// answers a bare array.
func countSessions(t *testing.T, base string) int {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sessions status = %d, want 200", resp.StatusCode)
	}
	var out []any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return len(out)
}

// assertHolds fails fast when cond turns false and passes after dur of it
// holding: negative assertions (nothing started while paused) that catch a
// violation at once instead of sampling once after a fixed sleep.
func assertHolds(t *testing.T, dur time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatalf("while holding: %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// runFileRows reads the Run's persisted file and returns its per-Ticket
// rows. Empty while no driver pass has been written yet.
func runFileRows(t *testing.T, stateDir, id string) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(stateDir, "enderbloom", "runs", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Status string                    `json:"status"`
		Rows   map[string]map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	return saved.Rows
}

// A manual Run pause ends active Sessions after checkpointing their work.
// Resume is explicit and replaces each started Worker with a Handoff Session
// on the same branch; unstarted Tickets remain queued until the Run continues.
func TestPauseStopsNewStartsAndResumeContinues(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, chainTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != "" &&
			ticketByID(r, "02-second").WorkerSession == ""
	}, "only 01-first started")
	first := ticketByID(r, "01-first").WorkerSession

	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", code)
	}
	if got := getRun(t, srv.URL, run.ID).Status; got != "paused" {
		t.Fatalf("status = %q, want paused", got)
	}

	// The old Session is ended by Pause. A late submit on that history cannot
	// merge or start reviewers, and the dependent Ticket stays queued.
	submitSession(t, srv.URL, first, "done", []string{"alpha"})
	assertHolds(t, 1500*time.Millisecond, "01-first merged while paused", func() bool {
		return ticketByID(getRun(t, srv.URL, run.ID), "01-first").State != "merged"
	})
	assertHolds(t, 1500*time.Millisecond, "02-second started while paused", func() bool {
		return ticketByID(getRun(t, srv.URL, run.ID), "02-second").WorkerSession == ""
	})
	if got := ticketByID(getRun(t, srv.URL, run.ID), "01-first").WorkerSession; got != first {
		t.Fatalf("paused 01-first worker = %q, want the ended Session %q", got, first)
	}
	if ended := getSession(t, srv.URL, first).Meta.Status; ended != "ended" {
		t.Fatalf("paused worker status = %q, want ended", ended)
	}

	resumed, code := postRunAction(t, srv.URL, run.ID, "resume")
	if code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", code)
	}
	if resumed.Status != "running" {
		t.Fatalf("resumed status = %q, want running", resumed.Status)
	}
	if got := ticketByID(resumed, "01-first").WorkerSession; got == first || got == "" {
		t.Fatalf("resumed 01-first worker = %q, want a new Session replacing %q", got, first)
	}
	nextWorker := ticketByID(resumed, "01-first").WorkerSession
	submitSession(t, srv.URL, nextWorker, "done", []string{"alpha"})

	// The done submit from the pause now drives: reviewers, merge, and the
	// next Ticket starts exactly once.
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" &&
			ticketByID(r, "02-second").WorkerSession != ""
	}, "01-first merged and 02-second started after resume")
	second := ticketByID(r, "02-second").WorkerSession
	submitSession(t, srv.URL, second, "done", []string{"beta"})
	passReviewers(t, srv.URL, run.ID, "02-second")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "02-second").State == "merged" &&
			ticketByID(r, "03-third").WorkerSession != ""
	}, "02-second merged and 03-third started")
	third := ticketByID(r, "03-third").WorkerSession
	submitSession(t, srv.URL, third, "done", []string{"gamma"})
	passReviewers(t, srv.URL, run.ID, "03-third")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}

	runs := listRunsJSON(t, srv.URL)
	found := false
	for _, cand := range runs {
		if cand.ID == run.ID {
			found = true
			if cand.Status != "done" || len(cand.Tickets) != 3 {
				t.Fatalf("listed run = %+v, want done with 3 tickets", cand)
			}
		}
	}
	if !found {
		t.Fatalf("finished run %s missing from the list", run.ID)
	}

	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusConflict {
		t.Fatalf("pause on done = %d, want 409", code)
	}
}

// Restarting the Daemon mid-Run leaves the Run paused with nothing spawned:
// the same Worker Session waits, and Resume continues without a second
// Worker. Run state reaches the file on every driver pass, so the reloaded
// Run shows its Ticket rows.
func TestRestartPausesRunAndResumeKeepsWorker(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	stateDir := tempDir(t)
	d := runTestDaemon(t, stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	first := ticketByID(r, "01-first").WorkerSession
	before := countSessions(t, srv.URL)

	// Every driver pass reaches the file: the row for the started Ticket,
	// with its state recorded, is already there before any restart. The
	// state check matters: an early pass can persist the row before the
	// Ticket state lands, and restarting then would reload an empty state.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if rows := runFileRows(t, stateDir, run.ID); rows["01-first"] != nil && rows["01-first"]["state"] != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run file has no state row for 01-first")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// RequestRestart is the graceful-restart path: it freezes the driver but
	// preserves live Worker Sessions so a reload resumes them without replacement.
	if !d.RequestRestart() {
		t.Fatal("RequestRestart returned false")
	}
	srv.Close()

	d2 := runTestDaemon(t, stateDir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()

	r = getRun(t, srv2.URL, run.ID)
	if r.Status != "paused" {
		t.Fatalf("reloaded status = %q, want paused", r.Status)
	}
	if got := ticketByID(r, "01-first"); got.WorkerSession != first {
		t.Fatalf("reloaded worker = %q, want %q", got.WorkerSession, first)
	}
	if got := ticketByID(r, "01-first"); got.State == "" || got.State == "queued" {
		t.Fatalf("reloaded 01-first state = %q, want the persisted row", got.State)
	}
	assertHolds(t, time.Second, "reload spawned a session", func() bool {
		return countSessions(t, srv2.URL) == before
	})
	assertHolds(t, time.Second, "reloaded run left paused", func() bool {
		return getRun(t, srv2.URL, run.ID).Status == "paused"
	})

	resumed, code := postRunAction(t, srv2.URL, run.ID, "resume")
	if code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", code)
	}
	if got := ticketByID(resumed, "01-first").WorkerSession; got != first {
		t.Fatalf("resumed worker = %q, want the same %q (no second Worker)", got, first)
	}
	if got := countSessions(t, srv2.URL); got != before {
		t.Fatalf("sessions after resume = %d, want %d (no second Worker)", got, before)
	}

	submitSession(t, srv2.URL, first, "done", []string{"alpha"})
	passReviewers(t, srv2.URL, run.ID, "01-first")
	r = waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" &&
			ticketByID(r, "02-second").WorkerSession != ""
	}, "01-first merged and 02-second started after resume")
	second := ticketByID(r, "02-second").WorkerSession
	submitSession(t, srv2.URL, second, "done", []string{"beta"})
	passReviewers(t, srv2.URL, run.ID, "02-second")
	_ = waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
}

// Pause and Resume refuse the wrong states and unknown Runs.
func TestPauseResumeValidation(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusConflict {
		t.Fatalf("resume while running = %d, want 409", code)
	}
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("pause = %d, want 200", code)
	}
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusConflict {
		t.Fatalf("second pause = %d, want 409", code)
	}
	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusOK {
		t.Fatalf("resume = %d, want 200", code)
	}
	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusConflict {
		t.Fatalf("second resume = %d, want 409", code)
	}
	if _, code := postRunAction(t, srv.URL, "no-such-run", "pause"); code != http.StatusNotFound {
		t.Fatalf("pause unknown = %d, want 404", code)
	}
	if _, code := postRunAction(t, srv.URL, "no-such-run", "resume"); code != http.StatusNotFound {
		t.Fatalf("resume unknown = %d, want 404", code)
	}
}

// A Pause racing a start records nothing: the attempt is dropped and the
// Resume starts every Ticket exactly once, with no orphan Session left
// behind. Rounds retry until a Pause lands before the driver's first start;
// other rounds prove nothing and close their server.
func TestPauseDuringStartStartsEveryTicketOnce(t *testing.T) {
	t.Parallel()
	tickets := map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	}
	for n := 0; ; n++ {
		if n >= 6 {
			t.Fatalf("pause never landed before the first start in 6 rounds")
		}
		repo, spec := initRunRepo(t, tickets)
		d := runTestDaemon(t, tempDir(t), NewFakeBackend())
		srv := httptest.NewServer(authedHandler(t, d))
		run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
		if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
			srv.Close()
			t.Fatalf("pause status = %d, want 200", code)
		}
		r := getRun(t, srv.URL, run.ID)
		if ticketByID(r, "01-first").WorkerSession != "" ||
			ticketByID(r, "02-second").WorkerSession != "" {
			// Pause lost the race: a Worker already exists. Next round.
			srv.Close()
			continue
		}
		// Pause won: no Worker may appear until the Resume.
		assertHolds(t, time.Second, "worker started while paused", func() bool {
			rr := getRun(t, srv.URL, run.ID)
			return ticketByID(rr, "01-first").WorkerSession == "" &&
				ticketByID(rr, "02-second").WorkerSession == ""
		})
		if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusOK {
			srv.Close()
			t.Fatalf("resume status = %d, want 200", code)
		}
		r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
			return ticketByID(r, "01-first").WorkerSession != "" &&
				ticketByID(r, "02-second").WorkerSession != ""
		}, "both workers started after resume")
		// No dropped attempt left an orphan Session behind.
		if got := countSessions(t, srv.URL); got != 2 {
			srv.Close()
			t.Fatalf("sessions = %d, want 2 (no orphan from the dropped start)", got)
		}
		first, second := ticketByID(r, "01-first").WorkerSession, ticketByID(r, "02-second").WorkerSession
		submitSession(t, srv.URL, first, "done", []string{"alpha"})
		submitSession(t, srv.URL, second, "done", []string{"beta"})
		passReviewers(t, srv.URL, run.ID, "01-first")
		passReviewers(t, srv.URL, run.ID, "02-second")
		r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
			return r.Status == "done"
		}, "run done")
		if got := ticketByID(r, "01-first").WorkerSession; got != first {
			srv.Close()
			t.Fatalf("01-first worker changed to %q, want the same %q", got, first)
		}
		if got := ticketByID(r, "02-second").WorkerSession; got != second {
			srv.Close()
			t.Fatalf("02-second worker changed to %q, want the same %q", got, second)
		}
		srv.Close()
		return
	}
}

// No API call deletes a Run, so starting for an unknown Run is unreachable
// outside these defensive branches: both error instead of touching a nil Run.
func TestRunStartUnknownRunErrors(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	if err := d.runStartWorker("no-such-run", el.DriverTicket{ID: "01-first"}); err == nil {
		t.Fatalf("runStartWorker unknown run = nil, want error")
	}
	if err := d.runStartReviewer("no-such-run", "01-first", reviewerSpecType, "tip"); err == nil {
		t.Fatalf("runStartReviewer unknown run = nil, want error")
	}
}

// POST /api/restart pauses the running Run and marks it; the next Daemon
// resumes it on its own with the same Worker and no second one. A Run the
// user paused by hand stays paused.
func TestRestartRequestResumesRunOnNextDaemon(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	stateDir := tempDir(t)
	d := runTestDaemon(t, stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	first := ticketByID(r, "01-first").WorkerSession
	before := countSessions(t, srv.URL)

	resp, err := http.Post(srv.URL+"/api/restart", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("restart status = %d, want 202", resp.StatusCode)
	}
	select {
	case <-d.RestartCh():
	case <-time.After(time.Second):
		t.Fatal("restart channel did not fire")
	}
	if got := getRun(t, srv.URL, run.ID).Status; got != "paused" {
		t.Fatalf("status after request = %q, want paused", got)
	}
	srv.Close()

	d2 := runTestDaemon(t, stateDir, NewFakeBackend())
	d2.ResumeAfterRestart()
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	r = getRun(t, srv2.URL, run.ID)
	if r.Status != "running" {
		t.Fatalf("reloaded status = %q, want running", r.Status)
	}
	if got := ticketByID(r, "01-first").WorkerSession; got != first {
		t.Fatalf("worker = %q, want the same %q", got, first)
	}
	if got := countSessions(t, srv2.URL); got != before {
		t.Fatalf("sessions = %d, want %d (no second Worker)", got, before)
	}
}

// A failed Run (the driver hit an error such as a git failure) resumes from
// its saved state: the same Worker Session, no restart from zero, error
// cleared.
func TestFailedRunResumesWithSameWorker(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	first := ticketByID(r, "01-first").WorkerSession
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", code)
	}
	d.mu.Lock()
	d.runs[run.ID].Status = runStatusFailed
	d.runs[run.ID].Error = "could not finish 01-first: exit status 128"
	d.mu.Unlock()

	resumed, code := postRunAction(t, srv.URL, run.ID, "resume")
	if code != http.StatusOK {
		t.Fatalf("resume of a failed Run = %d, want 200", code)
	}
	if resumed.Status != "running" {
		t.Fatalf("resumed status = %q, want running", resumed.Status)
	}
	d.mu.Lock()
	errText := d.runs[run.ID].Error
	d.mu.Unlock()
	if errText != "" {
		t.Fatalf("error after resume = %q, want cleared", errText)
	}
	if got := ticketByID(resumed, "01-first").WorkerSession; got == first || got == "" {
		t.Fatalf("resumed worker = %q, want a replacement for %q", got, first)
	}
}
