package web

// Quota API for the browser UI's Quota tab: GET /api/quota returns the cache
// immediately and starts a guarded refresh in the background. POST
// /api/quota/refresh with an optional key forces a fresh synchronous fetch
// past the 90s age guard. Both share the header panes' cache files and per-key
// lock through el, so N viewers cause one upstream request. Fetch failures
// keep the stale cache with error text; the page itself never fails.

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	el "example.com/emberloop-web/el"
)

// quotaTimeout bounds both background cache refreshes and manual refreshes.
const quotaTimeout = 25 * time.Second

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
		return []string{"claude", "opencode-go", "agy-gemini", "agy-3p"}
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

// serveQuota answers GET from the current cache and refreshes it in the
// background. The forced path keeps POST /api/quota/refresh synchronous so
// manual refresh callers receive the updated card list.
func (d *Daemon) serveQuota(w http.ResponseWriter, r *http.Request, key string, force bool) {
	if !force {
		now := time.Now()
		providers := el.QuotaSnapshot(quotaStateDir, now, d.quotaErrorSnapshot())
		d.quotaRefreshWG.Add(1)
		writeJSON(w, http.StatusOK, quotaResponse{Providers: providers})
		go func() {
			defer d.quotaRefreshWG.Done()
			ctx, cancel := context.WithTimeout(context.Background(), quotaTimeout)
			defer cancel()
			errs := el.RefreshQuota(ctx, quotaStateDir, key, time.Now(), false)
			d.rememberQuotaErrors(key, errs)
		}()
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), quotaTimeout)
	defer cancel()
	errs := el.RefreshQuota(ctx, quotaStateDir, key, time.Now(), true)
	d.rememberQuotaErrors(key, errs)
	writeJSON(w, http.StatusOK, quotaResponse{Providers: el.QuotaSnapshot(quotaStateDir, time.Now(), d.quotaErrorSnapshot())})
}
