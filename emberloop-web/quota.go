package web

// Quota API for the browser UI's Quota tab: GET /api/quota returns every
// provider's windows, state and update time; POST /api/quota/refresh with
// an optional key forces a fresh fetch past the 90s age guard. Both share
// the header panes' cache files and per-key lock through el, so N viewers
// cause one upstream request. A failure keeps the stale cache with the
// error text on that card; the page itself never fails.

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	el "example.com/emberloop-web/el"
)

// quotaTimeout bounds the whole read: fetches never hold the response past
// it, falling back to the stale cache instead.
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

// serveQuota refreshes (guarded on page load, forced on manual refresh) and
// answers the card list. The forced path skips the age guard on purpose: a
// manual refresh must fetch really fresh data, and the per-key flock still
// serializes concurrent refreshers into one upstream request.
func (d *Daemon) serveQuota(w http.ResponseWriter, r *http.Request, key string, force bool) {
	ctx, cancel := context.WithTimeout(r.Context(), quotaTimeout)
	defer cancel()
	now := time.Now()
	errs := el.RefreshQuota(ctx, quotaStateDir, key, now, force)
	writeJSON(w, http.StatusOK, quotaResponse{Providers: el.QuotaSnapshot(quotaStateDir, now, errs)})
}
