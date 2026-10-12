package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Slice 1: create a Session, send a prompt, read Events over SSE in order
// with sequence numbers.
func TestCreatePromptStreamsOrderedEvents(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/sessions/"+id+"/events?since="+strconv.FormatInt(since, 10), nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", resp.StatusCode)
	}
	// Scan in a goroutine: the scanner blocks on a live stream, so the
	// deadline must live in this select, not in the read loop.
	lines := make(chan string)
	scanErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		scanErr <- sc.Err()
	}()
	var events []Event
	for len(events) < want {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out with %d of %d events", len(events), want)
		case err := <-scanErr:
			if err != nil {
				t.Fatalf("stream read: %v", err)
			}
			t.Fatalf("stream closed with %d of %d events", len(events), want)
		case line := <-lines:
			if strings.HasPrefix(line, "data: ") {
				var e Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
					t.Fatal(err)
				}
				events = append(events, e)
			}
		}
	}
	return events
}

// Slice 2: reconnecting with a last-seen sequence number delivers only the
// missed Events.
func TestReconnectSinceDeliversOnlyMissed(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
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
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
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
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
	dir := tempDir(t)
	d := NewDaemon(dir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	srv.Close()

	d2 := NewDaemon(dir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
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
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
	if !strings.Contains(buf.String(), "Emberloop") {
		t.Fatalf("ui body missing Emberloop marker")
	}
	if !strings.Contains(buf.String(), "assets/") {
		t.Fatalf("ui body missing static bundle assets")
	}
}

// Review gaps: validation, not-found, ordering, malformed log lines,
// Last-Event-ID header and finite replay.
func TestValidationAndNotFound(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	for _, body := range []string{
		`{}`,
		`{"harness":"fake"}`,
		`not json`,
	} {
		resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("create %q status = %d, want 400", body, resp.StatusCode)
		}
	}

	resp, err := http.Post(srv.URL+"/api/sessions/nope/prompt", "application/json", strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("prompt unknown status = %d, want 404", resp.StatusCode)
	}

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	resp, err = http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", strings.NewReader(`{"text":""}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty prompt status = %d, want 400", resp.StatusCode)
	}

	for _, path := range []string{"/api/sessions/nope", "/api/sessions/nope/events?wait=0"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestListNewestActivityFirst(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	first := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	time.Sleep(10 * time.Millisecond)
	second := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

	resp, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second || list[1].ID != first {
		t.Fatalf("list order %+v", list)
	}

	// New activity moves the older Session to the front.
	sendPrompt(t, srv.URL, first, "bump")
	_ = readSSE(t, srv.URL, first, 0, 5, 5*time.Second)
	resp2, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	list = nil
	if err := json.NewDecoder(resp2.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != first {
		t.Fatalf("list order after activity %+v", list)
	}
}

func TestMalformedLogLineSkippedOnReload(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	d := NewDaemon(dir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	srv.Close()

	logPath := filepath.Join(dir, "enderbloom", "sessions", id, "events.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("this is not json\n[1,2\n")
	_ = f.Close()

	d2 := NewDaemon(dir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	resp, err := http.Get(srv2.URL + "/api/sessions/" + id)
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
	if len(body.Events) != 1 {
		t.Fatalf("events after reload = %d, want 1", len(body.Events))
	}
	for i, e := range body.Events {
		if e.Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d", i, e.Seq)
		}
	}
}

func TestLastEventIDHeaderAndFiniteReplay(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	_ = readSSE(t, srv.URL, id, 0, 5, 5*time.Second)

	// Last-Event-ID header replays only what follows it; wait=0 closes
	// the stream after the replay instead of holding it open.
	req, _ := http.NewRequest("GET", srv.URL+"/api/sessions/"+id+"/events?wait=0", nil)
	req.Header.Set("Last-Event-ID", "3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var seqs []int64
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			var e Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
				t.Fatal(err)
			}
			seqs = append(seqs, e.Seq)
		}
	}
	if len(seqs) != 2 || seqs[0] != 4 || seqs[1] != 5 {
		t.Fatalf("header replay seqs = %v, want [4 5]", seqs)
	}
}

func TestConcurrentPromptsStayOrdered(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	d := NewDaemon(dir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	const n = 5
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			body, _ := json.Marshal(map[string]string{"text": "burst " + strconv.Itoa(i)})
			resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
			if err != nil {
				return
			}
			resp.Body.Close()
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}

	// Messages queued behind a running turn go out as one merged message, so
	// the event count varies: wait until every burst is in the log and the
	// Session is idle again.
	events := waitBurstSettled(t, srv.URL, id, n)
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d", i, e.Seq)
		}
	}
	total := len(events)

	// The persisted log matches memory order after a restart.
	srv.Close()
	d2 := NewDaemon(dir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	replayed := readSSE(t, srv2.URL, id, 0, total, 10*time.Second)
	for i, e := range replayed {
		if e.Seq != int64(i+1) || e.Kind != events[i].Kind || e.Text != events[i].Text {
			t.Fatalf("replay differs at %d: %+v vs %+v", i, events[i], e)
		}
	}
}

// waitBurstSettled replays the Session log until it holds all n "burst i"
// texts and ends idle, then returns it.
func waitBurstSettled(t *testing.T, base, id string, n int) []Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/sessions/" + id + "/events?wait=0")
		if err != nil {
			t.Fatal(err)
		}
		var events []Event
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				var e Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
					t.Fatal(err)
				}
				events = append(events, e)
			}
		}
		resp.Body.Close()
		var prompts strings.Builder
		for _, e := range events {
			if e.Kind == "prompt" {
				prompts.WriteString(e.Text + "\n")
			}
		}
		all := true
		for i := 0; i < n; i++ {
			all = all && strings.Contains(prompts.String(), "burst "+strconv.Itoa(i))
		}
		if last := len(events) - 1; all && last >= 0 && events[last].Kind == "status" && events[last].Status == "idle" {
			return events
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("burst did not settle")
	return nil
}
