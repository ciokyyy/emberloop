package web

// Quota API seam: GET /api/quota and POST /api/quota/refresh against local
// HTTP stubs for the opencode and Antigravity endpoints, with a temp
// EL_STATE_DIR so the Daemon shares the header panes' cache files.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	el "example.com/emberloop-web/el"
)

// quotaStubState is the mutable stub behaviour.
type quotaStubState struct {
	mu              sync.Mutex
	opencodePct     int
	opencodeErr     bool
	opencodeCalls   int
	agyErr          bool
	calls           int
	opencodeStarted chan struct{}
	opencodeBlock   <-chan struct{}
	agyStarted      chan struct{}
	agyBlock        <-chan struct{}
	refreshDone     chan struct{}
}

func (s *quotaStubState) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isOpencode := strings.HasSuffix(r.URL.Path, "/zen/go/v1/usage")
		s.mu.Lock()
		s.calls++
		if isOpencode {
			s.opencodeCalls++
		}
		opencodePct, opencodeErr, agyErr := s.opencodePct, s.opencodeErr, s.agyErr
		opencodeStarted, opencodeBlock := s.opencodeStarted, s.opencodeBlock
		agyStarted, agyBlock, refreshDone := s.agyStarted, s.agyBlock, s.refreshDone
		s.mu.Unlock()
		switch {
		case isOpencode:
			if opencodeStarted != nil {
				select {
				case opencodeStarted <- struct{}{}:
				default:
				}
			}
			if opencodeBlock != nil {
				<-opencodeBlock
			}
			if opencodeErr {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"usage":{"rolling":{"percent":` + quotaItoa(opencodePct) + `,"resetsAt":"2030-01-01T00:00:00Z"},"weekly":{"percent":30,"resetsAt":"2030-02-01T00:00:00Z"},"monthly":{"percent":19,"resetsAt":"2030-03-01T00:00:00Z"}}}`))
		case strings.HasSuffix(r.URL.Path, "/token"):
			w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			if agyStarted != nil {
				select {
				case agyStarted <- struct{}{}:
				default:
				}
			}
			if agyBlock != nil {
				<-agyBlock
			}
			if agyErr {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"groups":[
			 {"buckets":[{"bucketId":"gemini-5h","window":"5h","resetTime":"2030-01-01T00:00:00Z","remainingFraction":0.8},
			             {"bucketId":"gemini-weekly","window":"weekly","resetTime":"2030-02-01T00:00:00Z","remainingFraction":0.75}]},
			 {"buckets":[{"bucketId":"3p-5h","window":"5h","resetTime":"2030-01-01T00:00:00Z","remainingFraction":0.9},
			             {"bucketId":"3p-weekly","window":"weekly","resetTime":"2030-02-01T00:00:00Z","remainingFraction":0.4}]}]}`))
			if refreshDone != nil {
				close(refreshDone)
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func quotaItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

type quotaCard struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	State   string `json:"state"`
	Windows []struct {
		Label  string `json:"label"`
		Pct    int    `json:"pct"`
		Resets int64  `json:"resets_at"`
	} `json:"windows"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error"`
}

// quotaEnv starts the stub, points the el fetchers and EL_STATE_DIR at temp
// dirs, and serves the Daemon. auth is the pig auth file body; "" means no
// auth file at all.
func quotaEnv(t *testing.T, stub *quotaStubState, auth string) (base, stateDir string) {
	base, stateDir, _ = quotaEnvWithDaemon(t, stub, auth)
	return base, stateDir
}

func quotaEnvWithDaemon(t *testing.T, stub *quotaStubState, auth string) (base, stateDir string, d *Daemon) {
	t.Helper()
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	restore := el.SetUsageEndpointsForTest(srv.URL+"/zen/go/v1/usage", srv.URL, srv.URL+"/token")
	t.Cleanup(restore)
	stateDir = tempDir(t)
	t.Setenv("EL_STATE_DIR", stateDir)
	if auth != "" {
		path := filepath.Join(tempDir(t), "auth.json")
		if err := os.WriteFile(path, []byte(auth), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("EL_PIG_AUTH", path)
	} else {
		t.Setenv("EL_PIG_AUTH", filepath.Join(stateDir, "missing.json"))
	}
	t.Setenv("EL_AGY_CLIENT_ID", "test-id")
	t.Setenv("EL_AGY_CLIENT_SECRET", "test-secret")
	d = NewDaemon(tempDir(t), NewFakeBackend())
	t.Cleanup(d.quotaRefreshWG.Wait)
	api := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(api.Close)
	return api.URL, stateDir, d
}

func TestQuotaEndpointTiming(t *testing.T) {
	stub := &quotaStubState{opencodePct: 12}
	inner := stub.handler()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(upstream.Close)
	restore := el.SetUsageEndpointsForTest(upstream.URL+"/zen/go/v1/usage", upstream.URL, upstream.URL+"/token")
	t.Cleanup(restore)

	stateDir := tempDir(t)
	t.Setenv("EL_STATE_DIR", stateDir)
	authPath := filepath.Join(tempDir(t), "auth.json")
	if err := os.WriteFile(authPath, []byte(quotaAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_PIG_AUTH", authPath)
	t.Setenv("EL_AGY_CLIENT_ID", "test-id")
	t.Setenv("EL_AGY_CLIENT_SECRET", "test-secret")

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), quotaTimeout)
		defer cancel()
		now := time.Now()
		errs := el.RefreshQuota(ctx, quotaStateDir, "", now, false)
		writeJSON(w, http.StatusOK, quotaResponse{Providers: el.QuotaSnapshot(quotaStateDir, now, errs)})
	}))
	t.Cleanup(legacy.Close)
	before := measureQuotaEndpoint(t, legacy.URL)

	if err := os.Setenv("EL_STATE_DIR", tempDir(t)); err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	api := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(api.Close)
	after := measureQuotaEndpoint(t, api.URL)
	d.quotaRefreshWG.Wait()
	t.Logf("quota GET timing: synchronous baseline %s; cached background read %s", before, after)
	if before < 200*time.Millisecond {
		t.Fatalf("synchronous baseline took %s, want at least 200ms with the delayed upstream", before)
	}
	if after >= 200*time.Millisecond {
		t.Fatalf("cached GET took %s, want under 200ms", after)
	}
}

func measureQuotaEndpoint(t *testing.T, base string) time.Duration {
	t.Helper()
	start := time.Now()
	resp, err := http.Get(base + "/api/quota")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/quota status = %d", resp.StatusCode)
	}
	return time.Since(start)
}

const quotaAuth = `{"opencode-go":{"key":"k1"},"google-antigravity":{"refresh":"r1","projectId":"p1"}}`

func writeClaudeFile(t *testing.T, dir string, fiveHour, sevenDay int) {
	t.Helper()
	raw := `{"five_hour":{"pct":` + quotaItoa(fiveHour) + `,"resets_at":1893456000},"seven_day":{"pct":` + quotaItoa(sevenDay) + `,"resets_at":1893456000}}`
	if err := os.WriteFile(filepath.Join(dir, "claude-usage.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func getQuota(t *testing.T, base string) []quotaCard {
	t.Helper()
	resp, err := http.Get(base + "/api/quota")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/quota status = %d", resp.StatusCode)
	}
	var out struct {
		Providers []quotaCard `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Providers
}

func TestQuotaGetReturnsCachedDataBeforeBlockedRefresh(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{}, 1)
	done := make(chan struct{})
	stub := &quotaStubState{
		opencodePct:     72,
		opencodeStarted: started,
		opencodeBlock:   blocked,
		refreshDone:     done,
	}
	base, stateDir := quotaEnv(t, stub, quotaAuth)
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(blocked) }) }
	t.Cleanup(unblock)
	cache := `{"fetched_at":1,"windows":[{"label":"5h","pct":41,"resets_at":1893456000}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "usage-opencode-go.json"), []byte(cache), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 200 * time.Millisecond}
	start := time.Now()
	resp, err := client.Get(base + "/api/quota")
	if err != nil {
		t.Fatalf("GET /api/quota did not return within 200ms: %v", err)
	}
	var out struct {
		Providers []quotaCard `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	elapsed := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/quota status = %d", resp.StatusCode)
	}
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("cached GET /api/quota took %s, want under 200ms", elapsed)
	}
	cached := quotaByKey(t, out.Providers)["opencode-go"]
	if cached.State != "ok" || len(cached.Windows) != 1 || cached.Windows[0].Pct != 41 || cached.UpdatedAt == 0 {
		t.Fatalf("GET did not return the cached quota card: %+v", cached)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background quota refresh did not reach the upstream stub")
	}
	// The upstream remains blocked until after the cache response has arrived.
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background quota refresh did not finish after unblocking the stub")
	}
	t.Logf("cached GET /api/quota returned in %s while the upstream was blocked", elapsed)
}

func TestQuotaGetReportsBackgroundRefreshError(t *testing.T) {
	stub := &quotaStubState{opencodeErr: true}
	base, stateDir, d := quotaEnvWithDaemon(t, stub, quotaAuth)
	cache := `{"fetched_at":1,"windows":[{"label":"5h","pct":41,"resets_at":1893456000}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "usage-opencode-go.json"), []byte(cache), 0o644); err != nil {
		t.Fatal(err)
	}

	first := quotaByKey(t, getQuota(t, base))["opencode-go"]
	if first.State != "ok" || len(first.Windows) != 1 || first.Windows[0].Pct != 41 {
		t.Fatalf("initial cached card = %+v", first)
	}
	// Wait until the detached failed refresh has recorded its error before
	// checking what a subsequent cache read exposes.
	d.quotaRefreshWG.Wait()
	got := quotaByKey(t, getQuota(t, base))["opencode-go"]
	d.quotaRefreshWG.Wait()
	if got.State != "error" || got.Error == "" || len(got.Windows) != 1 || got.Windows[0].Pct != 41 {
		t.Fatalf("GET after failed background refresh = %+v, want error and stale cached windows", got)
	}
}

type quotaPostResult struct {
	status int
	cards  []quotaCard
	err    error
}

func sendQuotaRefresh(base, key string) (int, []quotaCard, error) {
	var body io.Reader
	if key != "<empty>" {
		raw, err := json.Marshal(map[string]string{"key": key})
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(raw)
	}
	resp, err := http.Post(base+"/api/quota/refresh", "application/json", body)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, nil
	}
	var out struct {
		Providers []quotaCard `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, out.Providers, nil
}

func postQuotaRefresh(t *testing.T, base, key string) (int, []quotaCard) {
	t.Helper()
	status, cards, err := sendQuotaRefresh(base, key)
	if err != nil {
		t.Fatal(err)
	}
	return status, cards
}

func quotaByKey(t *testing.T, cards []quotaCard) map[string]quotaCard {
	t.Helper()
	m := map[string]quotaCard{}
	for _, c := range cards {
		m[c.Key] = c
	}
	return m
}

func TestQuotaGetAllProviders(t *testing.T) {
	stub := &quotaStubState{opencodePct: 12}
	base, stateDir := quotaEnv(t, stub, quotaAuth)
	writeClaudeFile(t, stateDir, 10, 40)
	if code, _ := postQuotaRefresh(t, base, "<empty>"); code != http.StatusOK {
		t.Fatalf("initial refresh status = %d", code)
	}
	cards := getQuota(t, base)
	if len(cards) != 4 {
		t.Fatalf("providers = %d, want 4", len(cards))
	}
	wantKeys := []string{"claude", "opencode-go", "agy-gemini", "agy-3p"}
	for i, k := range wantKeys {
		if cards[i].Key != k {
			t.Errorf("card %d key = %q, want %q", i, cards[i].Key, k)
		}
	}
	m := quotaByKey(t, cards)
	cl := m["claude"]
	if cl.Label != "Claude" || cl.State != "ok" || len(cl.Windows) != 2 || cl.Windows[0].Label != "5h" || cl.Windows[0].Pct != 10 || cl.Windows[1].Pct != 40 {
		t.Errorf("claude card = %+v", cl)
	}
	oc := m["opencode-go"]
	if oc.Label != "OpenCode-Go" || oc.State != "ok" || len(oc.Windows) != 3 || oc.Windows[0].Label != "5h" || oc.Windows[0].Pct != 12 || oc.Windows[1].Label != "wk" || oc.Windows[2].Label != "mo" {
		t.Errorf("opencode card = %+v", oc)
	}
	gem := m["agy-gemini"]
	if gem.State != "ok" || len(gem.Windows) != 2 || gem.Windows[0].Pct != 20 || gem.Windows[1].Pct != 25 {
		t.Errorf("gemini card = %+v", gem)
	}
	tp := m["agy-3p"]
	if tp.State != "ok" || len(tp.Windows) != 2 || tp.Windows[0].Pct != 10 || tp.Windows[1].Pct != 60 {
		t.Errorf("3p card = %+v", tp)
	}
	for _, c := range cards {
		if c.UpdatedAt == 0 {
			t.Errorf("%s updated_at = 0, want the cache mtime", c.Key)
		}
		if c.Error != "" {
			t.Errorf("%s error = %q, want empty", c.Key, c.Error)
		}
	}
}

func TestQuotaNotConfigured(t *testing.T) {
	stub := &quotaStubState{}
	base, _ := quotaEnv(t, stub, "")
	for _, c := range getQuota(t, base) {
		if c.State != "not_configured" {
			t.Errorf("%s state = %q, want not_configured", c.Key, c.State)
		}
		if len(c.Windows) != 0 || c.Error != "" {
			t.Errorf("%s = %+v, want empty card without error", c.Key, c)
		}
	}
}

func TestQuotaFailedFetchKeepsStaleCache(t *testing.T) {
	stub := &quotaStubState{opencodePct: 50}
	base, _ := quotaEnv(t, stub, quotaAuth)
	code, seededCards := postQuotaRefresh(t, base, "<empty>")
	if code != http.StatusOK {
		t.Fatalf("initial refresh status = %d", code)
	}
	seeded := quotaByKey(t, seededCards)["opencode-go"]
	if seeded.State != "ok" || seeded.Windows[0].Pct != 50 {
		t.Fatalf("seeded card = %+v", seeded)
	}
	stub.mu.Lock()
	stub.opencodeErr = true
	stub.mu.Unlock()
	// Forced past the guard so the failure is really attempted.
	_, cards := postQuotaRefresh(t, base, "opencode-go")
	got := quotaByKey(t, cards)["opencode-go"]
	if got.State != "error" || got.Error == "" {
		t.Errorf("failed card = %+v, want error with text", got)
	}
	if len(got.Windows) == 0 || got.Windows[0].Pct != 50 {
		t.Errorf("failed card dropped the stale cache: %+v", got)
	}
	if got.UpdatedAt != seeded.UpdatedAt {
		t.Errorf("failed fetch moved updated_at from %d to %d", seeded.UpdatedAt, got.UpdatedAt)
	}
	// The Antigravity groups are untouched by the opencode outage.
	for _, k := range []string{"agy-gemini", "agy-3p"} {
		if c := quotaByKey(t, cards)[k]; c.State != "ok" {
			t.Errorf("%s state = %q after an opencode outage, want ok", k, c.State)
		}
	}
}

func TestQuotaRefreshForcesPastGuard(t *testing.T) {
	stub := &quotaStubState{opencodePct: 12}
	base, _ := quotaEnv(t, stub, quotaAuth)
	code, cards := postQuotaRefresh(t, base, "opencode-go")
	if code != http.StatusOK {
		t.Fatalf("initial refresh status = %d", code)
	}
	if got := quotaByKey(t, cards)["opencode-go"]; got.Windows[0].Pct != 12 {
		t.Fatalf("seeded card = %+v", got)
	}
	stub.mu.Lock()
	stub.opencodePct = 77
	stub.mu.Unlock()
	// A plain GET stays guarded: the fresh cache holds.
	if got := quotaByKey(t, getQuota(t, base))["opencode-go"]; got.Windows[0].Pct != 12 {
		t.Fatalf("GET refetched inside the guard: %+v", got)
	}
	code, cards = postQuotaRefresh(t, base, "opencode-go")
	if code != http.StatusOK {
		t.Fatalf("refresh status = %d", code)
	}
	if got := quotaByKey(t, cards)["opencode-go"]; got.State != "ok" || got.Windows[0].Pct != 77 {
		t.Errorf("forced card = %+v, want ok at 77", got)
	}
}

func TestQuotaRefreshClaudeRereadsWithoutFetching(t *testing.T) {
	stub := &quotaStubState{opencodePct: 12}
	base, stateDir := quotaEnv(t, stub, quotaAuth)
	writeClaudeFile(t, stateDir, 10, 40)
	stub.mu.Lock()
	before := stub.calls
	stub.mu.Unlock()
	_, cards := postQuotaRefresh(t, base, "claude")
	got := quotaByKey(t, cards)["claude"]
	if got.State != "ok" || len(got.Windows) != 2 || got.Windows[0].Pct != 10 {
		t.Errorf("claude card = %+v", got)
	}
	writeClaudeFile(t, stateDir, 33, 44)
	_, cards = postQuotaRefresh(t, base, "claude")
	if got := quotaByKey(t, cards)["claude"]; got.Windows[0].Pct != 33 || got.Windows[1].Pct != 44 {
		t.Errorf("claude re-read missed the file: %+v", got)
	}
	stub.mu.Lock()
	after := stub.calls
	stub.mu.Unlock()
	if after != before {
		t.Errorf("claude refresh hit the network (%d calls), want file re-read only", after-before)
	}
}

func TestQuotaRefreshUnknownKeyIs400(t *testing.T) {
	stub := &quotaStubState{}
	base, _ := quotaEnv(t, stub, quotaAuth)
	code, _ := postQuotaRefresh(t, base, "nope")
	if code != http.StatusBadRequest {
		t.Fatalf("refresh unknown key status = %d, want 400", code)
	}
}

func TestQuotaRefreshAll(t *testing.T) {
	stub := &quotaStubState{opencodePct: 12}
	base, stateDir := quotaEnv(t, stub, quotaAuth)
	writeClaudeFile(t, stateDir, 10, 40)
	stub.mu.Lock()
	stub.opencodePct = 66
	stub.mu.Unlock()
	code, cards := postQuotaRefresh(t, base, "<empty>")
	if code != http.StatusOK {
		t.Fatalf("refresh all status = %d", code)
	}
	m := quotaByKey(t, cards)
	if m["opencode-go"].Windows[0].Pct != 66 {
		t.Errorf("refresh-all missed opencode: %+v", m["opencode-go"])
	}
	if m["claude"].State != "ok" || m["agy-gemini"].State != "ok" || m["agy-3p"].State != "ok" {
		t.Errorf("refresh-all cards = %+v", m)
	}
}

func waitForQuotaCaches(t *testing.T, stateDir string, keys ...string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		allPresent := true
		for _, key := range keys {
			if _, err := os.Stat(filepath.Join(stateDir, "usage-"+key+".json")); err != nil {
				allPresent = false
				break
			}
		}
		if allPresent {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("quota caches %v were not written while another provider was blocked", keys)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestQuotaRefreshAllDoesNotWaitForBlockedProviderToUpdateOtherCards(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{}, 1)
	agyDone := make(chan struct{})
	stub := &quotaStubState{
		opencodePct:     72,
		opencodeStarted: started,
		opencodeBlock:   blocked,
		refreshDone:     agyDone,
	}
	base, stateDir, _ := quotaEnvWithDaemon(t, stub, quotaAuth)
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(blocked) }) }
	t.Cleanup(unblock)
	staleCache := `{"fetched_at":1,"windows":[{"label":"5h","pct":41,"resets_at":1893456000}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "usage-opencode-go.json"), []byte(staleCache), 0o644); err != nil {
		t.Fatal(err)
	}

	result := make(chan quotaPostResult, 1)
	go func() {
		status, cards, err := sendQuotaRefresh(base, "<empty>")
		result <- quotaPostResult{status: status, cards: cards, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh-all did not reach the blocked OpenCode-Go stub")
	}
	select {
	case <-agyDone:
	case <-time.After(time.Second):
		t.Fatal("Antigravity refresh waited for the blocked OpenCode-Go provider")
	}
	waitForQuotaCaches(t, stateDir, "agy-gemini", "agy-3p")

	// A new read sees the completed providers even though refresh-all is still
	// waiting for OpenCode-Go's upstream response.
	cards := quotaByKey(t, getQuota(t, base))
	for _, key := range []string{"agy-gemini", "agy-3p"} {
		if card := cards[key]; card.State != "ok" || len(card.Windows) != 2 {
			t.Errorf("%s while OpenCode-Go is blocked = %+v, want fresh ok card", key, card)
		}
	}
	if card := cards["opencode-go"]; card.State != "ok" || len(card.Windows) != 1 || card.Windows[0].Pct != 41 {
		t.Errorf("OpenCode-Go cache while upstream is blocked = %+v, want previous cached value", card)
	}
	stub.mu.Lock()
	opencodeCalls := stub.opencodeCalls
	stub.mu.Unlock()
	if opencodeCalls != 1 {
		t.Errorf("OpenCode-Go upstream calls while blocked = %d, want 1", opencodeCalls)
	}

	unblock()
	select {
	case got := <-result:
		if got.err != nil || got.status != http.StatusOK {
			t.Fatalf("refresh-all result = status %d, err %v", got.status, got.err)
		}
		if card := quotaByKey(t, got.cards)["opencode-go"]; card.State != "ok" || card.Windows[0].Pct != 72 {
			t.Errorf("completed OpenCode-Go card = %+v, want fresh value", card)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh-all did not finish after unblocking OpenCode-Go")
	}
}

func TestQuotaRefreshFailureIsIsolatedToOneProvider(t *testing.T) {
	stub := &quotaStubState{opencodeErr: true}
	base, _ := quotaEnv(t, stub, quotaAuth)
	status, cards := postQuotaRefresh(t, base, "<empty>")
	if status != http.StatusOK {
		t.Fatalf("refresh-all status = %d, want 200", status)
	}
	m := quotaByKey(t, cards)
	if card := m["opencode-go"]; card.State != "error" || card.Error == "" {
		t.Errorf("failed OpenCode-Go card = %+v, want its own error", card)
	}
	for _, key := range []string{"agy-gemini", "agy-3p"} {
		if card := m[key]; card.State != "ok" || len(card.Windows) != 2 {
			t.Errorf("%s after OpenCode-Go failure = %+v, want intact ok card", key, card)
		}
	}
}

func TestQuotaBackgroundGetPublishesFailureBeforeAnotherProviderFinishes(t *testing.T) {
	agyBlocked := make(chan struct{})
	agyStarted := make(chan struct{}, 1)
	stub := &quotaStubState{opencodeErr: true, agyStarted: agyStarted, agyBlock: agyBlocked}
	base, stateDir, _ := quotaEnvWithDaemon(t, stub, quotaAuth)
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(agyBlocked) }) }
	t.Cleanup(unblock)
	staleCache := `{"fetched_at":1,"windows":[{"label":"5h","pct":21,"resets_at":1893456000}]}`
	for _, key := range []string{"agy-gemini", "agy-3p"} {
		if err := os.WriteFile(filepath.Join(stateDir, "usage-"+key+".json"), []byte(staleCache), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	initial := quotaByKey(t, getQuota(t, base))
	if initial["agy-gemini"].State != "ok" || initial["agy-3p"].State != "ok" {
		t.Fatalf("initial Antigravity cards = %+v, want cached ok values", initial)
	}
	select {
	case <-agyStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not reach the blocked Antigravity stub")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		cards := quotaByKey(t, getQuota(t, base))
		opencode := cards["opencode-go"]
		if opencode.State == "error" && opencode.Error != "" && opencode.Error != "no cached usage yet" {
			for _, key := range []string{"agy-gemini", "agy-3p"} {
				if card := cards[key]; card.State != "ok" || len(card.Windows) != 1 || card.Windows[0].Pct != 21 {
					t.Errorf("%s while its refresh is blocked = %+v, want retained ok cache", key, card)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("fast OpenCode-Go failure was not published while Antigravity was blocked: %+v", cards)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestQuotaConcurrentRefreshersShareProviderLock(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{}, 1)
	stub := &quotaStubState{opencodePct: 66, opencodeStarted: started, opencodeBlock: blocked}
	base, _, _ := quotaEnvWithDaemon(t, stub, quotaAuth)
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(blocked) }) }
	t.Cleanup(unblock)

	first := make(chan quotaPostResult, 1)
	go func() {
		status, cards, err := sendQuotaRefresh(base, "opencode-go")
		first <- quotaPostResult{status: status, cards: cards, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first refresher did not reach the OpenCode-Go stub")
	}

	second := make(chan quotaPostResult, 1)
	go func() {
		status, cards, err := sendQuotaRefresh(base, "opencode-go")
		second <- quotaPostResult{status: status, cards: cards, err: err}
	}()
	select {
	case got := <-second:
		if got.err != nil || got.status != http.StatusOK {
			t.Fatalf("second refresh = status %d, err %v", got.status, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("second refresh waited for the in-flight provider request")
	}
	stub.mu.Lock()
	opencodeCalls := stub.opencodeCalls
	stub.mu.Unlock()
	if opencodeCalls != 1 {
		t.Errorf("concurrent refreshers made %d upstream requests, want one", opencodeCalls)
	}

	unblock()
	select {
	case got := <-first:
		if got.err != nil || got.status != http.StatusOK {
			t.Fatalf("first refresh = status %d, err %v", got.status, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("first refresher did not finish after unblocking the upstream")
	}
}
