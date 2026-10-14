package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam: the Daemon HTTP API with the fake Backend. Settings behaviour is
// observed through GET/PUT responses and the live Session cap, never
// through internals.

func settingsServer(t *testing.T, settingsBody string) (*httptest.Server, string) {
	t.Helper()
	dir := tempDir(t)
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(settingsBody), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	t.Setenv("EL_MAX_TICKETS", "")
	t.Setenv("EL_MAX_WORKERS", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	return srv, settings
}

type settingsResp struct {
	Roles                map[string]roleSetting `json:"roles"`
	MaxTickets           int                    `json:"max_tickets"`
	MaxTicketsFromEnv    bool                   `json:"max_tickets_from_env"`
	OAuthRedirectBaseURL string                 `json:"oauth_redirect_base_url"`
}

func getSettingsView(t *testing.T, base string) (settingsResp, int) {
	t.Helper()
	resp, err := http.Get(base + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out settingsResp
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out, resp.StatusCode
}

func putSettingsBody(t *testing.T, base, body string) (settingsResp, int) {
	t.Helper()
	req, err := http.NewRequest("PUT", base+"/api/settings", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out settingsResp
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out, resp.StatusCode
}

// The settings read returns max_tickets and whether the environment
// overrides it.
func TestSettingsReadReturnsMaxTicketsAndEnvFlag(t *testing.T) {
	srv, _ := settingsServer(t, `{"max_tickets": 5, "roles": {}}`)

	out, code := getSettingsView(t, srv.URL)
	if code != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", code)
	}
	if out.MaxTickets != 5 || out.MaxTicketsFromEnv {
		t.Fatalf("settings = %+v, want max_tickets 5 with no env override", out)
	}

	t.Setenv("EL_MAX_TICKETS", "2")
	out, _ = getSettingsView(t, srv.URL)
	if out.MaxTickets != 2 || !out.MaxTicketsFromEnv {
		t.Fatalf("settings under env = %+v, want max_tickets 2 from env", out)
	}
}

// An old file's max_workers still reads as the ticket cap.
func TestSettingsReadFallsBackToOldMaxWorkers(t *testing.T) {
	srv, _ := settingsServer(t, `{"max_workers": 6, "roles": {}}`)

	out, code := getSettingsView(t, srv.URL)
	if code != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", code)
	}
	if out.MaxTickets != 6 || out.MaxTicketsFromEnv {
		t.Fatalf("settings = %+v, want max_tickets 6 from the old key", out)
	}
}

// The settings write validates and saves max_tickets without losing roles,
// and never writes the old key again.
func TestSettingsWriteValidatesAndSavesMaxTickets(t *testing.T) {
	srv, settings := settingsServer(t, `{"max_tickets": 2, "roles": {"worker": {"harness": "pig", "model": "auto"}}, "tools": {"bash": {}}}`)

	out, code := putSettingsBody(t, srv.URL, `{"max_tickets": 4}`)
	if code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	if out.MaxTickets != 4 {
		t.Fatalf("write max_tickets = %d, want 4", out.MaxTickets)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "max_workers") {
		t.Fatalf("the old key must never be written again: %s", raw)
	}
	// The file keeps roles and unknown fields next to the new cap.
	var kept struct {
		Roles map[string]roleSetting `json:"roles"`
		Tools map[string]any         `json:"tools"`
	}
	if err := json.Unmarshal(raw, &kept); err != nil {
		t.Fatal(err)
	}
	if kept.Roles["worker"].Model != "auto" {
		t.Fatalf("roles after max_tickets-only write = %+v, want the worker role kept", kept.Roles)
	}
	if _, ok := kept.Tools["bash"]; !ok {
		t.Fatalf("tools after write missing: %s", raw)
	}

	// Roles-only writes keep the cap.
	out, code = putSettingsBody(t, srv.URL, `{"roles": {"scout": {"harness": "pig", "model": "auto"}}}`)
	if code != http.StatusOK {
		t.Fatalf("roles-only write status = %d, want 200", code)
	}
	if out.Roles["worker"].Model != "auto" || out.Roles["scout"].Model != "auto" {
		t.Fatalf("roles after merge = %+v, want worker kept and scout added", out.Roles)
	}
	if out.MaxTickets != 4 {
		t.Fatalf("max_tickets after roles-only write = %d, want 4", out.MaxTickets)
	}

	for _, bad := range []string{`{"max_tickets": 0}`, `{"max_tickets": -2}`, `{"max_tickets": "soon"}`} {
		_, code := putSettingsBody(t, srv.URL, bad)
		if code != http.StatusBadRequest {
			t.Fatalf("write %s status = %d, want 400", bad, code)
		}
	}
	raw, _ = os.ReadFile(settings)
	var still struct {
		MaxTickets int `json:"max_tickets"`
	}
	_ = json.Unmarshal(raw, &still)
	if still.MaxTickets != 4 {
		t.Fatalf("max_tickets after rejected writes = %d, want 4 kept", still.MaxTickets)
	}
}

// The deprecated max_workers key still writes, saved under max_tickets.
func TestSettingsWriteAcceptsOldMaxWorkersAlias(t *testing.T) {
	srv, settings := settingsServer(t, `{"roles": {}}`)

	out, code := putSettingsBody(t, srv.URL, `{"max_workers": 3}`)
	if code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	if out.MaxTickets != 3 {
		t.Fatalf("alias write max_tickets = %d, want 3", out.MaxTickets)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "max_workers") {
		t.Fatalf("the old key must never be written again: %s", raw)
	}
}

// Sessions are uncapped: writes to max_tickets change the Run ticket cap,
// never whether a Session starts.
func TestSettingsWriteDoesNotCapSessions(t *testing.T) {
	srv, _ := settingsServer(t, `{"max_tickets": 1, "roles": {}}`)
	workDir := tempDir(t)

	_ = createSession(t, srv.URL, "fake", "fake-model", workDir)
	body, _ := json.Marshal(map[string]string{"harness": "fake", "model": "fake-model", "workDir": workDir})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second create status = %d, want 201: sessions are uncapped", resp.StatusCode)
	}

	if _, code := putSettingsBody(t, srv.URL, `{"max_tickets": 1}`); code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	resp, err = http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("third create status = %d, want 201: sessions are uncapped", resp.StatusCode)
	}
}

// The environment variable still wins: the read says so, and a write saves
// the file without moving the live cap.
func TestSettingsEnvOverrideWinsOverWrite(t *testing.T) {
	srv, settings := settingsServer(t, `{"max_tickets": 5, "roles": {}}`)
	t.Setenv("EL_MAX_TICKETS", "1")

	out, code := putSettingsBody(t, srv.URL, `{"max_tickets": 3}`)
	if code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	if out.MaxTickets != 1 || !out.MaxTicketsFromEnv {
		t.Fatalf("write under env = %+v, want the live cap 1 from env", out)
	}
	raw, _ := os.ReadFile(settings)
	var saved struct {
		MaxTickets int `json:"max_tickets"`
	}
	_ = json.Unmarshal(raw, &saved)
	if saved.MaxTickets != 3 {
		t.Fatalf("saved max_tickets = %d, want 3 kept for when the env is unset", saved.MaxTickets)
	}

	d := NewDaemon(tempDir(t), NewFakeBackend())
	if got := d.MaxTickets(); got != 1 {
		t.Fatalf("live cap under env = %d, want 1", got)
	}
}

// The title role is optional: an empty role clears it, and session_titles
// round-trips next to the roles; the view reports the env lock.
func TestSettingsTitleRoleAndSwitch(t *testing.T) {
	srv, file := settingsServer(t, `{"roles":{"worker":{"harness":"pig","model":"x"}}}`)
	t.Setenv("EL_SESSION_TITLES", "")

	out, code := putSettingsBody(t, srv.URL, `{"roles":{"pig-title":{"harness":"pig","model":"x"}},"session_titles":false}`)
	if code != http.StatusOK || out.Roles["pig-title"].Model != "x" || out.Roles["worker"].Model != "x" {
		t.Fatalf("put title = %+v (%d)", out, code)
	}
	raw, _ := os.ReadFile(file)
	if !strings.Contains(string(raw), `"session_titles": false`) {
		t.Fatalf("settings file lacks session_titles: %s", raw)
	}
	var view struct {
		SessionTitles        bool `json:"session_titles"`
		SessionTitlesFromEnv bool `json:"session_titles_from_env"`
	}
	resp, _ := http.Get(srv.URL + "/api/settings")
	_ = json.NewDecoder(resp.Body).Decode(&view)
	resp.Body.Close()
	if view.SessionTitles || view.SessionTitlesFromEnv {
		t.Fatalf("view = %+v, want off and not env-locked", view)
	}

	out, code = putSettingsBody(t, srv.URL, `{"roles":{"pig-title":{"harness":"","model":""}}}`)
	if code != http.StatusOK {
		t.Fatalf("clear status = %d", code)
	}
	if _, ok := out.Roles["pig-title"]; ok || out.Roles["worker"].Model != "x" {
		t.Fatalf("clear left roles = %+v", out.Roles)
	}
	if _, code := putSettingsBody(t, srv.URL, `{"roles":{"pig-title":{"harness":"pig","model":""}}}`); code != http.StatusBadRequest {
		t.Fatalf("half-empty role status = %d, want 400", code)
	}
	if _, code := putSettingsBody(t, srv.URL, `{"roles":{"pig-title":{"harness":"claude","model":"haiku"}}}`); code != http.StatusBadRequest {
		t.Fatalf("claude pig-title status = %d, want 400", code)
	}

	t.Setenv("EL_SESSION_TITLES", "0")
	resp, _ = http.Get(srv.URL + "/api/settings")
	_ = json.NewDecoder(resp.Body).Decode(&view)
	resp.Body.Close()
	if !view.SessionTitlesFromEnv {
		t.Fatal("env override not reported")
	}
}

func TestSettingsOAuthRedirectBaseDefaultsEmptyAndPersists(t *testing.T) {
	srv, settings := settingsServer(t, `{"roles":{}}`)
	view, status := getSettingsView(t, srv.URL)
	if status != http.StatusOK || view.OAuthRedirectBaseURL != "" {
		t.Fatalf("default redirect base = %q (status %d), want empty", view.OAuthRedirectBaseURL, status)
	}

	view, status = putSettingsBody(t, srv.URL, `{"oauth_redirect_base_url":"https://ember.example.ts.net/oauth/"}`)
	if status != http.StatusOK || view.OAuthRedirectBaseURL != "https://ember.example.ts.net/oauth" {
		t.Fatalf("saved redirect base = %q (status %d), want normalized HTTPS base", view.OAuthRedirectBaseURL, status)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := json.Unmarshal(saved[oauthRedirectBaseURLSettingKey], &stored); err != nil || stored != view.OAuthRedirectBaseURL {
		t.Fatalf("settings file redirect base = %q, %v", stored, err)
	}

	for _, bad := range []string{
		`{"oauth_redirect_base_url":"http://example.com"}`,
		`{"oauth_redirect_base_url":"https://user:pass@example.com"}`,
		`{"oauth_redirect_base_url":"https://example.com/callback#code"}`,
		`{"oauth_redirect_base_url":"https://example.com/callback?code=secret"}`,
	} {
		if _, status := putSettingsBody(t, srv.URL, bad); status != http.StatusBadRequest {
			t.Errorf("invalid redirect base %s status = %d, want 400", bad, status)
		}
	}

	view, status = putSettingsBody(t, srv.URL, `{"oauth_redirect_base_url":""}`)
	if status != http.StatusOK || view.OAuthRedirectBaseURL != "" {
		t.Fatalf("cleared redirect base = %q (status %d), want empty", view.OAuthRedirectBaseURL, status)
	}
}

func TestSettingsOAuthRedirectBaseWriteIsAdminOnly(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	if err := os.WriteFile(settings, []byte(`{"roles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	_ = signedIn(t, h, "admin")
	member := signedIn(t, h, "member")

	rec := authDo(t, h, http.MethodPut, "/api/settings", `{"oauth_redirect_base_url":"https://ember.example.ts.net"}`, member)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin redirect base write = %d: %s, want 403", rec.Code, rec.Body.String())
	}
	if got := oauthRedirectBaseURLSetting(); got != "" {
		t.Fatalf("redirect base after non-admin write = %q, want empty", got)
	}
}

func TestBrowserOAuthRedirectBaseRequiresProviderProbe(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	if err := os.WriteFile(settings, []byte(`{"oauth_redirect_base_url":"https://ember.example.ts.net"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settings)
	probeCalls := 0
	probe := func(_ context.Context, providerID, base string) (bool, error) {
		probeCalls++
		if providerID != "openrouter" || base != "https://ember.example.ts.net" {
			t.Errorf("probe args = %q, %q", providerID, base)
		}
		return true, nil
	}
	if err := os.WriteFile(settings, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "openrouter", probe); got != "" || probeCalls != 0 {
		t.Fatalf("unset redirect base = %q after %d probe calls, want empty and no probe", got, probeCalls)
	}
	if err := os.WriteFile(settings, []byte(`{"oauth_redirect_base_url":"https://ember.example.ts.net"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "openrouter", probe); got != "https://ember.example.ts.net" {
		t.Fatalf("accepted redirect base = %q", got)
	}
	if probeCalls != 1 {
		t.Fatalf("accepted provider probe calls = %d, want 1", probeCalls)
	}

	rejected := func(context.Context, string, string) (bool, error) { return false, nil }
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "openrouter", rejected); got != "" {
		t.Fatalf("rejected provider redirect base = %q, want empty", got)
	}
	failed := func(context.Context, string, string) (bool, error) { return false, os.ErrPermission }
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "openrouter", failed); got != "" {
		t.Fatalf("failed provider probe redirect base = %q, want empty", got)
	}
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "amazon-bedrock", probe); got != "" {
		t.Fatalf("non-OAuth provider redirect base = %q, want empty", got)
	}
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "openai-codex", probe); got != "" {
		t.Fatalf("device-code provider redirect base = %q, want empty", got)
	}
	if got := d.oauthRedirectBaseForBrowserOAuth(context.Background(), "xai", probe); got != "" {
		t.Fatalf("device-code OAuth provider redirect base = %q, want empty", got)
	}
	if probeCalls != 1 {
		t.Fatalf("probe calls after rejected/ineligible providers = %d, want 1", probeCalls)
	}
}
