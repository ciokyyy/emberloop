package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rewriteJSONFile loads a JSON object file, lets edit change it, and writes it
// back: the way a test stages the disk state a killed Daemon leaves behind.
func rewriteJSONFile(t *testing.T, path string, edit func(map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A Daemon killed with a Run still running leaves the Run file at running,
// with no graceful pause. The next Daemon resumes it on its own with the same
// Worker Session, and a Worker whose turn the kill cut short (status working
// on disk) gets a continue prompt in that same Session, not a second Worker.
func TestKilledDaemonRunResumesSameWorkerWithContinue(t *testing.T) {
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

	// Pause through a restart request so the old driver stops writing, then
	// rewrite the files into what a hard kill leaves: running, never marked
	// for resume, the Worker turn still open.
	resp, err := http.Post(srv.URL+"/api/restart", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	srv.Close()
	rewriteJSONFile(t, filepath.Join(stateDir, "enderbloom", "runs", run.ID+".json"), func(m map[string]any) {
		m["status"] = "running"
		delete(m, "resumeOnRestart")
	})
	rewriteJSONFile(t, filepath.Join(stateDir, "enderbloom", "sessions", first, "meta.json"), func(m map[string]any) {
		m["status"] = "working"
	})

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
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, e := range sessionEvents(t, srv2.URL, first) {
			if e.Kind == "prompt" && strings.Contains(e.Text, "The Daemon restarted while you were working") {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("worker got no continue prompt after the restart")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A Run the user paused by hand stays paused across a kill: only a Run that
// was running on disk resumes.
func TestKilledDaemonKeepsManualPause(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	stateDir := tempDir(t)
	d := runTestDaemon(t, stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("pause = %d, want 200", code)
	}
	srv.Close()

	d2 := runTestDaemon(t, stateDir, NewFakeBackend())
	d2.ResumeAfterRestart()
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if got := getRun(t, srv2.URL, run.ID).Status; got != "paused" {
		t.Fatalf("status = %q, want paused (manual pause survives a kill)", got)
	}
}

// Shutdown ends the open turn of a live Session in the log: the next Daemon
// reads it as interrupted by the restart, and a turn error from the closing
// process stays an idle edge instead of a failure.
func TestShutdownMarksOpenTurnInterruptedNotFailed(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	id := "sess-open-turn"
	d.mu.Lock()
	d.sessions[id] = &session{
		meta:   SessionMeta{ID: id, Harness: "fake", Status: "working"},
		live:   true,
		notify: make(chan struct{}),
	}
	d.mu.Unlock()

	d.Shutdown()
	d.appendEvent(id, "status", "adapter closed", "failed")

	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.sessions[id]
	if s.meta.Status != "idle" {
		t.Fatalf("status = %q, want idle after shutdown", s.meta.Status)
	}
	if !interruptedByRestartLocked(s) {
		t.Fatalf("session not read as interrupted by the restart; events = %+v", s.events)
	}
}

// A restart request does not wait for a Run Worker's turn to end: it stops the
// turn (no tool is open on the fake), keeps the Session and worktree, and the
// drain no longer lists it. A message queued behind the turn does not start a
// new turn; it rides to the next Daemon, where the same Session gets one
// continue prompt that carries it. The submit gate sends no reminder.
func TestRestartStopsRunWorkerTurnAndNextDaemonContinuesIt(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	stateDir := tempDir(t)
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, stateDir, fb)
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "01-first started")
	first := ticketByID(r, "01-first").WorkerSession
	deadline := time.Now().Add(5 * time.Second)
	for {
		d.mu.Lock()
		busy := d.sessions[first].busy
		d.mu.Unlock()
		if busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker turn never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code, out := postMode(t, srv.URL, first, "followup", "queued finding"); code != http.StatusAccepted {
		t.Fatalf("followup = %d %v", code, out)
	}

	resp, err := http.Post(srv.URL+"/api/restart", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	deadline = time.Now().Add(5 * time.Second)
	for fb.InterruptCount(first) == 0 || !d.Idle(0) {
		if time.Now().After(deadline) {
			t.Fatalf("worker turn not stopped: interrupts=%d state=%+v", fb.InterruptCount(first), d.RestartState())
		}
		time.Sleep(10 * time.Millisecond)
	}
	d.mu.Lock()
	stopped, carry := d.sessions[first].meta.RestartStopped, d.sessions[first].meta.Carry
	d.mu.Unlock()
	if !stopped || !strings.Contains(carry, "queued finding") {
		t.Fatalf("stopped=%v carry=%q, want the stop marked and the queued message carried", stopped, carry)
	}
	for _, e := range sessionEvents(t, srv.URL, first) {
		if e.Kind == "prompt" && strings.Contains(e.Text, "submit") && strings.Contains(e.Text, "reminder") {
			t.Fatalf("submit gate reminded a restart-stopped turn: %q", e.Text)
		}
	}
	srv.Close()

	d2 := runTestDaemon(t, stateDir, NewFakeBackend())
	d2.ResumeAfterRestart()
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if got := ticketByID(getRun(t, srv2.URL, run.ID), "01-first").WorkerSession; got != first {
		t.Fatalf("worker = %q, want the same %q", got, first)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		for _, e := range sessionEvents(t, srv2.URL, first) {
			if e.Kind == "prompt" && strings.Contains(e.Text, "The Daemon restarted while you were working") && strings.Contains(e.Text, "queued finding") {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("worker got no continue prompt carrying the queued message")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A Daemon that boots four times inside five minutes is crash-looping: it
// stops resuming Runs on its own, so a crash tied to a Run is not replayed on
// every boot. Boots spread out in time never trip it.
func TestBootLoopGuardTripsOnFourBootsInFiveMinutes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.clock = func() time.Time { return now }
	for i := 1; i <= 3; i++ {
		if d.bootLoopTripped() {
			t.Fatalf("boot %d tripped the guard, want 4 needed", i)
		}
		now = now.Add(30 * time.Second)
	}
	if !d.bootLoopTripped() {
		t.Fatal("fourth boot within 5 minutes did not trip the guard")
	}
	now = now.Add(10 * time.Minute)
	if d.bootLoopTripped() {
		t.Fatal("a boot long after the loop still tripped the guard")
	}
}
