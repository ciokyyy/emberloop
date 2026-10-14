package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
)

const oauthRedirectBaseURLSettingKey = "oauth_redirect_base_url"

// providerOAuthRedirectProbe must confirm that the provider accepts this exact
// redirect URI; checking host reachability alone is insufficient.
type providerOAuthRedirectProbe func(context.Context, string, string) (bool, error)

// normalizeOAuthRedirectBaseURL accepts a public HTTPS base or a loopback HTTP
// base for local development. Query strings, fragments, and credentials are
// rejected because they do not belong in a redirect base.
func normalizeOAuthRedirectBaseURL(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "", nil
	}
	parsed, err := url.Parse(base)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(base, "#") {
		return "", errors.New("oauth_redirect_base_url must be an absolute HTTPS URL or loopback HTTP URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("oauth_redirect_base_url must use HTTPS (HTTP is allowed only on loopback)")
	}
	if parsed.Scheme == "http" && !isLoopbackOAuthHost(parsed.Hostname()) {
		return "", errors.New("oauth_redirect_base_url must use HTTPS (HTTP is allowed only on loopback)")
	}
	return strings.TrimRight(base, "/"), nil
}

func isLoopbackOAuthHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// browserOAuthRedirectBase applies the configured base only to OAuth-capable
// providers and only after their flow-specific probe confirms acceptance. A
// probe error or rejection fails closed. Device-code login has no redirect URI
// and must not consume this setting.
func browserOAuthRedirectBase(ctx context.Context, providerID, configured string, probe providerOAuthRedirectProbe) string {
	base, err := normalizeOAuthRedirectBaseURL(configured)
	if err != nil || base == "" || providerID == "" || providerID == "openai-codex" || probe == nil {
		return ""
	}
	auth, err := ai.BuiltinProviderAuth(providerID)
	if err != nil || auth.OAuth == nil {
		return ""
	}
	oauthProvider, ok := ai.GetOAuthProvider(providerID)
	if !ok || !oauthProvider.UsesCallbackServer() {
		return ""
	}
	accepted, err := probe(ctx, providerID, base)
	if err != nil || !accepted {
		return ""
	}
	return base
}

// Daemon-scoped lookup is the entry point for browser OAuth flows. No current
// Providers-page flow consumes a configurable redirect; device-code login is
// independent of this setting.
func (d *Daemon) oauthRedirectBaseForBrowserOAuth(ctx context.Context, providerID string, probe providerOAuthRedirectProbe) string {
	all, err := d.settingsMap()
	if err != nil {
		return ""
	}
	raw, ok := all[oauthRedirectBaseURLSettingKey]
	if !ok {
		return ""
	}
	var configured string
	if err := json.Unmarshal(raw, &configured); err != nil {
		return ""
	}
	return browserOAuthRedirectBase(ctx, providerID, configured, probe)
}
