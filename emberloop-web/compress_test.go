package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// compactFake is a FakeBackend that compresses in place: it records calls,
// streams the SDK-style compaction Events, and reports a scripted context.
type compactFake struct {
	*FakeBackend
	mu         sync.Mutex
	emit       map[string]func(kind, text, status string)
	used, size int
	calls      []string
	err        error
	hold       chan struct{}
}

func newCompactFake() *compactFake {
	return &compactFake{FakeBackend: NewFakeBackend(), emit: map[string]func(kind, text, status string){}, size: 1000}
}

func (c *compactFake) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	c.mu.Lock()
	c.emit[meta.ID] = emit
	c.mu.Unlock()
	return nil
}

func (c *compactFake) Compact(id, instructions string) error {
	c.mu.Lock()
	c.calls = append(c.calls, instructions)
	emit, hold, err, before := c.emit[id], c.hold, c.err, c.used
	c.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if emit != nil {
		emit("compaction_started", CompactionEvent{Reason: "manual"}.text(), "")
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.used = 120
	c.mu.Unlock()
	if emit != nil {
		emit("compaction", CompactionEvent{Reason: "manual", TokensBefore: before, TokensAfter: 120, Window: c.size, Summary: "short summary"}.text(), "")
		emit("usage", usageText(120, c.size), "")
	}
	return nil
}

func (c *compactFake) ContextUsage(id string) (int, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used, c.size, c.used > 0
}

func (c *compactFake) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func useSettings(t *testing.T, json string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("EL_SETTINGS", p)
	if json != "" {
		if err := os.WriteFile(p, []byte(json), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func compressReq(t *testing.T, base, id, instructions string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"instructions": instructions})
	resp, err := http.Post(base+"/api/sessions/"+id+"/compress", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func transcriptOf(t *testing.T, base, id string) []Event {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Events []Event `json:"events"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Events
}

func waitEventKind(t *testing.T, base, id, kind string) Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs := transcriptOf(t, base, id)
		for i := len(evs) - 1; i >= 0; i-- {
			if evs[i].Kind == kind {
				return evs[i]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %q event in %+v", kind, transcriptOf(t, base, id))
	return Event{}
}

// Manual compress on an idle Session: 202, the Session works, the
// compaction Events land, the meter refreshes, and the Session returns to idle.
func TestCompressManualRunsAndReportsResult(t *testing.T) {
	useSettings(t, "")
	be := newCompactFake()
	be.used = 900
	d := NewDaemon(t.TempDir(), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())

	code, _ := compressReq(t, srv.URL, id, "keep the API notes")
	if code != http.StatusAccepted {
		t.Fatalf("compress = %d, want 202", code)
	}
	ev := waitEventKind(t, srv.URL, id, "compaction")
	var c CompactionEvent
	if err := json.Unmarshal([]byte(ev.Text), &c); err != nil || c.TokensBefore != 900 || c.TokensAfter != 120 || c.Summary != "short summary" {
		t.Fatalf("compaction = %+v, %v", c, err)
	}
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)
	if len(be.calls) != 1 || be.calls[0] != "keep the API notes" {
		t.Fatalf("instructions = %q", be.calls)
	}
	usage := waitEventKind(t, srv.URL, id, "usage")
	if !strings.Contains(usage.Text, `"used":120`) {
		t.Fatalf("usage = %s", usage.Text)
	}
}

// A failed compaction reports the error in the transcript and leaves the
// Session usable.
func TestCompressFailureIsReportedAndSessionRecovers(t *testing.T) {
	useSettings(t, "")
	be := newCompactFake()
	be.err = errString("summary model unavailable")
	d := NewDaemon(t.TempDir(), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())

	if code, _ := compressReq(t, srv.URL, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d", code)
	}
	ev := waitEventKind(t, srv.URL, id, "compaction")
	if !strings.Contains(ev.Text, "summary model unavailable") || !strings.Contains(ev.Text, `"aborted":true`) {
		t.Fatalf("failure event = %s", ev.Text)
	}
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)
	sendPrompt(t, srv.URL, id, "still works")
}

type errString string

func (e errString) Error() string { return string(e) }

// Compress refuses while a turn runs, for Backends without in-place
// compression, for unknown Sessions, and for scouts.
func TestCompressGuards(t *testing.T) {
	useSettings(t, "")
	be := newCompactFake()
	be.Delay = 300 * time.Millisecond
	d := NewDaemon(t.TempDir(), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())

	sendPrompt(t, srv.URL, id, "slow")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	if code, body := compressReq(t, srv.URL, id, ""); code != http.StatusConflict || !strings.Contains(body["error"].(string), "working") {
		t.Fatalf("busy compress = %d %v, want 409", code, body)
	}
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)

	if code, _ := compressReq(t, srv.URL, "nope", ""); code != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", code)
	}

	// A Backend without in-place compaction swaps by Handoff.
	plain := NewDaemon(t.TempDir(), NewFakeBackend())
	srv2 := httptest.NewServer(plain.Handler())
	defer srv2.Close()
	pid := createSession(t, srv2.URL, "fake", "fake-model", t.TempDir())
	if code, body := compressReq(t, srv2.URL, pid, ""); code != http.StatusAccepted {
		t.Fatalf("handoff compress = %d %v, want 202", code, body)
	}
	waitEventKind(t, srv2.URL, pid, "compressed")
	waitEventKind(t, srv2.URL, pid, "assistant_chunk")
	waitStatus(t, srv2.URL, pid, "idle", 5*time.Second)
}

// Messages sent during a compaction queue and deliver after it.
func TestCompressQueuesMessagesUntilDone(t *testing.T) {
	useSettings(t, "")
	be := newCompactFake()
	be.used = 900
	be.hold = make(chan struct{})
	d := NewDaemon(t.TempDir(), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())

	compressReq(t, srv.URL, id, "")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	if code, _ := postMode(t, srv.URL, id, "followup", "after compaction"); code != http.StatusAccepted {
		t.Fatalf("followup = %d", code)
	}
	if q := getQueue(t, srv.URL, id); len(q) != 1 {
		t.Fatalf("queue = %+v, want the held message", q)
	}
	close(be.hold)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range transcriptOf(t, srv.URL, id) {
			if e.Kind == "assistant_chunk" && strings.Contains(e.Text, "after compaction") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued message never delivered after compaction")
}

// Auto-compress fires above the threshold, honours the cooldown, and stays
// off when the setting is off or usage is low.
func TestAutoCompress(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		used     int
		prompts  int
		want     int
	}{
		{"above threshold", `{"compress_cooldown":0}`, 900, 1, 1},
		{"below threshold", `{"compress_cooldown":0}`, 500, 1, 0},
		{"custom threshold", `{"compress_cooldown":0,"compress_threshold_percent":40}`, 500, 1, 1},
		{"auto off", `{"auto_compress":false,"compress_cooldown":0}`, 900, 1, 0},
		{"cooldown holds", `{"compress_cooldown":3}`, 900, 2, 0},
		{"cooldown elapsed", `{"compress_cooldown":2}`, 900, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useSettings(t, tc.settings)
			be := newCompactFake()
			d := NewDaemon(t.TempDir(), be)
			srv := httptest.NewServer(d.Handler())
			defer srv.Close()
			id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
			for i := 0; i < tc.prompts; i++ {
				be.mu.Lock()
				be.used = tc.used
				be.mu.Unlock()
				sendPrompt(t, srv.URL, id, "turn")
				waitStatus(t, srv.URL, id, "working", 5*time.Second)
				waitStatus(t, srv.URL, id, "idle", 5*time.Second)
				time.Sleep(50 * time.Millisecond)
			}
			waitStatus(t, srv.URL, id, "idle", 5*time.Second)
			if got := be.callCount(); got != tc.want {
				t.Fatalf("compactions = %d, want %d", got, tc.want)
			}
		})
	}
}

// Settings round-trip through GET and PUT, with defaults and validation.
func TestCompressSettings(t *testing.T) {
	useSettings(t, "")
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	get := func() compressSettings {
		resp, err := http.Get(srv.URL + "/api/settings")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Compress compressSettings `json:"compress"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out.Compress
	}
	put := func(body string) int {
		req, _ := http.NewRequest("PUT", srv.URL+"/api/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if s := get(); !s.Auto || s.ThresholdPercent != 80 || s.Cooldown != 5 || s.Instructions != "" {
		t.Fatalf("defaults = %+v", s)
	}
	if code := put(`{"compress":{"auto_compress":false,"compress_threshold_percent":65,"compress_cooldown":2,"compress_instructions":"  keep ids  "}}`); code != http.StatusOK {
		t.Fatalf("put = %d", code)
	}
	if s := get(); s.Auto || s.ThresholdPercent != 65 || s.Cooldown != 2 || s.Instructions != "keep ids" {
		t.Fatalf("saved = %+v", s)
	}
	for _, bad := range []string{
		`{"compress":{"compress_threshold_percent":5}}`,
		`{"compress":{"compress_threshold_percent":100}}`,
		`{"compress":{"compress_cooldown":-1}}`,
		`{"compress":{"compress_instructions":"` + strings.Repeat("x", 2001) + `"}}`,
	} {
		if code := put(bad); code != http.StatusBadRequest {
			t.Fatalf("put %.60s = %d, want 400", bad, code)
		}
	}
	if s := get(); s.ThresholdPercent != 65 {
		t.Fatalf("a rejected PUT changed settings: %+v", s)
	}
}

// Real SDK, faux provider: turns report the context meter, and a manual
// compression runs through Session.Compact and reports a compaction Event.
func TestPigCompressUsesSDKCompaction(t *testing.T) {
	useSettings(t, `{"auto_compress":false}`)
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())

	for i := 0; i < 3; i++ {
		sendPrompt(t, srv.URL, id, "work item "+strings.Repeat("x", 200))
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && !hasStatusAfter(transcriptOf(t, srv.URL, id), i+2) {
			time.Sleep(20 * time.Millisecond)
		}
	}
	usage := waitEventKind(t, srv.URL, id, "usage")
	var u struct{ Used, Size int }
	if err := json.Unmarshal([]byte(usage.Text), &u); err != nil || u.Size <= 0 || u.Used <= 0 {
		t.Fatalf("pig usage = %s (%v), want used and size above zero", usage.Text, err)
	}

	if code, body := compressReq(t, srv.URL, id, "keep file names"); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitEventKind(t, srv.URL, id, "compaction_started")
	end := waitEventKind(t, srv.URL, id, "compaction")
	var c CompactionEvent
	if err := json.Unmarshal([]byte(end.Text), &c); err != nil {
		t.Fatalf("compaction text = %s: %v", end.Text, err)
	}
	if c.Reason != "manual" {
		t.Fatalf("compaction = %+v, want reason manual", c)
	}
	waitStatus(t, srv.URL, id, "idle", 15*time.Second)
}

// hasStatusAfter reports whether the transcript holds n idle edges (the
// first one is the Session creation edge).
func hasStatusAfter(evs []Event, n int) bool {
	c := 0
	for _, e := range evs {
		if e.Kind == "status" && e.Status == "idle" {
			c++
		}
	}
	return c >= n
}
