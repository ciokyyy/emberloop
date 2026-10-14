package el

// Claude account usage for the strip: the 5-hour and weekly rate-limit
// windows. Claude hands its statusline command a JSON blob after every
// assistant message, with rate_limits.five_hour and rate_limits.seven_day.
// `el claude-status` is that command: it stores the two windows in one
// small file in el's state dir, and every strip reads the file on its
// poll. The numbers are account-wide, so the newest write from any claude
// pane wins.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/emberloop-web/el/statedb"
)

// UsageWindow is one rate-limit window: percent used and when it resets
// (unix seconds).
type UsageWindow struct {
	Label    string `json:"label,omitempty"`
	Pct      int    `json:"pct"`
	ResetsAt int64  `json:"resets_at"`
}

// ClaudeUsage is the account's two windows. Has is false until claude has
// reported at least one.
type ClaudeUsage struct {
	FiveHour UsageWindow `json:"five_hour"`
	SevenDay UsageWindow `json:"seven_day"`
	Has      bool        `json:"-"`
}

func claudeUsagePath(stateDir string) string {
	if stateDir == "" {
		stateDir = statedb.StateDir()
	}
	return filepath.Join(stateDir, "claude-usage.json")
}

// statusInput is the part of claude's statusline JSON this reads.
type statusInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	RateLimits     struct {
		FiveHour *statusWindow `json:"five_hour"`
		SevenDay *statusWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type statusWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// RunClaudeStatus is claude's statusline command. It reads the JSON on in,
// stores the rate-limit windows it carries, and prints nothing, so claude
// draws no status row of its own. A blob without rate_limits (a pay-per-use
// account, or before the first reply) leaves the stored file alone. Errors
// never surface: a statusline command that fails would put noise in the pane.
func RunClaudeStatus(in io.Reader, stateDir string) error {
	var s statusInput
	if err := json.NewDecoder(in).Decode(&s); err != nil {
		return nil
	}
	storeLiveTranscript(stateDir, os.Getenv("EL_AGENT_ID"), s.TranscriptPath)
	if s.RateLimits.FiveHour == nil && s.RateLimits.SevenDay == nil {
		return nil
	}
	cur, _ := readClaudeUsageRaw(stateDir)
	if w := s.RateLimits.FiveHour; w != nil {
		cur.FiveHour = UsageWindow{Pct: int(w.UsedPercentage + 0.5), ResetsAt: w.ResetsAt}
	}
	if w := s.RateLimits.SevenDay; w != nil {
		cur.SevenDay = UsageWindow{Pct: int(w.UsedPercentage + 0.5), ResetsAt: w.ResetsAt}
	}
	raw, err := json.Marshal(cur)
	if err != nil {
		return nil
	}
	path := claudeUsagePath(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return nil
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
	return nil
}

func readClaudeUsageRaw(stateDir string) (ClaudeUsage, bool) {
	raw, err := os.ReadFile(claudeUsagePath(stateDir))
	if err != nil {
		return ClaudeUsage{}, false
	}
	var u ClaudeUsage
	if json.Unmarshal(raw, &u) != nil {
		return ClaudeUsage{}, false
	}
	u.Has = u.FiveHour.ResetsAt > 0 || u.SevenDay.ResetsAt > 0
	return u, true
}

// ReadClaudeUsage is the stored usage as of now. A window whose reset time
// has passed has rolled over since claude last reported, so it reads 0%.
func ReadClaudeUsage(stateDir string, now time.Time) ClaudeUsage {
	u, ok := readClaudeUsageRaw(stateDir)
	if !ok || !u.Has {
		return ClaudeUsage{}
	}
	if u.FiveHour.ResetsAt > 0 && u.FiveHour.ResetsAt <= now.Unix() {
		u.FiveHour.Pct = 0
	}
	if u.SevenDay.ResetsAt > 0 && u.SevenDay.ResetsAt <= now.Unix() {
		u.SevenDay.Pct = 0
	}
	return u
}

// usageResetIn is the time to a reset as the strip shows it: "4d2h",
// "1h40m", "25m", or "now" once it has passed.
func usageResetIn(resetsAt int64, now time.Time) string {
	d := time.Unix(resetsAt, 0).Sub(now)
	switch {
	case resetsAt <= 0:
		return ""
	case d <= 0:
		return "now"
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int((d+time.Minute-1)/time.Minute))
}

// usagePie is a glyph that fills with the percent: ○ ◔ ◑ ◕ ●.
func usagePie(pct int) string {
	switch {
	case pct < 13:
		return "○"
	case pct < 38:
		return "◔"
	case pct < 63:
		return "◑"
	case pct < 88:
		return "◕"
	}
	return "●"
}

// Set is the two windows as the strip's generic usage view: 5h then wk. A
// window claude has not reported yet is left out.
func (u ClaudeUsage) Set() UsageSet {
	if !u.Has {
		return UsageSet{}
	}
	var ws []UsageWindow
	if u.FiveHour.ResetsAt > 0 {
		ws = append(ws, UsageWindow{Label: "5h", Pct: u.FiveHour.Pct, ResetsAt: u.FiveHour.ResetsAt})
	}
	if u.SevenDay.ResetsAt > 0 {
		ws = append(ws, UsageWindow{Label: "wk", Pct: u.SevenDay.Pct, ResetsAt: u.SevenDay.ResetsAt})
	}
	return UsageSet{Has: len(ws) > 0, Windows: ws}
}

// liveTranscriptPath is the pointer file for the transcript an agent's
// claude is writing right now. A /resume inside a running claude switches
// to another session id and file, so the id from launch no longer finds the
// transcript; claude's statusline JSON always names the current one.
func liveTranscriptPath(stateDir, agentID string) string {
	if stateDir == "" {
		stateDir = statedb.StateDir()
	}
	return filepath.Join(stateDir, "claude-transcripts", agentID+".path")
}

// storeLiveTranscript records the transcript path the statusline reports for
// an agent. It writes only when the path changed and never fails loudly.
func storeLiveTranscript(stateDir, agentID, transcript string) {
	if agentID == "" || transcript == "" || strings.ContainsAny(agentID, `/\`) {
		return
	}
	path := liveTranscriptPath(stateDir, agentID)
	if cur, err := os.ReadFile(path); err == nil && string(cur) == transcript {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if os.WriteFile(tmp, []byte(transcript), 0o644) != nil {
		return
	}
	if os.Rename(tmp, path) != nil {
		_ = os.Remove(tmp)
	}
}
