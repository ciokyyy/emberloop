package web

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
)

const oauthRedirectBaseURLSettingKey = "oauth_redirect_base_url"

// Redirect use is deferred to Ticket 38: the pinned pig SDK exposes no
// per-login callback URI override for both authorization and token exchange.
// Until that seam exists, this setting is validated and stored but consumed by
// no production OAuth flow.

// oauthRedirectBaseFromSettings reads and validates the optional setting from a
// decoded settings map. Sharing this keeps the Settings view and future OAuth
// flow lookup on the same fail-closed normalization path.
func oauthRedirectBaseFromSettings(all map[string]json.RawMessage) string {
	raw, ok := all[oauthRedirectBaseURLSettingKey]
	if !ok {
		return ""
	}
	var configured string
	if err := json.Unmarshal(raw, &configured); err != nil {
		return ""
	}
	base, err := normalizeOAuthRedirectBaseURL(configured)
	if err != nil {
		return ""
	}
	return base
}

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
