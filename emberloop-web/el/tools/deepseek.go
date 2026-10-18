package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// DeepSeek Search and Fetch: DeepSeek's own search engine behind
// POST /v1/tools/search and POST /v1/tools/fetch, authenticated with a
// DeepSeek API key (DEEPSEEK_API_KEY, from the environment only).
//
// Both are the default until deepseekDefaultUntil: websearch's auto chain
// starts with DeepSeek and `read` on a URL tries DeepSeek Fetch before the
// native tier. After the cutoff DeepSeek leaves both defaults; websearch
// still answers with it when named (provider: deepseek). Both defaults also
// need the key, so a host without one sees no change at all.
//
// The cutoff is the date DeepSeek's docs give for the end of the free
// period: 00:00 Beijing time on 2026-11-01. The docs scope that free period to
// server-side calls made through DeepSeek Harness, Claude Code and Codex;
// calls made straight to the API, as here, are billed at $0.75 per 1,000
// successful requests (failed, empty and timed-out calls are free).

const (
	// SearchProviderDeepSeek names the DeepSeek search provider.
	SearchProviderDeepSeek = "deepseek"
	// defaultDeepSeekEndpoint is the DeepSeek API root; tests point at a
	// fake through EL_DEEPSEEK_ENDPOINT.
	defaultDeepSeekEndpoint = "https://api.deepseek.com"
	// deepseekMaxQueryChars is the longest query the search API accepts.
	deepseekMaxQueryChars = 128
)

// deepseekDefaultUntil is the instant DeepSeek stops being the default for
// search and fetch: 00:00 Beijing time, 2026-11-01.
var deepseekDefaultUntil = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))

// deepseekClock is the clock the default window reads; tests replace it.
var deepseekClock = time.Now

// deepseekDefaultActive reports whether DeepSeek is still the default.
func deepseekDefaultActive() bool { return deepseekClock().Before(deepseekDefaultUntil) }

func deepseekKey() string { return strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")) }

// deepseekEndpoint is the API root: the default, or the test fake.
func deepseekEndpoint() string {
	if s := strings.TrimSpace(os.Getenv("EL_DEEPSEEK_ENDPOINT")); s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return defaultDeepSeekEndpoint
}

// deepseekSearchQuery builds the query DeepSeek receives: the user's query
// plus operators for the freshness and domain arguments, which the API takes
// inside the query string.
func deepseekSearchQuery(q searchArgs) string {
	query := q.query
	if start := exaStartDate(q.freshness); start != "" {
		query += " after:" + start
	}
	if len(q.domains) > 0 {
		sites := make([]string, len(q.domains))
		for i, d := range q.domains {
			sites[i] = "site:" + d
		}
		query += " " + strings.Join(sites, " OR ")
	}
	return query
}

type deepseekAdapter struct{}

func (deepseekAdapter) name() string   { return SearchProviderDeepSeek }
func (deepseekAdapter) keyEnv() string { return "DEEPSEEK_API_KEY" }

func (deepseekAdapter) search(ctx context.Context, key string, q searchArgs) ([]searchResult, searchQuota, error) {
	query := deepseekSearchQuery(q)
	// The API refuses a query past 128 characters. That is not the caller's
	// fault once operators were appended, so the failure is retryable and
	// auto falls through to the next provider.
	if utf8.RuneCountInString(query) > deepseekMaxQueryChars {
		return nil, searchQuota{}, &providerError{
			provider: SearchProviderDeepSeek, msg: "query over 128 characters", retryable: true,
		}
	}
	encoded, err := json.Marshal(map[string]any{"query": query, "count": q.count})
	if err != nil {
		return nil, searchQuota{}, err
	}
	body, status, err := deepseekPost(ctx, key, "/v1/tools/search", encoded)
	if err != nil {
		return nil, searchQuota{}, err
	}
	if status != http.StatusOK {
		return nil, searchQuota{}, classifyProviderError(SearchProviderDeepSeek, status, deepseekErrorMessage(body), nil)
	}
	results, err := parseDeepSeekResults(body)
	if err != nil {
		return nil, searchQuota{}, fmt.Errorf("deepseek: bad response: %w", err)
	}
	// DeepSeek reports no quota or cost in the response: nothing is invented.
	return filterDomains(results, q.domains), searchQuota{}, nil
}

// parseDeepSeekResults parses a search response. The snippet is the long
// model-oriented summary when present, else the short display snippet.
func parseDeepSeekResults(body []byte) ([]searchResult, error) {
	var doc struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := make([]searchResult, 0, len(doc.Results))
	for _, item := range doc.Results {
		u, _ := item["url"].(string)
		if strings.TrimSpace(u) == "" {
			continue
		}
		out = append(out, searchResult{
			title:     firstString(item, "title", "site_name"),
			url:       strings.TrimSpace(u),
			snippet:   firstString(item, "summary", "snippet"),
			published: firstString(item, "date"),
		})
	}
	return out, nil
}

// deepseekPost sends one JSON POST to the DeepSeek API and returns the capped
// body and status. A transport failure is the error.
func deepseekPost(ctx context.Context, key, path string, payload []byte) ([]byte, int, error) {
	// #nosec G107 -- the endpoint is DeepSeek's API (or the test fake), not user input
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepseekEndpoint()+path, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := readProviderBody(resp, searchQuota{})
	return []byte(body), resp.StatusCode, nil
}

// deepseekErrorMessage reads the OpenAI-style error body's message, else the
// raw body, so the classified error names what DeepSeek said.
func deepseekErrorMessage(body []byte) string {
	var doc struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &doc) == nil && strings.TrimSpace(doc.Error.Message) != "" {
		return doc.Error.Message
	}
	return string(body)
}

// deepseekFetchActive reports whether `read` should try DeepSeek Fetch first
// for a URL: inside the default window, with a key, and with a URL that is
// allowed to leave the machine (no credentials, no token-like parameters).
func deepseekFetchActive(skipReason string) bool {
	return skipReason == "" && deepseekDefaultActive() && deepseekKey() != ""
}

// fetchDeepSeek fetches one page as Markdown through DeepSeek Fetch. Any
// non-200 answer, empty content or transport failure is an error; the caller
// falls through to the native tiers.
func fetchDeepSeek(ctx context.Context, target string) (string, error) {
	// A fragment is client-side only and can carry a secret (#access_token=),
	// so it never leaves the machine.
	if u, perr := url.Parse(target); perr == nil {
		u.Fragment = ""
		u.RawFragment = ""
		target = u.String()
	}
	encoded, err := json.Marshal(map[string]string{"url": target})
	if err != nil {
		return "", err
	}
	body, status, err := deepseekPost(ctx, deepseekKey(), "/v1/tools/fetch", encoded)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", status, firstLine(deepseekErrorMessage(body)))
	}
	var doc struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("bad response: %w", err)
	}
	content := strings.TrimSpace(doc.Content)
	if content == "" {
		return "", fmt.Errorf("empty content")
	}
	if title := strings.TrimSpace(doc.Title); title != "" && !strings.HasPrefix(content, "#") {
		content = "# " + title + "\n\n" + content
	}
	return content, nil
}
