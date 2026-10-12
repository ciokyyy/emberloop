package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// useSettings writes a settings file for the test and returns its path. The
// caller points each Daemon at it with SetSettingsPath before serving:
// settings never touch process env, so settings tests stay parallel-safe.
// TestMain's global EL_SETTINGS still covers tests that read host defaults.
func useSettings(t *testing.T, json string) string {
	t.Helper()
	p := filepath.Join(tempDir(t), "settings.json")
	if json != "" {
		if err := os.WriteFile(p, []byte(json), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// daemonWithSettings points a fresh Daemon at json settings: one line for
// the common useSettings-then-NewDaemon shape.
func daemonWithSettings(t *testing.T, json string, backend Backend) (*Daemon, string) {
	t.Helper()
	p := useSettings(t, json)
	d := NewDaemon(tempDir(t), backend)
	d.SetSettingsPath(p)
	return d, p
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

type errString string

func (e errString) Error() string { return string(e) }

// Compress refuses while a turn runs, for unknown Sessions, and for scouts.
// A Handoff Backend swaps by Handoff.
func TestCompressGuards(t *testing.T) {
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	be := NewFakeBackend()
	be.Delay = 300 * time.Millisecond
	d := NewDaemon(tempDir(t), be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

	sendPrompt(t, srv.URL, id, "slow")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	if code, body := compressReq(t, srv.URL, id, ""); code != http.StatusConflict || !strings.Contains(body["error"].(string), "working") {
		t.Fatalf("busy compress = %d %v, want 409", code, body)
	}
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)

	if code, _ := compressReq(t, srv.URL, "nope", ""); code != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", code)
	}

	if code, body := compressReq(t, srv.URL, id, ""); code != http.StatusAccepted {
		t.Fatalf("handoff compress = %d %v, want 202", code, body)
	}
	waitEventKind(t, srv.URL, id, "compressed")
	waitEventKind(t, srv.URL, id, "assistant_chunk")
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)
}

// Settings round-trip through GET and PUT, with defaults and validation.
func TestCompressSettings(t *testing.T) {
	t.Setenv("EL_SETTINGS", useSettings(t, ""))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
