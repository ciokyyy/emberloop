package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AgentKind names the role an agent plays on a ticket or a spec. Reviewer
// kinds reuse the core's built-in type names, so the TUI and the core meet
// on strings without an import.
type AgentKind string

const (
	AgentWorker            AgentKind = "worker"
	AgentOrchestrator      AgentKind = "orchestrator"
	AgentReviewerStandards AgentKind = "reviewer-standards"
	AgentReviewerSpec      AgentKind = "reviewer-spec"
	AgentScout             AgentKind = "scout"
)

// AgentSnap is one agent attached to a ticket (or a spec's orchestrator):
// who it is, how it runs, what the usage logs report and how much memory
// its latest sample holds. Token and cost sums come from kind "request"
// rows only; cost-state "cost" rows contribute the latest snapshot's
// reported cost, never a sum of snapshots. A missing cost stays missing:
// HasCost is false and the view shows "no cost reported" beside the
// tokens, never an estimate.
type AgentSnap struct {
	ID      string
	Kind    AgentKind
	Harness string
	Model   string
	Effort  string
	Pane    string
	Status  string
	Alive   bool

	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	Reasoning  int64
	Cost       float64
	HasCost    bool

	FindCalls   int
	FindInput   int64
	FindOutput  int64
	FindCost    float64
	HasFindCost bool

	RSS    int64
	HasRSS bool
}

// ModelEffort renders the `model · effort` pair the ticket detail shows.
// Without an effort it is just the model; without a model it is "".
func (a AgentSnap) ModelEffort() string {
	if a.Model == "" {
		return ""
	}
	if a.Effort == "" {
		return a.Model
	}
	return a.Model + " · " + a.Effort
}

// Totals sums agents: request tokens and reported costs, find calls with
// their own usage, and current RSS. Cost is the sum of what agents
// reported; HasCost says at least one agent reported any, so a total with
// no reported cost still reads "no cost reported" rather than zero.
type Totals struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	Reasoning  int64
	Cost       float64
	HasCost    bool

	FindCalls   int
	FindInput   int64
	FindOutput  int64
	FindCost    float64
	HasFindCost bool

	RSS    int64
	HasRSS bool
}

// add folds one agent into the totals.
func (t *Totals) add(a AgentSnap) {
	t.Input += a.Input
	t.Output += a.Output
	t.CacheRead += a.CacheRead
	t.CacheWrite += a.CacheWrite
	t.Reasoning += a.Reasoning
	if a.HasCost {
		t.Cost += a.Cost
		t.HasCost = true
	}
	t.FindCalls += a.FindCalls
	t.FindInput += a.FindInput
	t.FindOutput += a.FindOutput
	if a.HasFindCost {
		t.FindCost += a.FindCost
		t.HasFindCost = true
	}
	if a.HasRSS {
		t.RSS += a.RSS
		t.HasRSS = true
	}
}

// usageRow is the slice of a usage-log request or cost row the TUI sums:
// per-request tokens with the harness-reported cost exactly as reported.
// Find rows use findRow instead.
type usageRow struct {
	Kind         string   `json:"kind"`
	Input        int64    `json:"input"`
	Output       int64    `json:"output"`
	CacheRead    int64    `json:"cacheRead"`
	CacheWrite   int64    `json:"cacheWrite"`
	Reasoning    int64    `json:"reasoning"`
	ReportedCost *float64 `json:"reportedCost"`
	At           string   `json:"at"`
	Model        string   `json:"model"`
}

// findRow is the slice of a kind "find" usage line the TUI counts: one
// line is one decision-model call, with the endpoint's usage when it
// returned any. A present usage object with a zero cost still counts as
// reported — the endpoint's zero is data, not a missing value.
type findRow struct {
	Kind       string `json:"kind"`
	FindCallID string `json:"find_call_id"`
	Usage      *struct {
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		Cost         float64 `json:"cost"`
	} `json:"usage"`
}

// usageAcc accumulates one agent's usage across incremental reads: request
// sums grow monotonically, while each claude cost-state snapshot replaces
// the last. Snapshots are cumulative per-model totals, and every row the
// core writes for one session shares its start time as At — so timestamps
// cannot tell snapshots apart. Instead the accumulator keeps the latest
// cost per model in log order (the log is append-only, so a later row is a
// newer snapshot) and sums across models: one snapshot with two models
// sums both rows, while a newer snapshot overwrites its models' costs.
type usageAcc struct {
	input, output, cacheRead, cacheWrite, reasoning int64
	reqCost                                         float64
	hasReqCost                                      bool
	costByModel                                     map[string]float64
	findCalls                                       int
	findInput, findOutput                           int64
	findCost                                        float64
	hasFindCost                                     bool
}

// observe folds one decoded line into the accumulator.
func (a *usageAcc) observe(line []byte) {
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return
	}
	switch probe.Kind {
	case "request":
		var r usageRow
		if err := json.Unmarshal(line, &r); err != nil {
			return
		}
		a.input += r.Input
		a.output += r.Output
		a.cacheRead += r.CacheRead
		a.cacheWrite += r.CacheWrite
		a.reasoning += r.Reasoning
		if r.ReportedCost != nil {
			a.reqCost += *r.ReportedCost
			a.hasReqCost = true
		}
	case "cost":
		var r usageRow
		if err := json.Unmarshal(line, &r); err != nil {
			return
		}
		if r.ReportedCost == nil {
			return
		}
		if a.costByModel == nil {
			a.costByModel = map[string]float64{}
		}
		a.costByModel[r.Model] = *r.ReportedCost
	case "find":
		var r findRow
		if err := json.Unmarshal(line, &r); err != nil {
			return
		}
		a.findCalls++
		if r.Usage != nil {
			a.findInput += r.Usage.InputTokens
			a.findOutput += r.Usage.OutputTokens
			a.findCost += r.Usage.Cost
			a.hasFindCost = true
		}
	}
}

// funds renders the accumulator as agent-level sums.
func (a *usageAcc) funds() (input, output, cacheRead, cacheWrite, reasoning int64, cost float64, hasCost bool, findCalls int, findInput, findOutput int64, findCost float64, hasFindCost bool) {
	cost, hasCost = a.reqCost, a.hasReqCost
	for _, c := range a.costByModel {
		cost += c
		hasCost = true
	}
	return a.input, a.output, a.cacheRead, a.cacheWrite, a.reasoning,
		cost, hasCost, a.findCalls, a.findInput, a.findOutput, a.findCost, a.hasFindCost
}

// usageCache reads usage logs incrementally: per-file byte offsets kept
// across calls, so a one-second poll only parses what each agent appended
// since the last tick. Only complete lines (ending in newline) advance the
// offset, so a half-written line waits for the next pass. Usage logs are
// append-only, never rewritten; a file shorter than its offset restarts
// the accumulation rather than seeking past the new log's start.
type usageCache struct {
	offsets map[string]int64
	accs    map[string]*usageAcc
}

func newUsageCache() *usageCache {
	return &usageCache{offsets: map[string]int64{}, accs: map[string]*usageAcc{}}
}

// totals returns the accumulated sums for one agent, reading whatever the
// log grew by since the last call.
func (c *usageCache) totals(stateDir, agentID string) AgentSnap {
	key := agentID
	acc := c.accs[key]
	if acc == nil {
		acc = &usageAcc{}
		c.accs[key] = acc
	}
	off := c.offsets[key]
	path := usageLogPath(stateDir, agentID)
	f, err := os.Open(path)
	if err != nil {
		return AgentSnap{}
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() < off {
		off = 0
		acc = &usageAcc{}
		c.accs[key] = acc
	}
	if _, err := f.Seek(off, 0); err != nil {
		return AgentSnap{}
	}
	r := bufio.NewReader(f)
	pos := off
	for {
		chunk, err := r.ReadString('\n')
		if err != nil {
			break
		}
		acc.observe([]byte(chunk))
		pos += int64(len(chunk))
	}
	c.offsets[key] = pos
	out := AgentSnap{}
	out.Input, out.Output, out.CacheRead, out.CacheWrite, out.Reasoning,
		out.Cost, out.HasCost, out.FindCalls, out.FindInput, out.FindOutput,
		out.FindCost, out.HasFindCost = acc.funds()
	return out
}

// memorySample is the slice of a memory.jsonl row the TUI reads: when it
// was taken and the process's RSS in bytes.
type memorySample struct {
	At  string `json:"at"`
	RSS int64  `json:"rss"`
}

// latestRSS returns the latest sample's summed RSS: every row sharing the
// file's maximum timestamp. ok is false when the log holds no valid row.
func latestRSS(path string) (rss int64, ok bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var at string
	rows := map[string]int64{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s memorySample
		if err := json.Unmarshal([]byte(line), &s); err != nil || s.At == "" {
			continue
		}
		rows[s.At] += s.RSS
		if s.At > at {
			at = s.At
		}
	}
	if at == "" {
		return 0, false
	}
	return rows[at], true
}

// lockFileName mirrors the core's lockName: anything outside [a-zA-Z0-9-_]
// becomes '-', so the TUI finds the same usage file the core wrote without
// importing it.
func lockFileName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "lock"
	}
	return b.String()
}

// sanitizeArchive mirrors the core's archive name cleaning: path
// separators become '-', so run and agent ids map to one directory.
func sanitizeArchive(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	if s == "" {
		return "session"
	}
	return s
}

// usageLogPath is one agent's usage log beside the worker store.
func usageLogPath(stateDir, agentID string) string {
	return filepath.Join(stateDir, "usage", lockFileName(agentID)+".jsonl")
}

// memoryLogPath is one agent's memory log in the archive under its run.
func memoryLogPath(stateDir, run, agentID string) string {
	return filepath.Join(stateDir, "archive", sanitizeArchive(run), sanitizeArchive(agentID), "memory.jsonl")
}
