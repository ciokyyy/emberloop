package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// Pig model catalog sync. Pig's bundled catalog lags pi, and its own remote
// overlay points at pi-in-go.dev, which serves no catalog (404). This job
// fetches pi.dev's per-provider catalog and writes it into pig's
// models-store.json through pig's own locked store, so pig loads the new
// models as its remote overlay. A 404 refresh inside pig keeps stored models,
// so the seeded rows survive.

const (
	catalogSyncURL      = "https://pi.dev"
	catalogSyncEvery    = 4 * time.Hour
	catalogSyncFirst    = 45 * time.Second
	catalogSyncTypes    = "chat,image,classifier"
	catalogSyncMaxBytes = 16 << 20
)

// catalogSync is the sync's seam: host, clock, HTTP client and log behind
// fields so tests run it against a local server and a temp agent dir.
type catalogSync struct {
	baseURL  string
	agentDir string
	client   *http.Client
	now      func() time.Time
	logf     func(format string, args ...any)
}

func (d *Daemon) newCatalogSync() *catalogSync {
	return &catalogSync{
		baseURL:  catalogSyncURL,
		agentDir: d.hostPigDir,
		client:   &http.Client{Timeout: 20 * time.Second},
		now:      time.Now,
		logf:     log.Printf,
	}
}

// StartPigCatalogSync syncs soon after boot, then every 4 hours, until stop
// closes. EL_PIG_CATALOG_SYNC=0 disables it.
func (d *Daemon) StartPigCatalogSync(stop <-chan struct{}) {
	if os.Getenv("EL_PIG_CATALOG_SYNC") == "0" || d.hostPigDir == "" {
		return
	}
	s := d.newCatalogSync()
	t := time.NewTicker(catalogSyncEvery)
	defer t.Stop()
	first := time.After(catalogSyncFirst)
	for {
		select {
		case <-stop:
			return
		case <-first:
			s.run()
		case <-t.C:
			s.run()
		}
	}
}

func (s *catalogSync) run() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	changed, err := s.sync(ctx)
	if err != nil {
		s.logf("pig catalog sync: %v", err)
		return
	}
	if len(changed) > 0 {
		s.logf("pig catalog sync: updated %s", strings.Join(changed, ", "))
	}
}

// providers are the providers pig can refresh: the ones already in its
// models store plus the ones with credentials in auth.json.
func (s *catalogSync) providers() []string {
	set := map[string]bool{}
	for _, name := range []string{"models-store.json", "auth.json"} {
		b, err := os.ReadFile(filepath.Join(s.agentDir, name))
		if err != nil {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		for k := range m {
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sync fetches and stores each provider's catalog. It returns the providers
// whose stored models changed. A provider that fails is skipped; the others
// still sync. The error is non-nil only when every provider failed.
func (s *catalogSync) sync(ctx context.Context) ([]string, error) {
	provs := s.providers()
	if len(provs) == 0 {
		return nil, nil
	}
	store := ai.NewFileModelsStore(filepath.Join(s.agentDir, "models-store.json"))
	var changed []string
	failed := 0
	var lastErr error
	for _, p := range provs {
		models, err := s.fetch(ctx, p)
		if err != nil {
			failed++
			lastErr = err
			continue
		}
		if models == nil {
			continue
		}
		cur, err := store.Read(ctx, p)
		if err != nil {
			failed++
			lastErr = err
			continue
		}
		if cur != nil && sameModels(cur.Models, models) {
			continue
		}
		now := float64(s.now().UnixMilli())
		entry := ai.ModelsStoreEntry{Models: models, CheckedAt: &now, LastModified: &now}
		if err := store.Write(ctx, p, entry); err != nil {
			failed++
			lastErr = err
			continue
		}
		changed = append(changed, p)
	}
	if failed == len(provs) {
		return nil, fmt.Errorf("every provider failed, last: %w", lastErr)
	}
	return changed, nil
}

// fetch returns the provider's models as pig stores them: the catalog's
// entries that carry an id, each tagged with the provider. A 404 or 501 means
// the host serves no catalog for the provider and returns nil, nil.
func (s *catalogSync) fetch(ctx context.Context, provider string) ([]json.RawMessage, error) {
	u := strings.TrimRight(s.baseURL, "/") + "/api/models/providers/" + url.PathEscape(provider) + "?types=" + url.QueryEscape(catalogSyncTypes)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNotImplemented {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s: HTTP %d", provider, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, catalogSyncMaxBytes))
	if err != nil {
		return nil, err
	}
	return parseCatalog(provider, body)
}

func parseCatalog(provider string, body []byte) ([]json.RawMessage, error) {
	var entries []json.RawMessage
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("[")) {
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, fmt.Errorf("%s: %w", provider, err)
		}
	} else {
		var obj struct {
			Models []json.RawMessage `json:"models"`
		}
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return nil, fmt.Errorf("%s: %w", provider, err)
		}
		entries = obj.Models
	}
	pj, _ := json.Marshal(provider)
	out := make([]json.RawMessage, 0, len(entries))
	for _, e := range entries {
		var f map[string]json.RawMessage
		if json.Unmarshal(e, &f) != nil || f == nil {
			continue
		}
		if _, ok := f["id"]; !ok {
			continue
		}
		f["provider"] = pj
		enc, err := json.Marshal(f)
		if err != nil {
			return nil, err
		}
		out = append(out, enc)
	}
	// A catalog with no usable model would blank pig's overlay.
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: catalog has no models", provider)
	}
	return out, nil
}

func sameModels(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		var x, y bytes.Buffer
		if json.Compact(&x, a[i]) != nil || json.Compact(&y, b[i]) != nil || !bytes.Equal(x.Bytes(), y.Bytes()) {
			return false
		}
	}
	return true
}
