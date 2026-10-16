package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func newTestCatalogSync(t *testing.T, srvURL string) *catalogSync {
	t.Helper()
	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "models-store.json"), []byte(`{"anthropic":{"models":[],"lastModified":0,"checkedAt":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"mistral":{"type":"api_key","key":"x"},"gone":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return &catalogSync{
		baseURL:  srvURL,
		agentDir: dir,
		client:   http.DefaultClient,
		now:      func() time.Time { return time.UnixMilli(1_700_000_000_000) },
		logf:     func(string, ...any) {},
	}
}

func catalogServer(t *testing.T, hits *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits = append(*hits, r.URL.Path+"?"+r.URL.RawQuery)
		switch r.URL.Path {
		case "/api/models/providers/anthropic":
			w.Write([]byte(`[{"id":"claude-haiku-5-5","name":"Haiku"},{"name":"no id"}]`))
		case "/api/models/providers/mistral":
			w.Write([]byte(`{"models":[{"id":"mistral-large-4"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCatalogSyncWritesProviderModels(t *testing.T) {
	var hits []string
	srv := catalogServer(t, &hits)
	s := newTestCatalogSync(t, srv.URL)

	changed, err := s.sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(changed, ",") != "anthropic,mistral" {
		t.Fatalf("changed = %v", changed)
	}
	for _, h := range hits {
		if !strings.Contains(h, "types=chat%2Cimage%2Cclassifier") {
			t.Fatalf("request lacks types query: %s", h)
		}
	}

	store := ai.NewFileModelsStore(filepath.Join(s.agentDir, "models-store.json"))
	e, err := store.Read(context.Background(), "anthropic")
	if err != nil || e == nil || len(e.Models) != 1 {
		t.Fatalf("anthropic entry = %+v, %v", e, err)
	}
	var m map[string]any
	if err := json.Unmarshal(e.Models[0], &m); err != nil {
		t.Fatal(err)
	}
	if m["id"] != "claude-haiku-5-5" || m["provider"] != "anthropic" {
		t.Fatalf("model = %v", m)
	}
	if e.LastModified == nil || *e.LastModified == 0 {
		t.Fatalf("lastModified not set: %+v", e)
	}
	if m2, _ := store.Read(context.Background(), "mistral"); m2 == nil || len(m2.Models) != 1 {
		t.Fatalf("mistral entry = %+v", m2)
	}
}

func TestCatalogSyncSkipsUnchangedAndMissing(t *testing.T) {
	var hits []string
	srv := catalogServer(t, &hits)
	s := newTestCatalogSync(t, srv.URL)
	if _, err := s.sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	changed, err := s.sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("second sync rewrote %v", changed)
	}
	// "gone" got a 404: skipped, never written.
	store := ai.NewFileModelsStore(filepath.Join(s.agentDir, "models-store.json"))
	if e, _ := store.Read(context.Background(), "gone"); e != nil {
		t.Fatalf("404 provider was stored: %+v", e)
	}
}

func TestCatalogSyncKeepsStoreWhenHostDown(t *testing.T) {
	var hits []string
	srv := catalogServer(t, &hits)
	s := newTestCatalogSync(t, srv.URL)
	if _, err := s.sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if _, err := s.sync(context.Background()); err == nil {
		t.Fatal("want error when every provider fails")
	}
	store := ai.NewFileModelsStore(filepath.Join(s.agentDir, "models-store.json"))
	if e, _ := store.Read(context.Background(), "anthropic"); e == nil || len(e.Models) != 1 {
		t.Fatalf("stored models lost: %+v", e)
	}
}

func TestParseCatalogRejectsEmpty(t *testing.T) {
	if _, err := parseCatalog("x", []byte(`[]`)); err == nil {
		t.Fatal("empty catalog must error")
	}
}
