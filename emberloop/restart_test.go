package el

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunRestartPostsRequest(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := RunRestart([]string{"--url", srv.URL}, &out); err != nil {
		t.Fatal(err)
	}
	if got != "POST /api/restart" {
		t.Fatalf("request = %q", got)
	}
	if !strings.Contains(out.String(), "restart requested") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunRestartRefusesOldDaemon(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	err := RunRestart([]string{"--url", srv.URL}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "el update") {
		t.Fatalf("err = %v, want hint to run el update", err)
	}
}

func TestRestartWaitSeesDaemonReturn(t *testing.T) {
	start := time.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/restart" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		// Down for a moment, then back.
		if time.Since(start) < 1500*time.Millisecond && time.Since(start) > 600*time.Millisecond {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := requestRestart(srv.URL, "", true, &out, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "back") {
		t.Fatalf("output = %q", out.String())
	}
}

// The CLI presents EL_WEB_TOKEN as the Daemon's machine token.
func TestRunRestartSendsMachineToken(t *testing.T) {
	t.Setenv("EL_WEB_TOKEN", "machine-secret")
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	if err := RunRestart([]string{"--url", srv.URL}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer machine-secret" {
		t.Fatalf("Authorization = %q, want the machine token", auth)
	}
}

// A refused token names the fix.
func TestRunRestartNamesTokenWhenRefused(t *testing.T) {
	t.Setenv("EL_WEB_TOKEN", "wrong")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := RunRestart([]string{"--url", srv.URL}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "EL_WEB_TOKEN") {
		t.Fatalf("err = %v, want the EL_WEB_TOKEN hint", err)
	}
}

// Without EL_WEB_TOKEN the CLI reads the token file of the default state dir.
func TestDaemonTokenFallsBackToStateDirFile(t *testing.T) {
	t.Setenv("EL_WEB_TOKEN", "")
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := filepath.Join(xdg, "pig", "enderbloom")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "daemon.token"), []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := restartToken(false); got != "from-file" {
		t.Fatalf("restartToken() = %q, want the file token", got)
	}
}

// --dev reads the token of the dev state dir, EL_WEB_DEV_STATE when set.
func TestDefaultTokenPathForDevUsesDevState(t *testing.T) {
	t.Setenv("EL_WEB_DEV_STATE", "/srv/dev-state")
	if got, want := defaultTokenPath(true), filepath.Join("/srv/dev-state", "enderbloom", "daemon.token"); got != want {
		t.Fatalf("dev token path = %q, want %q", got, want)
	}
	t.Setenv("EL_WEB_DEV_STATE", "")
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	if got, want := defaultTokenPath(true), filepath.Join(xdg, "pig-dev", "enderbloom", "daemon.token"); got != want {
		t.Fatalf("dev token path = %q, want %q", got, want)
	}
}

// el restart prints the Daemon's restart state: when the restart was asked
// and what still blocks the drain.
func TestRunRestartPrintsRestartState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"restart":"requested","new":true,"state":{"requested":true,"since":"2026-10-12T07:38:00Z","drain":"5m0s","sessions":["s1"],"groups":["g1"],"runs":[],"starting":0}}`))
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := RunRestart([]string{"--url", srv.URL}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "restart requested") {
		t.Errorf("output = %q, want the request line", got)
	}
	if !strings.Contains(got, "restart state:") || !strings.Contains(got, "sessions s1") || !strings.Contains(got, "groups g1") {
		t.Errorf("output = %q, want the waiting state", got)
	}
}

// An older Daemon answers 202 without a state; el restart still prints the
// request line and skips the state.
func TestRunRestartWithoutStateStillPrints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := RunRestart([]string{"--url", srv.URL}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "restart requested") || strings.Contains(got, "restart state:") {
		t.Errorf("output = %q, want only the request line", got)
	}
}
