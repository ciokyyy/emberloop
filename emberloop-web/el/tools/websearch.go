package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"example.com/emberloop-web/el/statedb"
)

// Websearch: a provider-neutral web search tool (Brave, then Exa as fallback)
// served the same way as the existing file tools.
//
// Arguments: query (required), count (default 10), freshness
// (day|week|month|year), domains (include list), provider (brave|exa|auto;
// default from search.provider in settings, else auto), fresh (cache bypass).
// There is no mode argument: search.mode in settings picks the fallback chain
// or concurrent providers.
//
// Keys come from the environment (BRAVE_API_KEY, EXA_API_KEY). Under auto a
// provider with no key is skipped with a note; a named provider with no key
// is an error. Under auto Brave answers first and Exa is tried on network
// error, timeout, HTTP 429 or quota exhaustion, 5xx, and zero results; a 4xx
// the query caused does not fall back. A named provider never falls back.
//
// Output is plain text: a numbered list (title, URL, snippet, published date
// when known, sources when more than one) and a footer naming the answering
// provider, any fallbacks, whether the result came from cache, and the latest
// known quota.

const (
	// defaultBraveEndpoint and defaultExaEndpoint are the provider APIs the
	// adapters call. Tests point them at fake servers through
	// EL_BRAVE_ENDPOINT and EL_EXA_ENDPOINT.
	defaultBraveEndpoint = "https://api.search.brave.com"
	defaultExaEndpoint   = "https://api.exa.ai"
	// defaultSearchCount is the result count when count is absent or not
	// positive; maxSearchCount keeps one call inside both providers' page.
	defaultSearchCount = 10
	maxSearchCount     = 20
	// defaultSearchTimeout bounds one provider call; EL_SEARCH_TIMEOUT
	// overrides it (a Go duration), so tests own the clock.
	defaultSearchTimeout = 15 * time.Second
	// providerBodyLimit caps one provider response body: search answers are
	// small JSON, and a huge body must not reach the database or context.
	providerBodyLimit = 1 << 20
)

type websearchTool struct{}

func (websearchTool) Name() string   { return "websearch" }
func (websearchTool) ReadOnly() bool { return true }
func (websearchTool) Description() string {
	return "Search the web (Brave, with Exa as fallback): a compact numbered list of title, URL, snippet and publish date. " +
		"Takes a query plus optional count, freshness (day|week|month|year), domains (include list), provider (brave|exa|auto) and fresh (bypass the 15-minute cache)."
}
func (websearchTool) Schema() map[string]any {
	return obj(map[string]any{
		"query":     map[string]any{"type": "string", "description": "What to search for."},
		"count":     map[string]any{"type": "integer", "description": "How many results (default 10, at most 20)."},
		"freshness": map[string]any{"type": "string", "description": "Only recent results: day, week, month or year."},
		"domains":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Only these domains."},
		"provider":  map[string]any{"type": "string", "description": "brave, exa or auto (default from settings, else auto). A named provider never falls back."},
		"fresh":     map[string]any{"type": "boolean", "description": "True bypasses the 15-minute cache and refreshes it."},
	}, "query")
}

// searchArgs is one websearch call after validation.
type searchArgs struct {
	query     string
	count     int
	freshness string
	domains   []string
	provider  string
	fresh     bool
}

// parseSearchArgs validates the raw arguments. provider is the resolved
// default from settings (auto when nothing is stored); an explicit provider
// outside the vocabulary is an error, never silently auto.
func parseSearchArgs(raw json.RawMessage, defaultProvider string) (searchArgs, error) {
	var a struct {
		Query     string   `json:"query"`
		Count     int      `json:"count"`
		Freshness string   `json:"freshness"`
		Domains   []string `json:"domains"`
		Provider  string   `json:"provider"`
		Fresh     bool     `json:"fresh"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return searchArgs{}, fmt.Errorf("invalid websearch arguments: %w", err)
	}
	q := searchArgs{
		query:     strings.Join(strings.Fields(strings.TrimSpace(a.Query)), " "),
		count:     a.Count,
		freshness: strings.ToLower(strings.TrimSpace(a.Freshness)),
		provider:  strings.ToLower(strings.TrimSpace(a.Provider)),
		fresh:     a.Fresh,
	}
	if q.query == "" {
		return searchArgs{}, errors.New("query cannot be empty")
	}
	if q.count <= 0 {
		q.count = defaultSearchCount
	}
	if q.count > maxSearchCount {
		q.count = maxSearchCount
	}
	switch q.freshness {
	case "", "day", "week", "month", "year":
	default:
		return searchArgs{}, fmt.Errorf("unknown freshness %q (have day, week, month, year)", a.Freshness)
	}
	for _, d := range a.Domains {
		if norm := normaliseDomain(d); norm != "" {
			q.domains = append(q.domains, norm)
		}
	}
	if q.provider == "" {
		q.provider = defaultProvider
	}
	if !validSearchProvider(q.provider) {
		return searchArgs{}, fmt.Errorf("unknown provider %q (have brave, exa, auto)", a.Provider)
	}
	return q, nil
}

// normaliseDomain lowercases a domain entry and strips a scheme, path and
// port, so "https://Example.com/x" and "example.com" match the same host.
func normaliseDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	if i := strings.IndexByte(d, ':'); i >= 0 {
		d = d[:i]
	}
	return strings.TrimSpace(d)
}

// domainAllowed reports whether host passes the include list: empty means
// all, otherwise the host itself or a subdomain of an entry.
func domainAllowed(host string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	host = strings.ToLower(host)
	for _, d := range include {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// searchResult is one provider-neutral hit. Sources names every provider
// that returned the URL, in chain order; a fallback answer carries one.
type searchResult struct {
	title     string
	url       string
	snippet   string
	published string
	sources   []string
}

// searchQuota is what one live call reported: Exa's cost when it reports
// one, Brave's rate-limit remaining and reset when its headers carry them.
// Absent stays nil and empty: nothing is invented.
type searchQuota struct {
	cost      *float64
	remaining *int64
	reset     string
}

// providerError is one failed provider call: a transport error (status 0),
// a quota or server failure worth falling back over (retryable), or a 4xx
// the query caused (queryFault), which fails the call outright.
type providerError struct {
	provider   string
	status     int
	msg        string
	retryable  bool
	queryFault bool
}

func (e *providerError) Error() string {
	if e.status == 0 {
		return fmt.Sprintf("%s: %s", e.provider, e.msg)
	}
	if e.msg == "" {
		return fmt.Sprintf("%s: HTTP %d", e.provider, e.status)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.provider, e.status, e.msg)
}

// shortReason is the footer form of a failure: "HTTP 500", "timed out".
func (e *providerError) shortReason() string {
	if e.status == 0 {
		return e.msg
	}
	return fmt.Sprintf("HTTP %d", e.status)
}

// classifyProviderError sorts one failed call: transport errors, timeouts,
// 429, 402 and 5xx may fall back; any other 4xx is the query's fault,
// except a 403 whose body reads as quota exhaustion. The body snippet rides
// along for the named-provider error, truncated to one line.
func classifyProviderError(provider string, status int, body string, callErr error) *providerError {
	if callErr != nil {
		if isTimeoutError(callErr) {
			return &providerError{provider: provider, msg: "timed out", retryable: true}
		}
		return &providerError{provider: provider, msg: strings.TrimSpace(firstLine(callErr.Error())), retryable: true}
	}
	snip := strings.TrimSpace(firstLine(body))
	switch {
	case status == 429 || status == 402:
		return &providerError{provider: provider, status: status, msg: snip, retryable: true}
	case status >= 500:
		return &providerError{provider: provider, status: status, msg: snip, retryable: true}
	case status == 403 && quotaLikeBody(body):
		return &providerError{provider: provider, status: status, msg: snip, retryable: true}
	default:
		return &providerError{provider: provider, status: status, msg: snip, queryFault: true}
	}
}

// quotaLikeBody reports whether a 403 body reads as quota exhaustion rather
// than a refused key: quota, credit, billing and rate words mark it.
func quotaLikeBody(body string) bool {
	lower := strings.ToLower(body)
	for _, word := range []string{"quota", "credit", "billing", "rate limit", "ratelimit", "exceed", "allowance"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// isTimeoutError reports whether err is a client timeout: the stdlib wraps
// it as a *url.Error whose message names the timeout.
func isTimeoutError(err error) bool {
	type timeout interface{ Timeout() bool }
	var te timeout
	if errors.As(err, &te) && te.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "client.timeout")
}

// firstLine is the first line of s, capped for error text.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// searchAdapter is one engine behind the provider-neutral tool: it searches,
// parses its results and parses its quota report, so a later engine is one
// adapter each.
type searchAdapter interface {
	// name is brave or exa, the provider column and footer vocabulary.
	name() string
	// keyEnv names the environment variable holding the key.
	keyEnv() string
	// search runs one live query. quota carries whatever the provider
	// reported, nil and empty when it reported nothing.
	search(ctx context.Context, key string, q searchArgs) ([]searchResult, searchQuota, error)
}

// searchAdapters is the provider set in chain order: Brave, then Exa.
func searchAdapters() []searchAdapter { return []searchAdapter{braveAdapter{}, exaAdapter{}} }

// providerKeys reads every provider key from the environment.
func providerKeys() map[string]string {
	return map[string]string{
		SearchProviderBrave: strings.TrimSpace(os.Getenv("BRAVE_API_KEY")),
		SearchProviderExa:   strings.TrimSpace(os.Getenv("EXA_API_KEY")),
	}
}

// searchTimeout bounds one provider call: EL_SEARCH_TIMEOUT (a Go
// duration) wins, so tests own the clock; garbage reads as the default.
func searchTimeout() time.Duration {
	if s := strings.TrimSpace(os.Getenv("EL_SEARCH_TIMEOUT")); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return defaultSearchTimeout
}

func (websearchTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	settings := EffectiveSettings(root)
	q, err := parseSearchArgs(args, settings.EffectiveSearchProvider())
	if err != nil {
		return "", err
	}
	mode := settings.EffectiveSearchMode()
	now := time.Now().Unix()
	// Missing keys are configuration errors, so they fail before the cache:
	// a cached answer must not mask a key the user removed.
	if err := checkSearchKeys(q.provider); err != nil {
		return "", err
	}
	// An identical search within 15 minutes answers from the database,
	// without calling the provider again; fresh bypasses the cache.
	if !q.fresh {
		if hit, ok := lookupSearchCache(q, mode, now); ok {
			return hit, nil
		}
	}
	var answer searchAnswer
	if mode == SearchModeParallel {
		answer, err = parallelSearch(ctx, q)
	} else {
		answer, err = fallbackSearch(ctx, q)
	}
	if err != nil {
		return "", err
	}
	list := renderSearchList(q.query, answer.label, answer.results)
	storeSearchCache(q, mode, answer, list, now)
	return list + "---\n" + renderSearchFooter(answer, liveCacheStatus(q.fresh)), nil
}

// searchAnswer is one answered search: the results, the footer provider
// label, the providers whose latest quota the footer shows, and the
// fallback notes. Both runners return it; Execute renders and caches it.
type searchAnswer struct {
	results  []searchResult
	label    string
	quotaFor []string
	notes    []fallbackNote
}

// cachedFallback is the stored form of one fallback note.
type cachedFallback struct {
	Provider string `json:"provider"`
	Note     string `json:"note"`
}

// searchCacheArgs is the args column: the request plus the answer, so a hit
// renders the same provider and fallbacks without a live call.
type searchCacheArgs struct {
	Provider  string           `json:"provider"`
	Mode      string           `json:"mode"`
	Count     int              `json:"count"`
	Freshness string           `json:"freshness"`
	Domains   []string         `json:"domains"`
	Answer    string           `json:"answer"`
	QuotaFor  []string         `json:"quota_for"`
	Fallbacks []cachedFallback `json:"fallbacks"`
}

// lookupSearchCache reads the cached answer for an identical search. ok is
// false on a miss, an expired row, an unreadable row, or a database
// failure: the search then goes live, so a broken cache never breaks
// search.
func lookupSearchCache(q searchArgs, mode string, now int64) (hit string, ok bool) {
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		return "", false
	}
	defer db.Close()
	row, found, err := statedb.SearchCacheGet(db, searchCacheKey(q, mode), now)
	if err != nil || !found {
		return "", false
	}
	var stored searchCacheArgs
	if err := json.Unmarshal([]byte(row.Args), &stored); err != nil {
		return "", false
	}
	var notes []fallbackNote
	for _, f := range stored.Fallbacks {
		notes = append(notes, fallbackNote{provider: f.Provider, note: f.Note})
	}
	answer := searchAnswer{label: row.Provider, quotaFor: stored.QuotaFor, notes: notes}
	return row.Results + "---\n" + renderSearchFooter(answer, cacheCached), true
}

// storeSearchCache stores one answered search, refreshing the entry: only
// searches are cached, and a database failure never fails the search.
func storeSearchCache(q searchArgs, mode string, answer searchAnswer, list string, now int64) {
	var fallbacks []cachedFallback
	for _, n := range answer.notes {
		fallbacks = append(fallbacks, cachedFallback{Provider: n.provider, Note: n.note})
	}
	encoded, err := json.Marshal(searchCacheArgs{
		Provider: q.provider, Mode: mode, Count: q.count,
		Freshness: q.freshness, Domains: q.domains,
		Answer: answer.label, QuotaFor: answer.quotaFor, Fallbacks: fallbacks,
	})
	if err != nil {
		return
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		return
	}
	defer db.Close()
	_ = statedb.SearchCachePut(db, statedb.SearchCacheRow{
		Key: searchCacheKey(q, mode), Query: q.query, Provider: answer.label,
		Args: string(encoded), Results: list, CreatedAt: now,
	})
}

// orderedProviders is the provider set for one call, in run order: a named
// provider alone, or the whole chain under auto.
func orderedProviders(requested string) []searchAdapter {
	if requested != SearchProviderAuto {
		for _, a := range searchAdapters() {
			if a.name() == requested {
				return []searchAdapter{a}
			}
		}
	}
	return searchAdapters()
}

// fallbackNote is one footer note: a skipped or failed provider and why.
type fallbackNote struct {
	provider string
	note     string
}

// String renders one note: "brave skipped (BRAVE_API_KEY not set)".
func (n fallbackNote) String() string { return n.provider + " " + n.note }

// liveOutcome is one provider's live call: its results (possibly none), what
// it reported for quota, and how it failed (nil on success).
type liveOutcome struct {
	adapter searchAdapter
	results []searchResult
	quota   searchQuota
	failure *providerError
}

// callProvider runs one live provider call with the call timeout, recording
// a quota row for every call that reached the provider with a response.
func callProvider(ctx context.Context, a searchAdapter, key string, q searchArgs) liveOutcome {
	callCtx, cancel := context.WithTimeout(ctx, searchTimeout())
	defer cancel()
	results, quota, err := a.search(callCtx, key, q)
	out := liveOutcome{adapter: a, results: results, quota: quota}
	if err != nil {
		var perr *providerError
		if errors.As(err, &perr) {
			out.failure = perr
		} else if isTimeoutError(err) || errors.Is(err, context.DeadlineExceeded) {
			out.failure = &providerError{provider: a.name(), msg: "timed out", retryable: true}
		} else {
			out.failure = &providerError{provider: a.name(), msg: firstLine(err.Error()), retryable: true}
		}
		recordSearchQuota(a.name(), quota, out.failure.status != 0)
		return out
	}
	recordSearchQuota(a.name(), quota, true)
	return out
}

// recordSearchQuota writes one row per live provider call that carries an
// HTTP response: what the provider reported, NULLs when it reported
// nothing. A call that never reached the provider records nothing, and a
// database failure never fails the search: quota is observed, not gating.
func recordSearchQuota(provider string, quota searchQuota, reached bool) {
	if !reached {
		return
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		return
	}
	defer db.Close()
	_ = statedb.InsertSearchQuota(db, statedb.SearchQuotaRow{
		Provider:  provider,
		Cost:      quota.cost,
		Remaining: quota.remaining,
		ResetAt:   quota.reset,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// checkSearchKeys fails fast on missing keys: a named provider without its
// key, or auto with no key at all. The runners repeat the check per
// provider, so a key removed mid-call still reads correctly.
func checkSearchKeys(requested string) error {
	keys := providerKeys()
	if requested != SearchProviderAuto {
		for _, a := range searchAdapters() {
			if a.name() == requested && keys[a.name()] == "" {
				return fmt.Errorf("%s not set", a.keyEnv())
			}
		}
		return nil
	}
	var missing []string
	for _, a := range searchAdapters() {
		if keys[a.name()] == "" {
			missing = append(missing, a.keyEnv()+" not set")
		}
	}
	if len(missing) == len(searchAdapters()) {
		return errors.New(strings.Join(missing, "; "))
	}
	return nil
}

// parallelSearch runs the configured providers concurrently behind
// search.mode: parallel. Every keyed provider is called at once, each with
// its own timeout; failures become footer notes while the rest still merge.
// Results are deduplicated by normalised URL and fused by reciprocal rank
// fusion, and every result carries the providers that returned it. A single
// configured provider answers alone, the way a named provider does.
func parallelSearch(ctx context.Context, q searchArgs) (searchAnswer, error) {
	keys := providerKeys()
	type runnable struct {
		adapter searchAdapter
		key     string
	}
	var run []runnable
	var notes []fallbackNote
	for _, a := range orderedProviders(q.provider) {
		key := keys[a.name()]
		if key == "" {
			if q.provider == SearchProviderAuto {
				notes = append(notes, fallbackNote{provider: a.name(), note: "skipped (" + a.keyEnv() + " not set)"})
				continue
			}
			return searchAnswer{}, fmt.Errorf("%s not set", a.keyEnv())
		}
		run = append(run, runnable{adapter: a, key: key})
	}
	if len(run) == 0 {
		var missing []string
		for _, a := range orderedProviders(q.provider) {
			missing = append(missing, a.keyEnv()+" not set")
		}
		return searchAnswer{}, errors.New(strings.Join(missing, "; "))
	}
	outcomes := make([]liveOutcome, len(run))
	var wg sync.WaitGroup
	for i, r := range run {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i] = callProvider(ctx, r.adapter, r.key, q)
		}()
	}
	wg.Wait()
	lists := make([][]searchResult, 0, len(run))
	var answered []string
	for _, out := range outcomes {
		if out.failure != nil {
			if len(run) == 1 {
				return searchAnswer{}, out.failure
			}
			notes = append(notes, fallbackNote{provider: out.adapter.name(), note: "failed (" + out.failure.shortReason() + ")"})
			continue
		}
		answered = append(answered, out.adapter.name())
		lists = append(lists, out.results)
	}
	if len(answered) == 0 {
		if q.provider != SearchProviderAuto {
			return searchAnswer{}, fmt.Errorf("%s: no answer", q.provider)
		}
		return searchAnswer{}, fmt.Errorf("websearch: no provider answered (%s)", joinNotes(notes))
	}
	label := strings.Join(answered, ", ")
	if len(run) > 1 {
		label += " (parallel)"
	}
	merged := fuseResults(lists, answered)
	if len(merged) > q.count {
		merged = merged[:q.count]
	}
	return searchAnswer{results: merged, label: label, quotaFor: answered, notes: notes}, nil
}

// rrfK is the reciprocal-rank-fusion constant: 1/(k+rank) per listing, with
// the standard k of 60, so a URL both providers rank first fuses above one
// only one provider returned.
const rrfK = 60.0

// fuseResults deduplicates provider lists by normalised URL and fuses them
// by reciprocal rank fusion: every shared URL gains from each listing that
// holds it. names runs in chain order, so sources read Brave first. Ties
// break by best rank, then URL, so the order is deterministic.
func fuseResults(lists [][]searchResult, names []string) []searchResult {
	type fused struct {
		result   searchResult
		score    float64
		bestRank int
	}
	byURL := map[string]*fused{}
	for li, list := range lists {
		for rank, r := range list {
			key := normaliseSearchURL(r.url)
			entry, ok := byURL[key]
			if !ok {
				entry = &fused{result: r, bestRank: rank + 1}
				byURL[key] = entry
			}
			entry.score += 1 / (rrfK + float64(rank+1))
			if rank+1 < entry.bestRank {
				entry.bestRank = rank + 1
			}
			if !slicesContains(entry.result.sources, names[li]) {
				entry.result.sources = append(entry.result.sources, names[li])
			}
		}
	}
	scores := make([]fused, 0, len(byURL))
	for _, entry := range byURL {
		scores = append(scores, *entry)
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		if scores[i].bestRank != scores[j].bestRank {
			return scores[i].bestRank < scores[j].bestRank
		}
		return scores[i].result.url < scores[j].result.url
	})
	merged := make([]searchResult, 0, len(byURL))
	for _, s := range scores {
		merged = append(merged, s.result)
	}
	return merged
}

// slicesContains reports whether list holds name.
func slicesContains(list []string, name string) bool {
	for _, s := range list {
		if s == name {
			return true
		}
	}
	return false
}

// trackingParams are query parameters stripped before comparing URLs: they
// mark the click, not the page, so the same result shared with different
// tags still deduplicates.
var trackingParams = []string{
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
	"gclid", "fbclid", "msclkid",
}

// normaliseSearchURL canonicalises a result URL for dedupe: lowercase
// scheme and host, default ports and fragments dropped, tracking parameters
// removed, one trailing slash trimmed. Unparsable values compare
// case-insensitively on the raw string.
func normaliseSearchURL(raw string) string {
	s := strings.TrimSpace(raw)
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return strings.ToLower(s)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" &&
		!((u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443")) {
		host += ":" + port
	}
	u.Host = host
	u.Fragment = ""
	vals := u.Query()
	for _, t := range trackingParams {
		vals.Del(t)
	}
	u.RawQuery = vals.Encode()
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}

// fallbackSearch runs the Brave-then-Exa chain: the first provider with a key
// and a non-empty answer wins; failures and empty answers advance under auto,
// while a named provider and a bad query return the real error.
func fallbackSearch(ctx context.Context, q searchArgs) (searchAnswer, error) {
	keys := providerKeys()
	var notes []fallbackNote
	for _, a := range orderedProviders(q.provider) {
		key := keys[a.name()]
		if key == "" {
			if q.provider == SearchProviderAuto {
				notes = append(notes, fallbackNote{provider: a.name(), note: "skipped (" + a.keyEnv() + " not set)"})
				continue
			}
			return searchAnswer{}, fmt.Errorf("%s not set", a.keyEnv())
		}
		out := callProvider(ctx, a, key, q)
		if out.failure != nil {
			if out.failure.queryFault || q.provider != SearchProviderAuto {
				return searchAnswer{}, out.failure
			}
			notes = append(notes, fallbackNote{provider: a.name(), note: "failed (" + out.failure.shortReason() + ")"})
			continue
		}
		if len(out.results) == 0 {
			if q.provider != SearchProviderAuto {
				return searchAnswer{label: a.name(), quotaFor: []string{a.name()}}, nil
			}
			notes = append(notes, fallbackNote{provider: a.name(), note: "returned no results"})
			continue
		}
		return searchAnswer{results: tagSources(out.results, a.name()), label: a.name(), quotaFor: []string{a.name()}, notes: notes}, nil
	}
	if len(notes) > 0 && allSkipped(notes) {
		var missing []string
		for _, a := range orderedProviders(q.provider) {
			missing = append(missing, a.keyEnv()+" not set")
		}
		return searchAnswer{}, errors.New(strings.Join(missing, "; "))
	}
	if q.provider != SearchProviderAuto {
		return searchAnswer{}, fmt.Errorf("%s: no answer", q.provider)
	}
	return searchAnswer{}, fmt.Errorf("websearch: no provider answered (%s)", joinNotes(notes))
}

// allSkipped reports whether every note is a skipped provider: no key
// anywhere means the error names the keys, not the fallbacks.
func allSkipped(notes []fallbackNote) bool {
	if len(notes) == 0 {
		return false
	}
	for _, n := range notes {
		if !strings.HasPrefix(n.note, "skipped") {
			return false
		}
	}
	return true
}

// joinNotes joins fallback notes for an error: "brave failed (HTTP 500), exa
// failed (HTTP 500)".
func joinNotes(notes []fallbackNote) string {
	parts := make([]string, 0, len(notes))
	for _, n := range notes {
		parts = append(parts, n.String())
	}
	return strings.Join(parts, ", ")
}

// tagSources names the answering provider on every result of a fallback
// answer: one source each.
func tagSources(results []searchResult, provider string) []searchResult {
	for i := range results {
		results[i].sources = []string{provider}
	}
	return results
}

// cacheStatus is where the answer came from: a live call, a cache hit, or a
// live call that bypassed the cache.
type cacheStatus string

const (
	cacheLive   cacheStatus = "live"
	cacheCached cacheStatus = "cached"
	cacheFresh  cacheStatus = "fresh"
)

// cacheFresh is the status of a live call with the fresh flag: bypassed when
// set, plain live otherwise.
func liveCacheStatus(fresh bool) cacheStatus {
	if fresh {
		return cacheFresh
	}
	return cacheLive
}

// renderSearchList renders the numbered list: title, URL, snippet, published
// date when known, and sources when more than one provider returned the URL.
// The footer is rendered separately, so a cache hit re-renders it with the
// latest quota instead of the stored one.
func renderSearchList(query, label string, results []searchResult) string {
	var sb strings.Builder
	if len(results) == 0 {
		fmt.Fprintf(&sb, "no results for %q from %s\n", query, label)
	} else {
		for i, r := range results {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, r.title)
			fmt.Fprintf(&sb, "   URL: %s\n", r.url)
			if r.snippet != "" {
				fmt.Fprintf(&sb, "   Snippet: %s\n", singleLine(r.snippet))
			}
			if r.published != "" {
				fmt.Fprintf(&sb, "   Published: %s\n", r.published)
			}
			if len(r.sources) > 1 {
				fmt.Fprintf(&sb, "   Sources: %s\n", strings.Join(r.sources, ", "))
			}
		}
	}
	return sb.String()
}

// renderSearchFooter renders the answer footer: the answering provider, any
// fallbacks, the cache status and the latest known quota.
func renderSearchFooter(answer searchAnswer, status cacheStatus) string {
	footer := "provider: " + answer.label
	if len(answer.notes) > 0 {
		footer += " (fallback: " + joinNotes(answer.notes) + ")"
	}
	footer += " | cache: " + string(status)
	if quota := latestQuotaText(answer.quotaFor); quota != "" {
		footer += " | quota: " + quota
	}
	return footer
}

// singleLine collapses a snippet to one line: provider snippets carry
// newlines that would break the numbered list.
func singleLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

// latestQuotaText is the footer's quota note: the latest reported quota per
// provider the answer came from (remaining and reset, cost), joined when
// several providers answered. A provider whose latest rows report nothing
// contributes nothing, so a headerless failure never hides the last known
// quota.
func latestQuotaText(providers []string) string {
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		return ""
	}
	defer db.Close()
	var notes []string
	for _, provider := range providers {
		row, ok, err := statedb.LatestReportedSearchQuota(db, provider)
		if err != nil || !ok {
			continue
		}
		var parts []string
		if row.Remaining != nil {
			part := "remaining " + strconv.FormatInt(*row.Remaining, 10)
			if row.ResetAt != "" {
				part += " (reset " + row.ResetAt + ")"
			}
			parts = append(parts, part)
		}
		if row.Cost != nil {
			parts = append(parts, "last cost $"+strconv.FormatFloat(*row.Cost, 'f', -1, 64))
		}
		if len(parts) == 0 {
			continue
		}
		notes = append(notes, provider+" "+strings.Join(parts, ", "))
	}
	return strings.Join(notes, "; ")
}

// searchCacheKey hashes the normalised query, the resolved provider and the
// call arguments, so an identical search within 15 minutes answers from the
// row without calling the provider again.
func searchCacheKey(q searchArgs, mode string) string {
	domains := append([]string(nil), q.domains...)
	sort.Strings(domains)
	raw := strings.Join([]string{
		q.query, q.provider, mode,
		strconv.Itoa(q.count), q.freshness, strings.Join(domains, ","),
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
