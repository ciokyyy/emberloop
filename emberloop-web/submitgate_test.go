package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	el "example.com/emberloop"
)

// withSubmitGate runs one gate test with the submit tool and gate on. The
// flag defaults off (settle delivery needs no gate), so gate tests opt in
// and restore it afterwards. No test in this package runs parallel.
func withSubmitGate(t *testing.T) {
	t.Helper()
	el.WebSubmitEnabled = true
	t.Cleanup(func() { el.WebSubmitEnabled = false })
}

// submitScriptBackend replays the fake turn and, on the configured 1-based
// OnPrompt calls, also logs a submit the way the submit tool would: the gate
// reads submit Events, so this drives the reminded-then-submits path without
// HTTP.
type submitScriptBackend struct {
	*FakeBackend
	mu       sync.Mutex
	calls    int
	submitOn map[int]bool
}

func (b *submitScriptBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.mu.Lock()
	b.calls++
	submit := b.submitOn[b.calls]
	b.mu.Unlock()
	b.FakeBackend.OnPrompt(sessionID, text, appendEvent)
	if submit {
		appendEvent("submit", "done", "")
	}
}

func spawnOneWithGroup(t *testing.T, base, parent, prompt string) (group, child string) {
	t.Helper()
	code, out := postSpawn(t, base, parent, map[string]any{
		"agents": []map[string]any{{"prompt": prompt}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	group, _ = out["group"].(string)
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 1 {
		t.Fatalf("spawn sessions = %v, want 1", out["sessions"])
	}
	return group, kids[0].ID
}

func countReminderPrompts(events []Event) int {
	n := 0
	for _, e := range events {
		if e.Kind == "prompt" && strings.Contains(e.Text, "without calling submit") {
			n++
		}
	}
	return n
}

func waitGroupSteer(t *testing.T, base, parent, group string, timeout time.Duration) Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, events := sessionMeta(t, base, parent)
		for _, e := range events {
			if e.Kind == "prompt" && e.Mode == "steer" && strings.Contains(e.Text, group) {
				return e
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, events := sessionMeta(t, base, parent)
	t.Fatalf("grouped steer for %s never delivered in %+v", group, events)
	return Event{}
}

// A child that submits mid-turn is untouched: no reminder follows it, and the
// Group still delivers exactly once.
func TestGateChildThatSubmitsIsUntouched(t *testing.T) {
	withSubmitGate(t)
	fake := NewFakeBackend()
	fake.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	// Submit while the turn runs, the way the submit tool would; the working
	// status proves the gate's baseline is taken and the submit lands after it.
	waitStatus(t, srv.URL, child, "working", 5*time.Second)
	submitSession(t, srv.URL, child, "done", []string{"alpha"})
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	waitGroupSteer(t, srv.URL, parent, group, 10*time.Second)
	time.Sleep(300 * time.Millisecond)

	_, events := sessionMeta(t, srv.URL, child)
	if n := countReminderPrompts(events); n != 0 {
		t.Fatalf("reminder prompts = %d, want 0 (events %+v)", n, events)
	}
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// A child that settles without submitting is reminded once, submits on the
// reminder turn, and the Group still delivers exactly once.
func TestGateRemindsOnceThenChildSubmits(t *testing.T) {
	withSubmitGate(t)
	be := &submitScriptBackend{FakeBackend: NewFakeBackend(), submitOn: map[int]bool{2: true}}
	d := NewDaemon(tempDir(t), be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	waitGroupSteer(t, srv.URL, parent, group, 10*time.Second)
	time.Sleep(300 * time.Millisecond)

	_, events := sessionMeta(t, srv.URL, child)
	if n := countReminderPrompts(events); n != 1 {
		t.Fatalf("reminder prompts = %d, want exactly 1 (events %+v)", n, events)
	}
	if n := countKind(events, "submit"); n != 1 {
		t.Fatalf("submit events = %d, want 1", n)
	}
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// A child that never submits is reminded to the gate's bound, then gives up:
// the give-up is recorded as a stopped submit and the parent still gets one
// grouped notice naming it.
func TestGateGiveUpDeliversStoppedNoticeOnce(t *testing.T) {
	withSubmitGate(t)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	steer := waitGroupSteer(t, srv.URL, parent, group, 15*time.Second)
	if !strings.Contains(steer.Text, "stopped without submitting") {
		t.Fatalf("grouped steer missing stopped notice: %q", steer.Text)
	}
	time.Sleep(300 * time.Millisecond)

	_, events := sessionMeta(t, srv.URL, child)
	if n := countReminderPrompts(events); n != 3 {
		t.Fatalf("reminder prompts = %d, want 3 (events %+v)", n, events)
	}
	var stopped bool
	for _, e := range events {
		if e.Kind == "submit" && e.Text == "stopped" {
			stopped = true
		}
	}
	if !stopped {
		t.Fatalf("no stopped submit event in %+v", events)
	}
	d.mu.Lock()
	rec := d.submits[child]
	d.mu.Unlock()
	if rec == nil || rec.Status != "stopped" || !strings.Contains(rec.Report, "stopped without submitting") {
		t.Fatalf("stopped submit rec = %+v, want stopped with notice", rec)
	}
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// A killed child gets no reminder: ending is terminal, not a missing submit.
func TestGateKilledChildGetsNoReminder(t *testing.T) {
	withSubmitGate(t)
	fake := NewFakeBackend()
	fake.Delay = 500 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	_, child := spawnOneWithGroup(t, srv.URL, parent, "doomed job")
	waitStatus(t, srv.URL, child, "working", 5*time.Second)
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+child, nil)
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("delete status = %d", resp.StatusCode)
		}
	}
	time.Sleep(1200 * time.Millisecond)
	_, events := sessionMeta(t, srv.URL, child)
	if n := countReminderPrompts(events); n != 0 {
		t.Fatalf("reminder prompts on killed child = %d, want 0", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// An interrupted child gets no reminder loop: a user stop is deliberate.
func TestGateInterruptedChildGetsNoReminder(t *testing.T) {
	withSubmitGate(t)
	fake := NewFakeBackend()
	fake.Delay = 5 * time.Second
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	_, child := spawnOneWithGroup(t, srv.URL, parent, "slow job")
	waitStatus(t, srv.URL, child, "working", 5*time.Second)
	resp, err := http.Post(srv.URL+"/api/sessions/"+child+"/interrupt", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	time.Sleep(500 * time.Millisecond)
	_, events := sessionMeta(t, srv.URL, child)
	if n := countReminderPrompts(events); n != 0 {
		t.Fatalf("reminder prompts on interrupted child = %d, want 0", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// A child that never submits still delivers once it settles: with the gate
// off there are no reminders, no submit event, and the grouped result
// carries the child's whole final reply.
func TestSettleDeliversLastMessageWithoutSubmit(t *testing.T) {
	if el.WebSubmitEnabled {
		t.Fatal("WebSubmitEnabled must default false")
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	steer := waitGroupSteer(t, srv.URL, parent, group, 10*time.Second)
	if !strings.Contains(steer.Text, "fake reply to: job A") {
		t.Fatalf("grouped steer missing last message: %q", steer.Text)
	}
	time.Sleep(300 * time.Millisecond)

	_, events := sessionMeta(t, srv.URL, child)
	if n := countReminderPrompts(events); n != 0 {
		t.Fatalf("reminder prompts = %d, want 0 (events %+v)", n, events)
	}
	if n := countKind(events, "submit"); n != 0 {
		t.Fatalf("submit events = %d, want 0", n)
	}
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// A submit recorded before the child settles is the Group result: the
// submit text wins over the transcript reply.
func TestSettleDeliversSubmitTextWhenSubmitted(t *testing.T) {
	if el.WebSubmitEnabled {
		t.Fatal("WebSubmitEnabled must default false")
	}
	fake := NewFakeBackend()
	fake.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	waitStatus(t, srv.URL, child, "working", 5*time.Second)
	body, _ := json.Marshal(map[string]any{"status": "done", "completed": []string{"alpha"}, "report": "settled report"})
	resp, err := http.Post(srv.URL+"/api/sessions/"+child+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("submit status = %d, want 202", resp.StatusCode)
	}
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	steer := waitGroupSteer(t, srv.URL, parent, group, 10*time.Second)
	if !strings.Contains(steer.Text, "settled report") {
		t.Fatalf("grouped steer missing submit text: %q", steer.Text)
	}
	time.Sleep(300 * time.Millisecond)
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// failReplyBackend answers one turn with a reply, then settles failed.
type failReplyBackend struct {
	*FakeBackend
}

func (b *failReplyBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	appendEvent("status", "", "working")
	appendEvent("assistant_chunk", "failing reply to: "+text, "")
	appendEvent("status", "", "failed")
}

// A failed child delivers its Group result once, carrying its status.
func TestSettleFailedChildDeliversStatus(t *testing.T) {
	if el.WebSubmitEnabled {
		t.Fatal("WebSubmitEnabled must default false")
	}
	d := NewDaemon(tempDir(t), &failReplyBackend{FakeBackend: NewFakeBackend()})
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	waitStatus(t, srv.URL, child, "failed", 10*time.Second)
	steer := waitGroupSteer(t, srv.URL, parent, group, 10*time.Second)
	if !strings.Contains(steer.Text, "failing reply to: job A") {
		t.Fatalf("grouped steer missing last message: %q", steer.Text)
	}
	if !strings.Contains(steer.Text, "[failed]") {
		t.Fatalf("grouped steer missing failed status: %q", steer.Text)
	}
	time.Sleep(300 * time.Millisecond)
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	// The grouped delivery steers the parent, whose turn fails like every
	// turn under this backend: settled here means failed, not idle.
	waitStatus(t, srv.URL, parent, "failed", 20*time.Second)
}
