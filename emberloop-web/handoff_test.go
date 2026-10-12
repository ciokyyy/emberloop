package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func handoffDaemon(t *testing.T) (*Daemon, *FakeBackend, string, string) {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	be := NewFakeBackend()
	be.UsageSize = 100000
	be.ReplyFunc = func(_, text string) (string, bool) {
		if strings.HasPrefix(text, "# Handoff") {
			return "ok", true
		}
		return strings.Repeat("x", 8000), true
	}
	d := NewDaemon(tempDir(t), be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	return d, be, srv.URL, id
}

func eventsOfKind(evs []Event, kind string) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func waitIdleAfter(t *testing.T, base, id string, n int) []Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs := transcriptOf(t, base, id)
		if len(eventsOfKind(evs, "compressed")) >= n && len(evs) > 0 && evs[len(evs)-1].Kind == "status" && evs[len(evs)-1].Status == "idle" {
			return evs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("handoff %d did not settle: %+v", n, transcriptOf(t, base, id))
	return nil
}

func TestHandoffSwapKeepsSessionAndRotatesToken(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	for _, p := range []string{"build the thing", "and more", "and again"} {
		sendPrompt(t, base, id, p)
		waitStatus(t, base, id, "working", 5*time.Second)
		waitStatus(t, base, id, "idle", 5*time.Second)
	}
	d.mu.Lock()
	metaBefore := d.sessions[id].meta
	d.mu.Unlock()

	if code, body := compressReq(t, base, id, ""); code != 202 {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	// Prior Events stay, the Handoff prompt follows them.
	hand := eventsOfKind(evs, "prompt")
	if len(hand) != 4 || hand[0].Text != "build the thing" {
		t.Fatalf("history lost: %+v", hand)
	}
	last := hand[len(hand)-1]
	if !last.Handoff || last.Sender != "system" || last.User != "" || last.Mode != "" {
		t.Fatalf("handoff prompt flags = %+v", last)
	}
	if !strings.Contains(last.Text, "## Next steps\nrun the tests") || !strings.Contains(last.Text, "\n## Last user prompt (verbatim)\nand again\n") {
		t.Fatalf("handoff text = %q", last.Text)
	}
	if hand[0].Handoff || hand[0].Sender != "user" || hand[0].User != metaBefore.Owner {
		t.Fatalf("user prompt flagged or not owned by sender: %+v", hand[0])
	}

	// compressed Event: number, tokens before, model, fallback step, retries.
	c := eventsOfKind(evs, "compressed")[0]
	if !strings.Contains(c.Text, `"number":1`) || !strings.Contains(c.Text, `"model":"test-faux/faux-1"`) ||
		!strings.Contains(c.Text, `"fallback":"summary"`) || !strings.Contains(c.Text, `"tokensBefore":`) ||
		strings.Contains(c.Text, `"tokensBefore":0`) {
		t.Fatalf("compressed text = %s", c.Text)
	}

	// Token rotated; Session ID and Owner unchanged.
	d.mu.Lock()
	metaAfter := d.sessions[id].meta
	d.mu.Unlock()
	if metaBefore.Owner == "" || metaAfter.Owner != metaBefore.Owner {
		t.Fatalf("owner across swap: %q -> %q", metaBefore.Owner, metaAfter.Owner)
	}
	if metaAfter.ResumeToken == metaBefore.ResumeToken || metaAfter.ResumeToken == "" {
		t.Fatalf("token %q -> %q, want rotated", metaBefore.ResumeToken, metaAfter.ResumeToken)
	}
	metaAfter.ResumeToken, metaAfter.Status, metaBefore.ResumeToken, metaBefore.Status = "", "", "", ""
	if metaAfter != metaBefore {
		t.Fatalf("meta changed across swap: %+v vs %+v", metaBefore, metaAfter)
	}

	// Copy stored in the Session directory.
	raw, err := os.ReadFile(filepath.Join(d.sessionDir(id), "handoff-1.md"))
	if err != nil || string(raw) != last.Text {
		t.Fatalf("handoff copy err=%v match=%v", err, string(raw) == last.Text)
	}

	// Old handle released after the new turn started.
	if n := be.ReleasedHandles(id); n != 1 {
		t.Fatalf("released = %d, want 1", n)
	}

	// Context meter resets from the next usage Event.
	us := eventsOfKind(evs, "usage")
	if len(us) < 2 {
		t.Fatalf("usage events = %d", len(us))
	}
	if b, a := lastUsedTokens(evs[:indexOfSeq(evs, c.Seq)]), lastUsedTokens(evs); a >= b {
		t.Fatalf("meter before=%d after=%d, want a drop", b, a)
	}

	// Second compress numbers 2.
	compressReq(t, base, id, "")
	evs = waitIdleAfter(t, base, id, 2)
	if !strings.Contains(eventsOfKind(evs, "compressed")[1].Text, `"number":2`) {
		t.Fatalf("second compressed = %s", eventsOfKind(evs, "compressed")[1].Text)
	}
	// A Handoff prompt never counts as a user prompt in the next Handoff.
	h2 := eventsOfKind(evs, "prompt")
	if strings.Count(h2[len(h2)-1].Text, "# Handoff") != 1 {
		t.Fatalf("second handoff repeats earlier handoff: %q", h2[len(h2)-1].Text)
	}
}

func indexOfSeq(evs []Event, seq int64) int {
	for i, e := range evs {
		if e.Seq == seq {
			return i
		}
	}
	return len(evs)
}

func TestHandoffFailureKeepsOldHandle(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	be.StartErr = os.ErrPermission
	compressReq(t, base, id, "")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(eventsOfKind(transcriptOf(t, base, id), "error")) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	evs := transcriptOf(t, base, id)
	if len(eventsOfKind(evs, "error")) == 0 || len(eventsOfKind(evs, "compressed")) != 0 {
		t.Fatalf("events = %+v", evs)
	}
	if n := be.ReleasedHandles(id); n != 0 {
		t.Fatalf("released = %d, want 0", n)
	}
	// The Session settles: busy and active clear after the failed swap.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		sess := d.sessions[id]
		settled := !sess.busy && sess.active == 0
		d.mu.Unlock()
		if settled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("busy/active not cleared after StartHandle error")
}

func TestHandoffTurnErrorKeepsOldHandle(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	be.HandoffTurnErr = "boom"
	compressReq(t, base, id, "")
	waitEventKind(t, base, id, "error")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		sess := d.sessions[id]
		settled := !sess.busy && sess.active == 0
		d.mu.Unlock()
		if settled {
			if n := be.ReleasedHandles(id); n != 0 {
				t.Fatalf("released = %d, want 0 after a turn error", n)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("busy/active not cleared after the turn error")
}

// A Run Worker's task is a system prompt: the Run driver sends it. The
// deterministic Handoff still names it as the Task, so a compressed Worker
// keeps the work it was asked to do.
func TestHandoffOfRunWorkerKeepsTicketAsTask(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settingsPath)
	sum := &scriptedSummarizer{err: errString("bad request: malformed input")}
	d.summarizer = sum
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	srv := httptest.NewServer(withCookie(h, admin))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	worker := waitChildren(t, srv.URL, run.ID, 1)[0]
	task := firstPromptWhere(t, srv.URL, worker.ID, func(Event) bool { return true })
	wantSender(t, task, "system", "")
	waitIdleAfter(t, srv.URL, worker.ID, 0)

	if code, body := compressReq(t, srv.URL, worker.ID, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, srv.URL, worker.ID, 1)
	prompts := eventsOfKind(evs, "prompt")
	hand := prompts[len(prompts)-1]
	if !hand.Handoff || !strings.Contains(hand.Text, "## Task\n"+strings.TrimSpace(task.Text)) {
		t.Fatalf("handoff does not name the ticket as its Task: %q", hand.Text)
	}
	if len(sum.calls) == 0 || !strings.Contains(sum.calls[0].user, strings.TrimSpace(task.Text)) {
		t.Fatalf("summarizer input does not carry the ticket: %+v", sum.calls)
	}
}

// A Subagent's spawn prompt is a system prompt too; its Handoff keeps it as
// the Task for the same reason.
func TestHandoffOfSubagentKeepsSpawnPromptAsTask(t *testing.T) {
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settingsPath)
	sum := &scriptedSummarizer{err: errString("bad request: malformed input")}
	d.summarizer = sum
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	srv := httptest.NewServer(withCookie(h, admin))
	defer srv.Close()
	agent := httptest.NewServer(withBearer(h, d.MachineToken()))
	defer agent.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, agent.URL, parent, "write the subagent job")
	task := firstPromptWhere(t, srv.URL, child, func(e Event) bool { return e.Text == "write the subagent job" })
	wantSender(t, task, "system", "")
	waitIdleAfter(t, srv.URL, child, 0)

	if code, body := compressReq(t, srv.URL, child, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, srv.URL, child, 1)
	prompts := eventsOfKind(evs, "prompt")
	hand := prompts[len(prompts)-1]
	if !hand.Handoff || !strings.Contains(hand.Text, "## Task\nwrite the subagent job") {
		t.Fatalf("handoff does not name the spawn prompt as its Task: %q", hand.Text)
	}
	if len(sum.calls) == 0 || !strings.Contains(sum.calls[0].user, "write the subagent job") {
		t.Fatalf("summarizer input does not carry the spawn prompt: %+v", sum.calls)
	}
}
