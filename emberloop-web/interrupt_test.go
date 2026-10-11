package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func postInterrupt(t *testing.T, base, id string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(base+"/api/sessions/"+id+"/interrupt", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// Interrupt stops a working pig Session, drops what was queued behind the
// turn, and leaves the Session idle and usable.
func TestInterruptStopsWorkingPigSession(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	startChainedSlowTurns(t, srv.URL, id)
	if code, _ := postMode(t, srv.URL, id, "followup", "never runs"); code != http.StatusAccepted {
		t.Fatalf("followup status = %d", code)
	}
	code, body := postInterrupt(t, srv.URL, id)
	if code != http.StatusOK || body["interrupted"] != true {
		t.Fatalf("interrupt = %d %v, want 200 interrupted", code, body)
	}
	waitStatus(t, srv.URL, id, "idle", 30*time.Second)
	if q := getQueue(t, srv.URL, id); len(q) != 0 {
		t.Fatalf("queue after interrupt = %v, want empty", q)
	}
	// Settle the backend before TempDir cleanup: log writes outlive the
	// test otherwise ("directory not empty").
	pb.CloseSession(id)
}

func TestInterruptIdleSessionReportsFalse(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	code, body := postInterrupt(t, srv.URL, id)
	if code != http.StatusOK || body["interrupted"] != false {
		t.Fatalf("idle interrupt = %d %v, want 200 false", code, body)
	}
	if code, _ := postInterrupt(t, srv.URL, "nope"); code != http.StatusNotFound {
		t.Fatalf("unknown Session = %d, want 404", code)
	}
}

// A Steer with images reaches the pig native queue instead of degrading to a
// Follow-up in the Daemon pending list.
func TestPigSteerWithImagesQueuesNatively(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	startChainedSlowTurns(t, srv.URL, id)
	body, _ := json.Marshal(map[string]any{"text": "look", "mode": "steer",
		"images": []Image{{Mime: "image/png", Data: "iVBORw0KGgo="}}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	for _, e := range sessionEvents(t, srv.URL, id) {
		if e.Kind == "prompt" && e.Text == "look" && e.Mode != "steer" {
			t.Fatalf("image steer labelled %q, want steer", e.Mode)
		}
	}
	// Let the chained turns and the queued image steer settle before
	// TempDir cleanup: background image and log writes outlive the test
	// otherwise ("directory not empty").
	waitStatus(t, srv.URL, id, "idle", 60*time.Second)
	pb.CloseSession(id)
}
