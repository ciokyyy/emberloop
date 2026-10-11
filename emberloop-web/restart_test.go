package web

import "testing"

// A Daemon with no work is idle; a turn in flight, a starting Session or a
// driving Run each make it busy.
func TestDaemonIdle(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	if !d.Idle() {
		t.Fatal("empty Daemon should be idle")
	}
	d.sessions["live-idle"] = &session{live: true}
	if !d.Idle() {
		t.Fatal("a live Session awaiting a prompt must not block a restart")
	}
	d.sessions["busy"] = &session{live: true, busy: true}
	if d.Idle() {
		t.Fatal("a Session mid-turn must block a restart")
	}
	delete(d.sessions, "busy")
	d.starting = 1
	if d.Idle() {
		t.Fatal("a starting Session must block a restart")
	}
	d.starting = 0
	d.runs["r"] = &runState{Status: runStatusRunning}
	if d.Idle() {
		t.Fatal("a running Run must block a restart")
	}
	d.runs["r"].Status = runStatusPaused
	if !d.Idle() {
		t.Fatal("a paused Run must not block a restart")
	}
}
