package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The approval request to the Daemon presents the machine token it was given.
func TestSudoGatePresentsMachineToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"r1"}`))
	}))
	defer srv.Close()
	gate := HTTPSudoGate(srv.URL, "s1", "machine-secret")
	if _, handled := gate(context.Background(), "/work", "sudo ls"); !handled {
		t.Fatal("gate did not take the command")
	}
	if auth != "Bearer machine-secret" {
		t.Fatalf("Authorization = %q, want the machine token", auth)
	}
}
