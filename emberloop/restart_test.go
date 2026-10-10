package el

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
	if err := requestRestart(srv.URL, true, &out, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "back") {
		t.Fatalf("output = %q", out.String())
	}
}
