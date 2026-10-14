package tools

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// Seam: parallel mode through one websearch tool call with search.mode set
// to parallel. The configured providers run concurrently, results are
// deduplicated by normalised URL and fused by reciprocal rank fusion, and
// every result carries the providers that returned it.

// Parallel mode merges both providers' results and names both in the footer.
func TestWebsearchParallelMergesProviders(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_MODE", "parallel")
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Brave Title", "Exa Title", "provider: brave, exa (parallel)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
	if atomic.LoadInt64(&s.braveHits) != 1 || atomic.LoadInt64(&s.exaHits) != 1 {
		t.Fatalf("hits brave=%d exa=%d; parallel calls both once", s.braveHits, s.exaHits)
	}
}

// A URL both providers return fuses first and carries both sources; the rest
// follow in reciprocal-rank-fusion order.
func TestWebsearchParallelFusesSharedUrlsFirst(t *testing.T) {
	s := startSearchServers(t)
	s.braveBody = `{"web":{"results":[
		{"title":"Shared","url":"https://example.com/shared/","description":"both"},
		{"title":"Brave Only","url":"https://example.com/brave","description":"b"}]}}`
	s.exaBody = `{"results":[
		{"title":"Exa Only","url":"https://example.com/exa","text":"e"},
		{"title":"Shared","url":"https://example.com/shared","text":"both"}]}`
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_MODE", "parallel")
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	shared := strings.Index(out, "1. Shared")
	exaOnly := strings.Index(out, "Exa Only")
	braveOnly := strings.Index(out, "Brave Only")
	if shared < 0 || exaOnly < 0 || braveOnly < 0 {
		t.Fatalf("output %q misses a result", out)
	}
	if !(shared < exaOnly && exaOnly < braveOnly) {
		t.Fatalf("output %q is not in fusion order (shared, exa-only, brave-only)", out)
	}
	if strings.Count(out, "example.com/shared") != 1 {
		t.Fatalf("output %q does not dedupe the shared URL to one entry", out)
	}
	if !strings.Contains(out, "Sources: brave, exa") {
		t.Fatalf("output %q does not tag the shared URL with both sources", out)
	}
}

// A failing provider does not sink the merge: the other answers and the
// footer notes the failure.
func TestWebsearchParallelSurvivesOneFailure(t *testing.T) {
	s := startSearchServers(t)
	s.braveCode = http.StatusInternalServerError
	s.braveBody = "boom"
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_MODE", "parallel")
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Exa Title", "brave failed (HTTP 500)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
}

// A named provider in parallel mode runs alone: no merge, no fallback.
func TestWebsearchParallelNamedProviderRunsAlone(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_MODE", "parallel")
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite", "provider": "brave"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Brave Title") {
		t.Fatalf("output %q does not answer from Brave", out)
	}
	if atomic.LoadInt64(&s.exaHits) != 0 {
		t.Fatalf("exa called %d times; a named provider runs alone", s.exaHits)
	}
	if strings.Contains(out, "parallel") {
		t.Fatalf("output %q should not claim a parallel merge of one provider", out)
	}
}

// Parallel answers cache like fallback ones: the second identical search
// calls neither provider.
func TestWebsearchParallelCaches(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_MODE", "parallel")
	if _, err := callWebsearch(t, map[string]any{"query": "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&s.braveHits) != 1 || atomic.LoadInt64(&s.exaHits) != 1 {
		t.Fatalf("hits brave=%d exa=%d; the second search should hit the cache", s.braveHits, s.exaHits)
	}
	if !strings.Contains(out, "cache: cached") {
		t.Fatalf("output %q does not say cached", out)
	}
}

// Mode is a setting, not a tool argument: the schema carries no mode.
func TestWebsearchSchemaHasNoMode(t *testing.T) {
	props, ok := websearchTool{}.Schema()["properties"].(map[string]any)
	if !ok {
		t.Fatal("websearch schema has no properties")
	}
	if _, ok := props["mode"]; ok {
		t.Fatalf("websearch schema exposes mode; it is a setting, not an argument")
	}
	for _, want := range []string{"query", "count", "freshness", "domains", "provider", "fresh"} {
		if _, ok := props[want]; !ok {
			t.Fatalf("websearch schema misses %q", want)
		}
	}
}

// The parallel footer shows the latest quota of every answering provider.
func TestWebsearchParallelFooterShowsBothQuotas(t *testing.T) {
	s := startSearchServers(t)
	searchEnv(t, s)
	t.Setenv("EL_SEARCH_MODE", "parallel")
	out, err := callWebsearch(t, map[string]any{"query": "golang sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"brave remaining 9989", "exa last cost $0.0025"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
}
