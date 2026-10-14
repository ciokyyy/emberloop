package web

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	el "example.com/emberloop-web/el"
	"github.com/MichaelKinsy/PiG/ai"
)

type deviceCodeStub struct {
	pending     bool
	pollGate    <-chan struct{}
	quotaSeen   chan struct{}
	quotaAccess string
	accountID   string
}

func (s *deviceCodeStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/accounts/deviceauth/usercode":
		writeDeviceJSON(w, map[string]any{"device_auth_id": "device-id", "user_code": "WXYZ-7", "interval": 0})
	case "/api/accounts/deviceauth/token":
		if s.pollGate != nil {
			select {
			case <-s.pollGate:
			case <-r.Context().Done():
				return
			}
		}
		if s.pending {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeDeviceJSON(w, map[string]string{"authorization_code": "authorization-code", "code_verifier": "verifier"})
	case "/oauth/token":
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=authorization_code") || !strings.Contains(string(body), "code_verifier=verifier") {
			http.Error(w, "unexpected token exchange", http.StatusBadRequest)
			return
		}
		writeDeviceJSON(w, map[string]any{"access_token": s.quotaAccess, "refresh_token": "device-refresh-secret", "expires_in": 3600})
	case "/backend-api/wham/usage":
		if r.Header.Get("Authorization") != "Bearer "+s.quotaAccess || r.Header.Get("ChatGPT-Account-Id") != s.accountID {
			http.Error(w, "incorrect ChatGPT credentials", http.StatusUnauthorized)
			return
		}
		writeDeviceJSON(w, map[string]any{"rate_limit": map[string]any{
			"primary_window":   map[string]any{"used_percent": 12, "limit_window_seconds": 18000, "reset_at": 1893456000},
			"secondary_window": map[string]any{"used_percent": 23, "limit_window_seconds": 604800, "reset_at": 1893996000},
		}})
		select {
		case s.quotaSeen <- struct{}{}:
		default:
		}
	default:
		http.NotFound(w, r)
	}
}

func writeDeviceJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

type deviceCodeRewriteTransport struct {
	base *url.URL
	next http.RoundTripper
}

func (t deviceCodeRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	copyURL := *req.URL
	copyURL.Scheme = t.base.Scheme
	copyURL.Host = t.base.Host
	clone.URL = &copyURL
	clone.Host = t.base.Host
	return t.next.RoundTrip(clone)
}

type deviceLoginReply struct {
	Login chatGPTDeviceLoginView `json:"login"`
}

func chatGPTDeviceTestServer(t *testing.T, d *Daemon, stub *deviceCodeStub) (*http.Cookie, http.Handler) {
	t.Helper()
	upstream := httptest.NewServer(stub)
	t.Cleanup(upstream.Close)
	base, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: deviceCodeRewriteTransport{base: base, next: http.DefaultTransport}}
	t.Cleanup(func() { http.DefaultClient = oldClient })
	restoreQuotaEndpoint := el.SetChatGPTUsageEndpointForTest(upstream.URL + "/backend-api/wham/usage")
	t.Cleanup(restoreQuotaEndpoint)
	return signedIn(t, d.Handler(), "admin"), d.Handler()
}

func startChatGPTDeviceLogin(t *testing.T, h http.Handler, admin *http.Cookie) chatGPTDeviceLoginView {
	t.Helper()
	rec := authDo(t, h, http.MethodPost, chatGPTDeviceCodePath, "", admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start ChatGPT login = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("start Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	var response deviceLoginReply
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if response.Login.ID == "" || response.Login.Status != "starting" {
		t.Fatalf("start response = %+v, want a starting login with id", response.Login)
	}
	return response.Login
}

func getChatGPTDeviceLogin(t *testing.T, h http.Handler, admin *http.Cookie, method, id string) chatGPTDeviceLoginView {
	t.Helper()
	rec := authDo(t, h, method, chatGPTDeviceCodePath+"/"+id, "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s ChatGPT login = %d: %s", method, rec.Code, rec.Body.String())
	}
	var response deviceLoginReply
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode login status: %v", err)
	}
	if response.Login.ID != id {
		t.Fatalf("status id = %q, want %q", response.Login.ID, id)
	}
	return response.Login
}

func waitForChatGPTDeviceStatus(t *testing.T, h http.Handler, admin *http.Cookie, id, status string) chatGPTDeviceLoginView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec := authDo(t, h, http.MethodGet, chatGPTDeviceCodePath+"/"+id, "", admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET ChatGPT login = %d: %s", rec.Code, rec.Body.String())
		}
		var response deviceLoginReply
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode login status: %v", err)
		}
		if response.Login.Status == status {
			return response.Login
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("ChatGPT login %s never reached %q", id, status)
	return chatGPTDeviceLoginView{}
}

func waitChatGPTDeviceLoginDone(t *testing.T, d *Daemon, id string) {
	t.Helper()
	d.providerLoginMu.Lock()
	login := d.chatGPTLogins[id]
	d.providerLoginMu.Unlock()
	if login == nil {
		t.Fatalf("ChatGPT login %s disappeared", id)
	}
	select {
	case <-login.done:
	case <-time.After(3 * time.Second):
		t.Fatalf("ChatGPT login %s did not stop", id)
	}
}

func TestChatGPTDeviceCodeConnectWritesAuthAndRefreshesQuota(t *testing.T) {
	t.Setenv("HOME", tempDir(t))
	stateDir := tempDir(t)
	t.Setenv("EL_STATE_DIR", stateDir)
	authPath := filepath.Join(tempDir(t), "auth.json")
	t.Setenv("EL_PIG_AUTH", authPath)
	accountID := "chatgpt-account"
	payload, err := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": accountID}})
	if err != nil {
		t.Fatal(err)
	}
	accessToken := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
	pollGate := make(chan struct{})
	stub := &deviceCodeStub{pollGate: pollGate, quotaSeen: make(chan struct{}, 1), quotaAccess: accessToken, accountID: accountID}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	admin, h := chatGPTDeviceTestServer(t, d, stub)

	started := startChatGPTDeviceLogin(t, h, admin)
	waiting := waitForChatGPTDeviceStatus(t, h, admin, started.ID, "waiting")
	if waiting.UserCode != "WXYZ-7" || waiting.VerificationURI != "https://auth.openai.com/codex/device" || waiting.ExpiresAt == 0 {
		t.Fatalf("device-code information = %+v", waiting)
	}
	close(pollGate)
	complete := waitForChatGPTDeviceStatus(t, h, admin, started.ID, "complete")
	statusResponse := authDo(t, h, http.MethodGet, chatGPTDeviceCodePath+"/"+started.ID, "", admin)
	if complete.UserCode != "" || complete.VerificationURI != "" || strings.Contains(statusResponse.Body.String(), accessToken) || strings.Contains(statusResponse.Body.String(), "device-refresh-secret") {
		t.Fatalf("completed login response exposed credentials or retained the device code: %s", statusResponse.Body.String())
	}
	waitChatGPTDeviceLoginDone(t, d, started.ID)

	credential, err := ai.NewReadOnlyAuthStorage(authPath).Read(t.Context(), "openai-codex")
	if err != nil || credential == nil {
		t.Fatalf("read saved ChatGPT login = %#v, %v", credential, err)
	}
	if credential.Type != ai.CredentialOAuth || credential.Access != accessToken || credential.Refresh != "device-refresh-secret" || credential.AccountID != accountID {
		t.Fatalf("saved ChatGPT credential = %+v", credential)
	}
	select {
	case <-stub.quotaSeen:
	case <-time.After(3 * time.Second):
		t.Fatal("successful login did not refresh ChatGPT quota")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, provider := range d.quotaProviders() {
			if provider.Key == "openai-codex" && provider.State == "ok" && len(provider.Windows) == 2 && provider.Windows[0].Pct == 12 && provider.Windows[1].Pct == 23 {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("ChatGPT quota cache was not refreshed with the newly connected login")
}

func TestChatGPTDeviceCodeConnectExpiresAndCancels(t *testing.T) {
	t.Setenv("HOME", tempDir(t))
	t.Setenv("EL_STATE_DIR", tempDir(t))
	authPath := filepath.Join(tempDir(t), "auth.json")
	t.Setenv("EL_PIG_AUTH", authPath)
	stub := &deviceCodeStub{pending: true, quotaSeen: make(chan struct{}, 1)}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	var clockMillis atomic.Int64
	clockMillis.Store(time.Now().UnixMilli())
	d.clock = func() time.Time { return time.UnixMilli(clockMillis.Load()) }
	admin, h := chatGPTDeviceTestServer(t, d, stub)

	expiring := startChatGPTDeviceLogin(t, h, admin)
	waitForChatGPTDeviceStatus(t, h, admin, expiring.ID, "waiting")
	clockMillis.Add(int64(chatGPTDeviceCodeLifetime + time.Second))
	expired := getChatGPTDeviceLogin(t, h, admin, http.MethodGet, expiring.ID)
	if expired.Status != "expired" {
		t.Fatalf("expired login status = %q, want expired", expired.Status)
	}
	waitChatGPTDeviceLoginDone(t, d, expiring.ID)

	canceled := startChatGPTDeviceLogin(t, h, admin)
	waitForChatGPTDeviceStatus(t, h, admin, canceled.ID, "waiting")
	cancelled := getChatGPTDeviceLogin(t, h, admin, http.MethodDelete, canceled.ID)
	if cancelled.Status != "canceled" {
		t.Fatalf("cancelled login status = %q, want canceled", cancelled.Status)
	}
	waitChatGPTDeviceLoginDone(t, d, canceled.ID)
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Fatalf("expired or cancelled login wrote pig auth.json (stat error %v)", err)
	}
}

func TestChatGPTDeviceCodeConnectIsAdminOnly(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", filepath.Join(tempDir(t), "auth.json"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	signedIn(t, h, "admin")
	user := signedIn(t, h, "mira")

	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{name: "anonymous", want: http.StatusUnauthorized},
		{name: "non-admin", cookie: user, want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.cookie == nil {
				rec = authDo(t, h, http.MethodPost, chatGPTDeviceCodePath, "")
			} else {
				rec = authDo(t, h, http.MethodPost, chatGPTDeviceCodePath, "", tc.cookie)
			}
			if rec.Code != tc.want {
				t.Fatalf("start as %s = %d: %s, want %d", tc.name, rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}
