package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var errNoAdapter = errors.New("no adapter")

// A claude Session created through the API answers over SSE with normalized
// Events; unknown claude models are refused at creation.
func TestClaudeSessionOverAPIStreamsEvents(t *testing.T) {
	stub := &stubConn{updates: []map[string]any{chunk("real answer")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(t.TempDir(), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", t.TempDir())
	sendPrompt(t, srv.URL, id, "hello")
	events := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Kind] = true
	}
	for _, want := range []string{"prompt", "assistant_chunk", "status"} {
		if !seen[want] {
			t.Fatalf("missing %s in %+v", want, events)
		}
	}
}

func TestClaudeSessionCreationValidates(t *testing.T) {
	stub := &stubConn{}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(t.TempDir(), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"harness": "claude", "model": "gpt-9", "workDir": t.TempDir()})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad model status = %d, want 400", resp.StatusCode)
	}

	plain := NewDaemon(t.TempDir(), NewFakeBackend())
	srv2 := httptest.NewServer(plain.Handler())
	defer srv2.Close()
	body, _ = json.Marshal(map[string]string{"harness": "claude", "model": "sonnet", "workDir": t.TempDir()})
	resp, err = http.Post(srv2.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unwired claude status = %d, want 500", resp.StatusCode)
	}
}

// A claude start failure answers 500 and leaves no Session or backend
// state behind.
func TestClaudeStartFailureRemovesSession(t *testing.T) {
	be := newStubClaudeBackend(func() (acpConn, error) { return &stubConn{newErr: errNoAdapter}, nil })
	d := NewDaemon(t.TempDir(), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"harness": "claude", "model": "sonnet", "workDir": t.TempDir()})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("failed start status = %d, want 500", resp.StatusCode)
	}
	list, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	var metas []SessionMeta
	if err := json.NewDecoder(list.Body).Decode(&metas); err != nil {
		list.Body.Close()
		t.Fatal(err)
	}
	list.Body.Close()
	if len(metas) != 0 {
		t.Fatalf("sessions after failed start = %v", metas)
	}
	be.mu.Lock()
	leftover := len(be.sessions)
	be.mu.Unlock()
	if leftover != 0 {
		t.Fatalf("backend sessions after failed start = %d", leftover)
	}
}

func TestModelsEndpointOffersClaudeAliases(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	for harness, want := range map[string][]string{
		"claude": {"opus", "sonnet", "haiku"},
		"fake":   {"fake-model"},
	} {
		resp, err := http.Get(srv.URL + "/api/models?harness=" + harness)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Models []string `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if strings.Join(out.Models, ",") != strings.Join(want, ",") {
			t.Fatalf("harness %s models = %v, want %v", harness, out.Models, want)
		}
	}
	resp, err := http.Get(srv.URL + "/api/models?harness=nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown harness status = %d, want 400", resp.StatusCode)
	}
}
