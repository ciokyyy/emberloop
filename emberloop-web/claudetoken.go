package web

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Shared Claude login over a static setup token.
//
// Parallel claude adapters cannot share one OAuth login: the CLI replaces the
// symlinked .credentials.json with a private copy on its first write and the
// rotating refresh tokens are single-use, so concurrent adapters invalidate
// each other ("another Claude Code process is refreshing it"). `claude
// setup-token` instead yields a one-year static token; CLAUDE_CODE_OAUTH_TOKEN
// outranks /login credentials and never refreshes, so adapters cannot race.
// The Daemon loads that token once per harness start (loadClaudeOAuthToken)
// and claudeEnv injects it last-wins; per-User CLAUDE_CONFIG_DIR isolation of
// transcripts and settings is unchanged. The token value is never logged and
// never leaves the process except into the adapter's environment.

// claudeOAuthTokenEnvs are the Daemon environment variables carrying the
// shared token, in precedence order.
var claudeOAuthTokenEnvs = []string{"EL_CLAUDE_OAUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"}

// claudeOAuthTokenFile is the shared-token file under <stateDir>/enderbloom.
const claudeOAuthTokenFile = "claude-oauth-token"

// claudeLoginRace is the adapter error substring of a concurrent OAuth
// refresh: the lock error is documented as transient.
const claudeLoginRace = "another Claude Code process is refreshing"

// claudeLoginRetry waits between the failed prompt and its single retry.
var claudeLoginRetry = 2 * time.Second

// ClaudeOAuthTokenPath is the shared-token file for this Daemon state dir.
func ClaudeOAuthTokenPath(stateDir string) string {
	return filepath.Join(stateDir, "enderbloom", claudeOAuthTokenFile)
}

func claudeTokenPath(stateDir string) string { return ClaudeOAuthTokenPath(stateDir) }

// loadClaudeOAuthToken returns the shared token: the first set Daemon env
// var, else the token file trimmed of whitespace. Empty means fallback mode
// (per-User /login credentials). A group/world-readable file is refused: the
// token is owner-only, and a refusal warns on stderr without the value.
func loadClaudeOAuthToken(stateDir string) string {
	for _, name := range claudeOAuthTokenEnvs {
		if tok := strings.TrimSpace(os.Getenv(name)); tok != "" {
			return tok
		}
	}
	raw, err := os.ReadFile(claudeTokenPath(stateDir))
	if err != nil {
		return ""
	}
	if fi, err := os.Stat(claudeTokenPath(stateDir)); err == nil {
		if fi.Mode().Perm()&0o077 != 0 {
			fmt.Fprintln(os.Stderr, "el-web: shared claude token file is group/world-readable, ignoring it; chmod 600", claudeTokenPath(stateDir))
			return ""
		}
	}
	return strings.TrimSpace(string(raw))
}

// claudeTokenForEnv resolves the token claudeEnv injects: the Daemon-resolved
// one the owner's dirs carry (env-over-file from ownerDirsFor), else the
// Daemon process env for paths that never went through ownerDirsFor.
func claudeTokenForEnv(carried string) string {
	if carried != "" {
		return carried
	}
	for _, name := range claudeOAuthTokenEnvs {
		if tok := strings.TrimSpace(os.Getenv(name)); tok != "" {
			return tok
		}
	}
	return ""
}

// SetClaudeOAuthToken stores the shared token in the 0600 token file.
func SetClaudeOAuthToken(stateDir, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("empty claude token")
	}
	dir := filepath.Join(stateDir, "enderbloom")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeFileAtomic(ClaudeOAuthTokenPath(stateDir), []byte(token+"\n"), 0o600)
}

// isClaudeLoginRaceErr reports the concurrent-refresh lock error.
func isClaudeLoginRaceErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), claudeLoginRace)
}

// claudeLoginRaceAdvice is the actionable session line for a refresh race.
// tokenFile names the shared-token file; empty keeps the generic location.
// It never contains the token value.
func claudeLoginRaceAdvice(tokenFile string) string {
	where := "<state-dir>/enderbloom/" + claudeOAuthTokenFile + " (or EL_CLAUDE_OAUTH_TOKEN on the Daemon)"
	if tokenFile != "" {
		where = tokenFile + " (or EL_CLAUDE_OAUTH_TOKEN on the Daemon)"
	}
	return "Claude login race: parallel Sessions refreshed the OAuth login at once, invalidating each other. " +
		"Run `claude setup-token` once and store the token in " + where + ", then restart the Daemon. " +
		"The static token never refreshes, so adapters cannot race; without it every User keeps a diverging login copy."
}
