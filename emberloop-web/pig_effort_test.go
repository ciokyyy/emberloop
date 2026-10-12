package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The pig effort levels are the pig --thinking levels, with empty keeping
// the model default.
func TestValidPigEffort(t *testing.T) {
	t.Parallel()
	for _, e := range append([]string{""}, pigEfforts...) {
		if !validPigEffort(e) {
			t.Errorf("validPigEffort(%q) = false, want true", e)
		}
	}
	for _, e := range []string{"default", "ultra", "MAX", "Low", "none"} {
		if validPigEffort(e) {
			t.Errorf("validPigEffort(%q) = true, want false", e)
		}
	}
}

// An unknown pig effort rejects the create like an unknown model does.
func TestPigUnknownEffortRejected(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"harness": "pig", "model": pigFauxModel, "workDir": tempDir(t), "effort": "ultra"})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400", resp.StatusCode)
	}
}

// A known pig effort is accepted and kept on the Session.
func TestPigEffortAcceptedAndKept(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"harness": "pig", "model": pigFauxModel, "workDir": tempDir(t), "effort": "high"})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var out struct {
		ID     string `json:"id"`
		Effort string `json:"effort"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Effort != "high" {
		t.Fatalf("created session = %+v (%v)", out, err)
	}
	meta, _ := sessionMeta(t, srv.URL, out.ID)
	if meta.Effort != "high" {
		t.Fatalf("session effort = %q, want high", meta.Effort)
	}
}

// Effort validation runs before any SDK stack is built: a bad effort fails
// fast, and a good effort with a bad model still fails on the model.
func TestPigCreateSessionValidatesEffortFirst(t *testing.T) {
	t.Parallel()
	pb := NewPigBackend()
	emit := func(kind, text, status string) {}
	if err := pb.CreateSession(SessionMeta{ID: "effort-bad", Model: pigFauxModel, WorkDir: tempDir(t), Effort: "ultra"}, emit); err == nil {
		t.Fatal("bad effort accepted")
	}
	if err := pb.CreateSession(SessionMeta{ID: "effort-model", Model: "nope/nonexistent-1", WorkDir: tempDir(t), Effort: "high"}, emit); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("good effort with bad model err = %v", err)
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if len(pb.sessions) != 0 {
		t.Fatalf("failed creates left %d sessions", len(pb.sessions))
	}
}
