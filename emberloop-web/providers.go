package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

const providerListTimeout = 3 * time.Second

const (
	providerSubscriptionLogins = "subscription_logins"
	providerAPIKeys            = "api_keys"
	providerCLIOnly            = "cli_only"
)

// These are the API-key providers managed by the Providers page. The rest of
// the pig catalog remains visible with its SDK-reported auth status and CLI
// guidance; browser writes stay limited to the providers shown in the mock.
var providerKeyIDs = map[string]struct{}{
	"openrouter":  {},
	"opencode-go": {},
	"google":      {},
	"deepseek":    {},
	"openai":      {},
}

var providerNameOverrides = map[string]string{
	"openai-codex": "ChatGPT Plus",
	"opencode-go":  "OpenCode-Go",
	"google":       "Gemini API (direct)",
}

type providerView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Connected   bool   `json:"connected"`
	Source      string `json:"source"`
	SourceLabel string `json:"sourceLabel"`
	KeySet      bool   `json:"keySet"`
	KeyEditable bool   `json:"keyEditable"`
	Command     string `json:"command,omitempty"`
}

func (d *Daemon) providerAuthPath() string {
	if path := os.Getenv("EL_PIG_AUTH"); path != "" {
		return path
	}
	dir := d.hostPigDir
	if dir == "" {
		dir = userPigAgentDir()
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "auth.json")
}

func providerKeyEditable(id string, auth ai.ProviderAuth) bool {
	_, listed := providerKeyIDs[id]
	return listed && auth.APIKey != nil
}

func providerGroupFor(id string, auth ai.ProviderAuth) string {
	if providerKeyEditable(id, auth) {
		return providerAPIKeys
	}
	if auth.OAuth != nil && auth.OAuth.IsSubscription {
		return providerSubscriptionLogins
	}
	return providerCLIOnly
}

func providerName(id string) string {
	if name := providerNameOverrides[id]; name != "" {
		return name
	}
	return ai.ProviderDisplayName(id)
}

func providerAuthView(ctx context.Context, store ai.CredentialStore, id string, auth ai.ProviderAuth, editable bool) (providerView, error) {
	view := providerView{
		ID:          id,
		Name:        providerName(id),
		Group:       providerGroupFor(id, auth),
		Source:      "none",
		SourceLabel: "Not connected",
		KeyEditable: editable,
	}
	if view.Group == providerCLIOnly {
		view.Command = "pig login " + id
	}

	credential, hasCredential, err := ai.ReadRawCredential(ctx, store, id)
	if err != nil {
		return view, err
	}
	storedKey := hasCredential && credential.Type == ai.CredentialAPIKey && strings.TrimSpace(credential.Key) != ""
	view.KeySet = storedKey

	check, err := ai.CheckProviderAuth(ctx, id, auth, store, ai.DefaultProviderAuthContext())
	if err != nil {
		return view, err
	}
	if check == nil {
		return view, nil
	}
	view.Connected = true
	view.Source, view.SourceLabel = providerAuthSource(check, storedKey)
	return view, nil
}

func providerAuthSource(check *ai.AuthCheck, storedKey bool) (string, string) {
	switch {
	case check.Type == ai.CredentialOAuth || check.Source == "OAuth":
		return "oauth", "OAuth login"
	case check.Source == "stored credential" && storedKey:
		return "stored_key", "Stored key"
	case check.Source == "stored credential":
		return "pig_auth", "Pig auth configuration"
	default:
		return "environment", check.Source
	}
}

// claudeHostProviderView reports only whether the host Claude Code login is
// present. The credential bytes never enter a provider response or pig storage.
func (d *Daemon) claudeHostProviderView() providerView {
	view := providerView{
		ID: "claude", Name: "Claude", Group: providerSubscriptionLogins,
		Source: "none", SourceLabel: "Not connected",
	}
	dir := d.hostClaudeDir
	if dir == "" {
		dir = claudeHostDir()
	}
	data, err := os.ReadFile(filepath.Join(dir, claudeAuthFile))
	if os.IsNotExist(err) {
		return view
	}
	if err != nil {
		view.Source, view.SourceLabel = "unavailable", "Status unavailable"
		return view
	}
	var credentials struct {
		OAuth *struct {
			AccessToken string `json:"accessToken"`
			ExpiresAt   int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(data, &credentials) != nil {
		view.Source, view.SourceLabel = "unavailable", "Status unavailable"
		return view
	}
	if credentials.OAuth == nil || strings.TrimSpace(credentials.OAuth.AccessToken) == "" {
		return view
	}
	if expires := credentials.OAuth.ExpiresAt; expires != 0 {
		now := d.now()
		expired := expires <= now.Unix()
		if expires >= 1e11 {
			expired = expires <= now.UnixMilli()
		}
		if expired {
			view.Source, view.SourceLabel = "expired", "Claude Code login expired"
			return view
		}
	}
	view.Connected = true
	view.Source, view.SourceLabel = "host_claude", "Claude Code login"
	return view
}

func (d *Daemon) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requester, ok := d.requesterOf(r)
	if !ok || requester.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), providerListTimeout)
	defer cancel()
	store := ai.NewReadOnlyAuthStorage(d.providerAuthPath())
	providers := make([]providerView, 0, len(ai.ListProviders())+1)
	for _, id := range ai.ListProviders() {
		auth, err := ai.BuiltinProviderAuth(id)
		if err != nil {
			auth = ai.ProviderAuth{}
		}
		view, err := providerAuthView(ctx, store, id, auth, providerKeyEditable(id, auth))
		if err != nil {
			view.Source = "unavailable"
			view.SourceLabel = "Status unavailable"
		}
		providers = append(providers, view)
	}
	providers = append(providers, d.claudeHostProviderView())
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (d *Daemon) handleProviderKey(w http.ResponseWriter, r *http.Request) {
	requester, ok := d.requesterOf(r)
	if !ok || requester.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !requester.user.Admin {
		http.Error(w, "admin only", http.StatusForbidden)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "providers" || parts[3] != "key" {
		http.NotFound(w, r)
		return
	}
	provider := parts[2]
	auth, err := ai.BuiltinProviderAuth(provider)
	if err != nil || !providerKeyEditable(provider, auth) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store, err := ai.NewAuthStorage(d.providerAuthPath())
	if err != nil {
		http.Error(w, "could not open provider credentials", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodDelete {
		credential, exists, err := store.GetRaw(provider)
		if err != nil {
			http.Error(w, "could not read provider credentials", http.StatusInternalServerError)
			return
		}
		if exists && (credential.Type != ai.CredentialAPIKey || strings.TrimSpace(credential.Key) == "") {
			http.Error(w, "no stored API key", http.StatusConflict)
			return
		}
		if err := store.Delete(r.Context(), provider); err != nil {
			http.Error(w, "could not remove provider key", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	var body struct {
		Key string `json:"key"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	body.Key = strings.TrimSpace(body.Key)
	if body.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if err := store.Set(provider, ai.Credential{Type: ai.CredentialAPIKey, Key: body.Key}); err != nil {
		http.Error(w, "could not save provider key", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
