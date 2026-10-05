package wick

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// UsageRecord is one line in an agent's usage log: the raw per-request token
// breakdown with the harness-reported cost exactly as reported. No price table
// and no wick-side cost calculation: a missing cost stays missing.
// Kind is "request" for per-request rows; ticket 04's find-call rows use
// "find", and claude's cumulative cost-state snapshots use "cost", so a
// reader summing tokens knows which rows are per-request.
type UsageRecord struct {
	Agent        string   `json:"agent"`
	At           string   `json:"at"`
	Harness      string   `json:"harness"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Input        int64    `json:"input"`
	Output       int64    `json:"output"`
	CacheRead    int64    `json:"cacheRead"`
	CacheWrite   int64    `json:"cacheWrite"`
	Reasoning    int64    `json:"reasoning"`
	ReportedCost *float64 `json:"reportedCost,omitempty"`
	Kind         string   `json:"kind"`
}

// UsageParser turns transcript lines into usage records. One transcript line
// yields zero or more records plus the dedupe key the tailer remembers, so a
// line read again (a restarted tail) is never logged twice. The key is empty
// exactly when there are no records.
type UsageParser interface {
	ParseLine(agent, line string) (records []UsageRecord, key string)
}

// UsageParserFor returns the parser for a harness, or false for one whose
// transcript carries no usage wick reads.
func UsageParserFor(harness string) (UsageParser, bool) {
	switch harness {
	case "pig":
		return pigUsageParser{}, true
	case "claude":
		return claudeUsageParser{}, true
	}
	return nil, false
}

// pigUsageParser reads pig's session transcript: every assistant message
// carries its request's usage, including the cost pig reports for it.
type pigUsageParser struct{}

func (pigUsageParser) ParseLine(agent, line string) ([]UsageRecord, string) {
	var rec struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			Role     string `json:"role"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Usage    *struct {
				Input      int64 `json:"input"`
				Output     int64 `json:"output"`
				CacheRead  int64 `json:"cacheRead"`
				CacheWrite int64 `json:"cacheWrite"`
				Reasoning  int64 `json:"reasoning"`
				Cost       *struct {
					Total float64 `json:"total"`
				} `json:"cost"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil {
		return nil, ""
	}
	if rec.Type != "message" || rec.Message.Role != "assistant" || rec.Message.Usage == nil {
		return nil, ""
	}
	out := UsageRecord{Agent: agent, At: rec.Timestamp, Harness: "pig",
		Provider: rec.Message.Provider, Model: rec.Message.Model,
		Input: rec.Message.Usage.Input, Output: rec.Message.Usage.Output,
		CacheRead: rec.Message.Usage.CacheRead, CacheWrite: rec.Message.Usage.CacheWrite,
		Reasoning: rec.Message.Usage.Reasoning, Kind: "request"}
	if rec.Message.Usage.Cost != nil {
		cost := rec.Message.Usage.Cost.Total
		out.ReportedCost = &cost
	}
	// A record-bearing line with no id still needs a distinct key, or every
	// such line would share one and only the first would log. The line hash
	// keeps them apart; real transcripts always carry ids, so this is only
	// the backstop.
	key := rec.ID
	if key == "" {
		return []UsageRecord{out}, lineFallback("pig", line)
	}
	return []UsageRecord{out}, "pig:" + key
}

// lineFallback keys a record-bearing line that carries no id at all, so
// distinct id-less requests never share one dedupe key.
func lineFallback(prefix, line string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(line))
	return prefix + ":line:" + strconv.FormatUint(h.Sum64(), 16)
}

// transcript names models but no provider; Claude Code speaks the Anthropic
// API, so that is what the provider column carries.
const claudeProvider = "anthropic"

// claudeUsageParser reads claude's transcript: one request streams as several
// assistant blocks sharing a message id with identical usage, so the message
// id is the dedupe key. Per-message usage carries no cost — claude reports
// cost only in its cost-state record — so request rows report none and the
// cost-state rows carry the reported totals.
type claudeUsageParser struct{}

func (claudeUsageParser) ParseLine(agent, line string) ([]UsageRecord, string) {
	var rec struct {
		Type      string `json:"type"`
		UUID      string `json:"uuid"`
		Timestamp string `json:"timestamp"`
		RequestID string `json:"requestId"`
		Message   *struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				Input      int64 `json:"input_tokens"`
				Output     int64 `json:"output_tokens"`
				CacheRead  int64 `json:"cache_read_input_tokens"`
				CacheWrite int64 `json:"cache_creation_input_tokens"`
				Thinking   *struct {
					Thinking int64 `json:"thinking_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"message"`
		TotalCost *float64 `json:"totalCostUSD"`
		StartTime *int64   `json:"startTime"`
		Models    map[string]struct {
			Input      int64   `json:"inputTokens"`
			Output     int64   `json:"outputTokens"`
			Thinking   int64   `json:"thinkingTokens"`
			CacheRead  int64   `json:"cacheReadInputTokens"`
			CacheWrite int64   `json:"cacheCreationInputTokens"`
			Cost       float64 `json:"costUSD"`
		} `json:"modelUsage"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil {
		return nil, ""
	}
	switch rec.Type {
	case "assistant":
		if rec.Message == nil || rec.Message.Usage == nil {
			return nil, ""
		}
		reasoning := int64(0)
		if rec.Message.Usage.Thinking != nil {
			reasoning = rec.Message.Usage.Thinking.Thinking
		}
		out := UsageRecord{Agent: agent, At: stamp(rec.Timestamp), Harness: "claude",
			Provider: claudeProvider, Model: rec.Message.Model,
			Input: rec.Message.Usage.Input, Output: rec.Message.Usage.Output,
			CacheRead: rec.Message.Usage.CacheRead, CacheWrite: rec.Message.Usage.CacheWrite,
			Reasoning: reasoning, Kind: "request"}
		key := rec.Message.ID
		if key == "" {
			key = rec.RequestID
		}
		if key == "" {
			key = rec.UUID
		}
		if key == "" {
			return []UsageRecord{out}, lineFallback("claude", line)
		}
		return []UsageRecord{out}, "claude:" + key
	case "cost-state":
		if len(rec.Models) == 0 {
			return nil, ""
		}
		// The cost-state line carries no timestamp; At is the session start
		// it reports, the only time the line holds.
		at := ""
		if rec.StartTime != nil {
			at = time.UnixMilli(*rec.StartTime).UTC().Format(time.RFC3339)
		}
		names := make([]string, 0, len(rec.Models))
		for name := range rec.Models {
			names = append(names, name)
		}
		sort.Strings(names)
		var out []UsageRecord
		for _, name := range names {
			m := rec.Models[name]
			cost := m.Cost
			out = append(out, UsageRecord{Agent: agent, At: at, Harness: "claude",
				Provider: claudeProvider, Model: name,
				Input: m.Input, Output: m.Output,
				CacheRead: m.CacheRead, CacheWrite: m.CacheWrite,
				Reasoning: m.Thinking, ReportedCost: &cost, Kind: "cost"})
		}
		// The key fingerprints the snapshot: an unchanged cost-state the
		// tailer meets twice logs once. Go marshals maps with sorted keys,
		// so the fingerprint is stable for the same snapshot.
		raw, _ := json.Marshal(struct {
			Total  *float64 `json:"totalCostUSD"`
			Models any      `json:"modelUsage"`
		}{rec.TotalCost, rec.Models})
		return out, "claude-cost:" + string(raw)
	}
	return nil, ""
}

// stamp keeps a transcript timestamp as-is, falling back to now when the
// line carries none.
func stamp(ts string) string {
	if ts != "" {
		return ts
	}
	return time.Now().UTC().Format(time.RFC3339)
}

// ---------------------------------------------------------------------------
// tail
// ---------------------------------------------------------------------------

// usageDir holds the per-agent usage logs, beside the worker store, so the
// TUI and stats can read them as plain files. WICK_STATE_DIR redirects it
// the way it redirects the rest of the state dir: ticket 04's find lines
// share these files, and must land in the same directory under that seam.
func usageDir() string {
	if dir := os.Getenv("WICK_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "usage")
	}
	return filepath.Join(filepath.Dir(statePath), "usage")
}

// UsageLogPath is the agent's append-only usage log: one JSON record per
// line, one line per request (or per find call, or per cost snapshot).
func UsageLogPath(agent string) string {
	return filepath.Join(usageDir(), lockName(agent)+".jsonl")
}

// usageTailPath is where the tailer keeps its byte offset and seen keys.
func usageTailPath(agent string) string {
	return filepath.Join(usageDir(), lockName(agent)+".tail")
}

// usageTailState is the tailer's resume state: the byte offset past the last
// complete line read, and the dedupe keys already logged, oldest first.
type usageTailState struct {
	Offset int64    `json:"offset"`
	Seen   []string `json:"seen"`
}

// usageSeenCap bounds the dedupe memory: keys for lines already passed by the
// offset can never be re-read, so only recent keys matter (a replaced
// transcript resets the offset but keeps them).
const usageSeenCap = 4096

func loadUsageTail(agent string) (usageTailState, error) {
	var st usageTailState
	raw, err := os.ReadFile(usageTailPath(agent))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return usageTailState{}, fmt.Errorf("usage tail state is corrupt: %w", err)
	}
	return st, nil
}

func saveUsageTail(agent string, st usageTailState) error {
	if len(st.Seen) > usageSeenCap {
		st.Seen = st.Seen[len(st.Seen)-usageSeenCap:]
	}
	if err := os.MkdirAll(usageDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := usageTailPath(agent) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, usageTailPath(agent))
}

// appendUsageRecords appends records to the agent's log, one JSON object per
// line.
func appendUsageRecords(agent string, recs []UsageRecord) error {
	if err := os.MkdirAll(usageDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(UsageLogPath(agent), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, r := range recs {
		raw, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// TailUsage reads the transcript lines appended since the stored offset,
// parses each with the harness's parser, and appends unseen records to the
// agent's usage log. It returns how many records were appended. Only complete
// lines (ending in a newline) move the offset, so a half-written line waits
// for the next pass; a missing transcript is no error, only a harness that
// has not written one yet. The offset and key land before the records they
// cover, so killing the tailer mid-pass can lose the row it was writing but
// never logs one twice. A settled session's final pass is a plain re-tail:
// with nothing new it appends nothing.
func TailUsage(transcriptPath, agent, harness string) (int, error) {
	p, ok := UsageParserFor(harness)
	if !ok {
		return 0, fmt.Errorf("no usage parser for harness %q", harness)
	}
	appended := 0
	err := withFileLock("usage-"+lockName(agent), func() error {
		st, err := loadUsageTail(agent)
		if err != nil {
			return err
		}
		seen := make(map[string]bool, len(st.Seen)+16)
		for _, k := range st.Seen {
			seen[k] = true
		}
		f, err := os.Open(transcriptPath)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close()
		if info, err := f.Stat(); err != nil {
			return err
		} else if info.Size() < st.Offset {
			// The transcript was replaced, not appended to: start over,
			// keeping the keys so nothing already logged logs again.
			st.Offset = 0
		}
		if _, err := f.Seek(st.Offset, io.SeekStart); err != nil {
			return err
		}
		r := bufio.NewReader(f)
		pos := st.Offset
		for {
			chunk, err := r.ReadString('\n')
			if err != nil {
				// EOF, with or without a partial trailing line: the
				// partial line stays for the next pass.
				break
			}
			recs, key := p.ParseLine(agent, chunk)
			pos += int64(len(chunk))
			st.Offset = pos
			fresh := key != "" && !seen[key]
			if fresh {
				seen[key] = true
				st.Seen = append(st.Seen, key)
			}
			// State lands before the records it covers: a kill between the two
			// re-reads the line but finds its key already stored, so the row
			// may be lost but is never logged twice.
			if err := saveUsageTail(agent, st); err != nil {
				return err
			}
			if fresh && len(recs) > 0 {
				if err := appendUsageRecords(agent, recs); err != nil {
					return err
				}
				appended += len(recs)
			}
		}
		return nil
	})
	return appended, err
}

// ---------------------------------------------------------------------------
// session tail
// ---------------------------------------------------------------------------

// usageTailEnv names a harness session for its wick-mcp: the harness, the id
// the transcript file carries, and the session dir when the harness takes
// one. An empty session id means no transcript to tail (a headless or bare
// run), and the tailer stays off.
func usageTailEnv(harness, sessionID, dir string) []string {
	env := []string{"WICK_HARNESS=" + harness}
	if sessionID != "" {
		env = append(env, "WICK_SESSION_ID="+sessionID)
	}
	if dir != "" {
		env = append(env, "WICK_SESSION_DIR="+dir)
	}
	return env
}

// UsageTailConfig is what the session's own wick process needs to tail its
// harness's transcript: the harness, the session id naming the transcript,
// the agent id owning the usage log, and the session dir for harnesses that
// take one.
type UsageTailConfig struct {
	Harness   string
	SessionID string
	AgentID   string
	Dir       string
}

// UsageTailFromEnv reads the tail config from the session environment the
// harness launch sets. ok is false when this process serves no harness
// session with a tail-able transcript (a manual wick-mcp run, a headless
// session, an unknown harness), and the tailer stays off.
func UsageTailFromEnv() (cfg UsageTailConfig, ok bool) {
	cfg = UsageTailConfig{
		Harness:   os.Getenv("WICK_HARNESS"),
		SessionID: os.Getenv("WICK_SESSION_ID"),
		AgentID:   os.Getenv("WICK_AGENT_ID"),
		Dir:       os.Getenv("WICK_SESSION_DIR"),
	}
	if cfg.Harness == "" || cfg.SessionID == "" || cfg.AgentID == "" {
		return UsageTailConfig{}, false
	}
	if _, known := UsageParserFor(cfg.Harness); !known {
		return UsageTailConfig{}, false
	}
	return cfg, true
}

// TailUsageFile resolves the harness transcript for cfg and tails it once.
// An unstarted harness (no transcript file yet) is quiet, not an error. It
// is both the loop's tick and the final pass when the session settles.
func TailUsageFile(cfg UsageTailConfig) error {
	h := Registered(cfg.Harness)
	if h == nil {
		return fmt.Errorf("no harness %q", cfg.Harness)
	}
	path := findSession(h.SessionRoot(Session{Dir: cfg.Dir}), cfg.SessionID)
	if path == "" {
		return nil
	}
	_, err := TailUsage(path, cfg.AgentID, cfg.Harness)
	return err
}

// usageTailEvery is the tail poll interval; a var so tests run it fast. Five
// seconds matches the memory sampler's cadence: usage rows land promptly
// enough for live stats without churning on every line the harness writes.
var usageTailEvery = 5 * time.Second

// RunUsageTail tails the harness transcript until stop closes, for the
// session's own wick process. A failed pass is skipped, not fatal: the
// records stay in the transcript for the next tick or the final pass. The
// caller runs one last TailUsageFile after the harness settles, so requests
// written after the final tick still land.
func RunUsageTail(cfg UsageTailConfig, stop <-chan struct{}) {
	_ = TailUsageFile(cfg)
	t := time.NewTicker(usageTailEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = TailUsageFile(cfg)
		}
	}
}
