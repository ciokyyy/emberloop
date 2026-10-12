package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// singleIdleTicket is one unblocked ticket, so the run's other activity
// never interferes with the idle clock under test.
func singleIdleTicket() map[string]string {
	return map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	}
}

// bashToolText renders one ToolCall Event text the way the pig Backend does.
func bashToolText(id, status string) string {
	return ToolCall{ID: id, Name: "bash", Input: "go test ./...", Output: "", Status: status}.Text()
}

// waitTicketCond polls one ticket until cond holds, failing with its state
// and banner when the deadline passes.
func waitTicketCond(t *testing.T, base, runID, ticketID string, cond func(actionTicket) bool, what string) actionTicket {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := actionTicketByID(getActionRun(t, base, runID), ticketID)
		if cond(got) {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s: state=%q reason=%q", what, got.State, got.NeedsYou)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// An in-flight bash tool call dates worker activity: eleven idle minutes
// with the call open raise no needs-you, and closing the call re-arms the
// alert on the next pass.
func TestIdleAlertWaitsForInflightToolCall(t *testing.T) {
	// Serial: the fake-clock idle wait times out under parallel git load.
	repo, spec := initRunRepo(t, singleIdleTicket())
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

	d.appendEvent(worker, "tool_call_started", bashToolText("call_1", "running"), "")
	clock.advance(11 * time.Minute)
	// Several driver passes go by with the tool open: the ticket must stay
	// running with no idle banner.
	quietUntil := time.Now().Add(3 * time.Second)
	for time.Now().Before(quietUntil) {
		got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first")
		if got.State == "needs-you" && strings.Contains(got.NeedsYou, "no worker activity for 10 minutes") {
			t.Fatalf("idle needs-you fired with a bash call in flight: state=%q reason=%q", got.State, got.NeedsYou)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first"); got.State != "running" {
		t.Fatalf("ticket state = %q, want running with a bash call in flight", got.State)
	}
	// The completed update closes the call, restarting the idle clock at
	// the close: ten more idle minutes raise the normal alert.
	d.appendEvent(worker, "tool_call_updated", bashToolText("call_1", "completed"), "")
	clock.advance(11 * time.Minute)
	got := waitTicketCond(t, srv.URL, run.ID, "01-first", func(tk actionTicket) bool {
		return tk.State == "needs-you" && strings.Contains(tk.NeedsYou, "no worker activity for 10 minutes")
	}, "idle needs-you after the tool closed")
	_ = got
	pauseTestRun(t, srv.URL, run.ID)
}

// The in-flight grace ends at the cap: a tool call open for over thirty
// minutes no longer counts as activity and the idle alert fires.
func TestInflightToolCapExpires(t *testing.T) {
	// Serial: the fake-clock idle wait times out under parallel git load.
	repo, spec := initRunRepo(t, singleIdleTicket())
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

	d.appendEvent(worker, "tool_call_started", bashToolText("call_1", "running"), "")
	clock.advance(runToolActiveCap + time.Minute)
	got := waitTicketCond(t, srv.URL, run.ID, "01-first", func(tk actionTicket) bool {
		return tk.State == "needs-you" && strings.Contains(tk.NeedsYou, "no worker activity for 10 minutes")
	}, "idle needs-you past the tool cap")
	_ = got
	pauseTestRun(t, srv.URL, run.ID)
}

// The tracker only follows real ToolCall payloads: partial updates keep the
// call open, one close among several calls leaves the ticket active, and
// foreign text never tracks.
func TestSessionToolActiveLocked(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	s := &session{}
	if sessionToolActiveLocked(s, now) {
		t.Fatalf("empty session reads active")
	}
	if sessionToolActiveLocked(nil, now) {
		t.Fatalf("nil session reads active")
	}
	trackSessionToolEvent(s, "tool_call_started", "not json", now)
	if sessionToolActiveLocked(s, now) {
		t.Fatalf("foreign event text tracks a tool call")
	}
	trackSessionToolEvent(s, "assistant_chunk", bashToolText("call_x", "running"), now)
	if sessionToolActiveLocked(s, now) {
		t.Fatalf("non-tool event kind tracks a tool call")
	}
	trackSessionToolEvent(s, "tool_call_started", bashToolText("call_1", "running"), now)
	trackSessionToolEvent(s, "tool_call_started", ToolCall{ID: "call_2", Name: "read", Input: "x", Status: "running"}.Text(), now)
	if !sessionToolActiveLocked(s, now) {
		t.Fatalf("open calls do not read active")
	}
	// A partial update is not a close.
	trackSessionToolEvent(s, "tool_call_updated", ToolCall{ID: "call_1", Name: "bash", Output: "…", Status: "in_progress"}.Text(), now)
	if !sessionToolActiveLocked(s, now) {
		t.Fatalf("in_progress update closed the call")
	}
	trackSessionToolEvent(s, "tool_call_updated", ToolCall{ID: "call_1", Name: "bash", Output: "ok", Status: "completed"}.Text(), now)
	if !sessionToolActiveLocked(s, now) {
		t.Fatalf("second open call did not keep the session active")
	}
	trackSessionToolEvent(s, "tool_call_updated", ToolCall{ID: "call_2", Name: "read", Output: "boom", Status: "failed"}.Text(), now)
	if sessionToolActiveLocked(s, now) {
		t.Fatalf("closed calls still read active")
	}
	// A start older than the cap reads idle even with no close.
	trackSessionToolEvent(s, "tool_call_started", bashToolText("call_3", "running"), now)
	if !sessionToolActiveLocked(s, now) {
		t.Fatalf("fresh call does not read active")
	}
	if sessionToolActiveLocked(s, now.Add(runToolActiveCap+time.Minute)) {
		t.Fatalf("call older than the cap still reads active")
	}
}
