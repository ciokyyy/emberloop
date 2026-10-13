package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// probeBackend wraps the fake Backend, counts lifecycle calls and can pose
// as non-resumable. The seam stays the Daemon HTTP+SSE API.
type probeBackend struct {
	*FakeBackend
	mu       sync.Mutex
	creates  int
	closes   int
	releases int
	resumes  int
	// resumeErr, when set, makes every resume attempt fail.
	resumeErr error
	// resumable controls ResumeOK; reason explains a false.
	resumable bool
	reason    string
	tokens    map[string]string
}

func newProbeBackend() *probeBackend {
	return &probeBackend{FakeBackend: NewFakeBackend(), resumable: true, tokens: map[string]string{}}
}

func (b *probeBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	b.mu.Lock()
	b.creates++
	if b.tokens == nil {
		b.tokens = map[string]string{}
	}
	b.tokens[meta.ID] = "token-" + meta.ID
	b.mu.Unlock()
	return nil
}

func (b *probeBackend) CloseSession(sessionID string) {
	b.mu.Lock()
	b.closes++
	delete(b.tokens, sessionID)
	b.mu.Unlock()
}

func (b *probeBackend) ReleaseSession(sessionID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.releases++
	tok := b.tokens[sessionID]
	if tok == "" {
		tok = "token-" + sessionID
		b.tokens[sessionID] = tok
	}
	return tok
}

func (b *probeBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resumes++
	if b.resumeErr != nil {
		return b.resumeErr
	}
	if b.tokens == nil {
		b.tokens = map[string]string{}
	}
	b.tokens[meta.ID] = meta.ResumeToken
	return nil
}

func (b *probeBackend) ResumeToken(sessionID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens[sessionID]
}

func (b *probeBackend) ResumeOK(meta SessionMeta) (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.resumable {
		return false, b.reason
	}
	return true, ""
}

func (b *probeBackend) counts() (creates, closes, releases, resumes int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.creates, b.closes, b.releases, b.resumes
}

// Sessions are uncapped: starting past the old cap succeeds, since only
// concurrent Run tickets are limited.
func TestSessionsStartUncapped(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), newProbeBackend())
	d.SetMaxTickets(1)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	_ = createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	_ = createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
}

// The ticket cap reads the global max_tickets setting: env wins, then the
// settings file, then the built-in default. The pre-rename max_workers key
// and EL_MAX_WORKERS env still work as fallbacks.
func TestMaxTicketsReadsGlobalSetting(t *testing.T) {
	dir := tempDir(t)
	t.Setenv("HOME", dir)
	t.Setenv("EL_MAX_TICKETS", "")
	t.Setenv("EL_MAX_WORKERS", "")
	t.Setenv("EL_SETTINGS", "")
	if got := maxTicketsFromSettings(); got != 3 {
		t.Fatalf("default cap = %d, want 3", got)
	}
	t.Setenv("EL_MAX_TICKETS", "7")
	if got := maxTicketsFromSettings(); got != 7 {
		t.Fatalf("env cap = %d, want 7", got)
	}
	t.Setenv("EL_MAX_TICKETS", "")
	t.Setenv("EL_MAX_WORKERS", "9")
	if got := maxTicketsFromSettings(); got != 9 {
		t.Fatalf("old env cap = %d, want 9", got)
	}
	t.Setenv("EL_MAX_WORKERS", "")
	settings := `{"max_tickets": 5}`
	if err := os.WriteFile(dir+"/settings.json", []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", dir+"/settings.json")
	if got := maxTicketsFromSettings(); got != 5 {
		t.Fatalf("file cap = %d, want 5", got)
	}
	t.Setenv("EL_MAX_TICKETS", "2")
	if got := maxTicketsFromSettings(); got != 2 {
		t.Fatalf("env-over-file cap = %d, want 2", got)
	}
	t.Setenv("EL_MAX_TICKETS", "")
	if err := os.WriteFile(dir+"/settings.json", []byte(`{"max_workers": 6}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := maxTicketsFromSettings(); got != 6 {
		t.Fatalf("old file key cap = %d, want 6", got)
	}
}

// An idle Session releases its agent process; the next prompt resumes it and
// still answers.
func TestIdleSessionReleasedAndResumed(t *testing.T) {
	t.Parallel()
	be := newProbeBackend()
	d := NewDaemon(tempDir(t), be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "first")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	waitForLive(t, srv.URL, id, false, 5*time.Second)

	if _, _, releases, _ := be.counts(); releases != 1 {
		t.Fatalf("releases = %d, want 1 after idle", releases)
	}

	sendPrompt(t, srv.URL, id, "second")
	events := readSSE(t, srv.URL, id, 5, 4, 5*time.Second)
	found := false
	for _, e := range events {
		if e.Kind == "assistant_chunk" && e.Text == "fake reply to: second" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resumed reply missing in %+v", events)
	}
	if _, _, _, resumes := be.counts(); resumes != 1 {
		t.Fatalf("resumes = %d, want 1 after second prompt", resumes)
	}
	waitForLive(t, srv.URL, id, false, 5*time.Second)
}

// After a Daemon restart a resumable Session accepts a new prompt.
func TestRestartResumesSession(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	be := newProbeBackend()
	d := NewDaemon(dir, be)
	srv := httptest.NewServer(authedHandler(t, d))
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "before")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	srv.Close()

	d2 := NewDaemon(dir, newProbeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	view := sessionView(t, srv2.URL, id)
	if !view.Resumable {
		t.Fatalf("resumable = false after restart, note %q", view.ResumeNote)
	}
	sendPrompt(t, srv2.URL, id, "after")
	events := readSSE(t, srv2.URL, id, 5, 4, 10*time.Second)
	found := false
	for _, e := range events {
		if e.Kind == "assistant_chunk" && e.Text == "fake reply to: after" {
			found = true
		}
	}
	if !found {
		t.Fatalf("post-restart reply missing in %+v", events)
	}
}

// A non-resumable Session keeps readable history, says why, and refuses a
// new prompt.
func TestNonResumableSessionReadOnly(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	be := newProbeBackend()
	d := NewDaemon(dir, be)
	srv := httptest.NewServer(authedHandler(t, d))
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	srv.Close()

	stuck := newProbeBackend()
	stuck.resumable = false
	stuck.reason = "fake harness cannot resume: no saved agent for this Session"
	d2 := NewDaemon(dir, stuck)
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()

	view := sessionView(t, srv2.URL, id)
	if view.Resumable {
		t.Fatal("resumable = true, want false with reason")
	}
	if !strings.Contains(view.ResumeNote, "no saved agent") {
		t.Fatalf("resume note = %q, want the why", view.ResumeNote)
	}
	if len(view.Events) != 5 {
		t.Fatalf("history events = %d, want 5 readable", len(view.Events))
	}

	body, _ := json.Marshal(map[string]string{"text": "again"})
	resp, err := http.Post(srv2.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("prompt on read-only status = %d, want 409", resp.StatusCode)
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Error, "no saved agent") {
		t.Fatalf("read-only error = %q, want the why", out.Error)
	}
}

type sessionViewBody struct {
	Session    SessionMeta `json:"session"`
	Events     []Event     `json:"events"`
	Live       bool        `json:"live"`
	Resumable  bool        `json:"resumable"`
	ResumeNote string      `json:"resumeNote"`
}

func sessionView(t *testing.T, base, id string) sessionViewBody {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session status = %d", resp.StatusCode)
	}
	var out sessionViewBody
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func waitForLive(t *testing.T, base, id string, want bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if sessionView(t, base, id).Live == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("live never became %v", want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
