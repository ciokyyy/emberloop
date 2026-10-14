package el

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// quotaStub serves the opencode usage endpoint with a mutable percent and
// the Antigravity token plus quota-summary endpoints.
type quotaStub struct {
	opencodePct int
	agyFail     bool
}

func (s *quotaStub) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/zen/go/v1/usage"):
			w.Write([]byte(`{"usage":{"rolling":{"percent":` + strconv.Itoa(s.opencodePct) + `,"resetsAt":"2026-10-07T05:00:00Z"},"weekly":{"percent":30,"resetsAt":"2026-10-12T00:00:00Z"}}}`))
		case strings.HasSuffix(r.URL.Path, "/token"):
			w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			if s.agyFail {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"groups":[
			 {"buckets":[{"bucketId":"gemini-5h","window":"5h","resetTime":"2026-10-07T03:00:00Z","remainingFraction":0.8},
			             {"bucketId":"gemini-weekly","window":"weekly","resetTime":"2026-10-12T00:00:00Z","remainingFraction":0.75}]},
			 {"buckets":[{"bucketId":"3p-5h","window":"5h","resetTime":"2026-10-07T03:00:00Z","remainingFraction":0.9},
			             {"bucketId":"3p-weekly","window":"weekly","resetTime":"2026-10-12T00:00:00Z","remainingFraction":0.4}]}]}`))
		default:
			http.NotFound(w, r)
		}
	})
}

// quotaEnv aims the fetchers at the stub and writes a pig auth file with
// both logins, returning the state dir.
func quotaEnv(t *testing.T, stub *quotaStub) string {
	t.Helper()
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	restore := SetUsageEndpointsForTest(srv.URL+"/zen/go/v1/usage", srv.URL, srv.URL+"/token")
	t.Cleanup(restore)
	auth := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(auth, []byte(`{"opencode-go":{"type":"api_key","key":"k1"},"google-antigravity":{"type":"oauth","access":"agy-access","refresh":"r1","expires":1893456000000,"projectId":"p1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_PIG_AUTH", auth)
	t.Setenv("EL_AGY_CLIENT_ID", "test-id")
	t.Setenv("EL_AGY_CLIENT_SECRET", "test-secret")
	return t.TempDir()
}

func quotaSourceForTest(t *testing.T, key string) quotaSource {
	t.Helper()
	for _, source := range quotaSources {
		if source.Key() == key {
			return source
		}
	}
	t.Fatalf("quota source %q is not registered", key)
	return nil
}

func TestQuotaSnapshotNotConfigured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EL_PIG_AUTH", filepath.Join(dir, "missing.json"))
	t.Setenv("EL_AGY_CLIENT_ID", "")
	t.Setenv("EL_AGY_CLIENT_SECRET", "")
	t.Setenv("HOME", dir)
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	got := QuotaSnapshot(dir, now, nil)
	if len(got) != 5 {
		t.Fatalf("providers = %d, want 5", len(got))
	}
	for _, p := range got {
		if p.State != QuotaNotConfigured {
			t.Errorf("%s state = %q, want not_configured", p.Key, p.State)
		}
		if len(p.Windows) != 0 || p.UpdatedAt != 0 || p.Error != "" || p.Hint == "" {
			t.Errorf("%s = %+v, want empty card with a missing-login hint", p.Key, p)
		}
		if (p.Key == usageKeyAgyGem || p.Key == usageKeyAgy3P) && !strings.Contains(p.Hint, "~/.el/agy-oauth.json") {
			t.Errorf("%s hint = %q, want the missing Antigravity client path", p.Key, p.Hint)
		}
		if p.Key == usageKeyChatGPT && !strings.Contains(p.Hint, "openai-codex") {
			t.Errorf("ChatGPT Plus hint = %q, want missing pig login named", p.Hint)
		}

	}
}

func TestQuotaNotConfiguredWithCachedValuesIsNotError(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	t.Setenv("EL_PIG_AUTH", filepath.Join(dir, "missing-auth.json"))
	if err := writeUsageCache(dir, usageKeyOpencode, []UsageWindow{{Label: "5h", Pct: 52}}, now); err != nil {
		t.Fatal(err)
	}
	source := quotaSourceForTest(t, usageKeyOpencode)
	got := source.Snapshot(dir, now, context.DeadlineExceeded)
	if got.State != QuotaNotConfigured || got.Error != "" || got.Hint == "" {
		t.Fatalf("provider without login = %+v, want not_configured with hint and no error", got)
	}
	if len(got.Windows) != 1 || got.Windows[0].Pct != 52 {
		t.Errorf("not-configured provider dropped its cached window: %+v", got.Windows)
	}
}

func TestQuotaSourceOwnsProviderConfigurationAndCachePath(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	t.Setenv("EL_PIG_AUTH", filepath.Join(dir, "missing-auth.json"))
	t.Setenv("EL_AGY_CLIENT_ID", "")
	t.Setenv("EL_AGY_CLIENT_SECRET", "")
	if err := writeUsageCache(dir, "chatgpt-plus", []UsageWindow{{Label: "5h", Pct: 37}}, now); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(dir, "chatgpt-cache.json")
	if err := os.WriteFile(cachePath, []byte("provider cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	source := usageQuotaSource{
		key:   "chatgpt-plus",
		label: "ChatGPT Plus",
		configuredFn: func(string, time.Time) bool {
			return false
		},
		hintFn:      func(string) string { return "Missing ChatGPT login." },
		cachePathFn: func(string) string { return cachePath },
	}
	got := source.Snapshot(dir, now, nil)
	if got.State != QuotaNotConfigured || got.Error != "" || got.Hint != "Missing ChatGPT login." {
		t.Fatalf("provider configuration = %+v, want provider-owned not_configured state and hint", got)
	}
	if got.UpdatedAt != cacheInfo.ModTime().Unix() || len(got.Windows) != 1 || got.Windows[0].Pct != 37 {
		t.Errorf("provider cache = %+v, want custom cache mtime and retained percent window", got)
	}
}

func TestQuotaExpiredRetainsLastValuesAndHint(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if err := writeUsageCache(dir, usageKeyOpencode, []UsageWindow{{Label: "5h", Pct: 47, ResetsAt: now.Add(time.Hour).Unix()}}, now); err != nil {
		t.Fatal(err)
	}
	source := quotaSourceForTest(t, usageKeyOpencode)

	got := source.Snapshot(dir, now, &quotaFailure{state: QuotaExpired, hint: "Login expired. Sign in again."})
	if got.State != QuotaExpired || got.Hint == "" || got.Error != "" {
		t.Fatalf("expired provider = %+v", got)
	}
	if len(got.Windows) != 1 || got.Windows[0].Pct != 47 || got.Windows[0].Label != "5-hour" {
		t.Errorf("expired provider lost its cached window: %+v", got.Windows)
	}
}

func TestQuotaWindowsClampPercentAndUseWordLabels(t *testing.T) {
	got := quotaWindows([]UsageWindow{
		{Label: "5h", Pct: -4},
		{Label: "wk", Pct: 57},
		{Label: "mo", Pct: 103},
	})
	wantLabels := []string{"5-hour", "Weekly", "Monthly"}
	wantPcts := []int{0, 57, 100}
	for i, window := range got {
		if window.Label != wantLabels[i] || window.Pct != wantPcts[i] {
			t.Errorf("window %d = %+v, want label %q and pct %d", i, window, wantLabels[i], wantPcts[i])
		}
	}
}

func TestRefreshQuotaForceSkipsGuard(t *testing.T) {
	stub := &quotaStub{opencodePct: 12}
	dir := quotaEnv(t, stub)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if errs := RefreshQuota(ctx, dir, usageKeyOpencode, now, false); len(errs) != 0 {
		t.Fatal(errs)
	}
	stub.opencodePct = 77
	// Guarded: a fresh cache means no refetch.
	if errs := RefreshQuota(ctx, dir, usageKeyOpencode, now.Add(10*time.Second), false); len(errs) != 0 {
		t.Fatal(errs)
	}
	if u := ReadUsage(dir, usageKeyOpencode, now); u.Windows[0].Pct != 12 {
		t.Fatalf("guarded refresh refetched: %+v", u)
	}
	// Forced: past the guard, the new value lands.
	if errs := RefreshQuota(ctx, dir, usageKeyOpencode, now.Add(10*time.Second), true); len(errs) != 0 {
		t.Fatal(errs)
	}
	if u := ReadUsage(dir, usageKeyOpencode, now); u.Windows[0].Pct != 77 {
		t.Fatalf("forced refresh kept the old cache: %+v", u)
	}
	// The header path is unchanged: still guarded.
	stub.opencodePct = 99
	if err := RefreshUsage(ctx, dir, usageKeyOpencode, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	if u := ReadUsage(dir, usageKeyOpencode, now); u.Windows[0].Pct != 77 {
		t.Fatalf("RefreshUsage lost its guard: %+v", u)
	}
}

func TestQuotaStaleFallbackKeepsCache(t *testing.T) {
	stub := &quotaStub{opencodePct: 50}
	dir := quotaEnv(t, stub)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if errs := RefreshQuota(ctx, dir, usageKeyOpencode, now, true); len(errs) != 0 {
		t.Fatal(errs)
	}
	before := QuotaSnapshot(dir, now, nil)
	var opencodeUpdatedAt int64
	for _, provider := range before {
		if provider.Key == usageKeyOpencode {
			opencodeUpdatedAt = provider.UpdatedAt
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	restore := SetUsageEndpointsForTest(srv.URL+"/x", srv.URL, srv.URL+"/token")
	t.Cleanup(restore)
	_ = stub
	errs := RefreshQuota(ctx, dir, usageKeyOpencode, now.Add(5*time.Minute), true)
	if errs[usageKeyOpencode] == nil {
		t.Fatal("forced refresh over a failing upstream reported success")
	}
	got := QuotaSnapshot(dir, now.Add(5*time.Minute), errs)
	var card QuotaProvider
	for _, p := range got {
		if p.Key == usageKeyOpencode {
			card = p
		}
	}
	if card.State != QuotaStale || card.Error == "" {
		t.Errorf("failed card = %+v, want stale with error text", card)
	}
	if len(card.Windows) == 0 || card.Windows[0].Pct != 50 {
		t.Errorf("failed card dropped the stale cache: %+v", card)
	}
	if card.UpdatedAt != opencodeUpdatedAt {
		t.Errorf("failed fetch moved updated_at from %d to %d", opencodeUpdatedAt, card.UpdatedAt)
	}
}

func TestQuotaClaudeRereadOnly(t *testing.T) {
	dir := t.TempDir()
	raw := `{"five_hour":{"label":"5h","pct":10,"resets_at":1792000000},"seven_day":{"label":"wk","pct":40,"resets_at":1792000000}}`
	if err := os.WriteFile(filepath.Join(dir, "claude-usage.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	// No endpoints stubbed: a fetch would fail, a re-read succeeds.
	if errs := RefreshQuota(ctx, dir, usageKeyClaude, now, true); len(errs) != 0 {
		t.Fatal(errs)
	}
	got := QuotaSnapshot(dir, now, nil)
	if got[0].Key != usageKeyClaude || got[0].State != QuotaOK {
		t.Fatalf("claude card = %+v, want ok", got[0])
	}
	if len(got[0].Windows) != 2 || got[0].Windows[0].Label != "5-hour" || got[0].Windows[0].Pct != 10 || got[0].Windows[1].Label != "Weekly" {
		t.Errorf("claude windows = %+v", got[0].Windows)
	}
	if got[0].UpdatedAt == 0 {
		t.Error("claude updated_at = 0, want the file mtime")
	}
}

func TestQuotaAgyGroups(t *testing.T) {
	stub := &quotaStub{}
	dir := quotaEnv(t, stub)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if errs := RefreshQuota(ctx, dir, "", now, true); len(errs) != 0 {
		t.Fatal(errs)
	}
	got := QuotaSnapshot(dir, now, nil)
	byKey := map[string]QuotaProvider{}
	for _, p := range got {
		byKey[p.Key] = p
	}
	gem, ok := byKey[usageKeyAgyGem]
	if !ok || gem.State != QuotaOK || len(gem.Windows) != 2 || gem.Windows[0].Label != "5-hour" || gem.Windows[0].Pct != 20 || gem.Windows[1].Label != "Weekly" || gem.Windows[1].Pct != 25 {
		t.Errorf("gemini card = %+v", gem)
	}
	tp, ok := byKey[usageKeyAgy3P]
	if !ok || tp.State != QuotaOK || len(tp.Windows) != 2 || tp.Windows[0].Label != "5-hour" || tp.Windows[0].Pct != 10 || tp.Windows[1].Label != "Weekly" || tp.Windows[1].Pct != 60 {
		t.Errorf("3p card = %+v", tp)
	}
	if !QuotaKeyValid("") || !QuotaKeyValid(usageKeyClaude) || !QuotaKeyValid(usageKeyChatGPT) || QuotaKeyValid("nope") {
		t.Error("QuotaKeyValid misclassifies keys")
	}
}
