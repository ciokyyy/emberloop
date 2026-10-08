package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Slice 1: create a Session, send a prompt, read Events over SSE in order
// with sequence numbers.
func TestCreatePromptStreamsOrderedEvents(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	seq := sendPrompt(t, srv.URL, id, "hello")
	if seq < 1 {
		t.Fatalf("prompt event seq = %d, want >= 1", seq)
	}

	events := readSSE(t, srv.URL, id, 0, 4, 5*time.Second)
	for i := 1; i < len(events); i++ {
		if events[i].Seq != events[i-1].Seq+1 {
			t.Fatalf("events out of order: %+v", events)
		}
	}
	foundPrompt := false
	for _, e := range events {
		if e.Kind == "prompt" && strings.Contains(e.Text, "hello") {
			foundPrompt = true
		}
	}
	if !foundPrompt {
		t.Fatalf("prompt event missing in %+v", events)
	}
}

func createSession(t *testing.T, base, harness, model, workDir string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"harness": harness, "model": model, "workDir": workDir})
	resp, err := http.Post(base+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" {
		t.Fatal("empty session id")
	}
	return out.ID
}

func sendPrompt(t *testing.T, base, id, text string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	resp, err := http.Post(base+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("prompt status = %d", resp.StatusCode)
	}
	var out struct {
		Seq int64 `json:"seq"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Seq
}

func readSSE(t *testing.T, base, id string, since int64, want int, timeout time.Duration) []Event {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/sessions/"+id+"/events?since="+itoa(since), nil)
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Timeout: timeout + 5*time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", resp.StatusCode)
	}
	var events []Event
	sc := bufio.NewScanner(resp.Body)
	var data string
	deadline := time.Now().Add(timeout)
	for len(events) < want && time.Now().Before(deadline) {
		// Nudge the scanner with a read deadline via goroutine-free poll:
		// scanner blocks, so run Do in background is overkill here since
		// the fake Backend emits within milliseconds.
		if !sc.Scan() {
			break
		}
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			var e Event
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Fatal(err)
			}
			events = append(events, e)
		}
	}
	if len(events) < want {
		t.Fatalf("got %d events, want %d: %+v", len(events), want, events)
	}
	return events
}

func itoa(n int64) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(u64toa(uint64(n)), " ", ""), "\n", ""))
}

func u64toa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// Slice 2: reconnecting with a last-seen sequence number delivers only the
// missed Events.
func TestReconnectSinceDeliversOnlyMissed(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	sendPrompt(t, srv.URL, id, "first")
	all := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	last := all[len(all)-1].Seq

	sendPrompt(t, srv.URL, id, "second")
	missed := readSSE(t, srv.URL, id, last, 4, 5*time.Second)
	for _, e := range missed {
		if e.Seq <= last {
			t.Fatalf("replayed seen event %+v (last %d)", e, last)
		}
	}
	found := false
	for _, e := range missed {
		if e.Kind == "assistant_chunk" && e.Text == "fake reply to: second" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missed chunk %+v", missed)
	}
}

// Slice 3: reloading replays the whole log and shows the same transcript.
func TestReloadReplaysWholeLog(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	sendPrompt(t, srv.URL, id, "hello")
	first := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)

	// Reload: whole-log replay through the session GET plus SSE from zero.
	resp, err := http.Get(srv.URL + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Events []Event `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	second := readSSE(t, srv.URL, id, 0, len(first), 5*time.Second)
	if len(body.Events) != len(first) || len(second) != len(first) {
		t.Fatalf("reload mismatch: first %d body %d sse %d", len(first), len(body.Events), len(second))
	}
	for i := range first {
		if first[i].Seq != second[i].Seq || first[i].Kind != second[i].Kind || first[i].Text != second[i].Text {
			t.Fatalf("reload differs at %d: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// Slice 4: a request with a foreign Origin or Host is rejected.
func TestForeignOriginOrHostRejected(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/sessions", nil)
	req.Header.Set("Origin", "http://evil.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Origin status = %d, want 403", resp.StatusCode)
	}

	req, _ = http.NewRequest("GET", srv.URL+"/api/sessions", nil)
	req.Host = "evil.com"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Host status = %d, want 403", resp.StatusCode)
	}
}

// Slice 5: restarting the Daemon keeps the Session readable as history.
func TestRestartKeepsHistory(t *testing.T) {
	dir := t.TempDir()
	d := NewDaemon(dir, NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	sendPrompt(t, srv.URL, id, "hello")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	srv.Close()

	d2 := NewDaemon(dir, NewFakeBackend())
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	resp, err := http.Get(srv2.URL + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", resp.StatusCode)
	}
	var body struct {
		Events []Event `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 5 {
		t.Fatalf("history events = %d, want 5", len(body.Events))
	}
}

// Slice 6: starting the Daemon serves the embedded UI on loopback only.
func TestServesEmbeddedUI(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ui status = %d", resp.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "Sessions") {
		t.Fatalf("ui body missing Sessions marker")
	}
}
