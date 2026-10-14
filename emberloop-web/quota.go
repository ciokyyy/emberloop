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

// serveQuota answers GET from the current cache and refreshes it in the
// background. The forced path keeps POST /api/quota/refresh synchronous so
// manual refresh callers receive the updated card list.
func (d *Daemon) serveQuota(w http.ResponseWriter, r *http.Request, key string, force bool) {
	if !force {
		now := time.Now()
		writeJSON(w, http.StatusOK, quotaResponse{Providers: el.QuotaSnapshot(quotaStateDir, now, nil)})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), quotaTimeout)
			defer cancel()
			el.RefreshQuota(ctx, quotaStateDir, key, time.Now(), false)
		}()
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), quotaTimeout)
	defer cancel()
	now := time.Now()
	errs := el.RefreshQuota(ctx, quotaStateDir, key, now, true)
	writeJSON(w, http.StatusOK, quotaResponse{Providers: el.QuotaSnapshot(quotaStateDir, now, errs)})
}
