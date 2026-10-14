package el

// Quota snapshot for the browser UI's Quota tab. The Daemon serves it at
// GET /api/quota and POST /api/quota/refresh; the header strip never reads
// it. It reuses ReadClaudeUsage, ReadUsage, RefreshUsage, RefreshUsageForce
// and the UsageKeyFor mapping. Provider fetching stays in usage_providers.go;
// this file dispatches independent refreshes and assembles per-provider state.

import (
	"context"
	"os"
	"sync"
	"time"
)

// Quota states: ok has fresh or cached windows, not_configured has no auth
// (never an error), error carries the fetch failure while keeping stale
// windows when the cache has any.
const (
	QuotaOK            = "ok"
	QuotaNotConfigured = "not_configured"
	QuotaError         = "error"
)

// QuotaProvider is one card on the quota page: Claude, OpenCode-Go, or one
// Antigravity group. Windows reuse UsageWindow's JSON (label, pct,
// resets_at); UpdatedAt is the cache file's mtime (the last push for
// Claude, the last fetch for the rest), 0 when never written.
type QuotaProvider struct {
	Key       string        `json:"key"`
	Label     string        `json:"label"`
	State     string        `json:"state"`
	Windows   []UsageWindow `json:"windows"`
	UpdatedAt int64         `json:"updated_at"`
	Error     string        `json:"error,omitempty"`
}

// quotaCardDef is the one table a new provider edits: its cache key, card
// label, and the key a refresh fetches. Both Antigravity cards share one
// fetch; Claude is push-only, so its fetch key is empty.
type quotaCardDef struct {
	key, label, fetchKey string
}

// quotaCards names the four cards in display order.
var quotaCards = []quotaCardDef{
	{usageKeyClaude, "Claude", ""},
	{usageKeyOpencode, "OpenCode-Go", usageKeyOpencode},
	{usageKeyAgyGem, "Antigravity Gemini", usageKeyAgyGem},
	{usageKeyAgy3P, "Antigravity 3P", usageKeyAgyGem},
}

// quotaCard returns the card definition for a key.
func quotaCard(key string) (quotaCardDef, bool) {
	for _, c := range quotaCards {
		if c.key == key {
			return c, true
		}
	}
	return quotaCardDef{}, false
}

// QuotaKeyValid reports a key the refresh endpoint accepts: "" (all) or one
// of the four card keys.
func QuotaKeyValid(key string) bool {
	if key == "" {
		return true
	}
	_, ok := quotaCard(key)
	return ok
}

// quotaCachePath is the file whose mtime is the card's updated_at.
func quotaCachePath(stateDir, key string) string {
	if key == usageKeyClaude {
		return claudeUsagePath(stateDir)
	}
	return usageCachePath(stateDir, key)
}

// quotaUpdatedAt is the cache file's mtime, 0 when never written.
func quotaUpdatedAt(stateDir, key string) int64 {
	fi, err := os.Stat(quotaCachePath(stateDir, key))
	if err != nil {
		return 0
	}
	return fi.ModTime().Unix()
}

// quotaConfigured reports whether a fetch could ever succeed with local
// auth alone, without touching the network. Missing pig auth, or a missing
// Antigravity OAuth client, reads not_configured, never error. For Claude
// the file is the auth: no push yet means not configured.
func quotaConfigured(stateDir, key string, now time.Time) bool {
	switch key {
	case usageKeyClaude:
		return ReadClaudeUsage(stateDir, now).Has
	case usageKeyOpencode:
		auth, err := readPigAuth()
		return err == nil && auth.OpencodeGo.Key != ""
	default: // both Antigravity cards share one login
		auth, err := readPigAuth()
		if err != nil || auth.Antigravity.Refresh == "" {
			return false
		}
		_, _, cerr := agyClient()
		return cerr == nil
	}
}

// RefreshQuota refetches through the shared cache and lock, then returns
// the per-card refresh error (nil when fresh, fetched, or skipped). An
// empty key refreshes every fetchable provider concurrently; "claude" only
// re-reads its file, so it never fails. force skips the 90s age guard (manual
// refresh); without it the guard applies (page load). A failure keeps the old
// cache: the snapshot still serves stale windows with the error text.
//
// force carries no cooldown on purpose: a manual refresh must fetch really
// fresh data (spec story 14), and the per-key flock still serializes
// concurrent refreshers into one upstream request.
func RefreshQuota(ctx context.Context, stateDir, key string, now time.Time, force bool) map[string]error {
	refresh := RefreshUsage
	if force {
		refresh = RefreshUsageForce
	}
	fetchKeys := []string{}
	if key == "" {
		seen := map[string]bool{}
		for _, c := range quotaCards {
			if c.fetchKey != "" && !seen[c.fetchKey] {
				seen[c.fetchKey] = true
				fetchKeys = append(fetchKeys, c.fetchKey)
			}
		}
	} else if c, ok := quotaCard(key); ok && c.fetchKey != "" {
		fetchKeys = append(fetchKeys, c.fetchKey)
	}

	// Each fetch key owns an independent cache file and flock. Run them in
	// parallel so a slow provider cannot hold up another provider's cache.
	errsByFetchKey := make([]error, len(fetchKeys))
	var wg sync.WaitGroup
	wg.Add(len(fetchKeys))
	for i, fk := range fetchKeys {
		go func(i int, fk string) {
			defer wg.Done()
			errsByFetchKey[i] = refresh(ctx, stateDir, fk, now)
		}(i, fk)
	}
	wg.Wait()

	errs := map[string]error{}
	for i, err := range errsByFetchKey {
		if err == nil {
			continue
		}
		for _, c := range quotaCards {
			if c.fetchKey == fetchKeys[i] {
				errs[c.key] = err
			}
		}
	}
	return errs
}

// QuotaSnapshot reads the cache only (no network) and assembles the four
// cards. errs carries RefreshQuota's per-card errors: a card with windows
// and an error reads error with the stale windows kept; a card without
// windows and without auth reads not_configured.
func QuotaSnapshot(stateDir string, now time.Time, errs map[string]error) []QuotaProvider {
	out := make([]QuotaProvider, 0, len(quotaCards))
	for _, c := range quotaCards {
		set := ReadUsage(stateDir, c.key, now)
		p := QuotaProvider{
			Key:       c.key,
			Label:     c.label,
			UpdatedAt: quotaUpdatedAt(stateDir, c.key),
		}
		if set.Has {
			p.Windows = set.Windows
		} else {
			p.Windows = []UsageWindow{}
		}
		err := errs[c.key]
		switch {
		case set.Has && err == nil:
			p.State = QuotaOK
		case set.Has:
			p.State = QuotaError
			p.Error = err.Error()
		case !quotaConfigured(stateDir, c.key, now):
			p.State = QuotaNotConfigured
		case err != nil:
			p.State = QuotaError
			p.Error = err.Error()
		default:
			p.State = QuotaError
			p.Error = "no cached usage yet"
		}
		out = append(out, p)
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
