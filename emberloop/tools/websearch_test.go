package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Seam: one websearch tool call against fake local HTTP servers returning
// recorded Brave and Exa responses. No live calls.

// searchServers is a Brave fake and an Exa fake with hit counters, so tests
// assert chain order and no-fallback by who was called.
type searchServers struct {
	braveHits   int64
	exaHits     int64
	brave       *httptest.Server
	exa         *httptest.Server
	braveCode   int
	braveBody   string
	braveHead   http.Header
	exaCode     int
	exaBody     string
	mu          sync.Mutex
	braveQuery  url.Values
	exaBodySeen string
}

// lastBraveQuery is the query string of the latest Brave fake call.
func (s *searchServers) lastBraveQuery() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.braveQuery
}

// lastExaBody is the request body of the latest Exa fake call.
func (s *searchServers) lastExaBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exaBodySeen
}

func startSearchServers(t *testing.T) *searchServers {
	t.Helper()
	s := &searchServers{
		braveCode: http.StatusOK,
		braveBody: `{"web":{"results":[{"title":"Brave Title","url":"https://example.com/a","description":"brave snippet","age":"2 days ago"}]}}`,
		exaCode:   http.StatusOK,
		exaBody:   `{"results":[{"title":"Exa Title","url":"https://example.com/b","text":"exa snippet","publishedDate":"2026-10-01"}],"costDollars":0.0025}`,
	}
	s.braveHead = http.Header{
		"X-Ratelimit-Remaining": {"9989"},
		"X-Ratelimit-Reset":     {"2026-10-07T00:00:00Z"},
	}
	s.brave = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.braveHits, 1)
		s.mu.Lock()
		s.braveQuery = r.URL.Query()
		s.mu.Unlock()
		for k, vs := range s.braveHead {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(s.braveCode)
		fmt.Fprint(w, s.braveBody)
	}))
	s.exa = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.exaHits, 1)
		if raw, err := io.ReadAll(r.Body); err == nil {
			s.mu.Lock()
			s.exaBodySeen = string(raw)
			s.mu.Unlock()
		}
		w.WriteHeader(s.exaCode)
		fmt.Fprint(w, s.exaBody)
	}))
	t.Cleanup(s.brave.Close)
	t.Cleanup(s.exa.Close)
	return s
}

// searchEnv points the tool at the fakes with temp state and settings, so a
// call touches no live provider, no real state dir and no real settings.
func searchEnv(t *testing.T, s *searchServers) {
	t.Helper()
	t.Setenv("EL_STATE_DIR", t.TempDir())
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing-project.json"))
	t.Setenv("EL_SEARCH_PROVIDER", "")
	t.Setenv("EL_SEARCH_MODE", "")
	t.Setenv("BRAVE_API_KEY", "brave-test-key")
	t.Setenv("EXA_API_KEY", "exa-test-key")
	t.Setenv("EL_BRAVE_ENDPOINT", s.brave.URL)
	t.Setenv("EL_EXA_ENDPOINT", s.exa.URL)
	t.Setenv("EL_SEARCH_TIMEOUT", "5s")
}

func callWebsearch(t *testing.T, args map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return websearchTool{}.Execute(context.Background(), t.TempDir(), raw)
}

// Auto prefers Brave: Exa is never called and the footer names Brave.
func TestWebsearchAutoPrefersBrave(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Brave Title", "https://example.com/a", "brave snippet", "provider: brave"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
	if atomic.LoadInt64(&s.exaHits) != 0 {
		t.Fatalf("exa called %d times; auto should stop at Brave", s.exaHits)
	}
}

// Auto falls back to Exa on a Brave 500, and the footer says so.
func TestWebsearchAutoFallsBackOnBraveError(t *testing.T) {
	s := startSearchServers(t)
	s.braveCode = http.StatusInternalServerError
	s.braveBody = "boom"
	searchEnv(t, s)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Exa Title", "provider: exa", "brave"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
	if atomic.LoadInt64(&s.braveHits) != 1 || atomic.LoadInt64(&s.exaHits) != 1 {
		t.Fatalf("hits brave=%d exa=%d; want one each", s.braveHits, s.exaHits)
	}
}

// Auto falls back on a 429 and on zero Brave results.
func TestWebsearchAutoFallsBackOn429AndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		note string
	}{
		{"rate limited", http.StatusTooManyRequests, "slow down", "failed (HTTP 429)"},
		{"empty results", http.StatusOK, `{"web":{"results":[]}}`, "returned no results"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startSearchServers(t)
			s.braveCode = tc.code
			s.braveBody = tc.body
			searchEnv(t, s)
			out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "Exa Title") {
				t.Fatalf("output %q does not fall back to Exa", out)
			}
			// The footer note proves which path fell back: a parse failure
			// would read "failed", not "returned no results".
			if !strings.Contains(out, tc.note) {
				t.Fatalf("output %q does not contain %q", out, tc.note)
			}
		})
	}
}

// A bad-query 4xx from Brave fails without falling back: the query is wrong,
// retrying elsewhere cannot fix it.
func TestWebsearchBadQueryDoesNotFallBack(t *testing.T) {
	s := startSearchServers(t)
	s.braveCode = http.StatusBadRequest
	s.braveBody = `{"message":"query too short"}`
	searchEnv(t, s)
	_, err := callWebsearch(t, map[string]any{"query": "x"})
	if err == nil {
		t.Fatal("expected an error for a rejected query")
	}
	if atomic.LoadInt64(&s.exaHits) != 0 {
		t.Fatalf("exa called %d times; a bad query must not fall back", s.exaHits)
	}
}

// A named provider never falls back: its real error reaches the caller.
func TestWebsearchNamedProviderNeverFallsBack(t *testing.T) {
	s := startSearchServers(t)
	s.braveCode = http.StatusInternalServerError
	s.braveBody = "boom"
	searchEnv(t, s)
	_, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "provider": "brave"})
	if err == nil {
		t.Fatal("expected Brave's error with a named provider")
	}
	if atomic.LoadInt64(&s.exaHits) != 0 {
		t.Fatalf("exa called %d times; a named provider never falls back", s.exaHits)
	}
}

// Auto skips a provider with no key, with a note; both missing is an error
// naming both keys.
func TestWebsearchAutoSkipsMissingKeys(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	os.Setenv("BRAVE_API_KEY", "")
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Exa Title") {
		t.Fatalf("output %q does not answer from Exa", out)
	}
	if !strings.Contains(out, "BRAVE_API_KEY not set") {
		t.Fatalf("output %q does not note the skipped provider", out)
	}
	if atomic.LoadInt64(&s.braveHits) != 0 {
		t.Fatalf("brave called %d times without a key", s.braveHits)
	}

	os.Setenv("BRAVE_API_KEY", "")
	os.Setenv("EXA_API_KEY", "")
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err == nil {
		t.Fatal("expected an error when both keys are missing")
	} else if !strings.Contains(err.Error(), "BRAVE_API_KEY not set") || !strings.Contains(err.Error(), "EXA_API_KEY not set") {
		t.Fatalf("error %q must name both missing keys", err)
	}
}

// A named provider with no key is an error naming the key.
func TestWebsearchNamedMissingKeyErrors(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	os.Setenv("EXA_API_KEY", "")
	_, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "provider": "exa"})
	if err == nil {
		t.Fatal("expected a missing-key error")
	}
	if !strings.Contains(err.Error(), "EXA_API_KEY not set") {
		t.Fatalf("error %q must say EXA_API_KEY not set", err)
	}
}

// The default provider comes from settings when the call names none.
func TestWebsearchDefaultProviderFromSettings(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	settingsPath := os.Getenv("EL_SETTINGS")
	if err := os.WriteFile(settingsPath, []byte(`{"search":{"provider":"exa"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Exa Title") {
		t.Fatalf("output %q does not answer from the settings provider", out)
	}
	if atomic.LoadInt64(&s.braveHits) != 0 {
		t.Fatalf("brave called %d times; settings chose exa", s.braveHits)
	}
}

// A timeout on Brave falls back to Exa under auto.
func TestWebsearchAutoFallsBackOnTimeout(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_TIMEOUT", "50ms")
	s.brave.Close()
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.braveHits, 1)
		select {
		case <-r.Context().Done():
		case <-testingTDone(t):
		}
	}))
	t.Cleanup(hang.Close)
	t.Setenv("EL_BRAVE_ENDPOINT", hang.URL)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Exa Title") {
		t.Fatalf("output %q does not fall back on timeout", out)
	}
}

// testingTDone closes when the test ends, so a hanging handler never blocks
// server shutdown.
func testingTDone(t *testing.T) <-chan struct{} {
	ch := make(chan struct{})
	t.Cleanup(func() { close(ch) })
	return ch
}
