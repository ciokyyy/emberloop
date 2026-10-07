package tools

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/emberloop/statedb"
)

// second identical search answers from the 15-minute database cache, fresh
// bypasses it, each live call records what its provider reported (and
// nothing more), and the footer names provider, fallbacks, cache and quota.

// A second identical search hits the cache: Brave is called once and the
// footer says cached.
func TestWebsearchCachesIdenticalSearches(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	first, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "cache: live") {
		t.Fatalf("first output %q does not say cache: live", first)
	}
	second, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&s.braveHits) != 1 {
		t.Fatalf("brave called %d times; the second search should hit the cache", s.braveHits)
	}
	for _, want := range []string{"Brave Title", "cache: cached", "provider: brave"} {
		if !strings.Contains(second, want) {
			t.Fatalf("cached output %q does not contain %q", second, want)
		}
	}
}

// An expired row reads as a miss: backdate the entry and the next search
// goes live again.
func TestWebsearchCacheExpiryGoesLive(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE search_cache SET created_at = ?`, int64(1728211200)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&s.braveHits) != 2 {
		t.Fatalf("brave called %d times; an expired entry must go live", s.braveHits)
	}
}

// fresh bypasses the cache and refreshes the entry.
func TestWebsearchFreshBypassesCache(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "fresh": true})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&s.braveHits) != 2 {
		t.Fatalf("brave called %d times; fresh must bypass the cache", s.braveHits)
	}
	if !strings.Contains(out, "cache: fresh") {
		t.Fatalf("output %q does not say cache: fresh", out)
	}
}

// Different arguments are a different entry: a new count goes live.
func TestWebsearchCacheKeysOnArguments(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "count": 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "count": 7}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&s.braveHits) != 2 {
		t.Fatalf("brave called %d times; different arguments must miss", s.braveHits)
	}
}

// Quota rows carry what each provider reported: Brave's remaining and reset
// headers, Exa's cost.
func TestWebsearchRecordsProviderQuota(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	brave, ok, err := statedb.LatestSearchQuota(db, "brave")
	if err != nil || !ok {
		t.Fatalf("latest brave quota = %+v, %v, %v; want a row", brave, ok, err)
	}
	if brave.Remaining == nil || *brave.Remaining != 9989 {
		t.Fatalf("brave remaining = %+v; want 9989 from the headers", brave.Remaining)
	}
	if brave.ResetAt != "2026-10-07T00:00:00Z" {
		t.Fatalf("brave reset = %q; want the header value", brave.ResetAt)
	}
	if brave.Cost != nil {
		t.Fatalf("brave cost = %v; Brave reports no cost, so nothing is stored", *brave.Cost)
	}
}

// An Exa call records its reported cost and no remaining.
func TestWebsearchRecordsExaCost(t *testing.T) {
	s := startSearchServers(t)
	s.braveCode = http.StatusInternalServerError
	s.braveBody = "boom"
	searchEnv(t, s)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "quota: exa last cost $0.0025") {
		t.Fatalf("output %q does not report Exa's cost in the footer", out)
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exa, ok, err := statedb.LatestSearchQuota(db, "exa")
	if err != nil || !ok {
		t.Fatalf("latest exa quota = %+v, %v, %v; want a row", exa, ok, err)
	}
	if exa.Cost == nil || *exa.Cost != 0.0025 {
		t.Fatalf("exa cost = %+v; want the reported 0.0025", exa.Cost)
	}
	if exa.Remaining != nil {
		t.Fatalf("exa remaining = %v; Exa reports no remaining, so nothing is stored", *exa.Remaining)
	}
}

// A provider that reports nothing leaves NULLs: no quota is invented.
func TestWebsearchRecordsNothingWhenTheProviderReportsNothing(t *testing.T) {
	s := startSearchServers(t)
	// No quota headers and no reported cost: the fake reports nothing.
	s.braveHead = http.Header{}
	s.exaBody = `{"results":[{"title":"Exa Title","url":"https://example.com/b","text":"exa snippet"}]}`
	searchEnv(t, s)
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	brave, ok, err := statedb.LatestSearchQuota(db, "brave")
	if err != nil || !ok {
		t.Fatalf("latest brave quota = %+v, %v, %v; want a row", brave, ok, err)
	}
	if brave.Remaining != nil || brave.Cost != nil || brave.ResetAt != "" {
		t.Fatalf("brave quota = %+v; want empty when nothing was reported", brave)
	}
}

// The answer is a numbered list with title, URL, snippet and date, plus a
// footer naming provider, fallbacks, cache and quota.
func TestWebsearchRendersNumberedListWithFooter(t *testing.T) {
	s := startSearchServers(t)
	s.braveCode = http.StatusInternalServerError
	s.braveBody = "boom"
	searchEnv(t, s)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"1. Exa Title",
		"URL: https://example.com/b",
		"Snippet: exa snippet",
		"Published: 2026-10-01",
		"provider: exa",
		"fallback: brave failed (HTTP 500)",
		"cache: live",
		"quota: exa last cost $0.0025",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
}

// A named provider with no results returns its real answer, not an error.
func TestWebsearchNamedProviderEmptyResultsAreAnAnswer(t *testing.T) {
	s := startSearchServers(t)
	s.braveBody = `{"web":{"results":[]}}`
	searchEnv(t, s)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "provider": "brave"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no results for") {
		t.Fatalf("output %q should report no results", out)
	}
}

// Domains narrow the answer: Brave has no domains parameter, so the adapter
// filters client side and Exa receives the include list server side.
func TestWebsearchDomainsNarrowResults(t *testing.T) {
	s := startSearchServers(t)
	s.braveBody = `{"web":{"results":[
		{"title":"Keep","url":"https://example.com/keep","description":"kept"},
		{"title":"Drop","url":"https://other.org/drop","description":"dropped"}]}}`
	searchEnv(t, s)
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "domains": []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Keep") || strings.Contains(out, "Drop") {
		t.Fatalf("output %q does not filter to example.com", out)
	}
	exaOut, err := callWebsearch(t, map[string]any{"query": "fresh question", "provider": "exa", "domains": []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exaOut, "Exa Title") {
		t.Fatalf("output %q does not answer from Exa", exaOut)
	}
	if body := s.lastExaBody(); !strings.Contains(body, "includeDomains") || !strings.Contains(body, "example.com") {
		t.Fatalf("exa request body %q does not carry the include list", body)
	}
}

// Freshness reaches the providers in their own parameters.
func TestWebsearchFreshnessReachesProviders(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	if _, err := callWebsearch(t, map[string]any{"query": "q one", "freshness": "week"}); err != nil {
		t.Fatal(err)
	}
	if got := s.lastBraveQuery().Get("freshness"); got != "pw" {
		t.Fatalf("brave freshness = %q, want pw for week", got)
	}
	if _, err := callWebsearch(t, map[string]any{"query": "q two", "provider": "exa", "freshness": "week"}); err != nil {
		t.Fatal(err)
	}
	if body := s.lastExaBody(); !strings.Contains(body, "startPublishedDate") {
		t.Fatalf("exa request body %q does not carry the start date", body)
	}
	if _, err := callWebsearch(t, map[string]any{"query": "q three", "freshness": "sometimes"}); err == nil {
		t.Fatal("expected an error for an unknown freshness")
	}
}

// A headerless failure does not hide the last known quota in the footer:
// the footer reads the latest row that reports something.
func TestWebsearchFooterKeepsLastKnownQuota(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	remaining := int64(9989)
	if err := statedb.InsertSearchQuota(db, statedb.SearchQuotaRow{Provider: "brave", Remaining: &remaining, ResetAt: "reset", CreatedAt: "2026-10-06T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := statedb.InsertSearchQuota(db, statedb.SearchQuotaRow{Provider: "brave", CreatedAt: "2026-10-06T10:01:00Z"}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if got := latestQuotaText([]string{"brave"}); !strings.Contains(got, "remaining 9989") {
		t.Fatalf("quota text %q does not keep the last known remaining", got)
	}
}
