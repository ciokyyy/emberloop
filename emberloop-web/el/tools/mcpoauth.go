// OAuth for HTTP MCP servers: dynamic registration (RFC 7591), PKCE
// browser approval (RFC 7636), and refresh, against the authorization
// server the MCP server's 401 challenge names (RFC 8707 resource indicators
// plus RFC 9728 protected-resource metadata). DIDWW-style servers send no
// client id or secret: the client registers itself, the human approves in
// the browser, and the token sits per server under the user's home, so
// settings files never carry secrets.
//
// The flow runs at most once per failing call: a 401 names a challenge, the
// client registers, approves and stores, then retries the original call
// exactly once. A retry that still 401s, or a refresh the server rejects
// with invalid_grant, is a *ReauthNeededError: an actionable message naming
// the token file to remove, never a hang and never fatal to the session.
package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// mcpOAuthApproveTimeout bounds the human's browser approval: the callback
// wait ends here instead of hanging the session.
const mcpOAuthApproveTimeout = 10 * time.Minute

// MCPLoginTimeout bounds `el settings mcp-server <name> login`.
const MCPLoginTimeout = mcpOAuthApproveTimeout + time.Minute

// mcpOAuthBodyLimit bounds an OAuth endpoint's reply: metadata and tokens
// are small, and a compromised endpoint must not fill memory.
const mcpOAuthBodyLimit = 1 << 20

// ReauthNeededError is a dead OAuth credential: the stored token is expired
// or revoked and the server rejected the refresh, or the retried call still
// 401s after a fresh approval. It carries the token file to remove: the
// next call then runs the browser approval again from scratch.
type ReauthNeededError struct {
	Server    string
	TokenFile string
}

func (e *ReauthNeededError) Error() string {
	return fmt.Sprintf("mcp server %q: OAuth token expired or revoked; remove %s and retry to approve again", e.Server, e.TokenFile)
}

// NeedsLoginError is a server that wants a browser approval while nobody is
// there to give it. A listing reports it at once instead of stalling the
// start of every session; `el settings mcp-server <name> login` approves.
type NeedsLoginError struct{ Server string }

func (e *NeedsLoginError) Error() string {
	return fmt.Sprintf("mcp server %q: needs OAuth approval; run: el settings mcp-server %s login", e.Server, e.Server)
}

// oauthStored is one server's credential on disk.
type oauthStored struct {
	// TokenEndpoint and ClientID pin the credential to the server that
	// issued it: a settings edit pointing elsewhere re-registers instead of
	// sending the token to a stranger.
	TokenEndpoint string `json:"token_endpoint"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret,omitempty"`
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	// ExpiresAt is zero when the server gave no lifetime: the token is used
	// until the server 401s it.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// expired reports whether the access token is unusable, with a 30-second
// skew so a token dying mid-call still counts as alive.
func (t *oauthStored) expired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return !time.Now().Before(t.ExpiresAt.Add(-30 * time.Second))
}

// mcpTokenDir is where per-server OAuth credentials live. EL_MCP_TOKEN_DIR
// wins so tests never touch the real home.
func mcpTokenDir() string {
	if d := os.Getenv("EL_MCP_TOKEN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".emberloop", "mcp-tokens")
}

// mcpTokenPath names one server's credential file. The name is flattened so
// a settings key can never escape the directory.
func mcpTokenPath(server string) string {
	flat := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, server)
	if flat == "" || flat == "." || flat == ".." {
		flat = "server"
	}
	return filepath.Join(mcpTokenDir(), flat+".json")
}

// loadOAuthToken reads one server's credential, or nil when none is stored.
func loadOAuthToken(server string) (*oauthStored, error) {
	raw, err := os.ReadFile(mcpTokenPath(server))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var t oauthStored
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("mcp server %q: stored OAuth token is corrupt: %w", server, err)
	}
	if t.AccessToken == "" {
		return nil, nil
	}
	return &t, nil
}

// storeOAuthToken writes one server's credential with owner-only bits,
// creating the directory the same way.
func storeOAuthToken(server string, t *oauthStored) error {
	path := mcpTokenPath(server)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// oauthFlows runs the OAuth side of one HTTP MCP server: stored tokens,
// refresh, and the interactive approval. getCode approves authURL and
// returns its code; nil means the interactive default (open the browser and
// wait on the loopback callback). Tests inject a fake; the zero value never
// opens anything in tests because every test sets it.
type oauthFlows struct {
	server  string
	mcpURL  string
	setting *MCPOAuthSetting
	getCode func(ctx context.Context, authURL string) (string, error)
	// noBrowser refuses the browser approval: a startup listing has no human
	// to wait for, and a refresh with a stored token still works.
	noBrowser bool
}

// oauthMeta is the authorization server's discovery document: the three
// endpoints the flow needs.
type oauthMeta struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

// resourceMeta is the protected-resource document (RFC 9728) a 401
// challenge points at: it names the authorization servers to try.
type resourceMeta struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

// fetchJSON GETs endpoint and decodes its JSON body into out, bounded by
// ctx. Every error names what was fetched and why.
func fetchJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("bad OAuth endpoint %q: %w", endpoint, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %q: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, mcpOAuthBodyLimit))
	if err != nil {
		return fmt.Errorf("reading %q: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %q: status %s", endpoint, resp.Status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding %q: %w", endpoint, err)
	}
	return nil
}

// postForm POSTs values to endpoint and decodes its JSON body into out. A
// 400 naming invalid_grant is a typed refresh failure the caller turns into
// reauth-needed; any other non-200 is an error.
func postForm(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("bad OAuth endpoint %q: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting to %q: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, mcpOAuthBodyLimit))
	if err != nil {
		return fmt.Errorf("reading %q: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		var bodyErr struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &bodyErr)
		if resp.StatusCode == http.StatusBadRequest && (bodyErr.Error == "invalid_grant" || bodyErr.Error == "invalid_client") {
			return &oauthGrantError{endpoint: endpoint, grant: bodyErr.Error}
		}
		return fmt.Errorf("posting to %q: status %s", endpoint, resp.Status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding %q: %w", endpoint, err)
	}
	return nil
}

// oauthGrantError is a token endpoint's refusal of the grant itself: the
// stored credential is dead, not the network.
type oauthGrantError struct {
	endpoint string
	grant    string
}

func (e *oauthGrantError) Error() string {
	return fmt.Sprintf("token endpoint %q refused the grant (%s)", e.endpoint, e.grant)
}

// parseResourceMetadata pulls the resource_metadata URL out of a
// WWW-Authenticate challenge, or "" when the challenge names none. The
// leading auth scheme (Bearer) is not a parameter and is skipped.
func parseResourceMetadata(challenge string) string {
	challenge = strings.TrimSpace(challenge)
	if scheme, _, ok := strings.Cut(challenge, " "); ok && strings.TrimSpace(scheme) != "" && !strings.Contains(scheme, "=") {
		challenge = strings.TrimSpace(challenge[len(scheme):])
	}
	for _, part := range strings.Split(challenge, ",") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) != "resource_metadata" {
			continue
		}
		v = strings.TrimSpace(v)
		return strings.Trim(v, `"`)
	}
	return ""
}

// wellKnown appends path to issuer, keeping exactly one slash between them.
func wellKnown(issuer, path string) string {
	return strings.TrimRight(issuer, "/") + path
}

// discoverOAuthServer finds the authorization server for mcpURL: the
// settings override wins outright, else the 401 challenge's
// resource_metadata document names it, else the MCP host's well-known
// document is the fallback.
func discoverOAuthServer(ctx context.Context, server, mcpURL, challenge, override string) (*oauthMeta, error) {
	if override != "" {
		var meta oauthMeta
		if err := fetchJSON(ctx, override, &meta); err != nil {
			return nil, fmt.Errorf("mcp server %q: OAuth discovery at %q failed: %w", server, override, err)
		}
		if err := checkOAuthMeta(&meta); err != nil {
			return nil, fmt.Errorf("mcp server %q: OAuth discovery at %q: %w", server, override, err)
		}
		return &meta, nil
	}
	issuers := []string{}
	if rm := parseResourceMetadata(challenge); rm != "" {
		var res resourceMeta
		if err := fetchJSON(ctx, rm, &res); err != nil {
			return nil, fmt.Errorf("mcp server %q: OAuth resource metadata at %q failed: %w", server, rm, err)
		}
		issuers = append(issuers, res.AuthorizationServers...)
	}
	if u, err := url.Parse(mcpURL); err == nil && u.Scheme != "" && u.Host != "" {
		issuers = append(issuers, u.Scheme+"://"+u.Host)
	}
	var last error
	for _, issuer := range issuers {
		if issuer == "" {
			continue
		}
		doc := wellKnown(issuer, "/.well-known/oauth-authorization-server")
		var meta oauthMeta
		if err := fetchJSON(ctx, doc, &meta); err != nil {
			last = err
			continue
		}
		if err := checkOAuthMeta(&meta); err != nil {
			last = err
			continue
		}
		return &meta, nil
	}
	if last != nil {
		return nil, fmt.Errorf("mcp server %q: OAuth discovery failed: %w", server, last)
	}
	return nil, fmt.Errorf("mcp server %q: OAuth discovery failed: no authorization server found", server)
}

// checkOAuthMeta refuses a discovery document missing an endpoint the flow
// needs. Registration is optional: servers without it use the pre-registered
// client the settings imply, which for these servers means none, so a
// missing registration endpoint fails at registration time with its URL.
func checkOAuthMeta(meta *oauthMeta) error {
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return fmt.Errorf("discovery document names no authorization or token endpoint")
	}
	return nil
}

// storedBearer returns the stored access token when it is usable: present,
// unexpired, and issued by the same token endpoint discovery just found.
// Anything else is ("", nil): the caller runs refresh or approval.
func (o *oauthFlows) storedBearer(meta *oauthMeta) (string, error) {
	t, err := loadOAuthToken(o.server)
	if err != nil {
		return "", err
	}
	if t == nil || t.AccessToken == "" || t.expired() {
		return "", nil
	}
	if t.TokenEndpoint != "" && t.TokenEndpoint != meta.TokenEndpoint {
		return "", nil
	}
	return t.AccessToken, nil
}

// refreshBearer swaps a stored refresh token for a fresh access token and
// stores the result. A refused grant is a *ReauthNeededError: the credential
// is dead and only a fresh approval revives it.
func (o *oauthFlows) refreshBearer(ctx context.Context, meta *oauthMeta) (string, error) {
	t, err := loadOAuthToken(o.server)
	if err != nil {
		return "", err
	}
	if t == nil || t.RefreshToken == "" {
		return "", nil
	}
	if t.TokenEndpoint != "" && t.TokenEndpoint != meta.TokenEndpoint {
		return "", nil
	}
	clientID := t.ClientID
	if clientID == "" {
		clientID = oauthClientName(o.setting)
	}
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.RefreshToken},
		"client_id":     {clientID},
	}
	if t.ClientSecret != "" {
		values.Set("client_secret", t.ClientSecret)
	}
	var tok oauthTokenReply
	if err := postForm(ctx, meta.TokenEndpoint, values, &tok); err != nil {
		var grant *oauthGrantError
		if ok := errorAsGrant(err, &grant); ok {
			return "", &ReauthNeededError{Server: o.server, TokenFile: mcpTokenPath(o.server)}
		}
		return "", fmt.Errorf("mcp server %q: OAuth refresh failed: %w", o.server, err)
	}
	next := &oauthStored{
		TokenEndpoint: meta.TokenEndpoint,
		ClientID:      clientID,
		ClientSecret:  t.ClientSecret,
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		ExpiresAt:     tok.expiresAt(),
	}
	if next.RefreshToken == "" {
		next.RefreshToken = t.RefreshToken
	}
	if err := storeOAuthToken(o.server, next); err != nil {
		return "", fmt.Errorf("mcp server %q: storing OAuth token failed: %w", o.server, err)
	}
	return next.AccessToken, nil
}

// oauthTokenReply is a token endpoint's success body.
type oauthTokenReply struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

func (r *oauthTokenReply) expiresAt() time.Time {
	if r.ExpiresIn <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
}

// errorAsGrant is errors.As over the grant error without importing errors
// at every call site.
func errorAsGrant(err error, target **oauthGrantError) bool {
	for err != nil {
		if g, ok := err.(*oauthGrantError); ok {
			*target = g
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// newPKCE mints a verifier and its S256 challenge.
func newPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// newOAuthState mints the opaque state the callback must echo.
func newOAuthState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// oauthClientName is the name dynamic registration offers: the settings
// name, else "emberloop".
func oauthClientName(setting *MCPOAuthSetting) string {
	if setting != nil && setting.ClientName != "" {
		return setting.ClientName
	}
	return "emberloop"
}

// registerOAuthClient registers this client with the authorization server
// (RFC 7591) and returns its id and secret. DIDWW-style servers answer with
// an id and no secret: a public loopback client.
func registerOAuthClient(ctx context.Context, server string, meta *oauthMeta, name, redirectURI string, scopes []string) (id, secret string, err error) {
	if meta.RegistrationEndpoint == "" {
		return "", "", fmt.Errorf("mcp server %q: authorization server offers no registration endpoint", server)
	}
	body, err := json.Marshal(map[string]any{
		"client_name":                name,
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      strings.Join(scopes, " "),
	})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.RegistrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("mcp server %q: bad registration endpoint %q: %w", server, meta.RegistrationEndpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("mcp server %q: OAuth registration failed: %w", server, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, mcpOAuthBodyLimit))
	if err != nil {
		return "", "", fmt.Errorf("mcp server %q: reading registration reply: %w", server, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("mcp server %q: OAuth registration failed: status %s", server, resp.Status)
	}
	var reg struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(raw, &reg); err != nil || reg.ClientID == "" {
		return "", "", fmt.Errorf("mcp server %q: OAuth registration answered without a client id", server)
	}
	return reg.ClientID, reg.ClientSecret, nil
}

// authURL builds the approval URL the human (or the test hook) opens.
func authURL(meta *oauthMeta, clientID, redirectURI, challenge, state string, scopes []string, resource string) string {
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	if len(scopes) > 0 {
		v.Set("scope", strings.Join(scopes, " "))
	}
	if resource != "" {
		v.Set("resource", resource)
	}
	return meta.AuthorizationEndpoint + "?" + v.Encode()
}

// waitOAuthCode serves the loopback callback and waits for the approval's
// code. The listener is already open (its port fixed the redirect URI); this
// serves exactly one callback, checks the state, and answers with a page the
// human can close.
func waitOAuthCode(ctx context.Context, server string, ln net.Listener, state string) (string, error) {
	type outcome struct {
		code string
		err  error
	}
	done := make(chan outcome, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "unknown state; restart the approval", http.StatusBadRequest)
			return
		}
		if code := q.Get("code"); code != "" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintln(w, "<html><body><h1>Approved.</h1><p>You can close this tab and return to el.</p></body></html>")
			done <- outcome{code: code}
			return
		}
		if remote := q.Get("error"); remote != "" {
			http.Error(w, "authorization failed: "+remote, http.StatusBadRequest)
			done <- outcome{err: fmt.Errorf("mcp server %q: authorization failed: %s", server, remote)}
			return
		}
		http.Error(w, "no code; restart the approval", http.StatusBadRequest)
	})}
	defer srv.Close()
	go func() {
		_ = srv.Serve(ln)
	}()
	timeout, cancel := context.WithTimeout(ctx, mcpOAuthApproveTimeout)
	defer cancel()
	select {
	case out := <-done:
		return out.code, out.err
	case <-timeout.Done():
		if ctx.Err() != nil {
			return "", fmt.Errorf("mcp server %q: approval cancelled: %w", server, ctx.Err())
		}
		return "", fmt.Errorf("mcp server %q: approval timed out after %s; retry to approve again", server, mcpOAuthApproveTimeout)
	}
}

// openBrowser asks the desktop to open page without waiting for it. A
// missing opener is an error carrying the URL, so a headless user can open
// it by hand instead of hanging.
func openBrowser(page string) error {
	var name string
	var args []string
	switch {
	case lookPathOK("xdg-open"):
		name, args = "xdg-open", []string{page}
	case lookPathOK("open"):
		name, args = "open", []string{page}
	case lookPathOK("rundll32"):
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", page}
	default:
		return fmt.Errorf("no browser opener found; open this URL to approve:\n%s", page)
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening the browser failed; open this URL to approve:\n%s\n(%v)", page, err)
	}
	_ = cmd.Process.Release()
	return nil
}

// lookPathOK reports whether name resolves on PATH.
func lookPathOK(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// browserFlow runs the full interactive approval: listen on loopback,
// register, open the approval URL, wait for the code, exchange it, and store
// the token. getCode, when set, replaces the open-and-wait step (tests).
func (o *oauthFlows) browserFlow(ctx context.Context, meta *oauthMeta) (string, error) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		return "", fmt.Errorf("mcp server %q: OAuth setup failed: %w", o.server, err)
	}
	state, err := newOAuthState()
	if err != nil {
		return "", fmt.Errorf("mcp server %q: OAuth setup failed: %w", o.server, err)
	}
	port := 0
	if o.setting != nil && o.setting.CallbackPort != 0 {
		port = o.setting.CallbackPort
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", fmt.Errorf("mcp server %q: OAuth callback listen failed: %w", o.server, err)
	}
	defer ln.Close()
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	name := oauthClientName(o.setting)
	var scopes []string
	if o.setting != nil {
		scopes = o.setting.Scopes
	}
	clientID, secret, err := registerOAuthClient(ctx, o.server, meta, name, redirectURI, scopes)
	if err != nil {
		return "", err
	}
	approvalURL := authURL(meta, clientID, redirectURI, challenge, state, scopes, o.mcpURL)
	get := o.getCode
	if get == nil {
		get = func(ctx context.Context, authURL string) (string, error) {
			// No browser here (a headless host) is not fatal: the URL goes to
			// stderr and the listener keeps waiting, so the callback can be
			// fetched from another shell on this host.
			if err := openBrowser(authURL); err != nil {
				fmt.Fprintf(os.Stderr, "mcp server %q: %v\n", o.server, err)
			}
			return waitOAuthCode(ctx, o.server, ln, state)
		}
	}
	code, err := get(ctx, approvalURL)
	if err != nil {
		return "", err
	}
	values := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	if secret != "" {
		values.Set("client_secret", secret)
	}
	var tok oauthTokenReply
	if err := postForm(ctx, meta.TokenEndpoint, values, &tok); err != nil {
		return "", fmt.Errorf("mcp server %q: OAuth token exchange failed: %w", o.server, err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("mcp server %q: OAuth token exchange answered without a token", o.server)
	}
	stored := &oauthStored{
		TokenEndpoint: meta.TokenEndpoint,
		ClientID:      clientID,
		ClientSecret:  secret,
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		ExpiresAt:     tok.expiresAt(),
	}
	if err := storeOAuthToken(o.server, stored); err != nil {
		return "", fmt.Errorf("mcp server %q: storing OAuth token failed: %w", o.server, err)
	}
	return stored.AccessToken, nil
}

// bearerFor401 returns the bearer to retry a challenged call with: refresh
// when a refresh token exists, else the full browser approval. A refused
// refresh is reauth-needed, never a browser loop.
func (o *oauthFlows) bearerFor401(ctx context.Context, challenge string) (string, error) {
	override := ""
	if o.setting != nil {
		override = o.setting.MetadataURL
	}
	meta, err := discoverOAuthServer(ctx, o.server, o.mcpURL, challenge, override)
	if err != nil {
		return "", err
	}
	t, err := loadOAuthToken(o.server)
	if err != nil {
		return "", err
	}
	if t != nil && t.RefreshToken != "" && (t.TokenEndpoint == "" || t.TokenEndpoint == meta.TokenEndpoint) {
		return o.refreshBearer(ctx, meta)
	}
	if t != nil && (t.TokenEndpoint != "" && t.TokenEndpoint != meta.TokenEndpoint) {
		// The settings moved under a live token: re-register from scratch.
		_ = os.Remove(mcpTokenPath(o.server))
	}
	if o.noBrowser {
		return "", &NeedsLoginError{Server: o.server}
	}
	return o.browserFlow(ctx, meta)
}
