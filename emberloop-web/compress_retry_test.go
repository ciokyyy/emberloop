package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// stepSummarizer answers one step per call. A step with err fails that call.
// The last step repeats once the steps run out.
type stepSummarizer struct {
	mu    sync.Mutex
	steps []summaryStep
	calls int
}

type summaryStep struct {
	reply string
	err   error
}

func (f *stepSummarizer) Summarize(model, system, user string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i >= len(f.steps) {
		i = len(f.steps) - 1
	}
	return f.steps[i].reply, f.steps[i].err
}

// Window is a budget no test input reaches, so the writer never folds.
func (f *stepSummarizer) Window(string) int { return 1 << 20 }

func (f *stepSummarizer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// waitRecorder stands in for the backoff sleep and records each wait.
type waitRecorder struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (w *waitRecorder) wait(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.waits = append(w.waits, d)
}

func (w *waitRecorder) got() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Duration(nil), w.waits...)
}

// sessionMetaOf reads the Session metadata through the Daemon API.
func sessionMetaOf(t *testing.T, base, id string) SessionMeta {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Session SessionMeta `json:"session"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Session
}

// sessionLive reads the live flag of a Session through the Daemon API.
func sessionLive(t *testing.T, base, id string) bool {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Live bool `json:"live"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Live
}

// queuedTexts lists the texts the Session holds in its queue.
func queuedTexts(t *testing.T, base, id string) []string {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id + "/queue")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Queue []QueuedMessage `json:"queue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, q := range out.Queue {
		texts = append(texts, q.Text)
	}
	return texts
}

// postEmpty posts with no body and returns the status and the JSON answer.
func postEmpty(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// blockedSession blocks one Session on stateDir with a credit failure, then
// stops that Daemon's server, as a restart does. It returns the Session ID.
func blockedSession(t *testing.T, stateDir string) string {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(stateDir, NewFakeBackend())
	d.SetSettingsPath(settingsPath)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{err: errString("402 Payment Required")}}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)
	compressReq(t, srv.URL, id, "")
	waitSettled(t, srv.URL, id, hasBlockedEvent)
	return id
}

func hasBlockedEvent(evs []Event) bool {
	return len(eventsOfKind(evs, "compress_blocked")) > 0
}

func hasHeldPrompt(evs []Event) bool {
	_, ok := promptWith(evs, "held prompt", "")
	return ok
}

// Credit, auth and quota failures block. Network, 5xx and rate-limit failures
// retry. Prose with bare numbers or loose words is other. The texts are forms
// the pig SDK puts into its errors.
func TestCompressFailureClasses(t *testing.T) {
	cases := []struct {
		text string
		want compressFailure
	}{
		{"402 status code (no body)", failBlock},
		{"anthropic: HTTP 401: authentication_error: invalid x-api-key", failBlock},
		{"429 quota exceeded for this key", failBlock},
		{"insufficient_quota", failBlock},
		{"no credits left on this key", failBlock},
		{"pig one-off: 402 Payment Required", failBlock},
		{"Mistral API error (429): Too Many Requests", failTransient},
		{"anthropic: HTTP 529: overloaded_error: busy", failTransient},
		{"got status: UNAVAILABLE. {}", failTransient},
		{"pig one-off: 503 Service Unavailable", failTransient},
		{"Connection error.", failTransient},
		{"connection reset by peer", failTransient},
		{"fetch failed", failTransient},
		{"Request timed out.", failTransient},
		{"OpenAI API error (403): forbidden", failOther},
		{`400: {"message":"invalid request"}`, failOther},
		{"pig one-off stopped at the 5m0s limit", failOther},
		{"bad request: malformed input", failOther},
		{"context 500 tokens", failOther},
		{"max output 401 tokens", failOther},
		{"insufficient room in the context window", failOther},
		{"credit card form field is invalid", failOther},
		{"connection pool size 5", failOther},
		{"retry in 500ms", failOther},
	}
	for _, c := range cases {
		if got := classifyCompressError(errString(c.text)); got != c.want {
			t.Errorf("%q: class %d, want %d", c.text, got, c.want)
		}
	}
}

// A deadline of the one-off is other: retrying it would stall the Session.
func TestOneShotDeadlineIsOtherNotTransient(t *testing.T) {
	expired, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-expired.Done()
	err := oneShotDeadline(expired, 5*time.Minute, true)
	if err == nil {
		t.Fatal("deadline failure returned no error")
	}
	if got := classifyCompressError(err); got != failOther {
		t.Fatalf("deadline error %q classed %d, want other", err, got)
	}
	if oneShotDeadline(expired, 5*time.Minute, false) != nil {
		t.Fatal("a reply that landed is not a failure")
	}
	live, stop := context.WithCancel(context.Background())
	defer stop()
	if oneShotDeadline(live, 5*time.Minute, true) != nil {
		t.Fatal("a failure before the deadline is not a deadline failure")
	}
}

// StartErrs fail the next StartHandle calls in order, then StartErr applies.
func TestFakeBackendStartErrsFailInOrder(t *testing.T) {
	be := NewFakeBackend()
	emit := func(string, string, string) {}
	be.StartErrs = []error{errString("first"), errString("second")}
	for _, want := range []string{"first", "second"} {
		if _, _, err := be.StartHandle(SessionMeta{ID: "s"}, emit); err == nil || err.Error() != want {
			t.Fatalf("StartHandle = %v, want %q", err, want)
		}
	}
	if _, _, err := be.StartHandle(SessionMeta{ID: "s"}, emit); err != nil {
		t.Fatalf("StartHandle after StartErrs = %v, want success", err)
	}
	be.StartErr = errString("always")
	if _, _, err := be.StartHandle(SessionMeta{ID: "s"}, emit); err == nil || err.Error() != "always" {
		t.Fatalf("StartHandle with StartErr = %v, want always", err)
	}
}

// A transient summarizer failure waits 2s and retries. The Handoff lands with
// the retry count on the compressed Event, and the handle rotates.
func TestCompressTransientFailureRetriesThenSucceeds(t *testing.T) {
	d, _, base, id := summaryDaemon(t, goodSummary)
	sum := &stepSummarizer{steps: []summaryStep{{err: errString("503 Service Unavailable")}, {reply: goodSummary}}}
	d.summarizer = sum
	rec := &waitRecorder{}
	d.compressSleep = rec.wait
	before := sessionMetaOf(t, base, id).ResumeToken
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)
	if n := sum.callCount(); n != 2 {
		t.Fatalf("summarizer calls = %d, want 2", n)
	}
	if got, want := rec.got(), []time.Duration{2 * time.Second}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("waits = %v, want %v", got, want)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"transientRetries":1`) || !strings.Contains(c, `"fallback":"summary"`) {
		t.Fatalf("compressed = %s", c)
	}
	if n := len(eventsOfKind(evs, "compress_blocked")); n != 0 {
		t.Fatalf("compress_blocked events = %d, want 0", n)
	}
	if after := sessionMetaOf(t, base, id).ResumeToken; after == before {
		t.Fatalf("resume token %q not rotated", after)
	}
}

// Transient failures that outlast the three retries fall to the deterministic
// Handoff. The waits run 2s, 8s and 30s, and the error text stays on the Event.
func TestCompressTransientFailureExhaustsRetriesThenFallsBack(t *testing.T) {
	d, _, base, id := summaryDaemon(t, goodSummary)
	var steps []summaryStep
	for i := 0; i < 4; i++ {
		steps = append(steps, summaryStep{err: errString("503 Service Unavailable")})
	}
	sum := &stepSummarizer{steps: steps}
	d.summarizer = sum
	rec := &waitRecorder{}
	d.compressSleep = rec.wait
	compressReq(t, base, id, "")
	evs := waitIdleAfter(t, base, id, 1)
	if n := sum.callCount(); n != 4 {
		t.Fatalf("summarizer calls = %d, want 4", n)
	}
	want := []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}
	if fmt.Sprint(rec.got()) != fmt.Sprint(want) {
		t.Fatalf("waits = %v, want %v", rec.got(), want)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"fallback":"deterministic"`) || !strings.Contains(c, `"transientRetries":3`) || !strings.Contains(c, `"error":"503 Service Unavailable"`) {
		t.Fatalf("compressed = %s", c)
	}
}

// A credit failure does not retry. The Session blocks: no swap, the old handle
// and its resume token stay, and the compress_blocked Event carries Retry.
func TestCompressCreditFailureBlocksSession(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	sum := &stepSummarizer{steps: []summaryStep{{err: errString("402 Payment Required: insufficient credits")}}}
	d.summarizer = sum
	rec := &waitRecorder{}
	d.compressSleep = rec.wait
	before := sessionMetaOf(t, base, id).ResumeToken
	compressReq(t, base, id, "")
	evs := waitSettled(t, base, id, hasBlockedEvent)
	if n := sum.callCount(); n != 1 {
		t.Fatalf("summarizer calls = %d, want 1", n)
	}
	if len(rec.got()) != 0 {
		t.Fatalf("waits = %v, want none", rec.got())
	}
	if n := len(eventsOfKind(evs, "compressed")); n != 0 {
		t.Fatalf("compressed events = %d, want 0", n)
	}
	blocked := eventsOfKind(evs, "compress_blocked")[0].Text
	if !strings.Contains(blocked, `"action":"retry"`) || !strings.Contains(blocked, "402 Payment Required") {
		t.Fatalf("compress_blocked = %s", blocked)
	}
	meta := sessionMetaOf(t, base, id)
	if meta.CompressBlock == "" {
		t.Fatal("session not blocked")
	}
	if meta.ResumeToken != before {
		t.Fatalf("resume token = %q, want %q", meta.ResumeToken, before)
	}
	if n := be.ReleasedHandles(id); n != 0 {
		t.Fatalf("released = %d, want 0", n)
	}
}

// While blocked, a new prompt queues and does not run. Retry reruns the whole
// compress. The held prompt runs after the Handoff, and the block clears.
func TestCompressBlockHoldsPromptUntilRetrySucceeds(t *testing.T) {
	d, _, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{err: errString("401 Unauthorized")}}}
	compressReq(t, base, id, "")
	waitSettled(t, base, id, hasBlockedEvent)
	sendPrompt(t, base, id, "held prompt")
	if got := queuedTexts(t, base, id); len(got) != 1 || got[0] != "held prompt" {
		t.Fatalf("queue = %v, want the held prompt", got)
	}
	if hasHeldPrompt(transcriptOf(t, base, id)) {
		t.Fatal("held prompt ran while blocked")
	}
	d.summarizer = &stepSummarizer{steps: []summaryStep{{reply: goodSummary}}}
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("retry = %d %v", code, body)
	}
	evs := waitSettled(t, base, id, func(evs []Event) bool {
		return len(eventsOfKind(evs, "compressed")) == 1 && hasHeldPrompt(evs)
	})
	if meta := sessionMetaOf(t, base, id); meta.CompressBlock != "" {
		t.Fatalf("block still set after Retry: %q", meta.CompressBlock)
	}
	handoffAt, heldAt := -1, -1
	for i, e := range evs {
		if e.Kind == "prompt" && e.Handoff && handoffAt < 0 {
			handoffAt = i
		}
		if e.Kind == "prompt" && strings.Contains(e.Text, "held prompt") && heldAt < 0 {
			heldAt = i
		}
	}
	if handoffAt < 0 || heldAt < handoffAt {
		t.Fatalf("handoff at %d, held prompt at %d: want the Handoff first", handoffAt, heldAt)
	}
}

// Dismiss clears the block without a compress. The old handle stays, the
// resume token is unchanged, and the held prompt runs on the old handle.
func TestCompressBlockDismissKeepsOldHandleAndReleasesHeld(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{err: errString("402 Payment Required")}}}
	compressReq(t, base, id, "")
	waitSettled(t, base, id, hasBlockedEvent)
	before := sessionMetaOf(t, base, id).ResumeToken
	sendPrompt(t, base, id, "held prompt")
	if code, body := postEmpty(t, base+"/api/sessions/"+id+"/compress/dismiss"); code != http.StatusOK {
		t.Fatalf("dismiss = %d %v", code, body)
	}
	evs := waitSettled(t, base, id, hasHeldPrompt)
	meta := sessionMetaOf(t, base, id)
	if meta.CompressBlock != "" {
		t.Fatalf("block still set after dismiss: %q", meta.CompressBlock)
	}
	if meta.ResumeToken != before {
		t.Fatalf("resume token = %q, want %q", meta.ResumeToken, before)
	}
	if n := len(eventsOfKind(evs, "compressed")); n != 0 {
		t.Fatalf("compressed events = %d, want 0", n)
	}
	if n := be.ReleasedHandles(id); n != 0 {
		t.Fatalf("released = %d, want 0", n)
	}
}

// Dismiss answers 409 while no block is set and 404 for an unknown Session.
func TestDismissRefusedWithoutBlockAndForUnknownSession(t *testing.T) {
	_, _, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	if code, body := postEmpty(t, base+"/api/sessions/"+id+"/compress/dismiss"); code != http.StatusConflict {
		t.Fatalf("dismiss with no block = %d %v, want 409", code, body)
	}
	if code, _ := postEmpty(t, base+"/api/sessions/no-such-session/compress/dismiss"); code != http.StatusNotFound {
		t.Fatalf("dismiss of an unknown Session = %d, want 404", code)
	}
}

// Dismiss answers 409 while the Session works, and the block stays until the
// Retry settles. The backoff wait holds the Retry open.
func TestDismissRefusedWhileCompressRuns(t *testing.T) {
	d, _, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{err: errString("402 Payment Required")}}}
	compressReq(t, base, id, "")
	waitSettled(t, base, id, hasBlockedEvent)

	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	d.compressSleep = func(time.Duration) { <-gate }
	d.summarizer = &stepSummarizer{steps: []summaryStep{{err: errString("503 Service Unavailable")}, {reply: goodSummary}}}
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("retry = %d %v", code, body)
	}
	waitStatus(t, base, id, "working", 5*time.Second)
	if code, body := postEmpty(t, base+"/api/sessions/"+id+"/compress/dismiss"); code != http.StatusConflict {
		t.Fatalf("dismiss while working = %d %v, want 409", code, body)
	}
	if sessionMetaOf(t, base, id).CompressBlock == "" {
		t.Fatal("block cleared while the Retry runs")
	}
	release()
	waitSettled(t, base, id, func(evs []Event) bool {
		return len(eventsOfKind(evs, "compressed")) == 1
	})
	if sessionMetaOf(t, base, id).CompressBlock != "" {
		t.Fatal("block still set after the Retry succeeded")
	}
	if code, body := postEmpty(t, base+"/api/sessions/"+id+"/compress/dismiss"); code != http.StatusConflict {
		t.Fatalf("dismiss after the block cleared = %d %v, want 409", code, body)
	}
}

// The block lives in the Session metadata. A restarted Daemon keeps it: a send
// resumes the Session and queues behind the block, and Dismiss clears it.
func TestCompressBlockSurvivesRestart(t *testing.T) {
	stateDir := tempDir(t)
	id := blockedSession(t, stateDir)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	if reason := sessionMetaOf(t, srv.URL, id).CompressBlock; !strings.Contains(reason, "402 Payment Required") {
		t.Fatalf("block after restart = %q, want the 402 reason", reason)
	}
	sendPrompt(t, srv.URL, id, "held prompt")
	if got := queuedTexts(t, srv.URL, id); len(got) != 1 || got[0] != "held prompt" {
		t.Fatalf("queue after restart = %v, want the held prompt", got)
	}
	if hasHeldPrompt(transcriptOf(t, srv.URL, id)) {
		t.Fatal("held prompt ran while blocked after restart")
	}
	if code, body := postEmpty(t, srv.URL+"/api/sessions/"+id+"/compress/dismiss"); code != http.StatusOK {
		t.Fatalf("dismiss = %d %v", code, body)
	}
	waitSettled(t, srv.URL, id, hasHeldPrompt)
	if reason := sessionMetaOf(t, srv.URL, id).CompressBlock; reason != "" {
		t.Fatalf("block still set after dismiss: %q", reason)
	}
}

// Dismiss on a released Session resumes it first, clears the block, and the
// next prompt runs.
func TestDismissResumesReleasedSession(t *testing.T) {
	stateDir := tempDir(t)
	id := blockedSession(t, stateDir)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	if sessionLive(t, srv.URL, id) {
		t.Fatal("restarted Session is live, want released")
	}
	if code, body := postEmpty(t, srv.URL+"/api/sessions/"+id+"/compress/dismiss"); code != http.StatusOK {
		t.Fatalf("dismiss = %d %v", code, body)
	}
	if reason := sessionMetaOf(t, srv.URL, id).CompressBlock; reason != "" {
		t.Fatalf("block still set after dismiss: %q", reason)
	}
	sendPrompt(t, srv.URL, id, "after dismiss")
	waitSettled(t, srv.URL, id, func(evs []Event) bool {
		_, ok := promptWith(evs, "after dismiss", "")
		return ok
	})
}

// A released Session that cannot resume refuses Dismiss, and the block stays.
func TestDismissRefusedWhileSessionCannotResume(t *testing.T) {
	stateDir := tempDir(t)
	id := blockedSession(t, stateDir)
	be := NewFakeBackend()
	be.ResumeNote = "harness session is gone"
	d := NewDaemon(stateDir, be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	code, body := postEmpty(t, srv.URL+"/api/sessions/"+id+"/compress/dismiss")
	if code != http.StatusConflict || errorText(body) != "harness session is gone" {
		t.Fatalf("dismiss = %d %v, want 409 with the resume note", code, body)
	}
	if sessionMetaOf(t, srv.URL, id).CompressBlock == "" {
		t.Fatal("block cleared although the Session cannot resume")
	}
}

// While blocked, a compress that would wait on a Group answers 409 instead of
// waiting, so no block holds a wait that nothing sends.
func TestBlockedCompressRefusedWhileGroupRuns(t *testing.T) {
	d, _, base, parent := queueDaemon(t, time.Second)
	sendPrompt(t, base, parent, "plan the work")
	waitStatus(t, base, parent, "idle", 5*time.Second)
	sum := &stepSummarizer{steps: []summaryStep{{err: errString("402 Payment Required")}}}
	d.summarizer = sum
	compressReq(t, base, parent, "")
	waitSettled(t, base, parent, hasBlockedEvent)
	if code, out := postSpawn(t, base, parent, map[string]any{"agents": []map[string]any{{"prompt": "job A"}}}); code != http.StatusCreated {
		t.Fatalf("spawn = %d %v", code, out)
	}
	if code, body := compressReq(t, base, parent, ""); code != http.StatusConflict {
		t.Fatalf("compress while blocked and the Group runs = %d %v, want 409", code, body)
	}
	// The Group's result queues behind the block; nothing compresses.
	deadline := time.Now().Add(10 * time.Second)
	for len(queuedTexts(t, base, parent)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Group result never queued behind the block")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := sum.callCount(); n != 1 {
		t.Fatalf("summarizer calls = %d, want 1", n)
	}
	if n := len(eventsOfKind(transcriptOf(t, base, parent), "compressed")); n != 0 {
		t.Fatalf("compressed events = %d, want 0", n)
	}
}

// A block drops a compress that waits on a Group. A stale wait that reaches
// runDeferredCompress on a blocked Session starts nothing either.
func TestBlockCompressDropsDeferredCompress(t *testing.T) {
	d, _, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	sum := &stepSummarizer{steps: []summaryStep{{reply: goodSummary}}}
	d.summarizer = sum
	setWait := func() {
		d.mu.Lock()
		s := d.sessions[id]
		s.compressWait = true
		s.compressInstr = "keep it short"
		d.mu.Unlock()
	}
	waiting := func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.sessions[id].compressWait
	}
	setWait()
	d.blockCompress(id, CompressBlockedEvent{Error: "402 Payment Required"})
	if waiting() {
		t.Fatal("block kept the compress that waited on a Group")
	}
	setWait() // a stale wait reaches the Daemon after the block
	d.runDeferredCompress(id)
	if n := sum.callCount(); n != 0 {
		t.Fatalf("summarizer calls = %d, want 0 on a blocked Session", n)
	}
	if waiting() {
		t.Fatal("stale wait kept on a blocked Session")
	}
}

// A compress that gets a credit failure from the start-handle step blocks and
// leaves the old handle alone.
func TestCompressStartHandleTransientRetriesThenSwaps(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{reply: goodSummary}}}
	be.StartErrs = []error{errString("connection reset by peer"), errString("503 Service Unavailable")}
	rec := &waitRecorder{}
	d.compressSleep = rec.wait
	compressReq(t, base, id, "")
	evs := waitIdleAfter(t, base, id, 1)
	want := []time.Duration{2 * time.Second, 8 * time.Second}
	if fmt.Sprint(rec.got()) != fmt.Sprint(want) {
		t.Fatalf("waits = %v, want %v", rec.got(), want)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"startRetries":2`) {
		t.Fatalf("compressed = %s", c)
	}
	if n := be.ReleasedHandles(id); n != 1 {
		t.Fatalf("released = %d, want 1 after the swap", n)
	}
}

// An auth failure while starting the new handle blocks the Session. The old
// handle and its resume token stay.
func TestCompressStartHandleAuthFailureBlocks(t *testing.T) {
	d, be, base, id := handoffDaemon(t)
	sendPrompt(t, base, id, "hello")
	waitStatus(t, base, id, "idle", 5*time.Second)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{reply: goodSummary}}}
	be.StartErrs = []error{errString("401 Unauthorized")}
	rec := &waitRecorder{}
	d.compressSleep = rec.wait
	before := sessionMetaOf(t, base, id).ResumeToken
	compressReq(t, base, id, "")
	evs := waitSettled(t, base, id, hasBlockedEvent)
	if len(rec.got()) != 0 {
		t.Fatalf("waits = %v, want none", rec.got())
	}
	if n := len(eventsOfKind(evs, "compressed")); n != 0 {
		t.Fatalf("compressed events = %d, want 0", n)
	}
	meta := sessionMetaOf(t, base, id)
	if meta.CompressBlock == "" || meta.ResumeToken != before {
		t.Fatalf("blocked = %q, resume token = %q, want blocked with %q", meta.CompressBlock, meta.ResumeToken, before)
	}
	if n := be.ReleasedHandles(id); n != 0 {
		t.Fatalf("released = %d, want 0", n)
	}
}
