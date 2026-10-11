package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func postMode(t *testing.T, base, id, mode, text string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	resp, err := http.Post(base+"/api/sessions/"+id+"/"+mode, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getQueue(t *testing.T, base, id string) []QueuedMessage {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id + "/queue")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("queue status = %d", resp.StatusCode)
	}
	var out struct {
		Queue []QueuedMessage `json:"queue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Queue == nil {
		return []QueuedMessage{}
	}
	return out.Queue
}

func clearQueue(t *testing.T, base, id string) int {
	t.Helper()
	req, _ := http.NewRequest("DELETE", base+"/api/sessions/"+id+"/queue", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear status = %d", resp.StatusCode)
	}
	var out struct {
		Cleared int `json:"cleared"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Cleared
}

func waitStatus(t *testing.T, base, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		meta, _ := sessionMeta(t, base, id)
		if meta.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	meta, _ := sessionMeta(t, base, id)
	t.Fatalf("status = %q, want %q", meta.Status, want)
}

// waitParentIdle waits out a spawn parent's grouped delivery before the
// test's TempDir is removed: the last child to settle steers the parent, so
// the parent works briefly after its children go idle, and its writes race
// TempDir removal when the test returns first.
func waitParentIdle(t *testing.T, base, parent string) {
	t.Helper()
	waitStatus(t, base, parent, "idle", 20*time.Second)
}

// Slice 1: a Steer on an idle fake Session runs a turn labelled steer.
func TestSteerOnIdleRunsLabeledTurn(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	code, body := postMode(t, srv.URL, id, "steer", "go left")
	if code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	if body["queued"] == true {
		t.Fatalf("idle steer queued = true, want immediate run")
	}

	events := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	var found *Event
	for i := range events {
		if events[i].Kind == "prompt" && strings.Contains(events[i].Text, "go left") {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("steer prompt missing in %+v", events)
	}
	if found.Mode != "steer" {
		t.Fatalf("steer prompt mode = %q, want steer", found.Mode)
	}
}

// Slice 1: a Follow-up on an idle fake Session runs a turn labelled followup.
func TestFollowupOnIdleRunsLabeledTurn(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	code, _ := postMode(t, srv.URL, id, "followup", "and then")
	if code != http.StatusAccepted {
		t.Fatalf("followup status = %d, want 202", code)
	}

	events := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	var found *Event
	for i := range events {
		if events[i].Kind == "prompt" && strings.Contains(events[i].Text, "and then") {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("followup prompt missing in %+v", events)
	}
	if found.Mode != "followup" {
		t.Fatalf("followup prompt mode = %q, want followup", found.Mode)
	}
}

// Slice 2: while a fake Session works, steer and follow-up queue, list, and
// clear before delivery.
func TestQueuedModesListAndClearBeforeDelivery(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = 400 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)

	if code, _ := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	if code, _ := postMode(t, srv.URL, id, "followup", "and then"); code != http.StatusAccepted {
		t.Fatalf("followup status = %d, want 202", code)
	}

	q := getQueue(t, srv.URL, id)
	if len(q) != 2 || q[0].Mode != "steer" || q[0].Text != "go left" || q[1].Mode != "followup" || q[1].Text != "and then" {
		t.Fatalf("queue = %+v, want [steer go left followup and then]", q)
	}

	if n := clearQueue(t, srv.URL, id); n != 2 {
		t.Fatalf("cleared = %d, want 2", n)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 0 {
		t.Fatalf("queue after clear = %+v, want empty", q)
	}

	// The cleared items never deliver: only the first turn's reply streams.
	events := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	for _, e := range events {
		if e.Kind == "prompt" && (strings.Contains(e.Text, "go left") || strings.Contains(e.Text, "and then")) {
			t.Fatalf("cleared message delivered: %+v", e)
		}
	}
}

// Slice 3: a queued Follow-up runs after the current fake turn.
func TestQueuedFollowupRunsAfterTurn(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)

	if code, _ := postMode(t, srv.URL, id, "followup", "after that"); code != http.StatusAccepted {
		t.Fatalf("followup status = %d, want 202", code)
	}

	// First turn (5 events) plus the queued turn (prompt, working,
	// reply, idle): 9 events.
	events := readSSE(t, srv.URL, id, 0, 9, 10*time.Second)
	var follow *Event
	for i := range events {
		if events[i].Kind == "prompt" && events[i].Text == "after that" {
			follow = &events[i]
		}
	}
	if follow == nil {
		t.Fatalf("queued followup never delivered in %+v", events)
	}
	if follow.Mode != "followup" {
		t.Fatalf("queued followup mode = %q, want followup", follow.Mode)
	}
	found := false
	for _, e := range events {
		if e.Kind == "assistant_chunk" && e.Text == "fake reply to: after that" {
			found = true
		}
	}
	if !found {
		t.Fatalf("followup reply missing in %+v", events)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 0 {
		t.Fatalf("queue after delivery = %+v, want empty", q)
	}
}

// Slice 4: a queued Steer on the fake Backend delivers after the turn,
// labelled steer.
func TestQueuedSteerDeliversLabeledAfterTurn(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)

	if code, _ := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	events := readSSE(t, srv.URL, id, 0, 9, 10*time.Second)
	var found *Event
	for i := range events {
		if events[i].Kind == "prompt" && events[i].Text == "go left" {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("queued steer never delivered in %+v", events)
	}
	if found.Mode != "steer" {
		t.Fatalf("queued steer mode = %q, want steer", found.Mode)
	}
}

// Slice 5: a Steer to a working claude Session degrades to a Follow-up; the
// transcript shows it as one, with the text untouched.
func TestClaudeSteerDegradesToFollowup(t *testing.T) {
	stub := &stubConn{blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)

	if code, _ := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	q := getQueue(t, srv.URL, id)
	if len(q) != 1 || q[0].Mode != "steer" || q[0].Text != "go left" {
		t.Fatalf("claude queue = %+v, want one held steer", q)
	}

	close(stub.blockPrompt)
	events := readSSE(t, srv.URL, id, 0, 7, 10*time.Second)
	var found *Event
	for i := range events {
		if events[i].Kind == "prompt" && strings.Contains(events[i].Text, "go left") {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("degraded steer missing in %+v", events)
	}
	if found.Mode != "followup" {
		t.Fatalf("degraded steer mode = %q, want followup", found.Mode)
	}
	if found.Text != "go left" {
		t.Fatalf("degraded steer text = %q, want it unlabeled", found.Text)
	}
}

// An idle claude Steer needs no degradation: nothing runs, so it starts a
// turn at once labelled steer.
func TestClaudeSteerOnIdleKeepsSteerLabel(t *testing.T) {
	stub := &stubConn{updates: []map[string]any{chunk("done")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	if code, _ := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	events := readSSE(t, srv.URL, id, 0, 5, 10*time.Second)
	var found *Event
	for i := range events {
		if events[i].Kind == "prompt" && strings.Contains(events[i].Text, "go left") {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("steer prompt missing in %+v", events)
	}
	if found.Mode != "steer" {
		t.Fatalf("idle claude steer mode = %q, want steer", found.Mode)
	}
	if strings.Contains(found.Text, "degraded") {
		t.Fatalf("idle claude steer mislabeled: %q", found.Text)
	}
}

// Steer and follow-up validation: unknown Session, empty text, ended.
func TestSteerFollowupValidation(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	for _, mode := range []string{"steer", "followup"} {
		if code, _ := postMode(t, srv.URL, "nope", mode, "hi"); code != http.StatusNotFound {
			t.Fatalf("%s unknown status = %d, want 404", mode, code)
		}
	}
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	for _, mode := range []string{"steer", "followup"} {
		if code, _ := postMode(t, srv.URL, id, mode, ""); code != http.StatusBadRequest {
			t.Fatalf("%s empty status = %d, want 400", mode, code)
		}
	}
	deleteSession(t, srv.URL, id)
	for _, mode := range []string{"steer", "followup"} {
		if code, _ := postMode(t, srv.URL, id, mode, "late"); code != http.StatusGone {
			t.Fatalf("%s after end status = %d, want 410", mode, code)
		}
	}
}

func waitQueueLen(t *testing.T, base, id string, want int, timeout time.Duration) []QueuedMessage {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		q := getQueue(t, base, id)
		if len(q) == want {
			return q
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("queue len = %d, want %d", len(getQueue(t, base, id)), want)
	return nil
}

func waitChunk(t *testing.T, base, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, e := range sessionEvents(t, base, id) {
			if e.Kind == "assistant_chunk" && strings.Contains(e.Text, want) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("chunk %q never streamed", want)
}

// startChainedSlowTurns queues two PAUSE slow turns back-to-back (~4s of
// working), so a Steer or Follow-up posted next still finds a busy Session
// even on a loaded machine.
func startChainedSlowTurns(t *testing.T, base, id string) {
	t.Helper()
	sendPrompt(t, base, id, "TUI_LIVE_STREAM_PAUSE one")
	sendPrompt(t, base, id, "TUI_LIVE_STREAM_PAUSE two")
	waitStatus(t, base, id, "working", 30*time.Second)
}

// A Steer reaches a working pig Session through the SDK native queue: the
// response carries an immediate transcript seq with queued set (the Daemon
// pending path would carry no seq), the transcript labels it steer, and
// the steered question delivers as a continuation turn (the scripted
// backend answers 42).
func TestPigSteerReachesSessionNatively(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	startChainedSlowTurns(t, srv.URL, id)

	code, body := postMode(t, srv.URL, id, "steer", "What is 20+22?")
	if code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	if body["queued"] != true || body["seq"] == nil || body["seq"].(float64) == 0 {
		t.Fatalf("steer body = %v, want immediate seq with queued set", body)
	}
	waitChunk(t, srv.URL, id, "42", 60*time.Second)
	_ = waitQueueLen(t, srv.URL, id, 0, 30*time.Second)
	found := false
	for _, e := range sessionEvents(t, srv.URL, id) {
		if e.Kind == "prompt" && e.Mode == "steer" && e.Text == "What is 20+22?" {
			found = true
		}
	}
	if !found {
		t.Fatalf("steer prompt Event missing or unlabeled")
	}
	// Settle before TempDir cleanup: the steered turn's log writes
	// outlive the test otherwise ("directory not empty").
	waitStatus(t, srv.URL, id, "idle", 60*time.Second)
	pb.CloseSession(id)
}

// A Follow-up to a working pig Session runs after the current turn: same
// native response shape as a Steer, labelled followup.
func TestPigFollowupRunsAfterTurn(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	startChainedSlowTurns(t, srv.URL, id)

	code, body := postMode(t, srv.URL, id, "followup", "What is 20+22?")
	if code != http.StatusAccepted {
		t.Fatalf("followup status = %d, want 202", code)
	}
	if body["queued"] != true || body["seq"] == nil || body["seq"].(float64) == 0 {
		t.Fatalf("followup body = %v, want immediate seq with queued set", body)
	}
	waitChunk(t, srv.URL, id, "42", 60*time.Second)
	_ = waitQueueLen(t, srv.URL, id, 0, 30*time.Second)
	found := false
	for _, e := range sessionEvents(t, srv.URL, id) {
		if e.Kind == "prompt" && e.Mode == "followup" && e.Text == "What is 20+22?" {
			found = true
		}
	}
	if !found {
		t.Fatalf("followup prompt Event missing or unlabeled")
	}
	// Settle before TempDir cleanup: the follow-up turn's log writes
	// outlive the test otherwise ("directory not empty").
	waitStatus(t, srv.URL, id, "idle", 60*time.Second)
	pb.CloseSession(id)
}

// Queued pig messages clear before delivery: a prompt queued in the Daemon
// pending list clears over HTTP, and the SDK native queues clear through
// the backend seam.
func TestPigQueuedMessagesClearBeforeDelivery(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	// The PAUSE turn holds ~2s. The second prompt waits in the Daemon
	// pending list: it only drains when the turn ends, so the clear
	// always lands first.
	sendPrompt(t, srv.URL, id, "TUI_LIVE_STREAM_PAUSE please")
	waitStatus(t, srv.URL, id, "working", 30*time.Second)
	sendPrompt(t, srv.URL, id, "queued prompt")
	_ = waitQueueLen(t, srv.URL, id, 1, 5*time.Second)
	if n := clearQueue(t, srv.URL, id); n != 1 {
		t.Fatalf("cleared = %d, want 1", n)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 0 {
		t.Fatalf("queue after clear = %+v, want empty", q)
	}
	waitStatus(t, srv.URL, id, "idle", 30*time.Second)
	for _, e := range sessionEvents(t, srv.URL, id) {
		if e.Kind == "prompt" && e.Text == "queued prompt" {
			t.Fatalf("cleared prompt delivered: %+v", e)
		}
	}

	// The backend seam reaches the SDK native queues directly. A fresh
	// Session that never ran a turn holds its stack: the Daemon releases
	// the process once a turn settles, so the old Session's handle is
	// gone, and nothing consumes the queue until the next Send.
	nativeID := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	pb.Steer(nativeID, "steer one")
	pb.FollowUp(nativeID, "follow one")
	if got := pb.QueuedMessages(nativeID); len(got) != 2 {
		t.Fatalf("native queue = %+v, want steer plus follow-up", got)
	}
	if n := pb.ClearQueued(nativeID); n != 2 {
		t.Fatalf("native cleared = %d, want 2", n)
	}
	if got := pb.QueuedMessages(nativeID); len(got) != 0 {
		t.Fatalf("native queue after clear = %+v, want empty", got)
	}
	// Release both stacks before TempDir cleanup.
	pb.CloseSession(nativeID)
	pb.CloseSession(id)
}

// A send racing a resume or release in flight answers 409 and the Session
// recovers: transition flags single-flight the resume and fence the
// release, so a stale release never kills a freshly resumed process.
func TestSendDuringTransitionAnswers409(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	waitStatus(t, srv.URL, id, "idle", 30*time.Second)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if view := sessionView(t, srv.URL, id); !view.Live {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if view := sessionView(t, srv.URL, id); view.Live {
		t.Fatal("live = true after idle, want the release to settle first")
	}
	for _, flag := range []string{"resuming", "releasing"} {
		d.mu.Lock()
		s := d.sessions[id]
		s.live = false
		if flag == "resuming" {
			s.resuming = true
		} else {
			s.releasing = true
		}
		d.mu.Unlock()
		if code, _ := postMode(t, srv.URL, id, "prompt", "again"); code != http.StatusConflict {
			t.Fatalf("send during %s status = %d, want 409", flag, code)
		}
		d.mu.Lock()
		s.resuming, s.releasing = false, false
		d.mu.Unlock()
	}
	if code, _ := postMode(t, srv.URL, id, "prompt", "again"); code != http.StatusAccepted {
		t.Fatalf("send after transition status = %d, want 202", code)
	}
	waitStatus(t, srv.URL, id, "idle", 30*time.Second)
}

// steerStub is a stubConn whose adapter advertises _session/steering.
type steerStub struct {
	*stubConn
	outcome bool
	steered []string
}

func (s *steerStub) SteeringSupported() bool { return true }

func (s *steerStub) Steer(acpID, text string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steered = append(s.steered, text)
	return s.outcome, nil
}

// A Steer to a working claude Session whose adapter advertises steering is
// injected: it is not queued and the transcript keeps the steer label.
func TestClaudeSteerInjectsWhenAdapterSupportsIt(t *testing.T) {
	stub := &steerStub{stubConn: &stubConn{blockPrompt: make(chan struct{})}, outcome: true}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)

	if code, _ := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	// The Steer is held until a tool call finishes, never injected mid-tool.
	stub.mu.Lock()
	n := len(stub.steered)
	stub.mu.Unlock()
	if n != 0 {
		t.Fatalf("steer injected before a tool call finished: %v", stub.steered)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 1 || q[0].Mode != "steer" {
		t.Fatalf("held steer queue = %+v, want one steer", q)
	}
	d.appendEvent(id, "tool_call_updated", `{"id":"t1","name":"bash","status":"completed"}`, "")
	deadline := time.Now().Add(5 * time.Second)
	for {
		stub.mu.Lock()
		steered := append([]string{}, stub.steered...)
		stub.mu.Unlock()
		if len(steered) == 1 && steered[0] == "go left" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("adapter steered = %v, want [go left]", steered)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 0 {
		t.Fatalf("injected steer left a queue: %+v", q)
	}
	close(stub.blockPrompt)
	events := readSSE(t, srv.URL, id, 0, 5, 10*time.Second)
	for _, e := range events {
		if e.Kind == "prompt" && strings.Contains(e.Text, "go left") {
			if e.Mode != "steer" || strings.Contains(e.Text, "degraded") {
				t.Fatalf("injected steer event = %+v, want plain steer", e)
			}
			return
		}
	}
	t.Fatalf("injected steer missing in %+v", events)
}

// promptRequired from the adapter (no turn running) falls back to a labelled
// Follow-up.
func TestClaudeSteerFallsBackWhenNotInjected(t *testing.T) {
	stub := &steerStub{stubConn: &stubConn{blockPrompt: make(chan struct{})}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)

	if code, _ := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer status = %d, want 202", code)
	}
	q := getQueue(t, srv.URL, id)
	if len(q) != 1 || q[0].Mode != "steer" || q[0].Text != "go left" {
		t.Fatalf("queue = %+v, want one held steer", q)
	}
	close(stub.blockPrompt)
	// Let the turn and its Follow-up finish before TempDir cleanup.
	readSSE(t, srv.URL, id, 0, 7, 10*time.Second)
}

// Session meta replaces the Claude Code preset with el's prompt. Project
// context arrives as injected content only when a WorkDir is present, never
// as a read-these-files instruction.
func TestClaudeMetaCarriesSystemPrompt(t *testing.T) {
	meta := newStubClaudeBackend(nil).strippingMeta("sonnet")
	sp, _ := meta["systemPrompt"].(string)
	if !strings.Contains(sp, "level: ultra") {
		t.Fatalf("systemPrompt = %q, want the el rules", sp)
	}
	if strings.Contains(sp, "Before editing, read") {
		t.Fatalf("systemPrompt tells the agent to read files itself: %q", sp)
	}
}

// Messages queued behind a running turn go out as one message, in order.
func TestQueuedMessagesMergeIntoOne(t *testing.T) {
	stub := &stubConn{blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	postMode(t, srv.URL, id, "steer", "go left")
	postMode(t, srv.URL, id, "followup", "then test")
	postMode(t, srv.URL, id, "steer", "and lint")
	if q := getQueue(t, srv.URL, id); len(q) != 3 {
		t.Fatalf("queue = %+v, want 3 entries before delivery", q)
	}

	close(stub.blockPrompt)
	events := readSSE(t, srv.URL, id, 0, 7, 10*time.Second)
	stub.mu.Lock()
	prompts := append([]string{}, stub.prompts...)
	stub.mu.Unlock()
	want := "go left\n\nthen test\n\nand lint"
	if len(prompts) != 2 || prompts[1] != want {
		t.Fatalf("adapter prompts = %q, want [slow job %q]", prompts, want)
	}
	n := 0
	for _, e := range events {
		if e.Kind == "prompt" && e.Text == want {
			n++
			if e.Mode != "followup" {
				t.Fatalf("merged mode = %q, want followup for mixed modes", e.Mode)
			}
		}
	}
	if n != 1 {
		t.Fatalf("merged prompt events = %d in %+v", n, events)
	}
}
