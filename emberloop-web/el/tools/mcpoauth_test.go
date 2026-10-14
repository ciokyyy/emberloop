// OAuth tests: the 401 flow (register, approve, store, retry once) and the
// reauth-needed error, against the httptest fake. The approval hook plays
// the human by approving through the fake authorize endpoint, so no browser
// ever opens. Token files live under EL_MCP_TOKEN_DIR in a temp dir, and
// HOME points nowhere, so no test touches the real home.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"example.com/emberloop/tools/mcpfake"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// oauthEnv isolates the token store and keeps HOME out of reach.
func oauthEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("EL_MCP_TOKEN_DIR", filepath.Join(dir, "tokens"))
	t.Setenv("HOME", filepath.Join(dir, "nohome"))
	return dir
}

// approveViaFake plays the human: it GETs the approval URL without
// following redirects and returns the code the fake issued.
func approveViaFake(t *testing.T) func(context.Context, string) (string, error) {
	t.Helper()
	return func(ctx context.Context, authURL string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
		if err != nil {
			return "", err
		}
		client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusFound {
			return "", fmt.Errorf("authorize answered %s, want a redirect with a code", resp.Status)
		}
		u, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			return "", err
		}
		if code := u.Query().Get("code"); code != "" {
			return code, nil
		}
		return "", fmt.Errorf("authorize redirect carries no code: %q", resp.Header.Get("Location"))
	}
}

// oauthRegistry builds an HTTP registry with the hook installed and short
// waits, and closes it when the test ends.
func oauthRegistry(t *testing.T, name string, cfg MCPServerSetting, hook func(context.Context, string) (string, error)) (*Registry, *mcpHTTPClient) {
	t.Helper()
	reg, h := httpRegistry(t, name, cfg)
	reg.SetTimeouts(10*time.Second, 10*time.Second)
	h.setOAuthHook(hook)
	return reg, h
}

func readStoredToken(t *testing.T, server string) *oauthStored {
	t.Helper()
	raw, err := os.ReadFile(mcpTokenPath(server))
	if err != nil {
		t.Fatalf("reading stored token: %v", err)
	}
	var stored oauthStored
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decoding stored token: %v", err)
	}
	return &stored
}

// The first call to a challenging server approves through the browser flow
// and retries once: register, S256 authorize, PKCE exchange, store, then the
// call succeeds. The token sits 0600 on disk, and the next call reuses it
// with no second approval.
func TestMCPOAuthFirstCallApprovesAndRetries(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
	})
	reg, h := oauthRegistry(t, "didww", MCPServerSetting{
		URL:   fake.MCPURL,
		OAuth: &MCPOAuthSetting{ClientName: "el-test", Scopes: []string{"read"}},
	}, approveViaFake(t))
	if err := reg.Login(context.Background(), "didww"); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Tools(context.Background(), "didww")
	if err != nil {
		t.Fatal(err)
	}
	if names := httpToolNames(got); len(names) != 1 || names[0] != "search" {
		t.Fatalf("tools = %v, want [search]", names)
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "search({})" {
		t.Fatalf("call text = %q", text)
	}
	// Registration offered the configured client name with a public
	// loopback profile.
	regs := fake.Registrations()
	if len(regs) != 1 {
		t.Fatalf("registrations = %d, want one", len(regs))
	}
	if regs[0]["client_name"] != "el-test" {
		t.Errorf("registration client_name = %v, want el-test", regs[0]["client_name"])
	}
	if regs[0]["token_endpoint_auth_method"] != "none" {
		t.Errorf("registration auth method = %v, want none (public client)", regs[0]["token_endpoint_auth_method"])
	}
	// The approval asked for S256 PKCE over the MCP resource.
	auths := fake.Authorizes()
	if len(auths) != 1 {
		t.Fatalf("authorizes = %d, want one", len(auths))
	}
	if auths[0].Get("code_challenge_method") != "S256" || auths[0].Get("code_challenge") == "" {
		t.Errorf("authorize missed S256 PKCE: %v", auths[0])
	}
	if auths[0].Get("resource") != fake.MCPURL {
		t.Errorf("authorize resource = %q, want the MCP URL", auths[0].Get("resource"))
	}
	if auths[0].Get("scope") != "read" {
		t.Errorf("authorize scope = %q, want the configured scopes", auths[0].Get("scope"))
	}
	// The token is stored owner-only with a refresh token.
	stored := readStoredToken(t, "didww")
	if stored.AccessToken == "" || stored.RefreshToken == "" {
		t.Fatalf("stored token misses access or refresh: %+v", stored)
	}
	if stored.TokenEndpoint != fake.Server.URL+"/token" {
		t.Errorf("stored token endpoint = %q", stored.TokenEndpoint)
	}
	fi, err := os.Stat(mcpTokenPath("didww"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("token file perms = %o, want 600", fi.Mode().Perm())
	}
	// A second call reuses the stored token: no second approval happens.
	h.setOAuthHook(func(context.Context, string) (string, error) {
		return "", fmt.Errorf("must not approve twice")
	})
	text, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatalf("second call with stored token: %v", err)
	}
	if text != "search({})" {
		t.Fatalf("second call text = %q", text)
	}
	if len(fake.Registrations()) != 1 || len(fake.Authorizes()) != 1 {
		t.Fatalf("second call re-ran the flow: %d registrations, %d authorizes",
			len(fake.Registrations()), len(fake.Authorizes()))
	}
}

// An expired access token refreshes silently: no approval, one token call,
// and the new access is stored.
func TestMCPOAuthRefreshesExpiredToken(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
	})
	reg, h := oauthRegistry(t, "didww", MCPServerSetting{
		URL: fake.MCPURL, OAuth: &MCPOAuthSetting{},
	}, approveViaFake(t))
	if err := reg.Login(context.Background(), "didww"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Tools(context.Background(), "didww"); err != nil {
		t.Fatal(err)
	}
	first := readStoredToken(t, "didww")
	// Age the access token past its life; the refresh token stays valid.
	first.ExpiresAt = time.Now().Add(-time.Hour)
	raw, _ := json.Marshal(first)
	if err := os.WriteFile(mcpTokenPath("didww"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	h.setOAuthHook(func(context.Context, string) (string, error) {
		return "", fmt.Errorf("an expired token must refresh, not approve")
	})
	text, err := reg.Call(context.Background(), "didww", "search", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call with expired token: %v", err)
	}
	if text != "search({})" {
		t.Fatalf("call text = %q", text)
	}
	second := readStoredToken(t, "didww")
	if second.AccessToken == first.AccessToken {
		t.Fatal("refresh did not rotate the access token")
	}
	if len(fake.Authorizes()) != 1 {
		t.Fatalf("refresh must not approve: %d authorizes", len(fake.Authorizes()))
	}
}

// A revoked token is reauth-needed: the refresh fails with invalid_grant,
// the error names the server and the token file to remove, and nothing
// hangs or opens a browser.
func TestMCPOAuthRevokedYieldsReauthNeeded(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
	})
	reg, h := oauthRegistry(t, "didww", MCPServerSetting{
		URL: fake.MCPURL, OAuth: &MCPOAuthSetting{},
	}, approveViaFake(t))
	if err := reg.Login(context.Background(), "didww"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Tools(context.Background(), "didww"); err != nil {
		t.Fatal(err)
	}
	fake.RevokeAll()
	h.setOAuthHook(func(context.Context, string) (string, error) {
		return "", fmt.Errorf("a revoked token must not approve")
	})
	_, err := reg.Call(context.Background(), "didww", "search", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("a revoked token must fail")
	}
	var reauth *ReauthNeededError
	if !asReauthNeeded(err, &reauth) {
		t.Fatalf("error = %v, want reauth-needed", err)
	}
	if !strings.Contains(err.Error(), `"didww"`) || !strings.Contains(err.Error(), mcpTokenPath("didww")) {
		t.Errorf("reauth error must name the server and the token file: %v", err)
	}
}

// A retry that still 401s after a fresh approval is reauth-needed, not a
// second approval loop.
func TestMCPOAuthRetry401YieldsReauthNeeded(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
		IssueAccess:        "bogus-never-accepted",
	})
	reg, _ := oauthRegistry(t, "didww", MCPServerSetting{
		URL: fake.MCPURL, OAuth: &MCPOAuthSetting{},
	}, approveViaFake(t))
	err := reg.Login(context.Background(), "didww")
	if err == nil {
		t.Fatal("a retry that still 401s must fail")
	}
	var reauth *ReauthNeededError
	if !asReauthNeeded(err, &reauth) {
		t.Fatalf("error = %v, want reauth-needed", err)
	}
	if len(fake.Authorizes()) != 1 {
		t.Fatalf("the flow must run once, not loop: %d authorizes", len(fake.Authorizes()))
	}
}

// Without oauth configured a 401 stays a plain error telling the user to
// configure headers or oauth: no flow, no reauth type.
func TestMCPOAuthNoConfig401StaysPlain(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	reg.SetTimeouts(10*time.Second, 10*time.Second)
	_, err := reg.Tools(context.Background(), "docs")
	if err == nil {
		t.Fatal("a 401 without oauth must fail")
	}
	var reauth *ReauthNeededError
	if asReauthNeeded(err, &reauth) {
		t.Fatalf("a plain 401 must not be reauth-needed: %v", err)
	}
	for _, want := range []string{`mcp server "docs"`, "401", "oauth"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must contain %q", err, want)
		}
	}
}

// The metadata override skips the challenge: with no WWW-Authenticate at
// all, discovery through the override still approves and calls.
func TestMCPOAuthMetadataOverride(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
		HideResourceMeta:   true,
	})
	reg, _ := oauthRegistry(t, "didww", MCPServerSetting{
		URL:   fake.MCPURL,
		OAuth: &MCPOAuthSetting{MetadataURL: fake.Server.URL + "/.well-known/oauth-authorization-server"},
	}, approveViaFake(t))
	if err := reg.Login(context.Background(), "didww"); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Tools(context.Background(), "didww")
	if err != nil {
		t.Fatalf("override discovery: %v", err)
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "search({})" {
		t.Fatalf("call text = %q", text)
	}
}

// With no challenge and no override, discovery fails with an error naming
// the server instead of hanging.
func TestMCPOAuthDiscoveryFailureNamesTheServer(t *testing.T) {
	oauthEnv(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":"unauthorized"}`)
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	reg, _ := oauthRegistry(t, "didww", MCPServerSetting{
		URL: srv.URL, OAuth: &MCPOAuthSetting{},
	}, approveViaFake(t))
	_, err := reg.Tools(context.Background(), "didww")
	if err == nil {
		t.Fatal("undiscoverable OAuth must fail")
	}
	for _, want := range []string{`mcp server "didww"`, "discovery"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must contain %q", err, want)
		}
	}
}

// waitOAuthCode serves one loopback callback: the approved code comes back,
// a wrong state is refused without ending the wait.
func TestWaitOAuthCodeApprovesOneCallback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type outcome struct {
		code string
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		code, err := waitOAuthCode(context.Background(), "didww", ln, "state-1")
		done <- outcome{code, err}
	}()
	base := "http://" + ln.Addr().String() + "/callback"
	get := func(query string) (int, string) {
		resp, err := http.Get(base + query)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if status, _ := get("?code=nope&state=wrong"); status != http.StatusBadRequest {
		t.Fatalf("wrong state answered %d, want 400", status)
	}
	if status, body := get("?code=abc123&state=state-1"); status != http.StatusOK || !strings.Contains(body, "Approved") {
		t.Fatalf("approval answered %d %q, want 200 with an approval page", status, body)
	}
	select {
	case out := <-done:
		if out.err != nil {
			t.Fatal(out.err)
		}
		if out.code != "abc123" {
			t.Fatalf("code = %q, want abc123", out.code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("callback never delivered the code")
	}
}

// The settings schema carries url, headers and oauth: project overrides
// global per name, sources follow, the merge copies, and the new keys are
// known (never extras).
func TestMCPSettingsHTTPSchemaMerges(t *testing.T) {
	global := decodeSettings([]byte(`{"mcp_servers":{
		"docs":{"url":"https://global/mcp","headers":{"A":"1"},"oauth":{"client_name":"el"},"placement":"execute"},
		"chat":{"command":"npx"}}}`))
	project := decodeSettings([]byte(`{"mcp_servers":{
		"docs":{"headers":{"B":"2","A":""},"oauth":{"callback_port":8377},"summary":"Docs"}}}`))
	out := MergeSettings(global, project)
	e, ok := out.MCPServers["docs"]
	if !ok {
		t.Fatal("docs server lost in merge")
	}
	if e.URL != "https://global/mcp" {
		t.Errorf("url = %q, want the global value kept", e.URL)
	}
	if len(e.Headers) != 1 || e.Headers["B"] != "2" {
		t.Errorf("headers = %v, want the project merge with A deleted", e.Headers)
	}
	if e.OAuth == nil || e.OAuth.ClientName != "el" || e.OAuth.CallbackPort != 8377 {
		t.Errorf("oauth = %+v, want the field-wise merge", e.OAuth)
	}
	if e.Placement != "execute" || e.Summary != "Docs" {
		t.Errorf("placement/summary = %q/%q", e.Placement, e.Summary)
	}
	for key, want := range map[string]string{
		"mcp_servers.docs.url":     "file",
		"mcp_servers.docs.headers": "project",
		"mcp_servers.docs.oauth":   "project",
		"mcp_servers.docs.summary": "project",
	} {
		if got := out.SourceOf(key); got != want {
			t.Errorf("source of %s = %q, want %q", key, got, want)
		}
	}
	// No oauth block stays no block; a global block survives a project
	// without one.
	if out.MCPServers["chat"].OAuth != nil {
		t.Errorf("chat gained an oauth block: %+v", out.MCPServers["chat"].OAuth)
	}
	if _, ok := global.mcpExtra["docs"]["url"]; ok {
		t.Error("url must decode as a known field, not an extra")
	}
	if _, ok := global.mcpExtra["docs"]["oauth"]; ok {
		t.Error("oauth must decode as a known field, not an extra")
	}
	// The merge copies: mutating the result leaves both inputs alone.
	e.Headers["C"] = "3"
	e.OAuth.Scopes = append(e.OAuth.Scopes, "mutated")
	out.MCPServers["docs"] = e
	if _, ok := global.MCPServers["docs"].Headers["C"]; ok {
		t.Error("merge shared the headers map with the global")
	}
	if len(project.MCPServers["docs"].OAuth.Scopes) != 0 {
		t.Error("merge shared the oauth scopes with the project")
	}
	if len(global.MCPServers["docs"].OAuth.Scopes) != 0 {
		t.Error("merge shared the oauth scopes with the global")
	}
}

// asReauthNeeded reports whether err chains to a reauth-needed error.
func asReauthNeeded(err error, target **ReauthNeededError) bool {
	return errors.As(err, target)
}

// A listing has no human to wait for: with no stored token it fails at once
// with the login hint and never opens the approval flow.
func TestMCPOAuthListingNeverApproves(t *testing.T) {
	oauthEnv(t)
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:              []mcpfake.ToolDef{{Name: "search"}},
		RequireIssuedToken: true,
	})
	reg, _ := oauthRegistry(t, "didww", MCPServerSetting{
		URL: fake.MCPURL, OAuth: &MCPOAuthSetting{},
	}, func(context.Context, string) (string, error) {
		return "", fmt.Errorf("a listing must not open the browser")
	})
	_, err := reg.Tools(context.Background(), "didww")
	if err == nil || !strings.Contains(err.Error(), "el settings mcp-server didww login") {
		t.Fatalf("error = %v, want the login hint", err)
	}
	if len(fake.Authorizes()) != 0 {
		t.Fatalf("authorizes = %d, want none", len(fake.Authorizes()))
	}
}
