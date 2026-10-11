package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// waitIdle polls the Daemon-wide check until it holds at quiet. A turn's busy
// flag clears just after its status Event, so one check can trail the status.
func waitIdle(t *testing.T, d *Daemon, quiet time.Duration) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !d.Idle(quiet) {
		if time.Now().After(deadline) {
			t.Fatalf("Daemon never idle at quiet %v", quiet)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Idle needs a settled turn and quiet time on the Daemon clock. The fake clock
// moves only when the test advances it, so the quiet bound is exact.
func TestIdleFollowsTurnAndFakeClock(t *testing.T) {
	fb := NewFakeBackend()
	fb.Delay = time.Hour // holds the turn open until Interrupt
	d := NewDaemon(t.TempDir(), fb)
	clock := newFakeClock()
	d.clock = clock.get
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	if code, _ := postMode(t, srv.URL, id, "prompt", "hello"); code != http.StatusAccepted {
		t.Fatalf("prompt status = %d, want 202", code)
	}
	waitStatus(t, srv.URL, id, "working", 10*time.Second)
	clock.advance(24 * time.Hour)
	if d.Idle(0) {
		t.Fatal("a running turn must block idle at any quiet time")
	}

	fb.Interrupt(id)
	waitStatus(t, srv.URL, id, "idle", 10*time.Second)
	waitIdle(t, d, 0)
	if d.Idle(time.Minute) {
		t.Fatal("a Session that settled just now must wait out its quiet time")
	}
	clock.advance(time.Minute)
	waitIdle(t, d, time.Minute)
}

// session.Idle takes the Daemon clock and the open-Group flag as arguments, so
// each blocker is tested alone against a fixed time: one settled state per row,
// quiet bound last.
func TestSessionIdleBlockers(t *testing.T) {
	last := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	const quiet = 10 * time.Minute
	later := last.Add(time.Hour)
	cases := []struct {
		name  string
		s     session
		open  bool
		now   time.Time
		quiet time.Duration
		want  bool
	}{
		{"settled, quiet time met", session{last: last}, false, last.Add(quiet), quiet, true},
		{"settled, quiet time short", session{last: last}, false, last.Add(quiet - time.Second), quiet, false},
		{"settled, zero quiet bound", session{last: last}, false, last, 0, true},
		{"running turn", session{last: last, busy: true}, false, later, quiet, false},
		{"launched turn still running", session{last: last, active: 1}, false, later, quiet, false},
		{"queued input", session{last: last, pending: []queuedMsg{{mode: "followup", text: "next"}}}, false, later, quiet, false},
		{"resume in flight", session{last: last, resuming: true}, false, later, quiet, false},
		{"release in flight", session{last: last, releasing: true}, false, later, quiet, false},
		{"parent of an open Group", session{last: last}, true, later, quiet, false},
	}
	for _, c := range cases {
		s := c.s
		if got := s.Idle(c.now, c.quiet, c.open); got != c.want {
			t.Errorf("%s: Idle = %v, want %v", c.name, got, c.want)
		}
	}
}

// A Group holds its parent open while a member runs. Once the members settle,
// the Group closes even if its result never delivered (a restart cut it), so a
// stale Group cannot block idle.
func TestDaemonGroupHoldsParentOpen(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.sessions["parent"] = &session{live: true}
	d.sessions["child"] = &session{live: true, busy: true}
	d.groups["g"] = &spawnGroup{ID: "g", Parent: "parent", Members: []string{"child"}}
	if !d.openParentsLocked()["parent"] {
		t.Fatal("a running member must hold its parent's Group open")
	}
	if d.Idle(0) {
		t.Fatal("a running member must keep the Daemon busy")
	}
	d.sessions["child"].busy = false
	if d.openParentsLocked()["parent"] {
		t.Fatal("settled members must close the Group, delivered or not")
	}
	if !d.Idle(0) {
		t.Fatal("a stale undelivered Group must not block idle")
	}
}

// A waiting Follow-up is work the Daemon still owes, so it holds the Daemon
// busy even when no turn is in flight.
func TestDaemonIdleHoldsQueuedInput(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.sessions["queued"] = &session{live: true, pending: []queuedMsg{{mode: "followup", text: "next"}}}
	if d.Idle(0) {
		t.Fatal("queued input must keep the Daemon busy")
	}
}
