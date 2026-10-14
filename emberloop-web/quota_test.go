package web

// Quota API seam: GET /api/quota and POST /api/quota/refresh against local
// HTTP stubs for Claude, ChatGPT Plus, OpenCode-Go, OpenRouter and Antigravity,
// with temp credential and state dirs so tests never read host logins
// or touch real upstreams.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	el "example.com/emberloop-web/el"
)

// quotaStubState is the mutable stub behaviour.
type quotaStubState struct {
	mu                   sync.Mutex
	opencodePct          int
	opencodeErr          bool
	opencodeCalls        int
	agyErr               bool
	agyCalls             int
	openrouterCredits    int
	openrouterKey        int
	openrouterAuth       []string
	chatGPTStatus        int
	chatGPTCalls         int
	chatGPTMethod        string
	chatGPTAuthorization string
	chatGPTAccountID     string
	tokenCalls           int
	calls                int
	claudeCalls          int
	claudeStatus         int
	claudeRetryAfter     string
	claudeBody           string
	claudeMethod         string
	claudeAuthorization  string
	claudeUserAgent      string
	claudeBeta           string
	opencodeStarted      chan struct{}
	opencodeBlock        <-chan struct{}
	agyStarted           chan struct{}
	agyBlock             <-chan struct{}
	refreshDone          chan struct{}
}

func (s *quotaStubState) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isChatGPT := strings.HasSuffix(r.URL.Path, "/backend-api/wham/usage")
		isOpencode := strings.HasSuffix(r.URL.Path, "/zen/go/v1/usage")
		isAgy := strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary")
		isClaude := strings.HasSuffix(r.URL.Path, "/api/oauth/usage")
		isOpenRouterCredits := strings.HasSuffix(r.URL.Path, "/api/v1/credits")
		isOpenRouterKey := strings.HasSuffix(r.URL.Path, "/api/v1/key")
		s.mu.Lock()
		s.calls++
		if isChatGPT {
			s.chatGPTCalls++
			s.chatGPTMethod = r.Method
			s.chatGPTAuthorization = r.Header.Get("Authorization")
			s.chatGPTAccountID = r.Header.Get("ChatGPT-Account-Id")
		}
		if isOpencode {
			s.opencodeCalls++
		}
		if isAgy {
			s.agyCalls++
		}
		if strings.HasSuffix(r.URL.Path, "/token") {
			s.tokenCalls++
		}
		if isClaude {
			s.claudeCalls++
			s.claudeMethod = r.Method
			s.claudeAuthorization = r.Header.Get("Authorization")
			s.claudeUserAgent = r.Header.Get("User-Agent")
			s.claudeBeta = r.Header.Get("anthropic-beta")
		}
		if isOpenRouterCredits {
			s.openrouterCredits++
			s.openrouterAuth = append(s.openrouterAuth, r.Header.Get("Authorization"))
		}
		if isOpenRouterKey {
			s.openrouterKey++
			s.openrouterAuth = append(s.openrouterAuth, r.Header.Get("Authorization"))
		}
		opencodePct, opencodeErr, agyErr := s.opencodePct, s.opencodeErr, s.agyErr
		chatGPTStatus := s.chatGPTStatus
		claudeStatus, claudeRetryAfter, claudeBody := s.claudeStatus, s.claudeRetryAfter, s.claudeBody
		opencodeStarted, opencodeBlock := s.opencodeStarted, s.opencodeBlock
		agyStarted, agyBlock, refreshDone := s.agyStarted, s.agyBlock, s.refreshDone
		s.mu.Unlock()
		switch {
		case isOpenRouterCredits:
			w.Write([]byte(`{"data":{"total_credits":63,"total_usage":59.16}}`))
		case isOpenRouterKey:
			w.Write([]byte(`{"data":{"usage_daily":0.91,"usage_weekly":2.37,"usage_monthly":5.70}}`))
		case isChatGPT:
			if chatGPTStatus != 0 && chatGPTStatus != http.StatusOK {
				http.Error(w, "ChatGPT request failed", chatGPTStatus)
				return
			}
			w.Write([]byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":4,"limit_window_seconds":18000,"reset_at":1893456000},"secondary_window":{"used_percent":4,"limit_window_seconds":604800,"reset_at":1893996000}},"credits":{"has_credits":false}}`))
		case isClaude:
			if claudeRetryAfter != "" {
				w.Header().Set("Retry-After", claudeRetryAfter)
			}
			if claudeStatus != 0 && claudeStatus != http.StatusOK {
				w.WriteHeader(claudeStatus)
				_, _ = io.WriteString(w, claudeBody)
				return
			}
			if claudeBody == "" {
				claudeBody = `{"five_hour":{"utilization":36,"resets_at":"2030-01-01T00:00:00Z"},"seven_day":{"utilization":8,"resets_at":"2030-02-01T00:00:00Z"}}`
			}
			_, _ = io.WriteString(w, claudeBody)
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
		case isAgy:
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
	Key       string `json:"key"`
	Label     string `json:"label"`
	Plan      string `json:"plan"`
	State     string `json:"state"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error"`
	Hint      string `json:"hint"`
	Windows   []struct {
		Label  string `json:"label"`
		Pct    int    `json:"pct"`
		Resets int64  `json:"resets_at"`
		Detail string `json:"detail"`
	} `json:"windows"`
}

func assertQuotaContract(t *testing.T, body []byte) {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode quota response: %v", err)
	}
	providersRaw, ok := envelope["providers"]
	if !ok {
		t.Fatal("quota response is missing providers")
	}
	for name := range envelope {
		if name != "providers" {
			t.Errorf("quota response has unexpected field %q", name)
		}
	}
	var providers []json.RawMessage
	if err := json.Unmarshal(providersRaw, &providers); err != nil {
		t.Fatalf("decode quota providers: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("quota response contains no providers")
	}
	for _, raw := range providers {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("decode provider fields: %v", err)
		}
		for _, name := range []string{"key", "label", "plan", "state", "updated_at", "error", "windows"} {
			if _, ok := fields[name]; !ok {
				t.Errorf("provider is missing %q: %s", name, raw)
			}
		}
		allowedFields := map[string]bool{"key": true, "label": true, "plan": true, "state": true, "updated_at": true, "error": true, "hint": true, "windows": true}
		for name := range fields {
			if !allowedFields[name] {
				t.Errorf("provider has unexpected field %q: %s", name, raw)
			}
		}
		var card struct {
			Key       string `json:"key"`
			State     string `json:"state"`
			UpdatedAt int64  `json:"updated_at"`
			Error     string `json:"error"`
			Hint      string `json:"hint"`
		}
		if err := json.Unmarshal(raw, &card); err != nil {
			t.Fatalf("decode provider state: %v", err)
		}
		switch card.State {
		case "ok", "stale", "expired", "not_configured", "error":
		default:
			t.Errorf("%s has unknown state %q", card.Key, card.State)
		}
		if card.State == "not_configured" && (card.Hint == "" || card.Error != "") {
			t.Errorf("%s not_configured card needs a hint and no error: %+v", card.Key, card)
		}
		if card.State == "expired" && card.Hint == "" {
			t.Errorf("%s expired card needs a hint", card.Key)
		}
		if card.State == "stale" && card.UpdatedAt == 0 {
			t.Errorf("%s stale card needs its cached update time", card.Key)
		}
		var windows []map[string]json.RawMessage
		if err := json.Unmarshal(fields["windows"], &windows); err != nil {
			t.Errorf("%s windows: %v", card.Key, err)
			continue
		}
		if windows == nil {
			t.Errorf("%s windows must be an array, not null", card.Key)
		}
		if card.State == "stale" && len(windows) == 0 {
			t.Errorf("%s stale card dropped its cached windows", card.Key)
		}
		for _, windowFields := range windows {
			for _, name := range []string{"label", "pct", "resets_at"} {
				if _, ok := windowFields[name]; !ok {
					t.Errorf("%s window is missing %q: %v", card.Key, name, windowFields)
				}
			}
			for name := range windowFields {
				switch name {
				case "label", "pct", "resets_at", "detail":
				default:
					t.Errorf("%s window has unexpected field %q: %v", card.Key, name, windowFields)
				}
			}
			var window struct {
				Label string `json:"label"`
				Pct   *int   `json:"pct"`
			}
			if err := json.Unmarshal(windowFields["label"], &window.Label); err != nil {
				t.Errorf("%s window label: %v", card.Key, err)
				continue
			}
			if err := json.Unmarshal(windowFields["pct"], &window.Pct); err != nil {
				t.Errorf("%s window percent: %v", card.Key, err)
				continue
			}
			if window.Label == "5h" || window.Label == "wk" || window.Label == "mo" {
				t.Errorf("%s uses short quota label %q", card.Key, window.Label)
			}
			if window.Pct == nil {
				if card.Key != "openrouter" || window.Label != "Spend this week" {
					t.Errorf("%s %q unexpectedly has no percent", card.Key, window.Label)
				}
				continue
			}
			if *window.Pct < 0 || *window.Pct > 100 {
				t.Errorf("%s %q percent = %d, outside 0-100", card.Key, window.Label, *window.Pct)
			}
		}
	}
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
	restoreChatGPT := el.SetChatGPTUsageEndpointForTest(srv.URL + "/backend-api/wham/usage")
	t.Cleanup(restoreChatGPT)
	openRouterRestore := el.SetOpenRouterEndpointsForTest(srv.URL+"/api/v1/credits", srv.URL+"/api/v1/key")
	t.Cleanup(openRouterRestore)
	t.Setenv("HOME", tempDir(t))
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
	t.Setenv("HOME", tempDir(t))
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

const quotaChatGPTKey = "openai-codex"

const quotaAuth = `{"opencode-go":{"type":"api_key","key":"k1"},"google-antigravity":{"type":"oauth","access":"agy-access","refresh":"r1","expires":1893456000000,"projectId":"p1"}}`

const quotaOpenRouterAuth = `{"openrouter":{"type":"api_key","key":"or-test-key"}}`

func writeClaudeFile(t *testing.T, dir string, fiveHour, sevenDay int) {
	t.Helper()
	raw := `{"five_hour":{"pct":` + quotaItoa(fiveHour) + `,"resets_at":1893456000},"seven_day":{"pct":` + quotaItoa(sevenDay) + `,"resets_at":1893456000}}`
	if err := os.WriteFile(filepath.Join(dir, "claude-usage.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeHostClaudeCredentials(t *testing.T, home, token, plan string, expiresAt int64) (string, []byte) {
	t.Helper()
	path := filepath.Join(home, ".claude", ".credentials.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"refreshToken":"must-not-change","expiresAt":%d,"subscriptionType":%q,"scopes":["user:profile"]}}`, token, expiresAt, plan))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, raw
}

func writeClaudeOAuthCache(t *testing.T, stateDir string, fetchedAt time.Time) {
	t.Helper()
	raw := []byte(fmt.Sprintf(`{"fetched_at":%d,"windows":[{"label":"5h","pct":36,"resets_at":1893456000},{"label":"wk","pct":8,"resets_at":1893456000}]}`, fetchedAt.Unix()))
	if err := os.WriteFile(filepath.Join(stateDir, "usage-claude.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestQuotaClaudeHostOAuthRefreshIsReadOnly(t *testing.T) {
	stub := &quotaStubState{}
	base, _, d := quotaEnvWithDaemon(t, stub, "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	d.clock = func() time.Time { return now }
	credentialsPath, original := writeHostClaudeCredentials(t, home, "host-access-token", "pro", now.Add(time.Hour).UnixMilli())
	before, err := os.Stat(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	status, cards, raw, err := sendQuotaRefresh(base, "claude")
	if err != nil || status != http.StatusOK {
		t.Fatalf("Claude refresh: status %d, error %v", status, err)
	}
	assertQuotaContract(t, raw)
	card := quotaByKey(t, cards)["claude"]
	if card.State != "ok" || card.Plan != "Pro" || len(card.Windows) != 2 {
		t.Fatalf("Claude card = %+v, want Pro with two windows", card)
	}
	if card.Windows[0].Label != "5-hour" || card.Windows[0].Pct != 36 || card.Windows[1].Label != "Weekly" || card.Windows[1].Pct != 8 {
		t.Errorf("Claude windows = %+v, want 5-hour 36%% and Weekly 8%%", card.Windows)
	}
	stub.mu.Lock()
	calls, totalCalls, method, authorization, userAgent, beta := stub.claudeCalls, stub.calls, stub.claudeMethod, stub.claudeAuthorization, stub.claudeUserAgent, stub.claudeBeta
	stub.mu.Unlock()
	if calls != 1 || totalCalls != 1 || method != http.MethodGet || authorization != "Bearer host-access-token" || beta != "oauth-2025-04-20" || userAgent != "claude-code/1.0.0" {
		t.Errorf("Claude usage requests = calls:%d total:%d method:%q authorization:%q user-agent:%q beta:%q", calls, totalCalls, method, authorization, userAgent, beta)
	}
	after, err := os.Stat(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	afterRaw, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterRaw) != string(original) || !before.ModTime().Equal(after.ModTime()) || before.Mode().Perm() != after.Mode().Perm() {
		t.Errorf("Claude credentials changed during polling: bytesEqual=%t mtimeEqual=%t modeBefore=%#o modeAfter=%#o", string(afterRaw) == string(original), before.ModTime().Equal(after.ModTime()), before.Mode().Perm(), after.Mode().Perm())
	}
}

func TestQuotaClaudeExpiredLoginRetainsCachedValues(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		expiredAt time.Duration
		wantCalls int
	}{
		{name: "expired timestamp", expiredAt: -time.Minute, wantCalls: 0},
		{name: "unauthorized response", status: http.StatusUnauthorized, expiredAt: time.Hour, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &quotaStubState{claudeStatus: test.status}
			base, stateDir, d := quotaEnvWithDaemon(t, stub, "")
			now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
			d.clock = func() time.Time { return now }
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			writeHostClaudeCredentials(t, home, "host-access-token", "pro", now.Add(test.expiredAt).UnixMilli())
			writeClaudeOAuthCache(t, stateDir, now.Add(-5*time.Minute))
			status, cards := postQuotaRefresh(t, base, "claude")
			card := quotaByKey(t, cards)["claude"]
			if status != http.StatusOK || card.State != "expired" || !strings.Contains(card.Hint, "expired") {
				t.Fatalf("Claude card = %+v, status %d; want expired with a sign-in hint", card, status)
			}
			if card.Plan != "Pro" || len(card.Windows) != 2 || card.Windows[0].Pct != 36 || card.Windows[1].Pct != 8 {
				t.Errorf("expired Claude card dropped last values or plan: %+v", card)
			}
			stub.mu.Lock()
			calls := stub.claudeCalls
			stub.mu.Unlock()
			if calls != test.wantCalls {
				t.Errorf("Claude endpoint calls = %d, want %d", calls, test.wantCalls)
			}
			if test.status == http.StatusUnauthorized {
				now = now.Add(30 * time.Second)
				status, cards = postQuotaRefresh(t, base, "claude")
				card = quotaByKey(t, cards)["claude"]
				if status != http.StatusOK || card.State != "expired" || len(card.Windows) != 2 {
					t.Errorf("repeat after 401 = %+v, status %d; want retained expired card", card, status)
				}
				stub.mu.Lock()
				calls = stub.claudeCalls
				stub.mu.Unlock()
				if calls != 1 {
					t.Errorf("repeated expired token made %d upstream calls, want 1", calls)
				}
			}
		})
	}
}

func TestQuotaClaudeRateLimitBacksOffAndTransientFailureKeepsCache(t *testing.T) {
	t.Run("retry-after", func(t *testing.T) {
		stub := &quotaStubState{claudeStatus: http.StatusTooManyRequests, claudeRetryAfter: "120"}
		base, stateDir, d := quotaEnvWithDaemon(t, stub, "")
		now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
		d.clock = func() time.Time { return now }
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		writeHostClaudeCredentials(t, home, "host-access-token", "pro", now.Add(time.Hour).UnixMilli())
		writeClaudeOAuthCache(t, stateDir, now.Add(-5*time.Minute))
		_, cards := postQuotaRefresh(t, base, "claude")
		card := quotaByKey(t, cards)["claude"]
		if card.State != "stale" || card.Error == "" || len(card.Windows) != 2 || card.Windows[0].Pct != 36 {
			t.Fatalf("rate-limited Claude card = %+v, want stale with retained windows", card)
		}
		now = now.Add(30 * time.Second)
		_, cards = postQuotaRefresh(t, base, "claude")
		if card = quotaByKey(t, cards)["claude"]; card.State != "stale" || len(card.Windows) != 2 {
			t.Errorf("card during Retry-After backoff = %+v", card)
		}
		stub.mu.Lock()
		calls := stub.claudeCalls
		stub.mu.Unlock()
		if calls != 1 {
			t.Errorf("Claude endpoint calls during Retry-After = %d, want 1", calls)
		}
		if _, err := os.Stat(filepath.Join(stateDir, "usage-claude-backoff")); err != nil {
			t.Errorf("Retry-After backoff was not persisted: %v", err)
		}
	})

	t.Run("other failure", func(t *testing.T) {
		stub := &quotaStubState{claudeStatus: http.StatusInternalServerError}
		base, stateDir, d := quotaEnvWithDaemon(t, stub, "")
		now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
		d.clock = func() time.Time { return now }
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		writeHostClaudeCredentials(t, home, "host-access-token", "pro", now.Add(time.Hour).UnixMilli())
		writeClaudeOAuthCache(t, stateDir, now.Add(-5*time.Minute))
		cacheInfo, err := os.Stat(filepath.Join(stateDir, "usage-claude.json"))
		if err != nil {
			t.Fatal(err)
		}
		_, cards := postQuotaRefresh(t, base, "claude")
		card := quotaByKey(t, cards)["claude"]
		if card.State != "stale" || card.Error == "" || len(card.Windows) != 2 || card.Windows[0].Pct != 36 || card.UpdatedAt != cacheInfo.ModTime().Unix() {
			t.Errorf("transient failure card = %+v; want stale with its age and last values", card)
		}
	})
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
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	assertQuotaContract(t, raw)
	var out struct {
		Providers []quotaCard `json:"providers"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
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
	if got.State != "stale" || got.Error == "" || len(got.Windows) != 1 || got.Windows[0].Pct != 41 {
		t.Fatalf("GET after failed background refresh = %+v, want stale state, error text and retained windows", got)
	}
}

type quotaPostResult struct {
	status int
	cards  []quotaCard
	raw    []byte
	err    error
}

func sendQuotaRefresh(base, key string) (int, []quotaCard, []byte, error) {
	var body io.Reader
	if key != "<empty>" {
		raw, err := json.Marshal(map[string]string{"key": key})
		if err != nil {
			return 0, nil, nil, err
		}
		body = bytes.NewReader(raw)
	}
	resp, err := http.Post(base+"/api/quota/refresh", "application/json", body)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, nil, nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, nil, err
	}
	var out struct {
		Providers []quotaCard `json:"providers"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return resp.StatusCode, nil, raw, err
	}
	return resp.StatusCode, out.Providers, raw, nil
}

func postQuotaRefresh(t *testing.T, base, key string) (int, []quotaCard) {
	t.Helper()
	status, cards, raw, err := sendQuotaRefresh(base, key)
	if err != nil {
		t.Fatal(err)
	}
	if status == http.StatusOK {
		assertQuotaContract(t, raw)
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
	if len(cards) != 6 {
		t.Fatalf("providers = %d, want 6", len(cards))
	}
	wantKeys := []string{"claude", "openai-codex", "opencode-go", "agy-gemini", "agy-3p", "openrouter"}
	for i, k := range wantKeys {
		if cards[i].Key != k {
			t.Errorf("card %d key = %q, want %q", i, cards[i].Key, k)
		}
	}
	m := quotaByKey(t, cards)
	cl := m["claude"]
	if cl.Label != "Claude" || cl.State != "ok" || len(cl.Windows) != 2 || cl.Windows[0].Label != "5-hour" || cl.Windows[0].Pct != 10 || cl.Windows[1].Label != "Weekly" || cl.Windows[1].Pct != 40 {
		t.Errorf("claude card = %+v", cl)
	}
	chatGPT := m[quotaChatGPTKey]
	if chatGPT.Label != "ChatGPT Plus" || chatGPT.Plan != "Plus" || chatGPT.State != "not_configured" || len(chatGPT.Windows) != 0 {
		t.Errorf("ChatGPT Plus card = %+v, want an unconfigured card without windows", chatGPT)
	}
	openrouter := m["openrouter"]
	if openrouter.State != "not_configured" || openrouter.UpdatedAt != 0 || openrouter.Error != "" || !strings.Contains(openrouter.Hint, "openrouter.key") {
		t.Errorf("OpenRouter without a pig key = %+v, want not_configured with a key hint", openrouter)
	}
	oc := m["opencode-go"]
	if oc.Label != "OpenCode-Go" || oc.State != "ok" || oc.Plan != "Go" || len(oc.Windows) != 3 || oc.Windows[0].Label != "5-hour" || oc.Windows[0].Pct != 12 || oc.Windows[1].Label != "Weekly" || oc.Windows[2].Label != "Monthly" {
		t.Errorf("opencode card = %+v", oc)
	}
	gem := m["agy-gemini"]
	if gem.State != "ok" || gem.Plan != "Gemini" || len(gem.Windows) != 2 || gem.Windows[0].Label != "5-hour" || gem.Windows[0].Pct != 20 || gem.Windows[1].Label != "Weekly" || gem.Windows[1].Pct != 25 {
		t.Errorf("gemini card = %+v", gem)
	}
	tp := m["agy-3p"]
	if tp.State != "ok" || tp.Plan != "Claude + GPT-OSS" || len(tp.Windows) != 2 || tp.Windows[0].Label != "5-hour" || tp.Windows[0].Pct != 10 || tp.Windows[1].Label != "Weekly" || tp.Windows[1].Pct != 60 {
		t.Errorf("3p card = %+v", tp)
	}
	for _, c := range cards {
		if c.Key != quotaChatGPTKey && c.Key != "openrouter" && c.UpdatedAt == 0 {
			t.Errorf("%s updated_at = 0, want the cache mtime", c.Key)
		}
		if (c.Key == quotaChatGPTKey || c.Key == "openrouter") && c.UpdatedAt != 0 {
			t.Errorf("unconfigured %s updated_at = %d, want 0", c.Key, c.UpdatedAt)
		}
		if c.Error != "" {
			t.Errorf("%s error = %q, want empty", c.Key, c.Error)
		}
	}
}

func quotaChatGPTAuth(expiresAt int64) string {
	return `{"openai-codex":{"type":"oauth","access":"test-access-token","refresh":"test-refresh-token","expires":` + strconv.FormatInt(expiresAt, 10) + `,"accountId":"acct-test"}}`
}

func TestQuotaChatGPTPlusUsesPigLoginReadOnly(t *testing.T) {
	stub := &quotaStubState{}
	base, stateDir := quotaEnv(t, stub, quotaChatGPTAuth(time.Now().Add(time.Hour).UnixMilli()))
	authPath := os.Getenv("EL_PIG_AUTH")
	authBefore, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}

	status, cards := postQuotaRefresh(t, base, quotaChatGPTKey)
	if status != http.StatusOK {
		t.Fatalf("ChatGPT refresh status = %d", status)
	}
	card := quotaByKey(t, cards)[quotaChatGPTKey]
	if card.Label != "ChatGPT Plus" || card.Plan != "Plus" || card.State != "ok" || card.Error != "" || len(card.Windows) != 2 {
		t.Fatalf("ChatGPT card = %+v, want Plus with two ok windows", card)
	}
	if first := card.Windows[0]; first.Label != "5-hour" || first.Pct != 4 || first.Resets != 1893456000 {
		t.Errorf("ChatGPT primary window = %+v, want 5-hour at 4%%", first)
	}
	if second := card.Windows[1]; second.Label != "Weekly" || second.Pct != 4 || second.Resets != 1893996000 {
		t.Errorf("ChatGPT secondary window = %+v, want Weekly at 4%%", second)
	}
	if card.Windows[0].Detail != "" || card.Windows[1].Detail != "" {
		t.Errorf("ChatGPT windows should not expose extra credits: %+v", card.Windows)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "usage-openai-codex.json")); err != nil {
		t.Errorf("ChatGPT usage cache was not written: %v", err)
	}

	stub.mu.Lock()
	calls, method, authorization, accountID, tokenCalls := stub.chatGPTCalls, stub.chatGPTMethod, stub.chatGPTAuthorization, stub.chatGPTAccountID, stub.tokenCalls
	stub.mu.Unlock()
	if calls != 1 || method != http.MethodGet {
		t.Errorf("ChatGPT usage calls = %d method %q, want one GET", calls, method)
	}
	if authorization != "Bearer test-access-token" || accountID != "acct-test" {
		t.Errorf("ChatGPT request headers = authorization %q account %q", authorization, accountID)
	}
	if tokenCalls != 0 {
		t.Errorf("ChatGPT refresh contacted a token endpoint %d times", tokenCalls)
	}
	authAfter, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	infoAfter, err := os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(authBefore, authAfter) || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Error("ChatGPT quota refresh modified pig auth.json")
	}
}

func TestQuotaChatGPTMissingLoginIsNotConfigured(t *testing.T) {
	stub := &quotaStubState{}
	base, _, d := quotaEnvWithDaemon(t, stub, "")
	missingAuthPath := os.Getenv("EL_PIG_AUTH")
	cards := quotaByKey(t, getQuota(t, base))
	d.quotaRefreshWG.Wait()
	card := cards[quotaChatGPTKey]
	if card.State != "not_configured" || card.Error != "" || card.Hint == "" {
		t.Fatalf("ChatGPT card without a login = %+v, want not_configured and a hint", card)
	}
	if _, err := os.Stat(missingAuthPath); !os.IsNotExist(err) {
		t.Errorf("read-only ChatGPT login lookup created auth.json: stat err = %v", err)
	}
	stub.mu.Lock()
	calls := stub.chatGPTCalls
	stub.mu.Unlock()
	if calls != 0 {
		t.Errorf("missing ChatGPT login made %d upstream requests", calls)
	}
}

func TestQuotaChatGPTExpiredLoginKeepsCachedWindows(t *testing.T) {
	stub := &quotaStubState{}
	base, stateDir, d := quotaEnvWithDaemon(t, stub, quotaChatGPTAuth(time.Now().Add(-time.Hour).UnixMilli()))
	cache := `{"fetched_at":1,"windows":[{"label":"5-hour","pct":4,"resets_at":1893456000},{"label":"Weekly","pct":4,"resets_at":1893996000}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "usage-openai-codex.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	cards := quotaByKey(t, getQuota(t, base))
	d.quotaRefreshWG.Wait()
	card := cards[quotaChatGPTKey]
	if card.State != "expired" || card.Hint == "" || card.Error != "" || len(card.Windows) != 2 || card.Windows[0].Pct != 4 || card.Windows[1].Pct != 4 {
		t.Fatalf("expired ChatGPT card = %+v, want expired with cached windows", card)
	}
	stub.mu.Lock()
	calls := stub.chatGPTCalls
	stub.mu.Unlock()
	if calls != 0 {
		t.Errorf("expired ChatGPT token made %d upstream requests", calls)
	}
}

func TestQuotaChatGPTUnauthorizedMarksExpiredAndKeepsCache(t *testing.T) {
	stub := &quotaStubState{chatGPTStatus: http.StatusUnauthorized}
	base, stateDir := quotaEnv(t, stub, quotaChatGPTAuth(time.Now().Add(time.Hour).UnixMilli()))
	cache := `{"fetched_at":1,"windows":[{"label":"5-hour","pct":4,"resets_at":1893456000},{"label":"Weekly","pct":4,"resets_at":1893996000}]}`
	if err := os.WriteFile(filepath.Join(stateDir, "usage-openai-codex.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	status, cards := postQuotaRefresh(t, base, quotaChatGPTKey)
	if status != http.StatusOK {
		t.Fatalf("ChatGPT refresh status = %d", status)
	}
	card := quotaByKey(t, cards)[quotaChatGPTKey]
	if card.State != "expired" || card.Hint == "" || card.Error != "" || len(card.Windows) != 2 || card.Windows[0].Pct != 4 || card.Windows[1].Pct != 4 {
		t.Fatalf("unauthorized ChatGPT card = %+v, want expired with retained windows", card)
	}
}

func TestQuotaOpenRouterCreditsAndKeySpendAtHTTPSeam(t *testing.T) {
	stub := &quotaStubState{}
	base, stateDir := quotaEnv(t, stub, quotaOpenRouterAuth)
	status, cards, raw, err := sendQuotaRefresh(base, "openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("OpenRouter refresh status = %d", status)
	}
	assertQuotaContract(t, raw)
	if strings.Contains(string(raw), "or-test-key") {
		t.Fatal("quota response exposed the pig OpenRouter key")
	}
	card := quotaByKey(t, cards)["openrouter"]
	if card.State != "ok" || card.Label != "OpenRouter" || card.Plan != "Credits" || len(card.Windows) != 2 {
		t.Fatalf("OpenRouter card = %+v", card)
	}
	if got := card.Windows[0]; got.Label != "Credits" || got.Pct != 94 || got.Detail != "$59.16 of $63.00 · $3.84 left" {
		t.Errorf("OpenRouter credits window = %+v", got)
	}
	if got := card.Windows[1]; got.Label != "Spend this week" || got.Detail != "$2.37 · today $0.91 · month $5.70" {
		t.Errorf("OpenRouter spend window = %+v", got)
	}
	var rawCard struct {
		Providers []struct {
			Key     string `json:"key"`
			Windows []struct {
				Pct *int `json:"pct"`
			} `json:"windows"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &rawCard); err != nil {
		t.Fatal(err)
	}
	for _, provider := range rawCard.Providers {
		if provider.Key == "openrouter" && (len(provider.Windows) != 2 || provider.Windows[1].Pct != nil) {
			t.Errorf("OpenRouter spend window should be text-only: %+v", provider.Windows)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "usage-openrouter.json")); err != nil {
		t.Errorf("OpenRouter cache missing from temp state dir: %v", err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.openrouterCredits != 1 || stub.openrouterKey != 1 {
		t.Errorf("OpenRouter calls: credits=%d key=%d, want one each", stub.openrouterCredits, stub.openrouterKey)
	}
	if len(stub.openrouterAuth) != 2 || stub.openrouterAuth[0] != "Bearer or-test-key" || stub.openrouterAuth[1] != "Bearer or-test-key" {
		t.Errorf("OpenRouter Authorization headers = %v", stub.openrouterAuth)
	}
}

func TestQuotaOpenRouterMissingKeyIsNotConfigured(t *testing.T) {
	stub := &quotaStubState{}
	base, _, d := quotaEnvWithDaemon(t, stub, "")
	cards := quotaByKey(t, getQuota(t, base))
	card := cards["openrouter"]
	if card.State != "not_configured" || card.Error != "" || !strings.Contains(card.Hint, "openrouter.key") {
		t.Fatalf("OpenRouter missing key = %+v, want not_configured without an error", card)
	}
	d.quotaRefreshWG.Wait()
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.openrouterCredits != 0 || stub.openrouterKey != 0 {
		t.Errorf("missing OpenRouter key made upstream calls: credits=%d key=%d", stub.openrouterCredits, stub.openrouterKey)
	}
}
func TestQuotaNotConfigured(t *testing.T) {
	stub := &quotaStubState{}
	base, _ := quotaEnv(t, stub, "")
	for _, c := range getQuota(t, base) {
		if c.State != "not_configured" {
			t.Errorf("%s state = %q, want not_configured", c.Key, c.State)
		}
		if len(c.Windows) != 0 || c.Error != "" || c.Hint == "" {
			t.Errorf("%s = %+v, want empty card with a missing-login hint and no error", c.Key, c)
		}
	}
}

func TestQuotaFailedFetchKeepsStaleCache(t *testing.T) {
	stub := &quotaStubState{opencodePct: 50}
	base, _, d := quotaEnvWithDaemon(t, stub, quotaAuth)
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	d.clock = func() time.Time { return now }
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
	// Wait out the manual cooldown so the failing upstream is really attempted.
	now = now.Add(quotaManualRefreshMinInterval)
	_, cards := postQuotaRefresh(t, base, "opencode-go")
	got := quotaByKey(t, cards)["opencode-go"]
	if got.State != "stale" || got.Error == "" || got.UpdatedAt == 0 {
		t.Errorf("failed card = %+v, want stale with error text and its cache age", got)
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
	base, _, d := quotaEnvWithDaemon(t, stub, quotaAuth)
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	d.clock = func() time.Time { return now }
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
	d.quotaRefreshWG.Wait()
	now = now.Add(quotaManualRefreshMinInterval)
	code, cards = postQuotaRefresh(t, base, "opencode-go")
	if code != http.StatusOK {
		t.Fatalf("refresh status = %d", code)
	}
	if got := quotaByKey(t, cards)["opencode-go"]; got.State != "ok" || got.Windows[0].Pct != 77 {
		t.Errorf("forced card = %+v, want ok at 77", got)
	}
}

func TestQuotaManualRefreshMinimumIntervalPerProvider(t *testing.T) {
	stub := &quotaStubState{opencodePct: 12}
	base, _, d := quotaEnvWithDaemon(t, stub, quotaAuth)
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	d.clock = func() time.Time { return now }

	if code, cards := postQuotaRefresh(t, base, "opencode-go"); code != http.StatusOK || quotaByKey(t, cards)["opencode-go"].Windows[0].Pct != 12 {
		t.Fatalf("initial refresh: status %d, opencode-go %+v", code, quotaByKey(t, cards)["opencode-go"])
	}
	stub.mu.Lock()
	stub.opencodePct = 77
	stub.mu.Unlock()

	// A repeat within the default interval returns the last value without
	// reaching upstream, while another provider in refresh-all still refreshes.
	if code, cards := postQuotaRefresh(t, base, "opencode-go"); code != http.StatusOK || quotaByKey(t, cards)["opencode-go"].Windows[0].Pct != 12 {
		t.Errorf("rapid repeat: status %d, opencode-go %+v", code, quotaByKey(t, cards)["opencode-go"])
	}
	stub.mu.Lock()
	opencodeCalls := stub.opencodeCalls
	stub.mu.Unlock()
	if opencodeCalls != 1 {
		t.Fatalf("rapid repeated refresh made %d OpenCode-Go calls, want 1", opencodeCalls)
	}

	if code, cards := postQuotaRefresh(t, base, "<empty>"); code != http.StatusOK {
		t.Fatalf("refresh-all status = %d", code)
	} else if got := quotaByKey(t, cards); got["opencode-go"].Windows[0].Pct != 12 || got["agy-gemini"].State != "ok" {
		t.Errorf("partial refresh-all cards = %+v, want cached OpenCode-Go and refreshed Antigravity", got)
	}
	// The two Antigravity cards share one provider cooldown.
	if code, _ := postQuotaRefresh(t, base, "agy-3p"); code != http.StatusOK {
		t.Fatalf("Antigravity alias refresh status = %d", code)
	}
	stub.mu.Lock()
	opencodeCalls, agyCalls := stub.opencodeCalls, stub.agyCalls
	stub.mu.Unlock()
	if opencodeCalls != 1 || agyCalls != 1 {
		t.Fatalf("refresh-all cooldown counts: OpenCode-Go %d, Antigravity %d; want 1 each", opencodeCalls, agyCalls)
	}

	// At the interval boundary both providers may be refreshed again.
	now = now.Add(quotaManualRefreshMinInterval)
	code, cards := postQuotaRefresh(t, base, "<empty>")
	if code != http.StatusOK {
		t.Fatalf("refresh-all after cooldown status = %d", code)
	}
	if got := quotaByKey(t, cards)["opencode-go"]; got.State != "ok" || got.Windows[0].Pct != 77 {
		t.Errorf("OpenCode-Go after cooldown = %+v, want fresh 77%%", got)
	}
	stub.mu.Lock()
	opencodeCalls, agyCalls = stub.opencodeCalls, stub.agyCalls
	stub.mu.Unlock()
	if opencodeCalls != 2 || agyCalls != 2 {
		t.Errorf("refresh-all after cooldown counts: OpenCode-Go %d, Antigravity %d; want 2 each", opencodeCalls, agyCalls)
	}
}

func TestQuotaManualRefreshFailureStillStartsCooldown(t *testing.T) {
	stub := &quotaStubState{opencodeErr: true}
	base, _, d := quotaEnvWithDaemon(t, stub, quotaAuth)
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	d.clock = func() time.Time { return now }

	code, cards := postQuotaRefresh(t, base, "opencode-go")
	if code != http.StatusOK || quotaByKey(t, cards)["opencode-go"].State != "error" {
		t.Fatalf("first failed refresh: status %d, card %+v", code, quotaByKey(t, cards)["opencode-go"])
	}
	stub.mu.Lock()
	stub.opencodeErr = false
	stub.opencodePct = 77
	stub.mu.Unlock()
	code, cards = postQuotaRefresh(t, base, "opencode-go")
	if code != http.StatusOK || quotaByKey(t, cards)["opencode-go"].State != "error" {
		t.Errorf("rapid retry after failure: status %d, card %+v", code, quotaByKey(t, cards)["opencode-go"])
	}
	stub.mu.Lock()
	opencodeCalls := stub.opencodeCalls
	stub.mu.Unlock()
	if opencodeCalls != 1 {
		t.Fatalf("rapid retry after failure made %d upstream calls, want 1", opencodeCalls)
	}

	now = now.Add(quotaManualRefreshMinInterval)
	code, cards = postQuotaRefresh(t, base, "opencode-go")
	if code != http.StatusOK {
		t.Fatalf("refresh after cooldown status = %d", code)
	}
	if got := quotaByKey(t, cards)["opencode-go"]; got.State != "ok" || got.Windows[0].Pct != 77 {
		t.Errorf("refresh after cooldown = %+v, want fresh 77%%", got)
	}
	stub.mu.Lock()
	opencodeCalls = stub.opencodeCalls
	stub.mu.Unlock()
	if opencodeCalls != 2 {
		t.Errorf("refresh after cooldown made %d upstream calls, want 2", opencodeCalls)
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
		status, cards, raw, err := sendQuotaRefresh(base, "<empty>")
		result <- quotaPostResult{status: status, cards: cards, raw: raw, err: err}
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
		assertQuotaContract(t, got.raw)
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
		status, cards, raw, err := sendQuotaRefresh(base, "opencode-go")
		first <- quotaPostResult{status: status, cards: cards, raw: raw, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first refresher did not reach the OpenCode-Go stub")
	}

	second := make(chan quotaPostResult, 1)
	go func() {
		status, cards, raw, err := sendQuotaRefresh(base, "opencode-go")
		second <- quotaPostResult{status: status, cards: cards, raw: raw, err: err}
	}()
	select {
	case got := <-second:
		if got.err != nil || got.status != http.StatusOK {
			t.Fatalf("second refresh = status %d, err %v", got.status, got.err)
		}
		assertQuotaContract(t, got.raw)
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
		assertQuotaContract(t, got.raw)
	case <-time.After(time.Second):
		t.Fatal("first refresher did not finish after unblocking the upstream")
	}
}
