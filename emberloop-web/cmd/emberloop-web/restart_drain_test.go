package main

// The drain deadline forces the restart regardless of Idle: a Session that
// never settles delays the restart but cannot wedge it. These tests drive
// waitRestartIdle against a real Daemon: an idle one returns at once, one
// mid-turn through a slow fake Backend returns forced past a short drain.
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	web "example.com/emberloop-web"
	"example.com/emberloop-web/testsupport"
)

func TestWaitRestartIdleSettlesOnIdleDaemon(t *testing.T) {
	d := web.NewDaemon(filepath.Join(testsupport.TempDir(t), "state"), web.NewFakeBackend())
	if forced := waitRestartIdle(d, time.Second, 5*time.Millisecond); forced {
		t.Fatal("forced = true on an idle Daemon, want an immediate false")
	}
}

func TestWaitRestartIdleForcesPastBusySession(t *testing.T) {
	dir := testsupport.TempDir(t)
	fb := web.NewFakeBackend()
	fb.Delay = 5 * time.Second
	d := web.NewDaemon(filepath.Join(dir, "state"), fb)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	post := func(path string, body any) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := client.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := post("/api/auth/signup", map[string]string{"username": "admin", "password": "correct horse battery"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup = %d, want 201", resp.StatusCode)
	}
	resp = post("/api/sessions", map[string]string{"harness": "fake", "model": "fake-model", "workDir": dir})
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.ID == "" {
		t.Fatal("empty session id")
	}
	// The slow fake Backend holds this turn open past the drain below.
	resp = post("/api/sessions/"+created.ID+"/prompt", map[string]string{"text": "hello"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("prompt = %d, want 202", resp.StatusCode)
	}

	start := time.Now()
	if forced := waitRestartIdle(d, 200*time.Millisecond, 5*time.Millisecond); !forced {
		t.Fatal("forced = false with a turn running past the drain, want true")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("wait took %s, want about the drain", elapsed)
	}
}
