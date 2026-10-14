package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func decodeProviderList(t *testing.T, h http.Handler, cookie *http.Cookie) ([]providerView, *httptest.ResponseRecorder) {
	t.Helper()
	rec := authDo(t, h, http.MethodGet, "/api/providers", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/providers = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Providers []providerView `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode providers: %v (body %q)", err, rec.Body.String())
	}
	return body.Providers, rec
}

func findProvider(t *testing.T, providers []providerView, id string) providerView {
	t.Helper()
	for _, provider := range providers {
		if provider.ID == id {
			return provider
		}
	}
	t.Fatalf("provider %q missing from list", id)
	return providerView{}
}

func TestProviderAuthSourceDistinguishesNonKeyPigCredentials(t *testing.T) {
	source, label := providerAuthSource(&ai.AuthCheck{Source: "stored credential", Type: ai.CredentialType("other")}, false)
	if source != "pig_auth" || label != "Pig auth configuration" {
		t.Fatalf("stored non-key source = %q (%q), want pig_auth (Pig auth configuration)", source, label)
	}
}

func TestProvidersKeepsCatalogWhenAuthFileIsUnreadable(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	if err := os.MkdirAll(d.hostPigDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.hostPigDir, "auth.json"), []byte(`{"openai":{"type":`), 0600); err != nil {
		t.Fatal(err)
	}

	h := d.Handler()
	admin := signedIn(t, h, "admin")
	providers, _ := decodeProviderList(t, h, admin)
	if len(providers) != len(ai.ListProviders())+1 {
		t.Fatalf("provider count = %d after credential read error, SDK catalog has %d plus Claude", len(providers), len(ai.ListProviders()))
	}
	if got := findProvider(t, providers, "openai"); got.Source != "unavailable" || got.SourceLabel != "Status unavailable" {
		t.Fatalf("unreadable provider status = %+v, want unavailable row", got)
	}
}

func TestProvidersListsPigCatalogAndRedactsCredentials(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	t.Setenv("DEEPSEEK_API_KEY", "provider-environment-secret")
	t.Setenv("XAI_API_KEY", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	admin := signedIn(t, h, "admin")

	path := filepath.Join(d.hostPigDir, "auth.json")
	store, err := ai.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openrouter", ai.Credential{Type: ai.CredentialAPIKey, Key: "provider-stored-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("groq", ai.Credential{Type: ai.CredentialAPIKey, Key: "provider-cli-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openai-codex", ai.Credential{
		Type:    ai.CredentialOAuth,
		Access:  "provider-oauth-access",
		Refresh: "provider-oauth-refresh",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	providers, rec := decodeProviderList(t, h, admin)
	if len(providers) != len(ai.ListProviders())+1 {
		t.Fatalf("provider count = %d, SDK catalog has %d plus Claude", len(providers), len(ai.ListProviders()))
	}
	for i, id := range ai.ListProviders() {
		if providers[i].ID != id {
			t.Errorf("provider[%d].ID = %q, SDK order has %q", i, providers[i].ID, id)
		}
	}
	if got := providers[len(ai.ListProviders())]; got.ID != "claude" || got.Name != "Claude" || got.Group != providerSubscriptionLogins {
		t.Errorf("Claude row = %+v, want a host-login row", got)
	}

	stored := findProvider(t, providers, "openrouter")
	if !stored.Connected || stored.Source != "stored_key" || !stored.KeySet || !stored.KeyEditable || stored.Group != providerAPIKeys {
		t.Errorf("openrouter status = %+v, want stored editable key", stored)
	}
	env := findProvider(t, providers, "deepseek")
	if !env.Connected || env.Source != "environment" || env.SourceLabel != "DEEPSEEK_API_KEY" || env.KeySet {
		t.Errorf("deepseek status = %+v, want environment source without stored-key flag", env)
	}
	oauth := findProvider(t, providers, "openai-codex")
	if !oauth.Connected || oauth.Source != "oauth" || oauth.Group != providerSubscriptionLogins || oauth.KeySet {
		t.Errorf("openai-codex status = %+v, want subscription OAuth", oauth)
	}
	none := findProvider(t, providers, "xai")
	if none.Connected || none.Source != "none" || none.Group != providerSubscriptionLogins {
		t.Errorf("xai status = %+v, want unconfigured subscription provider", none)
	}
	if got := findProvider(t, providers, "google"); got.Name != "Gemini API (direct)" || got.Group != providerAPIKeys || !got.KeyEditable {
		t.Errorf("google row = %+v, want direct Gemini API-key row", got)
	}
	if got := findProvider(t, providers, "groq"); !got.Connected || got.Source != "stored_key" || !got.KeySet || got.KeyEditable {
		t.Errorf("groq status = %+v, want a stored CLI-managed key with no browser edit", got)
	}

	for _, secret := range []string{"provider-stored-secret", "provider-cli-secret", "provider-environment-secret", "provider-oauth-access", "provider-oauth-refresh"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("GET /api/providers response leaked a credential value")
		}
	}
}

func TestProvidersListDoesNotCreatePigAuthFile(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	admin := signedIn(t, h, "admin")

	providers, _ := decodeProviderList(t, h, admin)
	if len(providers) == 0 {
		t.Fatal("provider list is empty")
	}
	if _, err := os.Stat(filepath.Join(d.hostPigDir, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("listing providers created auth.json (stat error %v)", err)
	}
}

func TestProvidersAdminSetsAndRemovesAPIKeysWithoutReturningThem(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	t.Setenv("GEMINI_API_KEY", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	path := filepath.Join(d.hostPigDir, "auth.json")

	set := authDo(t, h, http.MethodPut, "/api/providers/google/key", `{"key":"provider-new-secret"}`, admin)
	if set.Code != http.StatusOK {
		t.Fatalf("PUT provider key = %d: %s", set.Code, set.Body.String())
	}
	if strings.Contains(set.Body.String(), "provider-new-secret") {
		t.Fatal("PUT provider key response leaked the key")
	}
	stored, err := ai.NewReadOnlyAuthStorage(path).Read(t.Context(), "google")
	if err != nil || stored == nil || stored.Type != ai.CredentialAPIKey || stored.Key != "provider-new-secret" {
		t.Fatalf("stored Google credential = %#v, %v", stored, err)
	}
	providers, list := decodeProviderList(t, h, admin)
	google := findProvider(t, providers, "google")
	if !google.KeySet || google.Source != "stored_key" || strings.Contains(list.Body.String(), "provider-new-secret") {
		t.Fatalf("Google status = %+v or response included the key", google)
	}

	remove := authDo(t, h, http.MethodDelete, "/api/providers/google/key", "", admin)
	if remove.Code != http.StatusOK {
		t.Fatalf("DELETE provider key = %d: %s", remove.Code, remove.Body.String())
	}
	if strings.Contains(remove.Body.String(), "provider-new-secret") {
		t.Fatal("DELETE provider key response leaked the key")
	}
	stored, err = ai.NewReadOnlyAuthStorage(path).Read(t.Context(), "google")
	if err != nil || stored != nil {
		t.Fatalf("credential after remove = %#v, %v; want absent", stored, err)
	}
	providers, list = decodeProviderList(t, h, admin)
	google = findProvider(t, providers, "google")
	if google.KeySet || google.Source != "none" || strings.Contains(list.Body.String(), "provider-new-secret") {
		t.Fatalf("Google status after remove = %+v or response included the key", google)
	}
}

func TestProvidersWritesAreAdminOnlyButStatusIsReadable(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	signedIn(t, h, "admin")
	user := signedIn(t, h, "mira")
	path := filepath.Join(d.hostPigDir, "auth.json")
	store, err := ai.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openrouter", ai.Credential{Type: ai.CredentialAPIKey, Key: "provider-admin-secret"}); err != nil {
		t.Fatal(err)
	}

	providers, list := decodeProviderList(t, h, user)
	if !findProvider(t, providers, "openrouter").KeySet || strings.Contains(list.Body.String(), "provider-admin-secret") {
		t.Fatalf("non-admin provider status = %+v or response leaked a key", findProvider(t, providers, "openrouter"))
	}
	set := authDo(t, h, http.MethodPut, "/api/providers/openrouter/key", `{"key":"provider-user-secret"}`, user)
	if set.Code != http.StatusForbidden || strings.Contains(set.Body.String(), "provider-user-secret") {
		t.Fatalf("non-admin PUT = %d: %s", set.Code, set.Body.String())
	}
	remove := authDo(t, h, http.MethodDelete, "/api/providers/openrouter/key", "", user)
	if remove.Code != http.StatusForbidden || strings.Contains(remove.Body.String(), "provider-admin-secret") {
		t.Fatalf("non-admin DELETE = %d: %s", remove.Code, remove.Body.String())
	}
	credential, ok, err := store.GetRaw("openrouter")
	if err != nil || !ok || credential.Key != "provider-admin-secret" {
		t.Fatalf("credential after non-admin attempts = %#v, %v, %v", credential, ok, err)
	}
}

func TestProvidersRejectsInvalidKeyRequests(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	admin := signedIn(t, h, "admin")

	for _, tc := range []struct {
		path string
		body string
		want int
	}{
		{path: "/api/providers/openrouter/key", body: `{"key":"  "}`, want: http.StatusBadRequest},
		{path: "/api/providers/unknown/key", body: `{"key":"nope"}`, want: http.StatusNotFound},
		{path: "/api/providers/openai-codex/key", body: `{"key":"nope"}`, want: http.StatusNotFound},
		{path: "/api/providers/openrouter/key", body: `{"key":"one","extra":true}`, want: http.StatusBadRequest},
	} {
		rec := authDo(t, h, http.MethodPut, tc.path, tc.body, admin)
		if rec.Code != tc.want {
			t.Errorf("PUT %s body %s = %d: %s, want %d", tc.path, tc.body, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func TestProvidersRemoveCannotDeleteAnOAuthCredential(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	store, err := ai.NewAuthStorage(filepath.Join(d.hostPigDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openai", ai.Credential{
		Type:    ai.CredentialOAuth,
		Access:  "provider-openai-oauth-access",
		Refresh: "provider-openai-oauth-refresh",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	rec := authDo(t, h, http.MethodDelete, "/api/providers/openai/key", "", admin)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), "provider-openai-oauth-access") {
		t.Fatalf("DELETE OAuth credential = %d: %s, want 409 without credentials", rec.Code, rec.Body.String())
	}
	credential, exists, err := store.GetRaw("openai")
	if err != nil || !exists || credential.Type != ai.CredentialOAuth || credential.Access != "provider-openai-oauth-access" {
		t.Fatalf("OpenAI credential after key removal = %#v, %v, %v", credential, exists, err)
	}
}
