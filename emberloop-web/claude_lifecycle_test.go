package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Release captures the ACP session id and drops the adapter; ResumeSession
// reloads it through session/load with the full stripping set.
func TestClaudeReleaseAndResumeRoundTrip(t *testing.T) {
	t.Parallel()
	stub := &stubConn{}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t)}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	if tok := be.ResumeToken("d1"); tok != "acp-session-1" {
		t.Fatalf("resume token = %q, want acp-session-1", tok)
	}
	if ok, _ := be.ResumeOK(SessionMeta{ResumeToken: "acp-session-1"}); !ok {
		t.Fatal("ResumeOK = false with a token")
	}
	if ok, reason := be.ResumeOK(SessionMeta{}); ok || !strings.Contains(reason, "read-only") {
		t.Fatalf("ResumeOK = %v, %q without a token", ok, reason)
	}
	if tok := be.ReleaseSession("d1"); tok != "acp-session-1" {
		t.Fatalf("release token = %q, want acp-session-1", tok)
	}
	be.mu.Lock()
	left := len(be.sessions)
	be.mu.Unlock()
	if left != 0 {
		t.Fatalf("backend sessions after release = %d, want 0", left)
	}
	meta.ResumeToken = "acp-session-1"
	if err := be.ResumeSession(meta, func(kind, text, status string) {}); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.loadID != "acp-session-1" {
		t.Fatalf("load session id = %q, want acp-session-1", stub.loadID)
	}
	mustStripped(t, stub.loadMeta, "sonnet")
}

// The full lifecycle through the API with the stub adapter: create, prompt,
// idle release, resume on the next prompt, and resume after a restart.
func TestClaudeLifecycleOverAPI(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	stub := &stubConn{updates: []map[string]any{chunk("hello")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(dir, NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "first")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	waitForLive(t, srv.URL, id, false, 5*time.Second)

	be.mu.Lock()
	left := len(be.sessions)
	be.mu.Unlock()
	if left != 0 {
		t.Fatalf("adapter sessions after idle = %d, want 0", left)
	}

	sendPrompt(t, srv.URL, id, "second")
	// Six, not four: the start seed plus the first turn's fallback reading
	// push the resumed reply two events later.
	events := readSSE(t, srv.URL, id, 5, 6, 5*time.Second)
	found := false
	for _, e := range events {
		if e.Kind == "assistant_chunk" && e.Text == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resumed reply missing in %+v", events)
	}
	stub.mu.Lock()
	prompts := append([]string{}, stub.prompts...)
	stub.mu.Unlock()
	if len(prompts) != 2 {
		t.Fatalf("adapter prompts = %v, want both turns", prompts)
	}
	waitForLive(t, srv.URL, id, false, 5*time.Second)
	srv.Close()

	be2 := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d2 := NewDaemon(dir, NewFakeBackend()).WithClaudeBackend(be2)
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if view := sessionView(t, srv2.URL, id); !view.Resumable {
		t.Fatalf("resumable = false after restart, note %q", view.ResumeNote)
	}
	sendPrompt(t, srv2.URL, id, "third")
	after := readSSE(t, srv2.URL, id, 9, 4, 5*time.Second)
	found = false
	for _, e := range after {
		if e.Kind == "assistant_chunk" && e.Text == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("post-restart reply missing in %+v", after)
	}
}
