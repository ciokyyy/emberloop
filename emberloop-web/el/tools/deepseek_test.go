package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Seam: DeepSeek as the default search provider and the default first fetch
// tier until deepseekDefaultUntil, against local fakes. No live calls. These
// tests set package state (the clock, env), so none runs in parallel.

type deepseekFake struct {
	server     *httptest.Server
	searchHits int64
	fetchHits  int64
	code       int
	body       string
	mu         sync.Mutex
	auth       string
	reqBody    string
}

func startDeepSeekFake(t *testing.T, code int, body string) *deepseekFake {
	t.Helper()
	f := &deepseekFake{code: code, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/v1/tools/search":
			atomic.AddInt64(&f.searchHits, 1)
		case "/v1/tools/fetch":
			atomic.AddInt64(&f.fetchHits, 1)
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.auth = r.Header.Get("Authorization")
		f.reqBody = string(raw)
		f.mu.Unlock()
		w.WriteHeader(f.code)
		fmt.Fprint(w, f.body)
	}))
	t.Cleanup(f.server.Close)
	t.Setenv("EL_DEEPSEEK_ENDPOINT", f.server.URL)
	return f
}

func (f *deepseekFake) seen() (auth, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth, f.reqBody
}

// setDeepSeekClock pins the default-window clock for one test.
func setDeepSeekClock(t *testing.T, now time.Time) {
	t.Helper()
	old := deepseekClock
	deepseekClock = func() time.Time { return now }
	t.Cleanup(func() { deepseekClock = old })
}

var (
	beforeCutoff = deepseekDefaultUntil.Add(-time.Hour)
	afterCutoff  = deepseekDefaultUntil.Add(time.Hour)
)

const deepseekSearchBody = `{"results":[{"rerank_score":2.4,"title":"DeepSeek Title","url":"https://example.com/d","snippet":"short snippet","summary":"long model summary","content":"","site_name":"Example","site_icon":null,"date":"2026-10-16T00:00:00Z","authority":3}]}`

// Until the cutoff, auto asks DeepSeek first and never reaches Brave or Exa.
func TestWebsearchAutoPrefersDeepSeekBeforeCutoff(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	d := startDeepSeekFake(t, http.StatusOK, deepseekSearchBody)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, beforeCutoff)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "freshness": "week", "domains": []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DeepSeek Title", "https://example.com/d", "long model summary", "Published: 2026-10-16T00:00:00Z", "provider: deepseek"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
	if atomic.LoadInt64(&s.braveHits) != 0 || atomic.LoadInt64(&s.exaHits) != 0 {
		t.Fatalf("hits brave=%d exa=%d; auto should stop at DeepSeek", atomic.LoadInt64(&s.braveHits), atomic.LoadInt64(&s.exaHits))
	}
	auth, body := d.seen()
	if auth != "Bearer ds-test-key" {
		t.Fatalf("authorization = %q", auth)
	}
	for _, want := range []string{`"count":10`, "golang sqlite", "after:", "site:example.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("request body %s misses %q", body, want)
		}
	}
}

// Without a key DeepSeek is absent from auto: no call, no skip note.
func TestWebsearchAutoSkipsDeepSeekWithoutKey(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	d := startDeepSeekFake(t, http.StatusOK, deepseekSearchBody)
	t.Setenv("DEEPSEEK_API_KEY", "")
	setDeepSeekClock(t, beforeCutoff)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "provider: brave |") || strings.Contains(out, "deepseek") {
		t.Fatalf("want a plain brave answer, got %q", out)
	}
	if atomic.LoadInt64(&d.searchHits) != 0 {
		t.Fatalf("deepseek called %d times without a key", atomic.LoadInt64(&d.searchHits))
	}
}

// At the cutoff DeepSeek leaves auto even with a key, but a named
// provider still answers.
func TestWebsearchDeepSeekLeavesAutoAfterCutoff(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	d := startDeepSeekFake(t, http.StatusOK, deepseekSearchBody)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, afterCutoff)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "provider: brave") || atomic.LoadInt64(&d.searchHits) != 0 {
		t.Fatalf("auto after cutoff used deepseek: hits=%d out=%q", atomic.LoadInt64(&d.searchHits), out)
	}
	out, err = callWebsearch(t, map[string]any{"query": "golang sqlite", "provider": "deepseek", "fresh": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "provider: deepseek") || atomic.LoadInt64(&d.searchHits) != 1 {
		t.Fatalf("named deepseek failed: hits=%d out=%q", atomic.LoadInt64(&d.searchHits), out)
	}
}

// A named DeepSeek without its key is a configuration error.
func TestWebsearchNamedDeepSeekNeedsKey(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	t.Setenv("DEEPSEEK_API_KEY", "")
	_, err := callWebsearch(t, map[string]any{"query": "x", "provider": "deepseek"})
	if err == nil || !strings.Contains(err.Error(), "DEEPSEEK_API_KEY not set") {
		t.Fatalf("err = %v", err)
	}
}

// Insufficient balance (402) and a long query both fall through to Brave.
func TestWebsearchDeepSeekFallsBack(t *testing.T) {
	long := strings.Repeat("word ", 30)
	for _, tc := range []struct {
		name  string
		code  int
		body  string
		query string
		note  string
		hits  int64
	}{
		{"balance", http.StatusPaymentRequired, `{"error":{"message":"Insufficient Balance","type":"invalid_request_error","param":null,"code":"x"}}`, "golang sqlite", "deepseek failed (HTTP 402)", 1},
		{"server error", http.StatusInternalServerError, "boom", "golang sqlite", "deepseek failed (HTTP 500)", 1},
		{"empty", http.StatusOK, `{"results":[]}`, "golang sqlite", "deepseek returned no results", 1},
		{"query too long", http.StatusOK, deepseekSearchBody, long, "deepseek failed (query over 128 characters)", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startSearchServers(t)
			searchEnv(t, s)
			d := startDeepSeekFake(t, tc.code, tc.body)
			t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
			setDeepSeekClock(t, beforeCutoff)
			out, err := callWebsearch(t, map[string]any{"query": tc.query})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "Brave Title") || !strings.Contains(out, tc.note) {
				t.Fatalf("output %q misses Brave answer or note %q", out, tc.note)
			}
			if atomic.LoadInt64(&d.searchHits) != tc.hits {
				t.Fatalf("deepseek hits = %d, want %d", atomic.LoadInt64(&d.searchHits), tc.hits)
			}
		})
	}
}

// A 400 the query caused does not fall back, the way Brave's does not.
func TestWebsearchDeepSeekBadRequestDoesNotFallBack(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	startDeepSeekFake(t, http.StatusBadRequest, `{"error":{"message":"'count' must be between 1 and 50"}}`)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, beforeCutoff)
	_, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("err = %v", err)
	}
	if atomic.LoadInt64(&s.braveHits) != 0 {
		t.Fatalf("brave called %d times after a query fault", atomic.LoadInt64(&s.braveHits))
	}
}

func TestSettingsAcceptDeepSeekProvider(t *testing.T) {
	if !validSearchProvider(SearchProviderDeepSeek) {
		t.Fatal("deepseek must be a valid search provider")
	}
}

const deepseekFetchBody = `{"title":"Fetched Page","content":"Body text from DeepSeek Fetch with plenty of descriptive words so the thin page check passes without any trouble at all, and then some more words."}`

// Until the cutoff, read asks DeepSeek Fetch first.
func TestReadURLDeepSeekFetchFirst(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "unused")
	d := startDeepSeekFake(t, http.StatusOK, deepseekFetchBody)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, beforeCutoff)
	native := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&native, 1)
		_, _ = w.Write([]byte(longBody("native page")))
	}))
	t.Cleanup(srv.Close)
	out, err := readURLPath(t, srv.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "tier0 (deepseek)", "text/markdown", "untrusted content", "# Fetched Page", "Body text from DeepSeek Fetch")
	if atomic.LoadInt64(&d.fetchHits) != 1 || atomic.LoadInt32(&native) != 0 || jf.count() != 0 {
		t.Fatalf("hits deepseek=%d native=%d jina=%d", atomic.LoadInt64(&d.fetchHits), atomic.LoadInt32(&native), jf.count())
	}
	if _, body := d.seen(); !strings.Contains(body, srv.URL+"/page") {
		t.Fatalf("fetch body %s misses the url", body)
	}
}

// A failed DeepSeek fetch falls through to the native tier with a note.
func TestReadURLDeepSeekFetchFailureFallsThrough(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	d := startDeepSeekFake(t, http.StatusNotFound, `{"error":{"message":"no usable content"}}`)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, beforeCutoff)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(longBody("native page")))
	}))
	t.Cleanup(srv.Close)
	out, err := readURLPath(t, srv.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "tier1 (native)", "deepseek fetch failed (HTTP 404", "native page")
	if atomic.LoadInt64(&d.fetchHits) != 1 {
		t.Fatalf("deepseek hits = %d", atomic.LoadInt64(&d.fetchHits))
	}
}

// DeepSeek Fetch stays out for raw reads, credentialed URLs, a missing key
// and every read after the cutoff.
func TestReadURLDeepSeekFetchSkipped(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		now  time.Time
		path func(base string) string
	}{
		{"raw", "ds-test-key", beforeCutoff, func(b string) string { return b + "/page:raw" }},
		{"credentials", "ds-test-key", beforeCutoff, func(b string) string { return strings.Replace(b+"/page", "://", "://user:pass@", 1) }},
		{"token params", "ds-test-key", beforeCutoff, func(b string) string { return b + "/page?access_token=abc" }},
		{"no key", "", beforeCutoff, func(b string) string { return b + "/page" }},
		{"after cutoff", "ds-test-key", afterCutoff, func(b string) string { return b + "/page" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowLocal(t)
			_ = startJinaFake(t, 200, "unused")
			d := startDeepSeekFake(t, http.StatusOK, deepseekFetchBody)
			t.Setenv("DEEPSEEK_API_KEY", tc.key)
			setDeepSeekClock(t, tc.now)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(longBody("native page")))
			}))
			t.Cleanup(srv.Close)
			out, err := readURLPath(t, tc.path(srv.URL))
			if err != nil {
				t.Fatal(err)
			}
			mustContain(t, out, "tier1 (native)")
			mustNotContain(t, out, "tier0")
			if atomic.LoadInt64(&d.fetchHits) != 0 {
				t.Fatalf("deepseek fetch called %d times", atomic.LoadInt64(&d.fetchHits))
			}
		})
	}
}

// A cached DeepSeek-window answer never serves once the window closes.
func TestWebsearchCacheDoesNotCrossCutoff(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	startDeepSeekFake(t, http.StatusOK, deepseekSearchBody)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, beforeCutoff)
	if _, err := callWebsearch(t, map[string]any{"query": "cache probe"}); err != nil {
		t.Fatal(err)
	}
	setDeepSeekClock(t, afterCutoff)
	out, err := callWebsearch(t, map[string]any{"query": "cache probe"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "provider: brave") || strings.Contains(out, "cache: cached") {
		t.Fatalf("post-cutoff answer came from the DeepSeek window: %q", out)
	}
}

// A URL fragment can hold a secret and never goes to DeepSeek.
func TestReadURLDeepSeekFetchDropsFragment(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	d := startDeepSeekFake(t, http.StatusOK, deepseekFetchBody)
	t.Setenv("DEEPSEEK_API_KEY", "ds-test-key")
	setDeepSeekClock(t, beforeCutoff)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	if _, err := readURLPath(t, srv.URL+"/page#access_token=xyz"); err != nil {
		t.Fatal(err)
	}
	if _, body := d.seen(); strings.Contains(body, "access_token") || !strings.Contains(body, "/page") {
		t.Fatalf("fetch body %s leaked the fragment", body)
	}
}
