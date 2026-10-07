package antigravity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The Antigravity OAuth application is Google's own desktop client. Its id and
// secret are not kept in this repo: they come from the environment or from
// ~/.emberloop/agy-oauth.json, and login asks for them once when neither
// exists. emberloop/usage_providers.go reads the same file.
const (
	envClientID     = "EMBERLOOP_AGY_CLIENT_ID"
	envClientSecret = "EMBERLOOP_AGY_CLIENT_SECRET"
)

type oauthClient struct {
	ID     string `json:"client_id"`
	Secret string `json:"client_secret"`
}

var errNoClient = errors.New("antigravity oauth client missing: set " + envClientID + " and " + envClientSecret + ", or add them to ~/.emberloop/agy-oauth.json")

func clientPath() string {
	return filepath.Join(os.Getenv("HOME"), ".emberloop", "agy-oauth.json")
}

// loadClient reads the pair from the environment, then from the file.
func loadClient() (oauthClient, error) {
	c := oauthClient{ID: strings.TrimSpace(os.Getenv(envClientID)), Secret: strings.TrimSpace(os.Getenv(envClientSecret))}
	if c.ID != "" && c.Secret != "" {
		return c, nil
	}
	raw, err := os.ReadFile(clientPath())
	if err != nil {
		return oauthClient{}, errNoClient
	}
	if json.Unmarshal(raw, &c) != nil || c.ID == "" || c.Secret == "" {
		return oauthClient{}, errNoClient
	}
	return c, nil
}

// saveClient stores the pair for later logins, refreshes and usage polls.
func saveClient(c oauthClient) error {
	path := clientPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// ensureClient loads the pair, or asks for it through prompt and saves it.
func ensureClient(prompt func(msg string) (string, error)) (oauthClient, error) {
	if c, err := loadClient(); err == nil {
		return c, nil
	}
	id, err := prompt("Paste the Antigravity OAuth client id:")
	if err != nil {
		return oauthClient{}, err
	}
	secret, err := prompt("Paste the Antigravity OAuth client secret:")
	if err != nil {
		return oauthClient{}, err
	}
	c := oauthClient{ID: strings.TrimSpace(id), Secret: strings.TrimSpace(secret)}
	if c.ID == "" || c.Secret == "" {
		return oauthClient{}, errNoClient
	}
	return c, saveClient(c)
}
