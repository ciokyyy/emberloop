package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Provider adapters: Brave and Exa behind the searchAdapter interface. Each
// adapter searches, parses its results and parses its quota report, so a
// later engine is one adapter each. Wire shapes follow the providers'
// documented APIs; parsing tolerates the field variants the providers have
// used, and quota stays NULL when the provider reports nothing.

// braveEndpoint is the Brave Web Search API root: the default, or the fake
// server tests point at through EMBERLOOP_BRAVE_ENDPOINT.
func braveEndpoint() string {
	if s := strings.TrimSpace(os.Getenv("EMBERLOOP_BRAVE_ENDPOINT")); s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return defaultBraveEndpoint
}

// exaEndpoint is the Exa Search API root: the default, or the fake server
// tests point at through EMBERLOOP_EXA_ENDPOINT.
func exaEndpoint() string {
	if s := strings.TrimSpace(os.Getenv("EMBERLOOP_EXA_ENDPOINT")); s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return defaultExaEndpoint
}

// braveFreshness maps the tool's freshness to Brave's freshness parameter:
// past day, week, month and year.
func braveFreshness(f string) string {
	switch f {
	case "day":
		return "pd"
	case "week":
		return "pw"
	case "month":
		return "pm"
	case "year":
		return "py"
	}
	return ""
}

// exaStartDate maps the tool's freshness to Exa's startPublishedDate: the
// cutoff a result must be newer than, as a calendar date.
func exaStartDate(f string) string {
	var back time.Duration
	switch f {
	case "day":
		back = 24 * time.Hour
	case "week":
		back = 7 * 24 * time.Hour
	case "month":
		back = 30 * 24 * time.Hour
	case "year":
		back = 365 * 24 * time.Hour
	default:
		return ""
	}
	return time.Now().Add(-back).Format("2006-01-02")
}

type braveAdapter struct{}

func (braveAdapter) name() string   { return SearchProviderBrave }
func (braveAdapter) keyEnv() string { return "BRAVE_API_KEY" }

func (braveAdapter) search(ctx context.Context, key string, q searchArgs) ([]searchResult, searchQuota, error) {
	params := url.Values{}
	params.Set("q", q.query)
	params.Set("count", strconv.Itoa(q.count))
	if f := braveFreshness(q.freshness); f != "" {
		params.Set("freshness", f)
	}
	endpoint := braveEndpoint() + "/res/v1/web/search?" + params.Encode()
	// #nosec G107 -- the endpoint is Brave's API (or the test fake), not user input
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, searchQuota{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", key)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, searchQuota{}, err
	}
	defer resp.Body.Close()
	body, quota := readProviderBody(resp, parseBraveQuota(resp.Header))
	if resp.StatusCode != http.StatusOK {
		return nil, quota, classifyProviderError(SearchProviderBrave, resp.StatusCode, body, nil)
	}
	results, err := parseBraveResults([]byte(body))
	if err != nil {
		return nil, quota, fmt.Errorf("brave: bad response: %w", err)
	}
	return filterDomains(results, q.domains), quota, nil
}

// parseBraveQuota extracts what Brave reports: the rate-limit remaining and
// reset headers. Absent headers stay nil and empty: nothing is estimated.
func parseBraveQuota(header http.Header) searchQuota {
	var quota searchQuota
	for _, name := range []string{"X-RateLimit-Remaining", "RateLimit-Remaining"} {
		if raw := strings.TrimSpace(header.Get(name)); raw != "" {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
				quota.remaining = &n
				break
			}
		}
	}
	for _, name := range []string{"X-RateLimit-Reset", "RateLimit-Reset", "X-RateLimit-Reset-After"} {
		if raw := strings.TrimSpace(header.Get(name)); raw != "" {
			quota.reset = raw
			break
		}
	}
	return quota
}

// parseBraveResults parses a Brave web search response: results under web,
// tolerating a top-level results list. The snippet is the description; the
// date is whichever publish field the response carries.
func parseBraveResults(body []byte) ([]searchResult, error) {
	var doc struct {
		Web *struct {
			Results []map[string]any `json:"results"`
		} `json:"web"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	items := doc.Results
	if doc.Web != nil {
		items = doc.Web.Results
	}
	out := make([]searchResult, 0, len(items))
	for _, item := range items {
		u, _ := item["url"].(string)
		if strings.TrimSpace(u) == "" {
			if link, _ := item["link"].(string); strings.TrimSpace(link) != "" {
				u = link
			} else {
				continue
			}
		}
		out = append(out, searchResult{
			title:     firstString(item, "title", "name"),
			url:       strings.TrimSpace(u),
			snippet:   firstString(item, "description", "snippet", "text"),
			published: firstString(item, "date", "published", "published_date", "age", "page_age"),
		})
	}
	return out, nil
}

type exaAdapter struct{}

func (exaAdapter) name() string   { return SearchProviderExa }
func (exaAdapter) keyEnv() string { return "EXA_API_KEY" }

func (exaAdapter) search(ctx context.Context, key string, q searchArgs) ([]searchResult, searchQuota, error) {
	call := map[string]any{"query": q.query, "numResults": q.count}
	if len(q.domains) > 0 {
		call["includeDomains"] = q.domains
	}
	if start := exaStartDate(q.freshness); start != "" {
		call["startPublishedDate"] = start
	}
	encoded, err := json.Marshal(call)
	if err != nil {
		return nil, searchQuota{}, err
	}
	// #nosec G107 -- the endpoint is Exa's API (or the test fake), not user input
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exaEndpoint()+"/search", bytes.NewReader(encoded))
	if err != nil {
		return nil, searchQuota{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, searchQuota{}, err
	}
	defer resp.Body.Close()
	body, quota := readProviderBody(resp, searchQuota{})
	if resp.StatusCode != http.StatusOK {
		return nil, quota, classifyProviderError(SearchProviderExa, resp.StatusCode, body, nil)
	}
	var doc struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return nil, quota, fmt.Errorf("exa: bad response: %w", err)
	}
	results := make([]searchResult, 0, len(doc.Results))
	for _, item := range doc.Results {
		u, _ := item["url"].(string)
		if strings.TrimSpace(u) == "" {
			continue
		}
		results = append(results, searchResult{
			title:     firstString(item, "title", "name"),
			url:       strings.TrimSpace(u),
			snippet:   firstString(item, "text", "snippet", "summary", "description"),
			published: firstString(item, "publishedDate", "published_date", "date", "published"),
		})
	}
	return results, parseExaQuota([]byte(body)), nil
}

// parseExaQuota extracts what Exa reports: the cost of the request. Absent
// stays nil: nothing is estimated when Exa reports nothing.
func parseExaQuota(body []byte) searchQuota {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return searchQuota{}
	}
	for _, field := range []string{"costDollars", "cost", "totalCost"} {
		if raw, ok := doc[field]; ok {
			if cost, ok := asFloat(raw); ok {
				return searchQuota{cost: &cost}
			}
		}
	}
	return searchQuota{}
}

// asFloat reads a JSON number in any of its decoded shapes.
func asFloat(raw any) (float64, bool) {
	switch n := raw.(type) {
	case float64:
		return n, true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// readProviderBody reads one provider response body up to the cap. quota is
// the report parsed from the headers; the body only adds to it, never
// replaces what the headers said.
func readProviderBody(resp *http.Response, quota searchQuota) (string, searchQuota) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, providerBodyLimit))
	if err != nil {
		return "", quota
	}
	return string(raw), quota
}

// filterDomains keeps results on the include list: Brave takes no domains
// parameter, so the adapter filters client side where Exa filters server
// side. An empty list keeps everything.
func filterDomains(results []searchResult, include []string) []searchResult {
	if len(include) == 0 {
		return results
	}
	out := results[:0]
	for _, r := range results {
		u, err := url.Parse(r.url)
		if err != nil || !domainAllowed(u.Hostname(), include) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// firstString is the first present, non-blank string field: providers name
// the same facts differently across versions.
func firstString(item map[string]any, fields ...string) string {
	for _, f := range fields {
		if s, _ := item[f].(string); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// httpClient is the provider client: one shared client with redirect
// following from the stdlib, the per-call timeout bounding each request.
func httpClient() *http.Client { return &http.Client{} }
