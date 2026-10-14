package el

// Usage windows for quota cards and the strip's right side. Claude's terminal
// strip keeps using the push-based statusline file; browser quota reads its
// host OAuth usage endpoint without refreshing credentials. OpenCode-Go and
// Antigravity also have no push channel, so their pollers share
// `usage-<key>.json` caches and per-key locks across viewers.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/emberloop-web/el/statedb"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
)

// UsageSet is the windows the strip shows for one account, in display
// order. Has is false until the account has reported.
type UsageSet struct {
	Has     bool
	Windows []UsageWindow
}

// Usage keys name one cached account view.
const (
	usageKeyClaude   = "claude"
	usageKeyChatGPT  = "openai-codex"
	usageKeyOpencode = "opencode-go"
	usageKeyAgyGem   = "agy-gemini"
	usageKeyAgy3P    = "agy-3p"
)

// usageRefreshEvery is how stale a cache may be before a poller refetches.
const usageRefreshEvery = 90 * time.Second

// Endpoints and clients are variables so tests can aim them at local servers.
var (
	opencodeUsageURL = "https://opencode.ai/zen/go/v1/usage"
	chatGPTUsageURL  = "https://chatgpt.com/backend-api/wham/usage"
	agyBaseURL       = "https://daily-cloudcode-pa.googleapis.com"
	googleTokenURL   = "https://oauth2.googleapis.com/token"
	claudeUsageURL   = "https://api.anthropic.com/api/oauth/usage"
	usageHTTP        = &http.Client{Timeout: 20 * time.Second}
)

const (
	claudeOAuthBeta       = "oauth-2025-04-20"
	claudeVersionFallback = "1.0.0"
)

var (
	claudeVersionMu     sync.Mutex
	claudeVersionLookup = discoverClaudeCodeVersion
	claudeVersionValue  string
)

var claudeVersionSegmentPattern = regexp.MustCompile(`^v?(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)$`)

func claudeCodeVersion() string {
	claudeVersionMu.Lock()
	defer claudeVersionMu.Unlock()
	if claudeVersionValue == "" {
		claudeVersionValue = claudeVersionLookup()
		if claudeVersionValue == "" {
			claudeVersionValue = claudeVersionFallback
		}
	}
	return claudeVersionValue
}

// discoverClaudeCodeVersion reads the installed binary's versioned path. It
// deliberately never starts Claude Code, so quota polling cannot launch a
// harness or agent process.
func discoverClaudeCodeVersion() string {
	path := findClaudeCodeBinary()
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		if version := parseClaudeCodeVersionPath(resolved); version != "" {
			return version
		}
	}
	return parseClaudeCodeVersionPath(path)
}

func findClaudeCodeBinary() string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, "claude")
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path
		}
	}
	return ""
}

func parseClaudeCodeVersionPath(path string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for i, part := range parts {
		if part == "claude-code" && i+1 < len(parts) {
			if version := parseClaudeCodeVersionSegment(parts[i+1]); version != "" {
				return version
			}
		}
		if part == "versions" && i > 0 && parts[i-1] == "claude" && i+1 < len(parts) {
			if version := parseClaudeCodeVersionSegment(parts[i+1]); version != "" {
				return version
			}
		}
	}
	return ""
}

func parseClaudeCodeVersionSegment(segment string) string {
	match := claudeVersionSegmentPattern.FindStringSubmatch(segment)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func claudeCredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate Claude Code credentials: %w", err)
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

type claudeOAuthCredentials struct {
	AccessToken      string `json:"accessToken"`
	ExpiresAt        *int64 `json:"expiresAt"`
	SubscriptionType string `json:"subscriptionType"`
}

func readClaudeOAuthCredentials() (claudeOAuthCredentials, bool, error) {
	path, err := claudeCredentialsPath()
	if err != nil {
		return claudeOAuthCredentials{}, false, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return claudeOAuthCredentials{}, false, nil
	}
	if err != nil {
		return claudeOAuthCredentials{}, false, fmt.Errorf("read Claude Code credentials: %w", err)
	}
	var root struct {
		OAuth *claudeOAuthCredentials `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return claudeOAuthCredentials{}, false, fmt.Errorf("decode Claude Code credentials: %w", err)
	}
	if root.OAuth == nil {
		return claudeOAuthCredentials{}, false, nil
	}
	root.OAuth.AccessToken = strings.TrimSpace(root.OAuth.AccessToken)
	if root.OAuth.AccessToken == "" {
		return claudeOAuthCredentials{}, false, nil
	}
	return *root.OAuth, true, nil
}

func claudeCredentialsExpired(credentials claudeOAuthCredentials, now time.Time) bool {
	if credentials.ExpiresAt == nil {
		return false
	}
	expiresAt := *credentials.ExpiresAt
	if expiresAt >= 1e11 { // Claude Code stores millisecond Unix timestamps.
		return expiresAt <= now.UnixMilli()
	}
	return expiresAt <= now.Unix()
}

func claudePlanName(subscriptionType string) string {
	switch strings.ToLower(strings.TrimSpace(subscriptionType)) {
	case "pro":
		return "Pro"
	case "max", "max_5", "max_20":
		return "Max"
	case "team":
		return "Team"
	case "enterprise":
		return "Enterprise"
	default:
		if subscriptionType == "" {
			return ""
		}
		return strings.ToUpper(subscriptionType[:1]) + subscriptionType[1:]
	}
}

func claudeQuotaConfigured(stateDir string, now time.Time) bool {
	_, configured, err := readClaudeOAuthCredentials()
	if configured || (err != nil && readUsageCache(stateDir, usageKeyClaude, now).Has) {
		return true
	}
	return ReadClaudeUsage(stateDir, now).Has
}

func claudeQuotaCachePath(stateDir string, now time.Time) string {
	_, configured, err := readClaudeOAuthCredentials()
	oauthPath := usageCachePath(stateDir, usageKeyClaude)
	if configured || (err != nil && readUsageCache(stateDir, usageKeyClaude, now).Has) {
		return oauthPath
	}
	return claudeUsagePath(stateDir)
}

func ReadClaudeQuotaUsage(stateDir string, now time.Time) UsageSet {
	_, configured, err := readClaudeOAuthCredentials()
	if configured {
		return readUsageCache(stateDir, usageKeyClaude, now)
	}
	if err != nil {
		if cached := readUsageCache(stateDir, usageKeyClaude, now); cached.Has {
			return cached
		}
	}
	return ReadClaudeUsage(stateDir, now).Set()
}

// The Antigravity OAuth client id and secret are not kept in source.
// pig-antigravity-auth saves them to ~/.el/agy-oauth.json at login; the
// environment overrides the file.
func agyClient() (id, secret string, err error) {
	id, secret = strings.TrimSpace(os.Getenv("EL_AGY_CLIENT_ID")), strings.TrimSpace(os.Getenv("EL_AGY_CLIENT_SECRET"))
	if id != "" && secret != "" {
		return id, secret, nil
	}
	raw, rerr := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".el", "agy-oauth.json"))
	var c struct {
		ID     string `json:"client_id"`
		Secret string `json:"client_secret"`
	}
	if rerr != nil || json.Unmarshal(raw, &c) != nil || c.ID == "" || c.Secret == "" {
		return "", "", fmt.Errorf("antigravity oauth client missing: log in to the provider, or set EL_AGY_CLIENT_ID and EL_AGY_CLIENT_SECRET")
	}
	return c.ID, c.Secret, nil
}

// UsageKeyFor is the cache key of the account a pane draws on: claude by
// harness, pig by the provider of its latest request. Empty means no usage
// view for this pane.
func UsageKeyFor(harness, provider, model string) string {
	switch {
	case harness == "claude":
		return usageKeyClaude
	case provider == "opencode-go":
		return usageKeyOpencode
	case provider == "google-antigravity":
		if strings.Contains(strings.ToLower(model), "gemini") {
			return usageKeyAgyGem
		}
		return usageKeyAgy3P
	}
	return ""
}

// ReadUsage is the usage the strip shows for key as of now. A window whose
// reset has passed has rolled over since the last fetch, so it reads 0%.
func ReadUsage(stateDir, key string, now time.Time) UsageSet {
	if key == "" {
		return UsageSet{}
	}
	if key == usageKeyClaude {
		return ReadClaudeUsage(stateDir, now).Set()
	}
	return readUsageCache(stateDir, key, now)
}

func readUsageCache(stateDir, key string, now time.Time) UsageSet {
	raw, err := os.ReadFile(usageCachePath(stateDir, key))
	if err != nil {
		return UsageSet{}
	}
	var c usageCache
	if json.Unmarshal(raw, &c) != nil || len(c.Windows) == 0 {
		return UsageSet{}
	}
	for i, w := range c.Windows {
		if w.ResetsAt > 0 && w.ResetsAt <= now.Unix() {
			c.Windows[i].Pct = 0
		}
	}
	return UsageSet{Has: true, Windows: c.Windows}
}

type usageCache struct {
	FetchedAt int64         `json:"fetched_at"`
	Windows   []UsageWindow `json:"windows"`
}

func usageCachePath(stateDir, key string) string {
	if stateDir == "" {
		stateDir = statedb.StateDir()
	}
	return filepath.Join(stateDir, "usage-"+key+".json")
}

func writeUsageCache(stateDir, key string, windows []UsageWindow, now time.Time) error {
	raw, err := json.Marshal(usageCache{FetchedAt: now.Unix(), Windows: windows})
	if err != nil {
		return err
	}
	path := usageCachePath(stateDir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func usageCacheAge(stateDir, key string, now time.Time) time.Duration {
	raw, err := os.ReadFile(usageCachePath(stateDir, key))
	if err != nil {
		return 1 << 62
	}
	var c usageCache
	if json.Unmarshal(raw, &c) != nil {
		return 1 << 62
	}
	return now.Sub(time.Unix(c.FetchedAt, 0))
}

// RefreshUsage refetches the account behind key when its cache is older than
// usageRefreshEvery. Viewers share a per-key file lock. Claude's fetch uses
// the host OAuth access token read-only, persists Retry-After backoff, and
// never refreshes or writes the credentials file; failures retain old values.
func RefreshUsage(ctx context.Context, stateDir, key string, now time.Time) error {
	return refreshUsage(ctx, stateDir, key, now, false)
}

// RefreshUsageForce refetches the account behind key even when its cache is
// still fresh: the quota page's manual refresh. The per-key lock stays, so N
// viewers still cause one upstream request, and failure still keeps the old
// cache. Header panes keep calling RefreshUsage.
func RefreshUsageForce(ctx context.Context, stateDir, key string, now time.Time) error {
	return refreshUsage(ctx, stateDir, key, now, true)
}

func refreshUsage(ctx context.Context, stateDir, key string, now time.Time, force bool) error {
	if key == "" {
		return nil
	}
	if key == usageKeyClaude {
		_, configured, err := claudeQuotaCredentialsForFetch(stateDir, now)
		if err != nil {
			return err
		}
		if !configured {
			return nil // Keep the statusline file as the fallback source.
		}
	}
	if !force && usageCacheAge(stateDir, key, now) < usageRefreshEvery {
		return nil
	}
	dir := stateDir
	if dir == "" {
		dir = statedb.StateDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "usage-"+key+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil // another pane is fetching
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if !force && usageCacheAge(stateDir, key, now) < usageRefreshEvery {
		return nil
	}
	switch key {
	case usageKeyChatGPT:
		w, err := fetchChatGPTPlusUsage(ctx, now)
		if err != nil {
			return err
		}
		if len(w) == 0 {
			return nil
		}
		return writeUsageCache(stateDir, key, w, now)
	case usageKeyClaude:
		return fetchClaudeQuota(ctx, stateDir, now)
	case usageKeyOpencode:
		w, err := fetchOpencodeUsage(ctx)
		if err != nil {
			return err
		}
		return writeUsageCache(stateDir, key, w, now)
	case usageKeyAgyGem, usageKeyAgy3P:
		gem, tp, err := fetchAgyUsage(ctx)
		if err != nil {
			return err
		}
		if err := writeUsageCache(stateDir, usageKeyAgyGem, gem, now); err != nil {
			return err
		}
		return writeUsageCache(stateDir, usageKeyAgy3P, tp, now)
	}
	return nil
}

func claudeExpiredError(hint string) error {
	return &quotaFailure{state: QuotaExpired, hint: hint, err: fmt.Errorf("Claude OAuth login expired")}
}

func claudeBackoffPath(stateDir string) string {
	if stateDir == "" {
		stateDir = statedb.StateDir()
	}
	return filepath.Join(stateDir, "usage-claude-backoff")
}

func claudeRejectedTokenPath(stateDir string) string {
	if stateDir == "" {
		stateDir = statedb.StateDir()
	}
	return filepath.Join(stateDir, "usage-claude-expired-token")
}

func claudeTokenFingerprint(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func claudeTokenRejected(stateDir, token string) bool {
	path := claudeRejectedTokenPath(stateDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(raw)) == claudeTokenFingerprint(token) {
		return true
	}
	_ = os.Remove(path)
	return false
}

func writeClaudeRejectedToken(stateDir, token string) error {
	path := claudeRejectedTokenPath(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, []byte(claudeTokenFingerprint(token)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func clearClaudeRejectedToken(stateDir string) {
	_ = os.Remove(claudeRejectedTokenPath(stateDir))
}

func claudeBackoffUntil(stateDir string, now time.Time) (time.Time, bool) {
	raw, err := os.ReadFile(claudeBackoffPath(stateDir))
	if err != nil {
		return time.Time{}, false
	}
	nanos, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return now.Add(usageRefreshEvery), true
	}
	return time.Unix(0, nanos), true
}

func writeClaudeBackoff(stateDir string, until time.Time) error {
	path := claudeBackoffPath(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(until.UnixNano(), 10)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func clearClaudeBackoff(stateDir string) {
	_ = os.Remove(claudeBackoffPath(stateDir))
}

func claudeBackoffError(stateDir string, now time.Time) error {
	if until, ok := claudeBackoffUntil(stateDir, now); ok && now.Before(until) {
		return fmt.Errorf("Claude quota rate limit backoff until %s", until.UTC().Format(time.RFC3339))
	}
	return nil
}

func claudeQuotaCredentialsForFetch(stateDir string, now time.Time) (claudeOAuthCredentials, bool, error) {
	credentials, configured, err := readClaudeOAuthCredentials()
	if err != nil || !configured {
		return credentials, configured, err
	}
	if claudeCredentialsExpired(credentials, now) || claudeTokenRejected(stateDir, credentials.AccessToken) {
		return credentials, true, claudeExpiredError("Claude Code login expired. Sign in again in Claude Code.")
	}
	if err := claudeBackoffError(stateDir, now); err != nil {
		return credentials, true, err
	}
	return credentials, true, nil
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := at.Sub(now); delay > 0 {
			return delay
		}
		return 0
	}
	return usageRefreshEvery
}

func fetchClaudeQuota(ctx context.Context, stateDir string, now time.Time) error {
	credentials, configured, err := claudeQuotaCredentialsForFetch(stateDir, now)
	if err != nil {
		return err
	}
	if !configured {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeUsageURL, nil)
	if err != nil {
		return fmt.Errorf("create Claude OAuth usage request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	request.Header.Set("anthropic-beta", claudeOAuthBeta)
	request.Header.Set("User-Agent", "claude-code/"+claudeCodeVersion())
	response, err := usageHTTP.Do(request)
	if err != nil {
		return fmt.Errorf("Claude OAuth usage request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		_ = writeClaudeRejectedToken(stateDir, credentials.AccessToken)
		clearClaudeBackoff(stateDir)
		return claudeExpiredError("Claude Code login expired. Sign in again in Claude Code.")
	}
	if response.StatusCode == http.StatusTooManyRequests {
		delay := parseRetryAfter(response.Header.Get("Retry-After"), now)
		until := now.Add(delay)
		if err := writeClaudeBackoff(stateDir, until); err != nil {
			return fmt.Errorf("Claude quota is rate limited; could not save retry backoff: %w", err)
		}
		return fmt.Errorf("Claude quota rate limited; retry after %s", delay.Round(time.Second))
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Claude OAuth usage request returned HTTP %d", response.StatusCode)
	}
	var usage struct {
		FiveHour *struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay *struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&usage); err != nil {
		return fmt.Errorf("decode Claude OAuth usage response: %w", err)
	}
	var windows []UsageWindow
	if usage.FiveHour != nil {
		windows = append(windows, UsageWindow{Label: "5h", Pct: claudeUsagePercent(usage.FiveHour.Utilization), ResetsAt: parseResetTime(usage.FiveHour.ResetsAt)})
	}
	if usage.SevenDay != nil {
		windows = append(windows, UsageWindow{Label: "wk", Pct: claudeUsagePercent(usage.SevenDay.Utilization), ResetsAt: parseResetTime(usage.SevenDay.ResetsAt)})
	}
	if len(windows) == 0 {
		return fmt.Errorf("Claude OAuth usage response has no supported windows")
	}
	if err := writeUsageCache(stateDir, usageKeyClaude, windows, now); err != nil {
		return err
	}
	clearClaudeBackoff(stateDir)
	clearClaudeRejectedToken(stateDir)
	return nil
}

func claudeUsagePercent(utilization float64) int {
	if utilization <= 0 {
		return 0
	}
	if utilization >= 100 {
		return 100
	}
	return int(utilization + 0.5)
}

// pigAuth is the part of pig's auth file the usage fetchers read.
type pigAuth struct {
	OpencodeGo struct {
		Key string `json:"key"`
	} `json:"opencode-go"`
	Antigravity struct {
		Refresh string `json:"refresh"`
		Project string `json:"projectId"`
	} `json:"google-antigravity"`
}

func pigAuthPath() string {
	if path := os.Getenv("EL_PIG_AUTH"); path != "" {
		return path
	}
	agentDirEnv := "PIG_CODING_AGENT_DIR"
	if os.Getenv("PIG_USE_PI_DIRS") == "1" {
		agentDirEnv = "PI_CODING_AGENT_DIR"
	}
	agentDir := os.Getenv(agentDirEnv)
	if agentDir == "" {
		agentDir = coding.DefaultAgentDir()
	} else if agentDir == "~" || strings.HasPrefix(agentDir, "~/") {
		home, _ := os.UserHomeDir()
		agentDir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(agentDir, "~"), string(filepath.Separator)))
	}
	return filepath.Join(agentDir, "auth.json")
}

// readPigCredential reads one credential through PiG's read-only auth store.
// The store neither creates a missing auth.json nor refreshes OAuth tokens.
func readPigCredential(provider string) (*ai.Credential, error) {
	return ai.NewReadOnlyAuthStorage(pigAuthPath()).Read(context.Background(), provider)
}

func readChatGPTPlusCredential() (ai.Credential, bool, error) {
	credential, err := readPigCredential(usageKeyChatGPT)
	if err != nil {
		return ai.Credential{}, false, err
	}
	if credential == nil || credential.Type != ai.CredentialOAuth || credential.Access == "" {
		return ai.Credential{}, false, nil
	}
	if credential.AccountID == "" {
		credential.AccountID = ai.CodexAccountID(credential.Access)
	}
	if credential.AccountID == "" {
		return ai.Credential{}, false, nil
	}
	return *credential, true, nil
}

func chatGPTPlusExpired(now time.Time) bool {
	credential, configured, err := readChatGPTPlusCredential()
	return err == nil && configured && credential.HasExpires() && credential.ExpiresMillis() <= float64(now.UnixMilli())
}

func chatGPTPlusConfigured(_ string, _ time.Time) bool {
	_, configured, err := readChatGPTPlusCredential()
	return err == nil && configured
}

func chatGPTPlusNotConfiguredHint(string) string {
	return "Missing pig openai-codex login. Connect ChatGPT in Providers."
}

const chatGPTPlusExpiredHint = "ChatGPT login expired. Reconnect in Providers."

func fetchChatGPTPlusUsage(ctx context.Context, now time.Time) ([]UsageWindow, error) {
	credential, configured, err := readChatGPTPlusCredential()
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, nil
	}
	if credential.HasExpires() && credential.ExpiresMillis() <= float64(now.UnixMilli()) {
		return nil, &quotaFailure{state: QuotaExpired, hint: chatGPTPlusExpiredHint}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, chatGPTUsageURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+credential.Access)
	request.Header.Set("ChatGPT-Account-Id", credential.AccountID)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "el")
	response, err := usageHTTP.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return nil, &quotaFailure{state: QuotaExpired, hint: chatGPTPlusExpiredHint}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ChatGPT usage request returned %s", response.Status)
	}
	type window struct {
		UsedPercent *float64 `json:"used_percent"`
		ResetAt     *int64   `json:"reset_at"`
	}
	var result struct {
		RateLimit struct {
			PrimaryWindow   *window `json:"primary_window"`
			SecondaryWindow *window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode ChatGPT usage response: %w", err)
	}
	windows := make([]UsageWindow, 0, 2)
	for _, item := range []struct {
		label  string
		window *window
	}{
		{label: "5-hour", window: result.RateLimit.PrimaryWindow},
		{label: "Weekly", window: result.RateLimit.SecondaryWindow},
	} {
		if item.window == nil || item.window.UsedPercent == nil || item.window.ResetAt == nil {
			continue
		}
		pct := math.Max(0, math.Min(100, *item.window.UsedPercent))
		windows = append(windows, UsageWindow{Label: item.label, Pct: int(math.Round(pct)), ResetsAt: *item.window.ResetAt})
	}
	if len(windows) != 2 {
		return nil, fmt.Errorf("ChatGPT usage response is missing a rate-limit window")
	}
	return windows, nil
}

func readPigAuth() (pigAuth, error) {
	path := os.Getenv("EL_PIG_AUTH")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".pig", "agent", "auth.json")
	}
	var a pigAuth
	raw, err := os.ReadFile(path)
	if err != nil {
		return a, err
	}
	return a, json.Unmarshal(raw, &a)
}

func getJSON(ctx context.Context, method, target string, header map[string]string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := usageHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %d", target, resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

// fetchOpencodeUsage asks the Go plan's usage endpoint with the stored key:
// rolling 5-hour, weekly and monthly percent used, each with its reset.
func fetchOpencodeUsage(ctx context.Context) ([]UsageWindow, error) {
	auth, err := readPigAuth()
	if err != nil || auth.OpencodeGo.Key == "" {
		return nil, fmt.Errorf("no opencode-go key in pig's auth file")
	}
	var r struct {
		Usage map[string]struct {
			Percent  float64 `json:"percent"`
			ResetsAt string  `json:"resetsAt"`
		} `json:"usage"`
	}
	err = getJSON(ctx, http.MethodGet, opencodeUsageURL,
		map[string]string{"Authorization": "Bearer " + auth.OpencodeGo.Key, "User-Agent": "el"}, nil, &r)
	if err != nil {
		return nil, err
	}
	var out []UsageWindow
	for _, d := range []struct{ field, label string }{{"rolling", "5h"}, {"weekly", "wk"}, {"monthly", "mo"}} {
		w, ok := r.Usage[d.field]
		if !ok {
			continue
		}
		out = append(out, UsageWindow{Label: d.label, Pct: int(w.Percent + 0.5), ResetsAt: parseResetTime(w.ResetsAt)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("opencode usage response has no windows")
	}
	return out, nil
}

func parseResetTime(s string) int64 {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	return 0
}

var (
	agyTokenMu  sync.Mutex
	agyToken    string
	agyTokenExp time.Time
)

// agyAccessToken refreshes the Antigravity login's access token from the
// refresh token pig stores, and keeps it in memory until a minute before it
// expires. Google does not rotate the refresh token, so this never
// invalidates pig's copy, and nothing is written back.
func agyAccessToken(ctx context.Context, refresh string) (string, error) {
	agyTokenMu.Lock()
	defer agyTokenMu.Unlock()
	if agyToken != "" && time.Now().Before(agyTokenExp) {
		return agyToken, nil
	}
	clientID, clientSecret, err := agyClient()
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh},
		"client_id": {clientID}, "client_secret": {clientSecret}}
	var r struct {
		Access  string `json:"access_token"`
		Expires int64  `json:"expires_in"`
	}
	err = getJSON(ctx, http.MethodPost, googleTokenURL,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, strings.NewReader(form.Encode()), &r)
	if err != nil {
		return "", err
	}
	if r.Access == "" {
		return "", fmt.Errorf("antigravity token refresh returned no token")
	}
	agyToken = r.Access
	agyTokenExp = time.Now().Add(time.Duration(r.Expires)*time.Second - time.Minute)
	return agyToken, nil
}

// fetchAgyUsage reads Antigravity's quota summary: two model groups (Gemini,
// and Claude plus GPT-OSS), each with a 5-hour and a weekly bucket that
// report the fraction still remaining.
func fetchAgyUsage(ctx context.Context) (gemini, thirdParty []UsageWindow, err error) {
	auth, err := readPigAuth()
	if err != nil || auth.Antigravity.Refresh == "" {
		return nil, nil, fmt.Errorf("no antigravity login in pig's auth file")
	}
	tok, err := agyAccessToken(ctx, auth.Antigravity.Refresh)
	if err != nil {
		return nil, nil, err
	}
	body, _ := json.Marshal(map[string]string{"project": auth.Antigravity.Project})
	var r struct {
		Groups []struct {
			Buckets []struct {
				ID        string  `json:"bucketId"`
				Window    string  `json:"window"`
				Reset     string  `json:"resetTime"`
				Remaining float64 `json:"remainingFraction"`
			} `json:"buckets"`
		} `json:"groups"`
	}
	err = getJSON(ctx, http.MethodPost, agyBaseURL+"/v1internal:retrieveUserQuotaSummary", map[string]string{
		"Authorization": "Bearer " + tok, "Content-Type": "application/json",
		"User-Agent": "antigravity/cli/1.0 (aidev_client; os_type=linux; arch=amd64; auth_method=consumer)",
	}, strings.NewReader(string(body)), &r)
	if err != nil {
		return nil, nil, err
	}
	for _, g := range r.Groups {
		for _, b := range g.Buckets {
			label := map[string]string{"5h": "5h", "weekly": "wk"}[b.Window]
			if label == "" {
				continue
			}
			w := UsageWindow{Label: label, Pct: int((1-b.Remaining)*100 + 0.5), ResetsAt: parseResetTime(b.Reset)}
			switch {
			case strings.HasPrefix(b.ID, "gemini"):
				gemini = append(gemini, w)
			case strings.HasPrefix(b.ID, "3p"):
				thirdParty = append(thirdParty, w)
			}
		}
	}
	if len(gemini) == 0 && len(thirdParty) == 0 {
		return nil, nil, fmt.Errorf("antigravity quota summary has no buckets")
	}
	sortWindows(gemini)
	sortWindows(thirdParty)
	return gemini, thirdParty, nil
}

// sortWindows orders windows short to long: 5h before wk.
func sortWindows(ws []UsageWindow) {
	rank := map[string]int{"5h": 0, "wk": 1, "mo": 2}
	for i := 1; i < len(ws); i++ {
		for j := i; j > 0 && rank[ws[j].Label] < rank[ws[j-1].Label]; j-- {
			ws[j], ws[j-1] = ws[j-1], ws[j]
		}
	}
}

// SetChatGPTUsageEndpointForTest aims the Codex usage fetcher at a local stub.
func SetChatGPTUsageEndpointForTest(endpoint string) func() {
	old := chatGPTUsageURL
	chatGPTUsageURL = endpoint
	return func() { chatGPTUsageURL = old }
}
