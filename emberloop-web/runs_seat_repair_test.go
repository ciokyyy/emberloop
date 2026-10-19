package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A tip-check claim with no red cached result is a check a restart cut: it
// must not stay "told", or every pass skips the tip forever.
func TestDropLostTipClaims(t *testing.T) {
	t.Parallel()
	rs := &runState{
		TipRedTold: map[string]string{
			"lost":   "aaa",
			"red":    "bbb",
			"green":  "ccc",
			"infra":  "ddd",
			"flaked": "eee",
		},
		TipCheckCache: map[string]baseCheckResult{
			"bbb": {OK: false, Failing: []string{"TestX"}},
			"ccc": {OK: true},
			"ddd": {OK: false, Failing: []string{"tip check could not run: boom"}},
			"eee": {OK: false, Failing: []string{checkFlakedPrefix + " TestY"}},
		},
	}
	dropLostTipClaims(rs)
	if len(rs.TipRedTold) != 1 || rs.TipRedTold["red"] != "bbb" {
		t.Fatalf("TipRedTold = %v, want only the red-backed claim", rs.TipRedTold)
	}
}

func TestSeatUnreachable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("x"), false},
		{"live refusal", &tellError{msg: "could not reach session"}, false},
		{"resuming conflict", &tellError{code: http.StatusConflict, msg: "could not reach session: session is resuming; send the message again"}, false},
		{"resume failed", &tellError{code: http.StatusInternalServerError, msg: "could not reach session: boom"}, true},
		{"no saved agent", &tellError{code: http.StatusConflict, msg: "could not reach session: claude Session has no saved agent to load"}, true},
		{"gone", &tellError{code: http.StatusGone, msg: "could not reach session: session ended"}, true},
	}
	for _, tc := range cases {
		if got := seatUnreachable(tc.err); got != tc.want {
			t.Errorf("%s: seatUnreachable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A Reviewer seat that stays unreachable for seatTellFailLimit repair passes
// is dropped so the driver starts a fresh one; a reachable pass resets the
// count.
func TestNoteSeatTellReplacesUnreachableSeat(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	const runID, ticket, typ, sess = "r1", "t1", reviewerStandardsType, "s1"
	d.sessions[sess] = &session{meta: SessionMeta{ID: sess, Status: "idle"}, notify: make(chan struct{}), last: time.Now()}
	d.runs[runID] = &runState{
		ID: runID, Status: runStatusRunning, notify: make(chan struct{}),
		Reviewers:      map[string]map[string]string{ticket: {typ: sess}},
		ReviewTips:     map[string]map[string]string{ticket: {typ: "tip"}},
		ReviewPending:  map[string]map[string][]string{ticket: {typ: {"tip"}}},
		ReviewSeen:     map[string]map[string]int64{ticket: {typ: 3}},
		FormatReprompt: map[string]map[string]bool{ticket: {typ: true}},
		ReviewStarted:  map[string]map[string]time.Time{ticket: {typ: time.Now()}},
	}
	dead := &tellError{code: http.StatusInternalServerError, msg: "could not reach session: boom"}
	for i := 0; i < seatTellFailLimit-1; i++ {
		d.noteSeatTell(runID, ticket, typ, sess, dead)
	}
	d.noteSeatTell(runID, ticket, typ, sess, nil)
	for i := 0; i < seatTellFailLimit-1; i++ {
		d.noteSeatTell(runID, ticket, typ, sess, dead)
	}
	d.mu.Lock()
	kept := d.runs[runID].Reviewers[ticket][typ]
	d.mu.Unlock()
	if kept != sess {
		t.Fatalf("seat dropped after %d failures with a reset between", seatTellFailLimit-1)
	}

	d.noteSeatTell(runID, ticket, typ, sess, dead)
	d.mu.Lock()
	rs := d.runs[runID]
	seat := rs.Reviewers[ticket][typ]
	tips, pending, seen, format := len(rs.ReviewTips[ticket]), len(rs.ReviewPending[ticket]), len(rs.ReviewSeen[ticket]), len(rs.FormatReprompt[ticket])
	status := d.sessions[sess].meta.Status
	d.mu.Unlock()
	if seat != "" || tips+pending+seen+format != 0 {
		t.Fatalf("seat state not cleared: seat=%q tips=%d pending=%d seen=%d format=%d", seat, tips, pending, seen, format)
	}
	if status != "ended" {
		t.Fatalf("old seat status = %q, want ended", status)
	}
}

// A hard kill leaves meta.json at working with no process. A Session outside
// a resuming Run settles idle on the next boot, logged as a restart cut; a
// resuming Run's Session is left for its own restart path.
func TestHealStrandedSessions(t *testing.T) {
	t.Parallel()
	stateDir := tempDir(t)
	write := func(id, parent string) {
		dir := filepath.Join(stateDir, "enderbloom", "sessions", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta, _ := json.Marshal(SessionMeta{ID: id, Harness: "fake", Status: "working", WorkDir: stateDir, ParentID: parent, CreatedAt: time.Now().UTC()})
		ev, _ := json.Marshal(Event{Version: 1, Seq: 1, SessionID: id, Kind: "status", Status: "working", CreatedAt: time.Now().UTC()})
		if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), append(ev, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stranded", "")
	write("worker", "run1")
	runs := filepath.Join(stateDir, "enderbloom", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	run, _ := json.Marshal(map[string]any{"id": "run1", "status": "running", "sessions": map[string]string{"t1": "worker"}})
	if err := os.WriteFile(filepath.Join(runs, "run1.json"), run, 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDaemon(stateDir, NewFakeBackend())
	d.mu.Lock()
	stranded, worker := d.sessions["stranded"], d.sessions["worker"]
	d.mu.Unlock()
	if stranded == nil || worker == nil {
		t.Fatal("sessions not loaded")
	}
	d.mu.Lock()
	sStatus, wStatus := stranded.meta.Status, worker.meta.Status
	last := stranded.events[len(stranded.events)-1]
	d.mu.Unlock()
	if sStatus != "idle" || last.Text != restartInterruptedText || last.Status != "idle" {
		t.Fatalf("stranded = %q, last event %+v; want idle with the restart marker", sStatus, last)
	}
	if wStatus != "working" {
		t.Fatalf("resuming Run's worker = %q, want working (its restart path settles it)", wStatus)
	}
	d.mu.Lock()
	again := interruptedByRestartLocked(stranded)
	d.mu.Unlock()
	if !again {
		t.Error("a healed Session no longer reads as cut by a restart")
	}
}
