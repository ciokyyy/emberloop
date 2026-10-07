package emberloop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageKeyFor(t *testing.T) {
	for _, c := range []struct{ harness, provider, model, want string }{
		{"claude", "", "claude-sonnet-5-5", usageKeyClaude},
		{"pig", "opencode-go", "muse-spark", usageKeyOpencode},
		{"pig", "google-antigravity", "gemini-3.1-pro-high", usageKeyAgyGem},
		{"pig", "google-antigravity", "claude-opus-4-6-thinking", usageKeyAgy3P},
		{"pig", "google-antigravity", "gpt-oss-120b-medium", usageKeyAgy3P},
		{"pig", "openrouter", "x", ""},
	} {
		if got := UsageKeyFor(c.harness, c.provider, c.model); got != c.want {
			t.Errorf("UsageKeyFor(%q,%q,%q) = %q, want %q", c.harness, c.provider, c.model, got, c.want)
		}
	}
}

// providerEnv points the fetchers at fake servers and a fake auth file, and
// resets the in-memory Antigravity token.
func providerEnv(t *testing.T, handler http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldOC, oldAgy, oldTok := opencodeUsageURL, agyBaseURL, googleTokenURL
	opencodeUsageURL, agyBaseURL, googleTokenURL = srv.URL+"/zen/go/v1/usage", srv.URL, srv.URL+"/token"
	agyTokenMu.Lock()
	agyToken = ""
	agyTokenMu.Unlock()
	t.Cleanup(func() { opencodeUsageURL, agyBaseURL, googleTokenURL = oldOC, oldAgy, oldTok })
	auth := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(auth, []byte(`{"opencode-go":{"key":"k1"},"google-antigravity":{"refresh":"r1","projectId":"p1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBERLOOP_PIG_AUTH", auth)
	t.Setenv("EMBERLOOP_AGY_CLIENT_ID", "test-id")
	t.Setenv("EMBERLOOP_AGY_CLIENT_SECRET", "test-secret")
	return t.TempDir()
}

func TestRefreshUsageOpencode(t *testing.T) {
	calls := 0
	dir := providerEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer k1" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"usage":{"rolling":{"percent":12,"resetsAt":"2026-10-07T01:00:00.000Z"},"weekly":{"percent":29,"resetsAt":"2026-10-12T00:00:00.000Z"},"monthly":{"percent":19,"resetsAt":"2026-10-16T12:00:00.000Z"}}}`))
	}))
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if err := RefreshUsage(context.Background(), dir, usageKeyOpencode, now); err != nil {
		t.Fatal(err)
	}
	u := ReadUsage(dir, usageKeyOpencode, now)
	if !u.Has || len(u.Windows) != 3 || u.Windows[0].Label != "5h" || u.Windows[0].Pct != 12 || u.Windows[1].Label != "wk" || u.Windows[2].Label != "mo" {
		t.Errorf("usage = %+v", u)
	}
	// A fresh cache means no second request, however many panes ask.
	_ = RefreshUsage(context.Background(), dir, usageKeyOpencode, now.Add(10*time.Second))
	if calls != 1 {
		t.Errorf("requests = %d, want 1 within the refresh interval", calls)
	}
	_ = RefreshUsage(context.Background(), dir, usageKeyOpencode, now.Add(2*time.Minute))
	if calls != 2 {
		t.Errorf("requests = %d, want a refetch after the interval", calls)
	}
	// A rolled window reads 0%.
	if u := ReadUsage(dir, usageKeyOpencode, now.Add(2*time.Hour)); u.Windows[0].Pct != 0 || u.Windows[1].Pct != 29 {
		t.Errorf("after the 5h reset = %+v", u)
	}
}

func TestRefreshUsageFailureKeepsCache(t *testing.T) {
	fail := false
	dir := providerEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"usage":{"rolling":{"percent":50,"resetsAt":"2026-10-07T05:00:00Z"}}}`))
	}))
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	_ = RefreshUsage(context.Background(), dir, usageKeyOpencode, now)
	fail = true
	if err := RefreshUsage(context.Background(), dir, usageKeyOpencode, now.Add(5*time.Minute)); err == nil {
		t.Error("a failed fetch reported success")
	}
	if u := ReadUsage(dir, usageKeyOpencode, now.Add(5*time.Minute)); !u.Has || u.Windows[0].Pct != 50 {
		t.Errorf("failed fetch dropped the cache: %+v", u)
	}
}

func TestRefreshUsageAgyGroups(t *testing.T) {
	dir := providerEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("auth header = %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte(`{"groups":[
			 {"buckets":[{"bucketId":"gemini-weekly","window":"weekly","resetTime":"2026-10-12T00:00:00Z","remainingFraction":0.75},
			             {"bucketId":"gemini-5h","window":"5h","resetTime":"2026-10-07T03:00:00Z","remainingFraction":1}]},
			 {"buckets":[{"bucketId":"3p-weekly","window":"weekly","resetTime":"2026-10-12T00:00:00Z","remainingFraction":0.4},
			             {"bucketId":"3p-5h","window":"5h","resetTime":"2026-10-07T03:00:00Z","remainingFraction":0.9}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if err := RefreshUsage(context.Background(), dir, usageKeyAgyGem, now); err != nil {
		t.Fatal(err)
	}
	g := ReadUsage(dir, usageKeyAgyGem, now)
	if len(g.Windows) != 2 || g.Windows[0].Label != "5h" || g.Windows[0].Pct != 0 || g.Windows[1].Label != "wk" || g.Windows[1].Pct != 25 {
		t.Errorf("gemini usage = %+v", g)
	}
	p := ReadUsage(dir, usageKeyAgy3P, now)
	if len(p.Windows) != 2 || p.Windows[0].Pct != 10 || p.Windows[1].Pct != 60 {
		t.Errorf("claude and gpt-oss usage = %+v", p)
	}
}

func TestRefreshUsageSkipsKeyless(t *testing.T) {
	dir := providerEnv(t, http.NotFoundHandler())
	for _, key := range []string{"", usageKeyClaude} {
		if err := RefreshUsage(context.Background(), dir, key, time.Now()); err != nil {
			t.Errorf("key %q: %v", key, err)
		}
	}
}
