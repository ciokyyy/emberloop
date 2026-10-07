package antigravity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// The client id and secret come from client.go, not from source.
const (
	redirectURI  = "http://localhost:51121/oauth-callback"
	authURL      = "https://accounts.google.com/o/oauth2/v2/auth"

	freeTier       = "free-tier"
	onboardTimeout = 30 * time.Second
	expirySkew     = 5 * time.Minute
)

var scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// Endpoints are variables so tests can point them at a local server.
var (
	tokenURL      = "https://oauth2.googleapis.com/token"
	cloudCodeBase = "https://daily-cloudcode-pa.googleapis.com"
	httpClient    = &http.Client{Timeout: 0}
)

// harnessUserAgent is the agy CLI identity every Antigravity request carries.
func harnessUserAgent() string {
	arch := runtime.GOARCH
	return fmt.Sprintf("antigravity/cli/%s (aidev_client; os_type=%s; arch=%s; cl=%s; auth_method=consumer)",
		agyCLIVersion, runtime.GOOS, arch, agyCLIChangeList)
}

type pkce struct{ verifier, challenge string }

func newPKCE() pkce {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	v := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(v))
	return pkce{v, base64.RawURLEncoding.EncodeToString(sum[:])}
}

// authorizeURL builds the consent URL. The PKCE verifier travels in state, as
// in the pi extension, so the pasted callback is enough to finish the login.
func authorizeURL(c oauthClient, p pkce) (string, string) {
	st, _ := json.Marshal(map[string]string{"verifier": p.verifier, "projectId": ""})
	state := base64.RawURLEncoding.EncodeToString(st)
	q := url.Values{}
	q.Set("client_id", c.ID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("code_challenge", p.challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	return authURL + "?" + q.Encode(), state
}

func verifierFromState(state string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(state, "="))
	if err != nil {
		return "", fmt.Errorf("bad oauth state: %w", err)
	}
	var s struct {
		Verifier string `json:"verifier"`
	}
	if err := json.Unmarshal(raw, &s); err != nil || s.Verifier == "" {
		return "", errors.New("missing PKCE verifier in state")
	}
	return s.Verifier, nil
}

// parseCallback accepts the whole callback URL or a bare code.
func parseCallback(input, defaultState string) (code, state string) {
	code, state = strings.TrimSpace(input), defaultState
	if u, err := url.Parse(code); err == nil && u.Scheme != "" {
		if c := u.Query().Get("code"); c != "" {
			code = c
		}
		if s := u.Query().Get("state"); s != "" {
			state = s
		}
	}
	return code, state
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func postToken(ctx context.Context, form url.Values) (tokenResponse, error) {
	var tr tokenResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tr, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return tr, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return tr, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return tr, err
	}
	if tr.AccessToken == "" {
		return tr, errors.New("token response carries no access token")
	}
	return tr, nil
}

func expiry(start time.Time, expiresIn int64) int64 {
	return start.Add(time.Duration(expiresIn) * time.Second).Add(-expirySkew).UnixMilli()
}

func exchangeCode(ctx context.Context, c oauthClient, code, verifier string) (sdk.OAuthCredentials, error) {
	start := time.Now()
	tr, err := postToken(ctx, url.Values{
		"client_id": {c.ID}, "client_secret": {c.Secret}, "code": {code},
		"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI}, "code_verifier": {verifier},
	})
	if err != nil {
		return sdk.OAuthCredentials{}, err
	}
	if tr.RefreshToken == "" {
		return sdk.OAuthCredentials{}, errors.New("no refresh token received, try again")
	}
	return sdk.OAuthCredentials{Refresh: tr.RefreshToken, Access: tr.AccessToken, Expires: expiry(start, tr.ExpiresIn)}, nil
}

// refreshCredentials keeps the project id: refresh responses do not carry it.
func refreshCredentials(ctx context.Context, c sdk.OAuthCredentials) (sdk.OAuthCredentials, error) {
	client, err := loadClient()
	if err != nil {
		return c, err
	}
	start := time.Now()
	tr, err := postToken(ctx, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {c.Refresh}, "client_id": {client.ID}, "client_secret": {client.Secret},
	})
	if err != nil {
		return c, fmt.Errorf("antigravity token refresh: %w", err)
	}
	next := c
	next.Access = tr.AccessToken
	next.Expires = expiry(start, tr.ExpiresIn)
	if tr.RefreshToken != "" {
		next.Refresh = tr.RefreshToken
	}
	return next, nil
}

// ---- Cloud Code Assist project discovery ----

type tier struct {
	ID string `json:"id"`
}

type loadResponse struct {
	CurrentTier  *tier  `json:"currentTier"`
	PaidTier     *tier  `json:"paidTier"`
	AllowedTiers []tier `json:"allowedTiers"`
	Ineligible   []struct {
		TierID        string `json:"tierId"`
		ReasonMessage string `json:"reasonMessage"`
		ValidationURL string `json:"validationUrl"`
	} `json:"ineligibleTiers"`
	Project json.RawMessage `json:"cloudaicompanionProject"`
}

func (l loadResponse) projectID() string {
	var s string
	if json.Unmarshal(l.Project, &s) == nil {
		return s
	}
	var o struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(l.Project, &o) == nil {
		return o.ID
	}
	return ""
}

func cloudCode(ctx context.Context, token, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, cloudCodeBase+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", harnessUserAgent())
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s failed: %d %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}

func loadCodeAssist(ctx context.Context, token string) (loadResponse, error) {
	meta := map[string]any{"ideType": "ANTIGRAVITY"}
	var l loadResponse
	if err := cloudCode(ctx, token, "POST", "/v1internal:loadCodeAssist", map[string]any{"metadata": meta}, &l); err != nil {
		return l, err
	}
	if l.PaidTier == nil && l.projectID() != "" {
		var again loadResponse
		err := cloudCode(ctx, token, "POST", "/v1internal:loadCodeAssist",
			map[string]any{"cloudaicompanionProject": l.projectID(), "metadata": meta}, &again)
		if err != nil {
			return l, err
		}
		l = again
	}
	return l, nil
}

func onboardUser(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, onboardTimeout)
	defer cancel()
	var op struct {
		Name  string `json:"name"`
		Done  bool   `json:"done"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	err := cloudCode(ctx, token, "POST", "/v1internal:onboardUser",
		map[string]any{"tierId": freeTier, "metadata": map[string]any{"ideType": "ANTIGRAVITY"}}, &op)
	for {
		if err != nil {
			return err
		}
		if op.Done {
			if op.Error != nil {
				return fmt.Errorf("onboardUser failed: %d: %s", op.Error.Code, op.Error.Message)
			}
			return nil
		}
		if op.Name == "" {
			return errors.New("onboardUser returned an operation without a name")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("onboardUser timed out: %w", ctx.Err())
		case <-time.After(time.Second):
		}
		name := op.Name
		op.Done, op.Error = false, nil
		err = cloudCode(ctx, token, "GET", "/v1internal/"+name, nil, &op)
	}
}

// discoverProject resolves the Cloud Code Assist project for a fresh login,
// onboarding the free tier when the account has none.
func discoverProject(ctx context.Context, token string, progress func(string)) (string, error) {
	progress("Checking Cloud Code Assist account status...")
	l, err := loadCodeAssist(ctx, token)
	if err != nil {
		return "", fmt.Errorf("could not discover an Antigravity project: %w", err)
	}
	free := false
	for _, t := range l.AllowedTiers {
		free = free || t.ID == freeTier
	}
	if !free {
		for _, in := range l.Ineligible {
			if in.TierID == freeTier && in.ReasonMessage != "" {
				return "", fmt.Errorf("%s %s", in.ReasonMessage, in.ValidationURL)
			}
		}
	}
	if l.CurrentTier == nil {
		progress("Provisioning the Antigravity free tier...")
		if err := onboardUser(ctx, token); err != nil {
			return "", err
		}
	}
	progress("Refreshing Cloud Code Assist project...")
	l, err = loadCodeAssist(ctx, token)
	if err != nil {
		return "", err
	}
	if id := l.projectID(); id != "" {
		return id, nil
	}
	return "", errors.New("loadCodeAssist did not return a cloudaicompanionProject")
}

// ---- the sdk OAuth provider ----

// login runs the paste flow: show the consent URL, take the callback URL (or
// bare code) back, exchange it and resolve the project.
func login(cb *sdk.OAuthLoginCallbacks) (sdk.OAuthCredentials, error) {
	ctx := context.Background()
	client, err := ensureClient(func(msg string) (string, error) {
		return cb.OnPrompt(sdk.OAuthPrompt{Message: msg})
	})
	if err != nil {
		return sdk.OAuthCredentials{}, err
	}
	p := newPKCE()
	u, state := authorizeURL(client, p)
	cb.OnAuth(sdk.OAuthAuthInfo{URL: u, Instructions: "Sign in, then paste the localhost callback URL (or the code) here. The browser page will fail to load; that is expected."})
	input, err := cb.OnPrompt(sdk.OAuthPrompt{Message: "Paste the Antigravity OAuth callback URL or code:"})
	if err != nil {
		return sdk.OAuthCredentials{}, err
	}
	code, st := parseCallback(input, state)
	verifier, err := verifierFromState(st)
	if err != nil {
		return sdk.OAuthCredentials{}, err
	}
	creds, err := exchangeCode(ctx, client, code, verifier)
	if err != nil {
		return sdk.OAuthCredentials{}, fmt.Errorf("antigravity oauth exchange failed: %w", err)
	}
	project, err := discoverProject(ctx, creds.Access, cb.OnProgress)
	if err != nil {
		return sdk.OAuthCredentials{}, err
	}
	creds.ProjectID = project
	return creds, nil
}

// apiKey is what the host hands to streamSimple: the access token and project
// as one JSON string, because a request needs both and only this string
// crosses.
func apiKey(c sdk.OAuthCredentials) string {
	b, _ := json.Marshal(map[string]string{"token": c.Access, "projectId": c.ProjectID})
	return string(b)
}

func parseAPIKey(k string) (token, project string) {
	var v struct{ Token, ProjectID string }
	if json.Unmarshal([]byte(k), &v) == nil && v.Token != "" {
		return v.Token, v.ProjectID
	}
	return k, ""
}
