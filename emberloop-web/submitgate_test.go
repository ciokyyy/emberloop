package web

import (
	"bytes"
	"encoding/json"
	"errors"
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
	time.Sleep(150 * time.Millisecond)

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
	time.Sleep(150 * time.Millisecond)

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
	time.Sleep(150 * time.Millisecond)

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
	time.Sleep(800 * time.Millisecond)
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
	// Only the child's turn stays open: the parent's delivery turn settles
	// fast, so waitParentIdle never waits out the hold.
	fake.DelayFunc = func(text string) time.Duration {
		if strings.Contains(text, "slow job") {
			return 5 * time.Second
		}
		return 0
	}
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
	time.Sleep(300 * time.Millisecond)
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
	time.Sleep(150 * time.Millisecond)

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
	time.Sleep(150 * time.Millisecond)
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
	time.Sleep(150 * time.Millisecond)
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	// The grouped delivery steers the parent, whose turn fails like every
	// turn under this backend: settled here means failed, not idle.
	waitStatus(t, srv.URL, parent, "failed", 20*time.Second)
}

// A dead gated turn records a failure instead of burning reminders: with no
// usage on the meter there is nothing to compress, so the turn ends failed
// with a dead-turn error, the Chain classifier leaves it (no advance), and
// the Group still delivers exactly once without a stopped notice.
func TestDeadTurnSkipsRemindersAndFails(t *testing.T) {
	withSubmitGate(t)
	be := NewFakeBackend()
	be.ReplyFunc = func(_, _ string) (string, bool) { return "", true }
	d := NewDaemon(tempDir(t), be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	group, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	meta, events := waitDeadFailed(t, srv.URL, child)
	if meta.Status != "failed" {
		t.Fatalf("child status = %q, want failed", meta.Status)
	}
	if n := countReminderPrompts(events); n != 0 {
		t.Fatalf("reminder prompts = %d, want 0", n)
	}
	var dead bool
	for _, e := range events {
		if e.Kind == "error" && strings.Contains(e.Text, "dead turn") {
			dead = true
		}
	}
	if !dead {
		t.Fatalf("no dead-turn error in %+v", events)
	}
	steer := waitGroupSteer(t, srv.URL, parent, group, 10*time.Second)
	if strings.Contains(steer.Text, "stopped without submitting") {
		t.Fatalf("grouped steer carries a give-up notice: %q", steer.Text)
	}
	_, pevents := sessionMeta(t, srv.URL, parent)
	if n := countGroupSteers(pevents, group); n != 1 {
		t.Fatalf("grouped steers = %d, want exactly 1", n)
	}
	waitParentIdle(t, srv.URL, parent)
}

// waitDeadFailed polls a Session until its status flips to failed, then
// returns its meta and transcript.
func waitDeadFailed(t *testing.T, base, id string) (SessionMeta, []Event) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		meta, events := sessionMeta(t, base, id)
		if meta.Status == "failed" {
			return meta, events
		}
		time.Sleep(20 * time.Millisecond)
	}
	meta, events := sessionMeta(t, base, id)
	t.Fatalf("session %s never failed: %+v", id, events)
	return meta, events
}

// A dead gated turn with usage past the threshold compresses instead of
// reminding: the Handoff runs with no reminder prompt before it.
func TestDeadTurnNearWindowCompressesBeforeGate(t *testing.T) {
	withSubmitGate(t)
	t.Setenv("PIG_TEST_FAUX", "1")
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":10,"compress_cooldown":0}`)
	be := NewFakeBackend()
	be.UsageSize = 10
	be.ReplyFunc = func(_, _ string) (string, bool) { return "", true }
	d := NewDaemon(tempDir(t), be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	_, child := spawnOneWithGroup(t, srv.URL, parent, "job A")
	evs := waitIdleAfter(t, srv.URL, child, 1)
	if n := len(eventsOfKind(evs, "compressed")); n != 1 {
		t.Fatalf("compressed = %d, want 1", n)
	}
	if n := countReminderPrompts(evs); n != 0 {
		t.Fatalf("reminder prompts = %d, want 0", n)
	}
	for _, e := range evs {
		if e.Kind == "error" && strings.Contains(e.Text, "dead turn") {
			t.Fatalf("dead-turn failure recorded despite due compress")
		}
	}
	waitParentIdle(t, srv.URL, parent)
}

// deadEvents builds one turn after mark from kinds, stamping CreatedAt so
// the duration check is deterministic. The turn's prompt sits at mark (as
// senders append it before the turn launches); kinds run after it. "working"
// and "status" stamp those statuses, "status" settling idle.
func deadEvents(mark int64, start time.Time, kinds ...string) []Event {
	evs := []Event{{Seq: mark, Kind: "prompt", Text: "do the job", CreatedAt: start.Add(-time.Second)}}
	seq := mark
	at := start
	for _, k := range kinds {
		seq++
		kind, text, status := k, "", ""
		switch k {
		case "working":
			kind, status = "status", "working"
		case "assistant_chunk":
			// Blank by design: the "blank chunks stay dead" case needs
			// chunks with no text, and the text case appends its own.
			text = ""
		case "status":
			status = "idle"
		case "error":
			text = "boom"
		}
		evs = append(evs, Event{Seq: seq, Kind: kind, Text: text, Status: status, CreatedAt: at})
		at = at.Add(time.Second)
	}
	return evs
}

// The dead-turn predicate fires only on a fast working-to-idle settle with
// no output: text, a tool call, a submit, an error, a slow settle or a
// non-idle end all clear it, and no working edge after the mark is no turn.
func TestDeadTurnSince(t *testing.T) {
	start := time.Now()
	slow := deadEvents(7, start, "working", "status")
	slow[len(slow)-1].CreatedAt = start.Add(deadTurnMaxDuration + time.Second)
	failed := deadEvents(7, start, "working", "status")
	failed[len(failed)-1].Status = "failed"
	for _, tc := range []struct {
		name   string
		events []Event
		want   bool
	}{
		{"fast empty turn is dead", deadEvents(7, start, "working", "status"), true},
		{"blank chunks stay dead", deadEvents(7, start, "working", "assistant_chunk", "status"), true},
		{"assistant text clears it", append(deadEvents(7, start, "working"), Event{Seq: 9, Kind: "assistant_chunk", Text: "working on it", CreatedAt: start.Add(2 * time.Second)}, Event{Seq: 10, Kind: "status", Status: "idle", CreatedAt: start.Add(3 * time.Second)}), false},
		{"tool call clears it", append(deadEvents(7, start, "working"), Event{Seq: 9, Kind: "tool_call_started", Text: `{"id":"t1"}`, CreatedAt: start.Add(2 * time.Second)}, Event{Seq: 10, Kind: "status", Status: "idle", CreatedAt: start.Add(3 * time.Second)}), false},
		{"submit clears it", append(deadEvents(7, start, "working"), Event{Seq: 9, Kind: "submit", Text: "done", CreatedAt: start.Add(2 * time.Second)}, Event{Seq: 10, Kind: "status", Status: "idle", CreatedAt: start.Add(3 * time.Second)}), false},
		{"error clears it", deadEvents(7, start, "working", "error", "status"), false},
		{"slow settle clears it", slow, false},
		{"failed end clears it", failed, false},
		{"no working edge is no turn", deadEvents(7, start, "status"), false},
		{"events before the mark do not count", deadEvents(9, start, "status"), false},
	} {
		if got := deadTurnSince(tc.events, 7); got != tc.want {
			t.Errorf("%s: deadTurnSince = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The synthetic dead-turn failure must not advance the Run Chain and must
// not read as a capped Session: it is evidence of an empty turn, not of a
// quota, credit, auth or session-limit failure.
func TestDeadTurnErrorStaysOutOfChain(t *testing.T) {
	for _, text := range []string{
		deadTurnErrorText(296883, 272000, true),
		deadTurnErrorText(0, 0, false),
	} {
		if got := classifyFailure(errors.New(text)); got != failureOther {
			t.Errorf("classifyFailure(%q) = %q, want other", text, got)
		}
		if chainAdvances(errors.New(text)) {
			t.Errorf("chainAdvances(%q) = true, want false", text)
		}
	}
	now := time.Now()
	evs := []Event{
		{Seq: 1, Kind: "prompt", Text: "job", CreatedAt: now},
		{Seq: 2, Kind: "error", Text: deadTurnErrorText(0, 0, false), CreatedAt: now.Add(time.Second)},
		{Seq: 3, Kind: "status", Status: "failed", CreatedAt: now.Add(2 * time.Second)},
	}
	if sessionLimitFailureInTurn(evs) {
		t.Error("dead-turn error reads as a session-limit failure, want false")
	}
	if got := lastTurnFailure(evs); !strings.Contains(got, "dead turn") {
		t.Errorf("lastTurnFailure = %q, want the dead-turn text", got)
	}
}
