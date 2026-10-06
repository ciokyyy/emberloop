package wick

// Usage windows of the model account behind a pane, for the strip's right
// side. claude reports its own through the statusline (claude_usage.go);
// the opencode Go plan and Antigravity have no push channel, so one poller
// per machine asks their usage endpoints and every strip reads the cache:
// `usage-<key>.json` in wick's state dir. Panes race for one lock per key, so
// N panes cost one request per interval, never N.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/wick/statedb"
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
	usageKeyOpencode = "opencode-go"
	usageKeyAgyGem   = "agy-gemini"
	usageKeyAgy3P    = "agy-3p"
)

// usageRefreshEvery is how stale a cache may be before a poller refetches.
const usageRefreshEvery = 90 * time.Second

// Endpoints and clients are variables so tests can aim them at a local
// server.
var (
	opencodeUsageURL = "https://opencode.ai/zen/go/v1/usage"
	agyBaseURL       = "https://daily-cloudcode-pa.googleapis.com"
	googleTokenURL   = "https://oauth2.googleapis.com/token"
	usageHTTP        = &http.Client{Timeout: 20 * time.Second}
)

// The Antigravity OAuth application is Google's own public desktop client,
// shipped inside the Antigravity IDE and agy CLI; pig-antigravity-auth uses
// the same pair to refresh the stored login.
const (
	agyClientID     = "REDACTED"
	agyClientSecret = "REDACTED"
)

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
// usageRefreshEvery. Panes race for a per-key file lock without waiting, so
// one request serves them all. Failure keeps the old cache: a stale number
// beats a blank one, and the next interval retries.
func RefreshUsage(ctx context.Context, stateDir, key string, now time.Time) error {
	if key == "" || key == usageKeyClaude {
		return nil
	}
	if usageCacheAge(stateDir, key, now) < usageRefreshEvery {
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
	if usageCacheAge(stateDir, key, now) < usageRefreshEvery {
		return nil
	}
	switch key {
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

func readPigAuth() (pigAuth, error) {
	path := os.Getenv("WICK_PIG_AUTH")
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
		map[string]string{"Authorization": "Bearer " + auth.OpencodeGo.Key, "User-Agent": "wick"}, nil, &r)
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
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh},
		"client_id": {agyClientID}, "client_secret": {agyClientSecret}}
	var r struct {
		Access  string `json:"access_token"`
		Expires int64  `json:"expires_in"`
	}
	err := getJSON(ctx, http.MethodPost, googleTokenURL,
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
