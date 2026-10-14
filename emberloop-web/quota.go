package web

// Quota API for the browser UI's Quota tab: GET /api/quota returns the cache
// immediately and starts guarded provider refreshes concurrently in the
// background, publishing each result as it completes. POST /api/quota/refresh
// with an optional key forces a fresh synchronous fetch past the 90s age
// guard, subject to a 10s per-provider manual cooldown. Both share the header
// panes' cache files and per-key lock through el. Failures retain stale values.

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	el "example.com/emberloop-web/el"
)

// quotaTimeout bounds both background cache refreshes and manual refreshes.
const (
	quotaTimeout                  = 25 * time.Second
	quotaManualRefreshMinInterval = 10 * time.Second
)

// quotaStateDir is the shared quota cache: the empty el default, the same
// dir the header panes read, never the Daemon's own session dir.
const quotaStateDir = ""

// quotaResponse is the GET and POST shape: one card per provider.
type quotaResponse struct {
	Providers []el.QuotaProvider `json:"providers"`
}

func (d *Daemon) handleQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.serveQuota(w, r, "", false)
}

func (d *Daemon) handleQuotaRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body.Key == "" {
		body.Key = r.URL.Query().Get("key")
	}
	if !el.QuotaKeyValid(body.Key) {
		http.Error(w, "unknown quota key", http.StatusBadRequest)
		return
	}
	d.serveQuota(w, r, body.Key, true)
}

// quotaErrorKeys returns the cards a refresh may update. Antigravity's two
// cards share one upstream fetch and therefore share its error state.
func quotaErrorKeys(key string) []string {
	switch key {
	case "":
		return []string{"claude", "openai-codex", "opencode-go", "agy-gemini", "agy-3p"}
	case "agy-gemini", "agy-3p":
		return []string{"agy-gemini", "agy-3p"}
	default:
		return []string{key}
	}
}

func (d *Daemon) rememberQuotaErrors(key string, errs map[string]error) {
	d.quotaMu.Lock()
	defer d.quotaMu.Unlock()
	if d.quotaErrors == nil {
		d.quotaErrors = make(map[string]error)
	}
	for _, cardKey := range quotaErrorKeys(key) {
		delete(d.quotaErrors, cardKey)
	}
	for cardKey, err := range errs {
		if err != nil {
			d.quotaErrors[cardKey] = err
		}
	}
}

func (d *Daemon) quotaErrorSnapshot() map[string]error {
	d.quotaMu.RLock()
	defer d.quotaMu.RUnlock()
	if len(d.quotaErrors) == 0 {
		return nil
	}
	errs := make(map[string]error, len(d.quotaErrors))
	for key, err := range d.quotaErrors {
		errs[key] = err
	}
	return errs
}

// admitManualQuotaRefreshes reserves one provider's manual refresh interval
// before starting the fetch, so even repeated failures cannot hammer upstream.
func (d *Daemon) admitManualQuotaRefreshes(fetchKeys []string, now time.Time) []string {
	d.quotaMu.Lock()
	defer d.quotaMu.Unlock()
	if d.quotaManualRefreshAt == nil {
		d.quotaManualRefreshAt = make(map[string]time.Time)
	}
	var admitted []string
	for _, key := range fetchKeys {
		if last, ok := d.quotaManualRefreshAt[key]; ok && now.Before(last.Add(quotaManualRefreshMinInterval)) {
			continue
		}
		d.quotaManualRefreshAt[key] = now
		admitted = append(admitted, key)
	}
	return admitted
}

// refreshQuotaFetchKeys runs an admitted refresh-all subset concurrently;
// one recently refreshed provider must not suppress the others.
func (d *Daemon) refreshQuotaFetchKeys(ctx context.Context, fetchKeys []string, now time.Time) {
	var wg sync.WaitGroup
	wg.Add(len(fetchKeys))
	for _, key := range fetchKeys {
		go func(key string) {
			defer wg.Done()
			d.refreshQuota(ctx, key, now, true)
		}(key)
	}
	wg.Wait()
}

// refreshQuota records each provider's outcome as it completes, before
// returning the aggregate errors for the synchronous refresh response.
func (d *Daemon) refreshQuota(ctx context.Context, key string, now time.Time, force bool) map[string]error {
	errs := el.RefreshQuotaWithCallback(ctx, quotaStateDir, key, now, force, func(fetchKey string, fetchErrs map[string]error) {
		d.rememberQuotaErrors(fetchKey, fetchErrs)
	})
	d.rememberQuotaErrors(key, errs)
	return errs
}

// serveQuota answers GET from the current cache and refreshes it in the
// background. Manual POST stays synchronous, skips the age guard, and reserves
// the 10s interval independently for each upstream provider.
func (d *Daemon) serveQuota(w http.ResponseWriter, r *http.Request, key string, force bool) {
	if !force {
		now := d.now()
		providers := el.QuotaSnapshot(quotaStateDir, now, d.quotaErrorSnapshot())
		d.quotaRefreshWG.Add(1)
		writeJSON(w, http.StatusOK, quotaResponse{Providers: providers})
		go func() {
			defer d.quotaRefreshWG.Done()
			ctx, cancel := context.WithTimeout(context.Background(), quotaTimeout)
			defer cancel()
			d.refreshQuota(ctx, key, d.now(), false)
		}()
		return
	}

	now := d.now()
	fetchKeys := el.QuotaFetchKeys(key)
	ctx, cancel := context.WithTimeout(r.Context(), quotaTimeout)
	defer cancel()
	if key == "" {
		admitted := d.admitManualQuotaRefreshes(fetchKeys, now)
		d.refreshQuotaFetchKeys(ctx, admitted, now)
	} else if len(fetchKeys) == 0 {
		// Local-only providers such as Claude have no upstream cooldown.
		d.refreshQuota(ctx, key, now, true)
	} else if admitted := d.admitManualQuotaRefreshes(fetchKeys, now); len(admitted) > 0 {
		d.refreshQuota(ctx, key, now, true)
	}
	writeJSON(w, http.StatusOK, quotaResponse{Providers: el.QuotaSnapshot(quotaStateDir, d.now(), d.quotaErrorSnapshot())})
}
