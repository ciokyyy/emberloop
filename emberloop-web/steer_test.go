package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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

func queueItemRequest(t *testing.T, base, id, itemID, method, text string) (int, string) {
	t.Helper()
	var body []byte
	if method == http.MethodPatch {
		body, _ = json.Marshal(map[string]string{"text": text})
	}
	req, err := http.NewRequest(method, base+"/api/sessions/"+id+"/queue/"+itemID, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if method == http.MethodPatch {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
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

// waitTurnEnd waits for the turn a prompt started to finish: an idle status
// Event after the prompt's seq. A bare working-then-idle wait misses fast
// fake turns (a few ms), because the idle status read before the turn begins
// looks the same as the idle after it.
func waitTurnEnd(t *testing.T, base, id string, promptSeq int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range transcriptOf(t, base, id) {
			if e.Seq > promptSeq && e.Kind == "status" && e.Status == "idle" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("turn after seq %d never ended: %+v", promptSeq, transcriptOf(t, base, id))
}

func waitDaemonIdle(t *testing.T, d *Daemon, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		s := d.sessions[id]
		idle := s != nil && !s.busy && s.active == 0
		d.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Session did not settle after its idle Event")
}

// waitParentIdle waits out a spawn parent's grouped delivery before the
// test's TempDir is removed: the last child to settle steers the parent, so
// the parent works briefly after its children go idle, and its writes race
// TempDir removal when the test returns first.
func waitParentIdle(t *testing.T, base, parent string) {
	t.Helper()
	waitStatus(t, base, parent, "idle", 20*time.Second)
}

// waitGroupedDelivery waits for a spawn Group's grouped result to reach the
// parent as a steer prompt naming the child. Delivery lands after the last
// child settles, so a bare parent-idle wait can slip in before it and the
// next compress hits a working session (409): wait for the delivery itself.
func waitGroupedDelivery(t *testing.T, base, parent, child string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, events := sessionMeta(t, base, parent)
		for _, e := range events {
			if e.Kind == "prompt" && e.Mode == "steer" && strings.Contains(e.Text, child[:8]) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no grouped delivery for child %s in parent transcript", child[:8])
}

// Slice 1: a Steer on an idle fake Session runs a turn labelled steer.
func TestSteerOnIdleRunsLabeledTurn(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	fake := NewFakeBackend()
	fake.Delay = 400 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
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
	waitDaemonIdle(t, d, id)
}

// Per-item edits and removes preserve the ids and order of the remaining
// Daemon-owned messages.
func TestQueuedItemEditAndRemovePreserveOrder(t *testing.T) {
	t.Parallel()
	fake := NewFakeBackend()
	fake.Delay = 500 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	seq := sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	for _, message := range []struct{ mode, text string }{
		{"steer", "first"}, {"followup", "second"}, {"followup", "third"},
	} {
		if code, _ := postMode(t, srv.URL, id, message.mode, message.text); code != http.StatusAccepted {
			t.Fatalf("queue %s status = %d", message.text, code)
		}
	}
	initial := getQueue(t, srv.URL, id)
	if len(initial) != 3 {
		t.Fatalf("initial queue = %+v, want 3 messages", initial)
	}
	for i, item := range initial {
		if item.ID == "" || item.ImageIDs == nil {
			t.Fatalf("queue item %d lacks stable id or imageIds: %+v", i, item)
		}
		for j := 0; j < i; j++ {
			if initial[j].ID == item.ID {
				t.Fatalf("duplicate queue id %q", item.ID)
			}
		}
	}
	if code, body := queueItemRequest(t, srv.URL, id, initial[1].ID, http.MethodPatch, "edited second"); code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", code, body)
	}
	if code, body := queueItemRequest(t, srv.URL, id, initial[0].ID, http.MethodDelete, ""); code != http.StatusOK {
		t.Fatalf("remove status = %d: %s", code, body)
	}
	updated := getQueue(t, srv.URL, id)
	if len(updated) != 2 || updated[0].ID != initial[1].ID || updated[0].Text != "edited second" || updated[1].ID != initial[2].ID || updated[1].Text != "third" {
		t.Fatalf("queue after edit/remove = %+v; want edited second then third with stable ids", updated)
	}
	if code, body := queueItemRequest(t, srv.URL, id, "unknown", http.MethodDelete, ""); code != http.StatusNotFound || !strings.Contains(body, "queued message not found") {
		t.Fatalf("unknown remove = %d %s, want clear 404", code, body)
	}
	if n := clearQueue(t, srv.URL, id); n != 2 {
		t.Fatalf("cleared = %d, want 2", n)
	}
	waitTurnEnd(t, srv.URL, id, seq)
	waitDaemonIdle(t, d, id)
}

func TestDeliveredQueueItemCannotBeEdited(t *testing.T) {
	t.Parallel()
	fake := NewFakeBackend()
	fake.Delay = 200 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	seq := sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	if code, _ := postMode(t, srv.URL, id, "followup", "already delivered"); code != http.StatusAccepted {
		t.Fatalf("queue status = %d", code)
	}
	queued := getQueue(t, srv.URL, id)
	if len(queued) != 1 {
		t.Fatalf("queue = %+v, want one item", queued)
	}
	waitTurnEnd(t, srv.URL, id, seq)
	firstPromptWhere(t, srv.URL, id, func(e Event) bool { return e.Text == "already delivered" })
	waitDaemonIdle(t, d, id)
	if code, body := queueItemRequest(t, srv.URL, id, queued[0].ID, http.MethodPatch, "too late"); code != http.StatusConflict || !strings.Contains(body, "already delivered") {
		t.Fatalf("edit delivered item = %d %s, want clear 409", code, body)
	}
}

// Slice 3: a queued Follow-up runs after the current fake turn.
func TestQueuedFollowupRunsAfterTurn(t *testing.T) {
	t.Parallel()
	fake := NewFakeBackend()
	fake.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	fake := NewFakeBackend()
	fake.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	stub := &stubConn{blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	stub := &stubConn{updates: []map[string]any{chunk("done")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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

// Queued pig messages clear before delivery: a prompt queued in the Daemon
// pending list clears over HTTP, and the SDK native queues clear through
// the backend seam.
func TestPigQueuedMessagesClearBeforeDelivery(t *testing.T) {
	t.Parallel()
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(authedHandler(t, d))
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
	queued := getQueue(t, srv.URL, nativeID)
	if len(queued) != 2 || queued[0].Mode != "steer" || queued[1].Mode != "followup" || queued[0].ID == "" || queued[1].ID == "" {
		t.Fatalf("native queue = %+v, want identified steer and follow-up", queued)
	}
	if again := getQueue(t, srv.URL, nativeID); len(again) != 2 || again[0].ID != queued[0].ID || again[1].ID != queued[1].ID {
		t.Fatalf("native ids changed across reads: first=%+v second=%+v", queued, again)
	}
	if code, body := queueItemRequest(t, srv.URL, nativeID, "unknown-native", http.MethodDelete, ""); code != http.StatusNotFound || !strings.Contains(body, "queued message not found") {
		t.Fatalf("unknown native remove = %d %s, want clear 404", code, body)
	}
	if code, body := queueItemRequest(t, srv.URL, nativeID, queued[0].ID, http.MethodPatch, "steer edited"); code != http.StatusOK {
		t.Fatalf("native patch = %d %s", code, body)
	}
	if code, body := queueItemRequest(t, srv.URL, nativeID, queued[1].ID, http.MethodDelete, ""); code != http.StatusOK {
		t.Fatalf("native remove = %d %s", code, body)
	}
	remaining := getQueue(t, srv.URL, nativeID)
	if len(remaining) != 1 || remaining[0].ID != queued[0].ID || remaining[0].Mode != "steer" || remaining[0].Text != "steer edited" {
		t.Fatalf("native queue after edit/remove = %+v, want updated steer with stable id", remaining)
	}
	if n := clearQueue(t, srv.URL, nativeID); n != 1 {
		t.Fatalf("native cleared = %d, want 1", n)
	}
	if got := pb.QueuedMessages(nativeID); len(got) != 0 {
		t.Fatalf("native queue after clear = %+v, want empty", got)
	}
	deliveredID := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	pb.Steer(deliveredID, "native delivered")
	delivered := getQueue(t, srv.URL, deliveredID)
	if len(delivered) != 1 {
		t.Fatalf("native queue before delivery = %+v, want one item", delivered)
	}
	if n := pb.ClearQueued(deliveredID); n != 1 {
		t.Fatalf("simulated native delivery cleared = %d, want 1", n)
	}
	if code, body := queueItemRequest(t, srv.URL, deliveredID, delivered[0].ID, http.MethodPatch, "too late"); code != http.StatusConflict || !strings.Contains(body, "already delivered") {
		t.Fatalf("edit delivered native item = %d %s, want clear 409", code, body)
	}
	if code, body := queueItemRequest(t, srv.URL, deliveredID, "unknown-native", http.MethodDelete, ""); code != http.StatusNotFound || !strings.Contains(body, "queued message not found") {
		t.Fatalf("unknown native remove = %d %s, want clear 404", code, body)
	}
	// Release both stacks before TempDir cleanup.
	pb.CloseSession(deliveredID)
	pb.CloseSession(nativeID)
	pb.CloseSession(id)
}

func TestReconcileNativeQueuePreservesIDsAfterFIFODrain(t *testing.T) {
	previous := []QueuedMessage{
		{ID: "steer-first", Mode: "steer", Text: "same"},
		{ID: "steer-next", Mode: "steer", Text: "same"},
		{ID: "followup", Mode: "followup", Text: "same"},
	}
	current := []QueuedMessage{{Mode: "steer", Text: "same"}, {Mode: "followup", Text: "same"}}
	got := reconcileNativeQueue(previous, current)
	if len(got) != 2 || got[0].ID != "steer-next" || got[1].ID != "followup" {
		t.Fatalf("reconciled queue = %+v, want stable ids for the undelivered duplicate and follow-up", got)
	}
}

type scriptedNativeQueueBackend struct {
	Backend
	mu        sync.Mutex
	queue     []QueuedMessage
	addCalls  int
	failAddAt int
	blockOp   string
	started   chan struct{}
	release   chan struct{}
}

func (b *scriptedNativeQueueBackend) block(op string) {
	b.mu.Lock()
	blocked := b.blockOp == op
	started, release := b.started, b.release
	if blocked {
		b.blockOp = ""
	}
	b.mu.Unlock()
	if blocked {
		started <- struct{}{}
		<-release
	}
}

func (b *scriptedNativeQueueBackend) NativeQueue(string) ([]QueuedMessage, bool) {
	b.block("read")
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]QueuedMessage(nil), b.queue...), true
}

func (b *scriptedNativeQueueBackend) QueuedMessages(id string) []QueuedMessage {
	queue, _ := b.NativeQueue(id)
	return queue
}

func (b *scriptedNativeQueueBackend) ClearNativeQueue(string) ([]QueuedMessage, bool) {
	b.block("clear")
	b.mu.Lock()
	defer b.mu.Unlock()
	queue := append([]QueuedMessage(nil), b.queue...)
	b.queue = nil
	return queue, true
}

func (b *scriptedNativeQueueBackend) ClearQueued(id string) int {
	queue, _ := b.ClearNativeQueue(id)
	return len(queue)
}

func (b *scriptedNativeQueueBackend) add(mode, text string) bool {
	b.block("add")
	b.mu.Lock()
	defer b.mu.Unlock()
	b.addCalls++
	if b.failAddAt != 0 && b.addCalls == b.failAddAt {
		return false
	}
	b.queue = append(b.queue, QueuedMessage{Mode: mode, Text: text, ImageIDs: []string{}})
	return true
}

func (b *scriptedNativeQueueBackend) Steer(_, text string) bool {
	return b.add("steer", text)
}

func (b *scriptedNativeQueueBackend) FollowUp(_, text string) bool {
	return b.add("followup", text)
}

func queueControlDaemon(backend *scriptedNativeQueueBackend) (*Daemon, string) {
	id := "queue-control-test"
	d := &Daemon{
		sessions: map[string]*session{
			id: {meta: SessionMeta{ID: id, Harness: "pig", Status: "working"}, live: true, busy: true},
		},
		backends: map[string]Backend{"pig": backend},
	}
	return d, id
}

func TestPigNativeReaddFailureRetainsQueueTail(t *testing.T) {
	backend := &scriptedNativeQueueBackend{
		queue: []QueuedMessage{
			{Mode: "steer", Text: "first"},
			{Mode: "followup", Text: "second"},
			{Mode: "followup", Text: "third"},
		},
		failAddAt: 3,
	}
	d, id := queueControlDaemon(backend)
	initial, code := d.listQueue(id)
	if code != http.StatusOK || len(initial) != 3 {
		t.Fatalf("initial native queue = %+v, %d", initial, code)
	}
	if code, msg := d.editQueueItem(id, initial[1].ID, "edited second", false); code != http.StatusOK {
		t.Fatalf("edit with a failed native re-add = %d %s", code, msg)
	}
	got, code := d.listQueue(id)
	if code != http.StatusOK || len(got) != 3 || got[0].ID != initial[0].ID || got[0].Text != "first" || got[1].ID != initial[1].ID || got[1].Text != "edited second" || got[2].ID != initial[2].ID || got[2].Text != "third" {
		t.Fatalf("queue after failed re-add = %+v, %d; want all items in order", got, code)
	}
}

func TestQueueBackendIPCDoesNotHoldDaemonLock(t *testing.T) {
	for _, op := range []string{"read", "clear", "add"} {
		t.Run(op, func(t *testing.T) {
			backend := &scriptedNativeQueueBackend{
				queue:   []QueuedMessage{{Mode: "followup", Text: "queued"}},
				started: make(chan struct{}, 1),
				release: make(chan struct{}),
			}
			d, id := queueControlDaemon(backend)
			initial, code := d.listQueue(id)
			if code != http.StatusOK || len(initial) != 1 {
				t.Fatalf("initial queue = %+v, %d", initial, code)
			}
			backend.mu.Lock()
			backend.blockOp = op
			backend.mu.Unlock()
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch op {
				case "read", "add":
					d.editQueueItem(id, initial[0].ID, "edited", false)
				case "clear":
					d.dropQueue(id)
				}
			}()
			select {
			case <-backend.started:
			case <-time.After(2 * time.Second):
				close(backend.release)
				t.Fatal("queue backend call did not start")
			}
			free := d.mu.TryLock()
			close(backend.release)
			if !free {
				t.Fatal("queue backend I/O held the Daemon lock")
			}
			d.mu.Unlock()
			<-done
		})
	}
}

func TestPigACPQueueControlEditsThroughShimAndRetainsFailedTail(t *testing.T) {
	sock := filepath.Join(tempDir(t), "queue.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var mu sync.Mutex
	queue := pigQueue{Steering: []string{"first"}, FollowUp: []string{"second", "third", "fourth"}}
	addCalls, failAddAt := 0, 0
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var req struct {
					Op      string `json:"op"`
					Message string `json:"message"`
				}
				if json.NewDecoder(conn).Decode(&req) != nil {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				success := true
				var data any
				switch req.Op {
				case "queue":
					data = queue
				case "clear_queue":
					data = queue
					queue = pigQueue{}
				case "steer", "follow_up":
					addCalls++
					if failAddAt != 0 && addCalls == failAddAt {
						success = false
						data = map[string]string{"disposition": "failed"}
					} else {
						if req.Op == "steer" {
							queue.Steering = append(queue.Steering, req.Message)
						} else {
							queue.FollowUp = append(queue.FollowUp, req.Message)
						}
						data = map[string]string{"disposition": "queued"}
					}
				default:
					success = false
				}
				_ = json.NewEncoder(conn).Encode(map[string]any{"success": success, "data": data})
			}(conn)
		}
	}()

	id := "pig-acp-queue-test"
	backend := &PigACPBackend{ClaudeBackend: &ClaudeBackend{}, socks: map[string]string{id: sock}}
	d := &Daemon{
		sessions: map[string]*session{
			id: {meta: SessionMeta{ID: id, Harness: "pig", Status: "working"}, live: true, busy: true},
		},
		backends: map[string]Backend{"pig": backend},
	}
	initial, code := d.listQueue(id)
	if code != http.StatusOK || len(initial) != 4 {
		t.Fatalf("initial ACP queue = %+v, %d", initial, code)
	}
	if code, msg := d.editQueueItem(id, initial[1].ID, "", true); code != http.StatusOK {
		t.Fatalf("ACP native remove = %d %s", code, msg)
	}
	mu.Lock()
	failAddAt = addCalls + 2
	mu.Unlock()
	if code, msg := d.editQueueItem(id, initial[2].ID, "edited third", false); code != http.StatusOK {
		t.Fatalf("ACP edit with failed re-add = %d %s", code, msg)
	}
	got, code := d.listQueue(id)
	if code != http.StatusOK || len(got) != 3 || got[0].ID != initial[0].ID || got[0].Text != "first" || got[1].ID != initial[2].ID || got[1].Text != "edited third" || got[2].ID != initial[3].ID || got[2].Text != "fourth" {
		t.Fatalf("ACP queue after edit/remove/failure = %+v, %d; want retained order", got, code)
	}
	mu.Lock()
	queue.Steering = nil // simulate the native queue delivering its known item
	mu.Unlock()
	if code, msg := d.editQueueItem(id, initial[0].ID, "too late", false); code != http.StatusConflict || !strings.Contains(msg, "already delivered") {
		t.Fatalf("ACP delivered edit = %d %s, want 409", code, msg)
	}
	if code, msg := d.editQueueItem(id, "unknown-acp", "", true); code != http.StatusNotFound || !strings.Contains(msg, "queued message not found") {
		t.Fatalf("ACP unknown remove = %d %s, want 404", code, msg)
	}
}

func (f *failingSteerBackend) NativeQueue(sessionID string) ([]QueuedMessage, bool) {
	return []QueuedMessage{}, true
}

func (f *failingSteerBackend) ClearNativeQueue(sessionID string) ([]QueuedMessage, bool) {
	return []QueuedMessage{}, true
}

// A send racing a resume or release in flight answers 409 and the Session
// recovers: transition flags single-flight the resume and fence the
// release, so a stale release never kills a freshly resumed process.
func TestSendDuringTransitionAnswers409(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	stub := &steerStub{stubConn: &stubConn{blockPrompt: make(chan struct{})}, outcome: true}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
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
	// Six, not five: the Session seeds a context reading at start, so the
	// injected steer prompt sits one event later.
	events := readSSE(t, srv.URL, id, 0, 6, 10*time.Second)
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
	t.Parallel()
	stub := &steerStub{stubConn: &stubConn{blockPrompt: make(chan struct{})}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
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
	waitDaemonIdle(t, d, id)
}

func TestClaudeDegradedSteerReportsFollowupMode(t *testing.T) {
	t.Parallel()
	stub := &steerStub{stubConn: &stubConn{blockPrompt: make(chan struct{})}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	seq := sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	d.mu.Lock()
	d.sessions[id].swapping = true
	d.mu.Unlock()
	code, body := postMode(t, srv.URL, id, "steer", "go after handoff")
	if code != http.StatusAccepted || body["mode"] != "followup" || body["degraded"] != true {
		t.Fatalf("degraded steer response = %d %+v, want followup mode", code, body)
	}
	queued := getQueue(t, srv.URL, id)
	if len(queued) != 1 || queued[0].Mode != "followup" || !queued[0].Degraded {
		t.Fatalf("degraded queue = %+v, want one degraded follow-up", queued)
	}
	if code, body := queueItemRequest(t, srv.URL, id, queued[0].ID, http.MethodPatch, "edited follow-up"); code != http.StatusOK {
		t.Fatalf("patch degraded steer = %d %s, want queued follow-up edit", code, body)
	}
	edited := getQueue(t, srv.URL, id)
	if len(edited) != 1 || edited[0].ID != queued[0].ID || edited[0].Mode != "followup" || !edited[0].Degraded || edited[0].Text != "edited follow-up" {
		t.Fatalf("edited degraded queue = %+v, want same follow-up id and mode", edited)
	}
	if n := clearQueue(t, srv.URL, id); n != 1 {
		t.Fatalf("cleared = %d, want 1", n)
	}
	close(stub.blockPrompt)
	waitTurnEnd(t, srv.URL, id, seq)
	waitDaemonIdle(t, d, id)
}

// Session meta replaces the Claude Code preset with el's prompt. Project
// context arrives as injected content only when a WorkDir is present, never
// as a read-these-files instruction.
func TestClaudeMetaCarriesSystemPrompt(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	stub := &stubConn{blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
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
	// Wait for the adapter to receive the merged message before reading the
	// event stream; queued delivery can lag behind the first seven events.
	want := "go left\n\nthen test\n\nand lint"
	deadline := time.Now().Add(5 * time.Second)
	var prompts []string
	for {
		stub.mu.Lock()
		prompts = append([]string(nil), stub.prompts...)
		stub.mu.Unlock()
		if len(prompts) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("adapter prompts after queued delivery timeout = %q, want merged prompt %q", prompts, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	eventCount := len(getSession(t, srv.URL, id).Events)
	events := readSSE(t, srv.URL, id, 0, eventCount, 10*time.Second)
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
