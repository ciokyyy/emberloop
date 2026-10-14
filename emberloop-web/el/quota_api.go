package el

// Quota snapshot for the browser UI. Registered sources own provider fetch
// and cache behavior; shared snapshots and concurrent refresh dispatch stay
// provider-agnostic.

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

// Quota states describe whether a card has usable, current values.
const (
	QuotaOK            = "ok"
	QuotaStale         = "stale"
	QuotaExpired       = "expired"
	QuotaNotConfigured = "not_configured"
	QuotaError         = "error"
)

// QuotaWindow is one comparable account limit: percent used and its reset.
type QuotaWindow struct {
	Label    string `json:"label"`
	Pct      int    `json:"pct"`
	ResetsAt int64  `json:"resets_at"`
	Detail   string `json:"detail,omitempty"`
}

// QuotaProvider is one card on the quota page. Error is always present in
// JSON; Hint is reserved for actionable states such as expired or missing
// credentials.
type QuotaProvider struct {
	Key       string        `json:"key"`
	Label     string        `json:"label"`
	Plan      string        `json:"plan"`
	State     string        `json:"state"`
	UpdatedAt int64         `json:"updated_at"`
	Error     string        `json:"error"`
	Hint      string        `json:"hint,omitempty"`
	Windows   []QuotaWindow `json:"windows"`
}

// quotaSource keeps provider-specific fetch, configuration, hint and cache
// path behavior behind the shared snapshot, endpoint and refresher. Related
// cards may share a FetchKey while returning their own snapshots.
type quotaSource interface {
	Key() string
	FetchKey() string
	Fetch(context.Context, string, time.Time, bool) error
	Configured(string, time.Time) bool
	NotConfiguredHint(string) string
	CachePath(string) string
	Snapshot(string, time.Time, error) QuotaProvider
}

// usageQuotaSource adapts the existing shared usage cache and lock used by
// terminal header panes to the quota card contract.
type usageQuotaSource struct {
	key, label, plan, fetchKey string
	configuredFn               func(string, time.Time) bool
	expiredFn                  func(string, time.Time) bool
	expiredHint                string
	hintFn                     func(string) string
	cachePathFn                func(string) string
}

func (s usageQuotaSource) Key() string      { return s.key }
func (s usageQuotaSource) FetchKey() string { return s.fetchKey }

func (s usageQuotaSource) Configured(stateDir string, now time.Time) bool {
	return s.configuredFn(stateDir, now)
}

func (s usageQuotaSource) NotConfiguredHint(stateDir string) string {
	return s.hintFn(stateDir)
}

func (s usageQuotaSource) CachePath(stateDir string) string {
	return s.cachePathFn(stateDir)
}

func (s usageQuotaSource) Fetch(ctx context.Context, stateDir string, now time.Time, force bool) error {
	if s.fetchKey == "" { // Claude's statusline file is the fallback source.
		return nil
	}
	if force {
		return RefreshUsageForce(ctx, stateDir, s.fetchKey, now)
	}
	return RefreshUsage(ctx, stateDir, s.fetchKey, now)
}

func (s usageQuotaSource) Snapshot(stateDir string, now time.Time, fetchErr error) QuotaProvider {
	set := ReadUsage(stateDir, s.key, now)
	p := QuotaProvider{
		Key:       s.key,
		Label:     s.label,
		Plan:      s.plan,
		UpdatedAt: quotaUpdatedAt(s.CachePath(stateDir)),
		Windows:   quotaWindows(set.Windows),
	}
	if p.Windows == nil {
		p.Windows = []QuotaWindow{}
	}

	var failure *quotaFailure
	if errors.As(fetchErr, &failure) && failure.state == QuotaExpired {
		p.State = QuotaExpired
		p.Hint = failure.hint
		return p
	}
	if s.expiredFn != nil && s.expiredFn(stateDir, now) {
		p.State = QuotaExpired
		p.Hint = s.expiredHint
		return p
	}
	if !s.Configured(stateDir, now) {
		p.State = QuotaNotConfigured
		p.Hint = s.NotConfiguredHint(stateDir)
		return p
	}
	switch {
	case set.Has && fetchErr != nil:
		p.State = QuotaStale
		p.Error = fetchErr.Error()
		if errors.As(fetchErr, &failure) {
			p.Hint = failure.hint
		}
	case set.Has:
		p.State = QuotaOK
	case fetchErr != nil:
		p.State = QuotaError
		p.Error = fetchErr.Error()
	default:
		p.State = QuotaError
		p.Error = "no cached usage yet"
	}
	return p
}

// quotaFailure lets a provider distinguish an expired login from a transient
// fetch failure while retaining the common cache and response path.
type quotaFailure struct {
	state string
	hint  string
	err   error
}

func (e *quotaFailure) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.hint
}

func (e *quotaFailure) Unwrap() error { return e.err }

// quotaSources is the registry used by the cache snapshot, validation, and
// refresher. Add a provider here without teaching those paths its fetch logic.
var quotaSources = []quotaSource{
	usageQuotaSource{
		key: usageKeyClaude, label: "Claude",
		configuredFn: func(stateDir string, now time.Time) bool {
			return ReadClaudeUsage(stateDir, now).Has
		},
		hintFn: func(string) string {
			return "Claude quota needs a host Claude login or a Claude Code statusline report."
		},
		cachePathFn: claudeUsagePath,
	},
	usageQuotaSource{
		key: usageKeyChatGPT, label: "ChatGPT Plus", plan: "Plus", fetchKey: usageKeyChatGPT,
		configuredFn: chatGPTPlusConfigured,
		expiredFn: func(_ string, now time.Time) bool {
			return chatGPTPlusExpired(now)
		},
		expiredHint: chatGPTPlusExpiredHint,
		hintFn:      chatGPTPlusNotConfiguredHint,
		cachePathFn: func(stateDir string) string { return usageCachePath(stateDir, usageKeyChatGPT) },
	},
	usageQuotaSource{
		key: usageKeyOpencode, label: "OpenCode-Go", plan: "Go", fetchKey: usageKeyOpencode,
		configuredFn: func(string, time.Time) bool {
			auth, err := readPigAuth()
			return err == nil && auth.OpencodeGo.Key != ""
		},
		hintFn:      func(string) string { return "Missing OpenCode-Go login in pig auth.json (opencode-go.key)." },
		cachePathFn: func(stateDir string) string { return usageCachePath(stateDir, usageKeyOpencode) },
	},
	usageQuotaSource{
		key: usageKeyAgyGem, label: "Antigravity Gemini", plan: "Gemini", fetchKey: usageKeyAgyGem,
		configuredFn: antigravityQuotaConfigured,
		hintFn:       antigravityQuotaNotConfiguredHint,
		cachePathFn:  func(stateDir string) string { return usageCachePath(stateDir, usageKeyAgyGem) },
	},
	usageQuotaSource{
		key: usageKeyAgy3P, label: "Antigravity 3P", plan: "Claude + GPT-OSS", fetchKey: usageKeyAgyGem,
		configuredFn: antigravityQuotaConfigured,
		hintFn:       antigravityQuotaNotConfiguredHint,
		cachePathFn:  func(stateDir string) string { return usageCachePath(stateDir, usageKeyAgy3P) },
	},
}

// QuotaFetchKeys returns the distinct upstreams selected by a card key, or
// every fetchable source when key is empty. Local-only sources have no key.
func QuotaFetchKeys(key string) []string {
	seen := make(map[string]bool)
	var keys []string
	for _, source := range quotaSources {
		if key != "" && source.Key() != key {
			continue
		}
		fetchKey := source.FetchKey()
		if fetchKey == "" || seen[fetchKey] {
			continue
		}
		seen[fetchKey] = true
		keys = append(keys, fetchKey)
	}
	return keys
}

// quotaUpdatedAt is the cache file's mtime, 0 when never written.
func quotaUpdatedAt(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.ModTime().Unix()
}

func antigravityQuotaConfigured(_ string, _ time.Time) bool {
	auth, err := readPigAuth()
	if err != nil || auth.Antigravity.Refresh == "" {
		return false
	}
	_, _, err = agyClient()
	return err == nil
}

func antigravityQuotaNotConfiguredHint(_ string) string {
	var missing []string
	auth, err := readPigAuth()
	if err != nil || auth.Antigravity.Refresh == "" {
		missing = append(missing, "pig auth.json google-antigravity.refresh login")
	}
	if _, _, err := agyClient(); err != nil {
		missing = append(missing, "~/.el/agy-oauth.json client credentials")
	}
	if len(missing) == 0 {
		return "Antigravity login is not configured."
	}
	return "Missing " + strings.Join(missing, " and ") + "."
}

func quotaWindowLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "5h", "5-hour":
		return "5-hour"
	case "wk", "weekly":
		return "Weekly"
	case "mo", "monthly":
		return "Monthly"
	default:
		return label
	}
}

func quotaWindows(windows []UsageWindow) []QuotaWindow {
	out := make([]QuotaWindow, 0, len(windows))
	for _, window := range windows {
		pct := window.Pct
		if pct < 0 {
			pct = 0
		} else if pct > 100 {
			pct = 100
		}
		out = append(out, QuotaWindow{
			Label:    quotaWindowLabel(window.Label),
			Pct:      pct,
			ResetsAt: window.ResetsAt,
		})
	}
	return out
}

// QuotaKeyValid reports a key the refresh endpoint accepts: "" (all) or one
// of the registered card keys.
func QuotaKeyValid(key string) bool {
	if key == "" {
		return true
	}
	for _, source := range quotaSources {
		if source.Key() == key {
			return true
		}
	}
	return false
}

// RefreshQuota refreshes selected registered sources through the shared cache
// and per-key lock. An empty key refreshes every fetchable source concurrently.
// force skips the 90s age guard; the Daemon's manual refresh endpoint reserves
// its per-provider cooldown before calling here. Failed fetches retain cached
// windows, which the snapshot reports as stale. Local-only sources such as
// Claude have no upstream fetch key and do not require a fetch.
func RefreshQuota(ctx context.Context, stateDir, key string, now time.Time, force bool) map[string]error {
	return RefreshQuotaWithCallback(ctx, stateDir, key, now, force, nil)
}

// RefreshQuotaWithCallback reports each independent fetch as it completes so
// the Daemon can publish one provider's result without waiting for another.
func RefreshQuotaWithCallback(ctx context.Context, stateDir, key string, now time.Time, force bool, onFetchComplete func(string, map[string]error)) map[string]error {
	sources := make([]quotaSource, 0, len(quotaSources))
	seen := map[string]bool{}
	for _, source := range quotaSources {
		if key != "" && source.Key() != key {
			continue
		}
		fetchKey := source.FetchKey()
		if fetchKey == "" || seen[fetchKey] {
			continue
		}
		seen[fetchKey] = true
		sources = append(sources, source)
	}

	errsBySource := make([]error, len(sources))
	var wg sync.WaitGroup
	wg.Add(len(sources))
	for i, source := range sources {
		go func(i int, source quotaSource) {
			defer wg.Done()
			fetchErr := source.Fetch(ctx, stateDir, now, force)
			errsBySource[i] = fetchErr
			if onFetchComplete != nil {
				onFetchComplete(source.FetchKey(), quotaFetchErrors(source.FetchKey(), fetchErr))
			}
		}(i, source)
	}
	wg.Wait()

	errs := map[string]error{}
	for i, fetchErr := range errsBySource {
		for cardKey, err := range quotaFetchErrors(sources[i].FetchKey(), fetchErr) {
			errs[cardKey] = err
		}
	}
	return errs
}

func quotaFetchErrors(fetchKey string, err error) map[string]error {
	if err == nil {
		return nil
	}
	errs := map[string]error{}
	for _, source := range quotaSources {
		if source.FetchKey() == fetchKey {
			errs[source.Key()] = err
		}
	}
	return errs
}

// QuotaSnapshot reads the provider caches only and assembles the registered
// cards. A failed refresh retains its values as stale; expired failures keep
// the values but carry an actionable hint.
func QuotaSnapshot(stateDir string, now time.Time, errs map[string]error) []QuotaProvider {
	out := make([]QuotaProvider, 0, len(quotaSources))
	for _, source := range quotaSources {
		out = append(out, source.Snapshot(stateDir, now, errs[source.Key()]))
	}
	return out
}

// SetUsageEndpointsForTest aims the usage fetchers at local stubs and clears
// the cached Antigravity token, so cross-package HTTP-seam tests never touch
// the real upstreams. It returns a restore func.
func SetUsageEndpointsForTest(opencodeURL, agyBase, tokenURL string) func() {
	oldOC, oldAgy, oldTok := opencodeUsageURL, agyBaseURL, googleTokenURL
	opencodeUsageURL, agyBaseURL, googleTokenURL = opencodeURL, agyBase, tokenURL
	agyTokenMu.Lock()
	agyToken = ""
	agyTokenMu.Unlock()
	return func() {
		opencodeUsageURL, agyBaseURL, googleTokenURL = oldOC, oldAgy, oldTok
		agyTokenMu.Lock()
		agyToken = ""
		agyTokenMu.Unlock()
	}
}
