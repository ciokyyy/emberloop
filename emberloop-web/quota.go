package web

// Quota API for the browser UI's Quota tab: GET /api/quota returns the shared
// cache only. A subscriber to the live quota topic starts the guarded
// background refresher and receives changed provider lists. POST
// /api/quota/refresh remains synchronous, skips the 90s age guard, and applies
// a 10s per-provider manual cooldown. Both paths share the header panes' cache
// files and per-key lock through el. Failures retain stale values.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	el "example.com/emberloop-web/el"
)

// quotaTimeout bounds provider fetches. Refresh checks run more often than
// the shared 90s cache guard so each source is fetched as soon as it is due.
const (
	quotaTimeout                  = 25 * time.Second
	quotaManualRefreshMinInterval = 10 * time.Second
	quotaRefreshCheckInterval     = 15 * time.Second
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

// refreshQuota publishes each upstream result as it completes. Local-only
// providers have no callback, so their error state is refreshed once after
// the read.
func (d *Daemon) refreshQuota(ctx context.Context, key string, now time.Time, force bool) map[string]error {
	errs := el.RefreshQuotaWithCallback(ctx, quotaStateDir, key, now, force, func(fetchKey string, fetchErrs map[string]error) {
		d.rememberQuotaErrors(fetchKey, fetchErrs)
		d.publishQuotaIfChanged()
	})
	if len(el.QuotaFetchKeys(key)) == 0 {
		d.rememberQuotaErrors(key, errs)
		d.publishQuotaIfChanged()
	}
	return errs
}

func (d *Daemon) quotaProviders() []el.QuotaProvider {
	return el.QuotaSnapshot(quotaStateDir, d.now(), d.quotaErrorSnapshot())
}

// seedQuotaLiveSnapshot establishes the baseline so the first background pass
// only publishes a genuine change from the cache served to the page.
func (d *Daemon) seedQuotaLiveSnapshot() {
	payload, err := json.Marshal(d.quotaProviders())
	if err != nil {
		return
	}
	d.quotaMu.Lock()
	d.quotaPublishedJSON = payload
	d.quotaPublishedReady = true
	d.quotaMu.Unlock()
}

func (d *Daemon) hasQuotaSubscribers() bool {
	d.quotaLiveMu.Lock()
	defer d.quotaLiveMu.Unlock()
	return d.quotaLiveSubscribers > 0
}

func (d *Daemon) publishQuotaIfChanged() {
	if !d.hasQuotaSubscribers() {
		return
	}
	providers := d.quotaProviders()
	payload, err := json.Marshal(providers)
	if err != nil {
		return
	}
	d.quotaMu.Lock()
	if !d.quotaPublishedReady {
		d.quotaPublishedJSON = payload
		d.quotaPublishedReady = true
		d.quotaMu.Unlock()
		return
	}
	changed := !bytes.Equal(d.quotaPublishedJSON, payload)
	if changed {
		d.quotaPublishedJSON = append(d.quotaPublishedJSON[:0], payload...)
	}
	d.quotaMu.Unlock()
	if changed {
		d.live.publishQuota(providers)
	}
}

// subscribeQuotaRefresher keeps one shared refresher alive while any live
// quota consumer is connected. Its immediate pass reads cache first and the
// existing provider locks and 90s age guard decide whether upstream work is due.
func (d *Daemon) subscribeQuotaRefresher() func() {
	d.quotaLiveMu.Lock()
	if d.quotaLiveSubscribers == 0 {
		d.seedQuotaLiveSnapshot()
		ctx, cancel := context.WithCancel(context.Background())
		d.quotaLiveCancel = cancel
		d.quotaLiveWG.Add(1)
		go func() {
			defer d.quotaLiveWG.Done()
			d.runQuotaRefresher(ctx)
		}()
	}
	d.quotaLiveSubscribers++
	d.quotaLiveMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			d.quotaLiveMu.Lock()
			d.quotaLiveSubscribers--
			if d.quotaLiveSubscribers == 0 {
				cancel := d.quotaLiveCancel
				d.quotaLiveCancel = nil
				if cancel != nil {
					cancel()
				}
			}
			d.quotaLiveMu.Unlock()
		})
	}
}

func (d *Daemon) runQuotaRefresher(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	d.refreshQuotaWithTimeout(ctx, "")
	ticker := time.NewTicker(quotaRefreshCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.refreshQuotaWithTimeout(ctx, "")
		}
	}
}

func (d *Daemon) refreshQuotaWithTimeout(parent context.Context, key string) {
	ctx, cancel := context.WithTimeout(parent, quotaTimeout)
	defer cancel()
	d.refreshQuota(ctx, key, d.now(), false)
}

// serveQuota answers GET from the shared cache only. The live-topic subscriber
// owns background refreshes; a manual POST stays synchronous, skips the age
// guard and reserves the 10s interval independently per upstream provider.
func (d *Daemon) serveQuota(w http.ResponseWriter, r *http.Request, key string, force bool) {
	if !force {
		writeJSON(w, http.StatusOK, quotaResponse{Providers: d.quotaProviders()})
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
