package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An unstamped Daemon says dev with an empty UI hash: the test binary
// carries no ldflags stamps, so the version falls back to the placeholder
// and the UI hash to nothing (which the UI reads as unknown, never stale).
func TestCurrentDefaults(t *testing.T) {
	oldV, oldC, oldH := Version, Commit, UIHash
	Version, Commit, UIHash = "", "", ""
	defer func() { Version, Commit, UIHash = oldV, oldC, oldH }()
	c := Current()
	if c.Version != "dev" {
		t.Errorf("unstamped version = %q, want dev", c.Version)
	}
	if c.Commit == "" {
		t.Error("unstamped commit must not be blank")
	}
	if c.UIHash != "" {
		t.Errorf("unstamped uiHash = %q, want empty", c.UIHash)
	}
}

// Stamped values always win, and the commit arrives short.
func TestCurrentPrefersStamps(t *testing.T) {
	oldV, oldC, oldH := Version, Commit, UIHash
	Version, Commit, UIHash = "v1.2.3", "abc1234def5678", "0123456789ab"
	defer func() { Version, Commit, UIHash = oldV, oldC, oldH }()
	c := Current()
	if c.Version != "v1.2.3" || c.Commit != "abc1234" || c.UIHash != "0123456789ab" {
		t.Errorf("stamped Current = %+v", c)
	}
}

// GET /api/version answers the build identity with no login and no state;
// anything else is 405.
func TestVersionEndpoint(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("version status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"version", "commit", "uiHash"} {
		v, ok := body[key].(string)
		if !ok {
			t.Errorf("version body missing string %q: %v", key, body)
			continue
		}
		if key != "uiHash" && v == "" {
			t.Errorf("version body %q must not be blank", key)
		}
	}

	post, err := http.Post(srv.URL+"/api/version", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("version POST status = %d, want 405", post.StatusCode)
	}
}

// GET /api/health is the update gate's probe: 200 with ok true; anything
// else is 405.
func TestHealthEndpoint(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK {
		t.Error("health body ok = false, want true")
	}

	post, err := http.Post(srv.URL+"/api/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("health POST status = %d, want 405", post.StatusCode)
	}
}
