package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// createPigSessionWithEffort creates a pig Session with an effort level and
// returns its ID.
func createPigSessionWithEffort(t *testing.T, base, model, workDir, effort string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"harness": "pig", "model": model, "workDir": workDir, "effort": effort})
	resp, err := http.Post(base+"/api/sessions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
		t.Fatalf("create = %+v (%v)", out, err)
	}
	return out.ID
}

// compressibleOf reads the compressible flag of a Session through the Daemon API.
func compressibleOf(t *testing.T, base, id string) bool {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Compressible bool `json:"compressible"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Compressible
}

// A pig SDK Session compresses by Handoff restart: the in-place compact path
// is gone. Thinking level and model survive the new handle, the resume token
// rotates, and a Daemon restart then resumes the new handle.
func TestPigSDKCompressesByHandoff(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":false}`)
	stateDir := tempDir(t)
	agentDir := tempDir(t)
	pb := newPigTestBackendAt(t, agentDir)
	d := NewDaemon(stateDir, NewFakeBackend())
	d.RegisterBackend("pig", pb)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createPigSessionWithEffort(t, srv.URL, pigFauxModel, tempDir(t), "high")
	sendPrompt(t, srv.URL, id, "build the thing")
	waitStatus(t, srv.URL, id, "idle", 15*time.Second)

	if !compressibleOf(t, srv.URL, id) {
		t.Fatal("pig SDK Session not compressible, want Handoff")
	}
	d.mu.Lock()
	metaBefore := d.sessions[id].meta
	d.mu.Unlock()
	if metaBefore.Model != pigFauxModel || metaBefore.Effort != "high" {
		t.Fatalf("meta before = %+v, want model %q effort high", metaBefore, pigFauxModel)
	}
	beforeToken := metaBefore.ResumeToken

	if code, body := compressReq(t, srv.URL, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v, want 202", code, body)
	}
	evs := waitIdleAfter(t, srv.URL, id, 1)

	// Handoff prompt flagged, not a user message.
	hands := eventsOfKind(evs, "prompt")
	last := hands[len(hands)-1]
	if !last.Handoff || last.Sender != "system" {
		t.Fatalf("handoff flags = %+v", last)
	}
	// Compressed Event names the model and the summarizer step.
	c := eventsOfKind(evs, "compressed")
	if len(c) != 1 || !strings.Contains(c[0].Text, `"model":"test-faux/faux-1"`) {
		t.Fatalf("compressed = %+v", c)
	}
	// Token rotated; model, effort and owner unchanged.
	d.mu.Lock()
	metaAfter := d.sessions[id].meta
	d.mu.Unlock()
	if metaAfter.ResumeToken == "" || metaAfter.ResumeToken == beforeToken {
		t.Fatalf("token %q -> %q, want rotated", beforeToken, metaAfter.ResumeToken)
	}
	if metaAfter.Model != pigFauxModel || metaAfter.Effort != "high" || metaAfter.Owner != metaBefore.Owner {
		t.Fatalf("meta after = %+v, want model, effort and owner kept", metaAfter)
	}
	// Handoff copy stored in the Session directory matches the prompt.
	raw, err := os.ReadFile(filepath.Join(d.sessionDir(id), "handoff-1.md"))
	if err != nil || string(raw) != last.Text {
		t.Fatalf("handoff copy err=%v match=%v", err, string(raw) == last.Text)
	}

	// A Daemon restart resumes the new handle. Release both stacks before
	// TempDir cleanup so no stall-watchdog goroutines leak into later tests.
	seq := lastSeqOf(sessionEvents(t, srv.URL, id))
	srv.Close()
	pb2 := newPigTestBackendAt(t, agentDir)
	d2 := NewDaemon(stateDir, NewFakeBackend())
	d2.RegisterBackend("pig", pb2)
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if view := sessionView(t, srv2.URL, id); !view.Resumable {
		t.Fatalf("resumable = false after Handoff restart, note %q", view.ResumeNote)
	}
	sendPrompt(t, srv2.URL, id, "What is 20+22?")
	after := readSSE(t, srv2.URL, id, seq, 4, 30*time.Second)
	if !hasChunkWith(after, "42") {
		t.Fatalf("post-restart answer 42 missing in %+v", after)
	}
	meta, _ := sessionMeta(t, srv2.URL, id)
	if meta.Model != pigFauxModel || meta.Effort != "high" {
		t.Fatalf("meta after restart = %+v, want model and effort kept", meta)
	}
	pb.CloseSession(id)
	pb2.CloseSession(id)
}

// A pig-over-ACP Session compresses by Handoff restart and reports
// compressible. Thinking level and model survive the new handle, the resume
// token rotates, and a Daemon restart then resumes the new handle.
func TestPigACPCompressesByHandoff(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":false}`)
	stateDir := tempDir(t)
	model := oneCatalogModel(t)

	stub := &thoughtStubConn{}
	stub.updates = []map[string]any{chunk("real answer")}
	be := newThoughtStubBackend(stub)
	d := NewDaemon(stateDir, NewFakeBackend())
	d.RegisterBackend("pig", be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createPigSessionWithEffort(t, srv.URL, model, tempDir(t), "high")
	sendPrompt(t, srv.URL, id, "hello")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	waitForLive(t, srv.URL, id, false, 5*time.Second)

	if !compressibleOf(t, srv.URL, id) {
		t.Fatal("pig ACP Session not compressible, want Handoff")
	}
	before := sessionMetaOf(t, srv.URL, id).ResumeToken
	if before == "" {
		t.Fatal("no resume token before compress")
	}

	if code, body := compressReq(t, srv.URL, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v, want 202", code, body)
	}
	evs := waitIdleAfter(t, srv.URL, id, 1)

	hands := eventsOfKind(evs, "prompt")
	last := hands[len(hands)-1]
	if !last.Handoff || last.Sender != "system" {
		t.Fatalf("handoff flags = %+v", last)
	}
	if n := len(eventsOfKind(evs, "compressed")); n != 1 {
		t.Fatalf("compressed events = %d, want 1", n)
	}
	meta := sessionMetaOf(t, srv.URL, id)
	if meta.ResumeToken == "" || meta.ResumeToken == before {
		t.Fatalf("token %q -> %q, want rotated", before, meta.ResumeToken)
	}
	if meta.Model != model || meta.Effort != "high" {
		t.Fatalf("meta after = %+v, want model %q effort high", meta, model)
	}
	// Thinking level applied at create, at any idle resume, and at the
	// Handoff swap: every level is high and the last targets the new handle.
	stub.mu.Lock()
	levels := append([]string(nil), stub.levels...)
	stub.mu.Unlock()
	if len(levels) < 2 || levels[0] != "acp-session-1:high" || levels[len(levels)-1] != "acp-session-2:high" {
		t.Fatalf("thought levels = %v, want first acp-session-1:high and last acp-session-2:high", levels)
	}
	for _, l := range levels {
		if l != "acp-session-1:high" && l != "acp-session-2:high" {
			t.Fatalf("thought levels = %v, want only high", levels)
		}
	}

	// A Daemon restart resumes the new handle.
	srv.Close()
	stub2 := &thoughtStubConn{stubConn: stubConn{updates: []map[string]any{chunk("real answer")}}}
	be2 := newThoughtStubBackend(stub2)
	d2 := NewDaemon(stateDir, NewFakeBackend())
	d2.RegisterBackend("pig", be2)
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if view := sessionView(t, srv2.URL, id); !view.Resumable {
		t.Fatalf("resumable = false after Handoff restart, note %q", view.ResumeNote)
	}
	sendPrompt(t, srv2.URL, id, "third")
	after := readSSE(t, srv2.URL, id, lastSeqOf(sessionEvents(t, srv2.URL, id))-4, 4, 5*time.Second)
	found := false
	for _, e := range after {
		if e.Kind == "assistant_chunk" && e.Text == "real answer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("post-restart reply missing in %+v", after)
	}
	stub2.mu.Lock()
	loadID := stub2.loadID
	stub2.mu.Unlock()
	if loadID != meta.ResumeToken {
		t.Fatalf("resumed ACP session = %q, want %q (the Handoff token)", loadID, meta.ResumeToken)
	}
}

// Each pig-over-ACP handle gets its own steer socket, so a Handoff swap never
// shares one: the new shim cannot steal the Listen and the old shim's exit
// cannot unlink the new shim's live socket.
func TestPigACPSocksRotateAcrossHandoff(t *testing.T) {
	model := oneCatalogModel(t)
	stub := &thoughtStubConn{}
	be := newThoughtStubBackend(stub)
	meta := SessionMeta{ID: "sock-rotate", Model: model, WorkDir: tempDir(t), Effort: "high"}
	emit := func(kind, text, status string) {}
	if err := be.CreateSession(meta, emit); err != nil {
		t.Fatalf("create: %v", err)
	}
	first := be.steerSock(meta.ID)
	if first == "" || first == legacySteerSock(meta.ID) {
		t.Fatalf("first sock = %q, want unique", first)
	}
	tok, release, err := be.StartHandle(meta, emit)
	if err != nil {
		t.Fatalf("StartHandle: %v", err)
	}
	if tok == "" {
		t.Fatal("empty Handoff token")
	}
	second := be.steerSock(meta.ID)
	if second == "" || second == first {
		t.Fatalf("second sock = %q, first = %q, want distinct", second, first)
	}
	// Releasing the old handle must not disturb the new socket mapping.
	release()
	if got := be.steerSock(meta.ID); got != second {
		t.Fatalf("sock after old release = %q, want %q", got, second)
	}
	be.CloseSession(meta.ID)
}

// Queued native messages move to the new pig-over-ACP handle as follow-ups:
// a held steer degrades so the Handoff leads.
func TestPigACPCarryDegradesSteerToFollowup(t *testing.T) {
	oldSock := filepath.Join(tempDir(t), "old.sock")
	newSock := filepath.Join(tempDir(t), "new.sock")
	serveQueue := func(sock string, q pigQueue, received *[]string) func() {
		ln, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatalf("listen %s: %v", sock, err)
		}
		done := make(chan struct{})
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					var req struct {
						Op      string `json:"op"`
						Message string `json:"message"`
					}
					_ = json.NewDecoder(c).Decode(&req)
					switch req.Op {
					case "queue":
						_ = json.NewEncoder(c).Encode(map[string]any{"success": true, "data": q})
					case "follow_up":
						if received != nil {
							*received = append(*received, req.Message)
						}
						_ = json.NewEncoder(c).Encode(map[string]any{"success": true, "data": map[string]any{"disposition": "queued"}})
					case "steer":
						_ = json.NewEncoder(c).Encode(map[string]any{"success": true, "data": map[string]any{"disposition": "queued"}})
					}
				}(c)
			}
		}()
		return func() { close(done); ln.Close() }
	}
	var got []string
	stopOld := serveQueue(oldSock, pigQueue{Steering: []string{"go left"}, FollowUp: []string{"later"}}, nil)
	defer stopOld()
	stopNew := serveQueue(newSock, pigQueue{}, &got)
	defer stopNew()
	q := readSockQueue(oldSock)
	if len(q.Steering) != 1 || len(q.FollowUp) != 1 {
		t.Fatalf("old queue = %+v, want one steer and one follow-up", q)
	}
	carrySockQueue(newSock, q)
	if len(got) != 2 || got[0] != "go left" || got[1] != "later" {
		t.Fatalf("carried = %q, want both as follow-ups in order", got)
	}
}

// The pig SDK swap degrades held steers to follow-ups so the Handoff leads.
func TestPigSDKCarryDegradesSteerToFollowup(t *testing.T) {
	pb := newPigTestBackend(t)
	meta := SessionMeta{ID: "degrade", Model: pigFauxModel, WorkDir: tempDir(t), Effort: "high"}
	emit := func(kind, text, status string) {}
	if err := pb.CreateSession(meta, emit); err != nil {
		t.Fatalf("create: %v", err)
	}
	pb.Steer(meta.ID, "go left")
	pb.FollowUp(meta.ID, "later")
	if _, _, err := pb.StartHandle(meta, emit); err != nil {
		t.Fatalf("StartHandle: %v", err)
	}
	queued := pb.QueuedMessages(meta.ID)
	if len(queued) != 2 {
		t.Fatalf("queued = %+v, want 2", queued)
	}
	for _, q := range queued {
		if q.Mode != "followup" {
			t.Fatalf("queued = %+v, want all followup after degrade", queued)
		}
	}
	pb.CloseSession(meta.ID)
}

// failingSteerBackend is a FakeBackend whose native pig queue always misses,
// so routeLiveOpt must fall back to the Daemon pending list instead of
// dropping the message.
type failingSteerBackend struct {
	*FakeBackend
}

func (f *failingSteerBackend) Steer(sessionID, text string) bool               { return false }
func (f *failingSteerBackend) FollowUp(sessionID, text string) bool            { return false }
func (f *failingSteerBackend) QueuedMessages(sessionID string) []QueuedMessage { return nil }
func (f *failingSteerBackend) ClearQueued(sessionID string) int                { return 0 }

// A pig steer that misses the native queue waits in the Daemon pending list
// and is not lost.
func TestPigSteerMissFallsBackToPending(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	be := &failingSteerBackend{FakeBackend: NewFakeBackend()}
	be.Delay = time.Second
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.RegisterBackend("pig", be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "pig", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	if code, out := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer = %d %v", code, out)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 1 {
		t.Fatalf("queue = %+v, want the missed steer in pending", q)
	}
	evs := waitSettled(t, srv.URL, id, func(evs []Event) bool {
		_, ok := promptWith(evs, "go left", "steer")
		return ok
	})
	if _, ok := promptWith(evs, "go left", "steer"); !ok {
		t.Fatalf("missed steer never delivered: %+v", evs)
	}
}

// A pig steer that arrives while a Handoff swap runs waits in pending as a
// degraded follow-up, so the Handoff leads and nothing is lost on the dying
// handle.
func TestPigSteerDuringHandoffQueuesFollowup(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	be := NewFakeBackend()
	be.Delay = time.Second
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.RegisterBackend("pig", be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "pig", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	d.mu.Lock()
	d.sessions[id].swapping = true
	d.mu.Unlock()
	if code, out := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer = %d %v", code, out)
	}
	q := getQueue(t, srv.URL, id)
	if len(q) != 1 || q[0].Mode != "followup" {
		t.Fatalf("queue = %+v, want one degraded followup while swapping", q)
	}
	d.mu.Lock()
	d.sessions[id].swapping = false
	d.mu.Unlock()
	be.Delay = 0
	waitSettled(t, srv.URL, id, func(evs []Event) bool {
		_, ok := promptWith(evs, "go left", "followup")
		return ok
	})
}
