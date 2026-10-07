package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"example.com/emberloop/statedb"
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

// usageAcc accumulates one agent's usage across incremental reads: request
// sums grow monotonically, while each claude cost-state snapshot replaces
// the last. Snapshots are cumulative per-model totals, and every row the
// core writes for one session shares its start time as At — so timestamps
// cannot tell snapshots apart. Instead the accumulator keeps the latest
// cost per model in row order (rows are append-only, so a later row is a
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

// observeRow folds one usage-table row into the accumulator, by kind.
// Request rows sum their tokens with the harness-reported cost exactly as
// reported; cost rows replace the model's snapshot; find rows count one
// decision-model call with the endpoint's tokens when it returned any. A
// present find cost of zero still counts as reported — the endpoint's zero
// is data, not a missing value. Rows of kinds the TUI does not sum are
// ignored here.
func (a *usageAcc) observeRow(r statedb.UsageRow) {
	switch r.Kind {
	case statedb.KindRequest:
		a.input += r.Input
		a.output += r.Output
		a.cacheRead += r.CacheRead
		a.cacheWrite += r.CacheWrite
		a.reasoning += r.Reasoning
		if r.ReportedCost != nil {
			a.reqCost += *r.ReportedCost
			a.hasReqCost = true
		}
	case statedb.KindCost:
		if r.ReportedCost == nil {
			return
		}
		if a.costByModel == nil {
			a.costByModel = map[string]float64{}
		}
		a.costByModel[r.Model] = *r.ReportedCost
	case statedb.KindFind:
		a.findCalls++
		a.findInput += r.Input
		a.findOutput += r.Output
		if r.ReportedCost != nil {
			a.findCost += *r.ReportedCost
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

// usageCache reads usage rows incrementally from the state database: the row
// id high-water mark kept across calls, so a one-second poll only folds what
// landed since the last tick. Row ids never repeat within one database file,
// so a pruned row never comes back and a fresh row is never missed; a
// replaced database file is detected by identity and starts the accumulation
// over. The zero value reads everything on its first call.
type usageCache struct {
	lastID int64
	accs   map[string]*usageAcc
	// dbDev and dbIno identify the database file last read (with dbKnown):
	// a deleted and recreated database always takes a new identity, so the
	// next poll restarts even when the new file already holds past the old
	// high-water mark.
	dbDev, dbIno uint64
	dbKnown      bool
}

func newUsageCache() *usageCache {
	return &usageCache{accs: map[string]*usageAcc{}}
}

// noteIdentity records the database file's identity, reporting whether it
// changed since the last poll. A changed identity means the file was
// replaced (deleted and recreated), so the caller restarts the accumulation.
// Identity is unavailable on some filesystems; then it reports false and the
// maximum-row check covers the common case.
func (c *usageCache) noteIdentity(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	dev, ino := uint64(st.Dev), st.Ino
	changed := c.dbKnown && (c.dbDev != dev || c.dbIno != ino)
	c.dbDev, c.dbIno, c.dbKnown = dev, ino, true
	return changed
}

// refresh folds every row past the high-water mark into its agent's
// accumulator and advances the mark. A state with no database yet (no usage
// ever recorded) leaves every total empty instead of failing, so a fresh
// checkout still renders; a transient read failure keeps the last totals and
// retries on the next tick. A database file replaced since the last poll
// (deleted and recreated takes a new identity) restarts the accumulation
// from zero instead of skipping every row, even when the new file already
// reaches past the old mark; the maximum-row check below stays as the
// backstop for filesystems where identity is unavailable.
func (c *usageCache) refresh(stateDir string) {
	path := statedb.DBPathForDir(stateDir)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return
	}
	if err == nil && c.noteIdentity(info) {
		c.lastID = 0
		c.accs = map[string]*usageAcc{}
	}
	db, err := statedb.OpenReadOnly(path)
	if err != nil {
		return
	}
	defer db.Close()
	maxID, err := statedb.MaxUsageID(db)
	if err != nil {
		return
	}
	if maxID < c.lastID {
		c.lastID = 0
		c.accs = map[string]*usageAcc{}
	}
	rows, err := statedb.UsageRowsSince(db, c.lastID)
	if err != nil {
		return
	}
	for _, r := range rows {
		acc := c.accs[r.Agent]
		if acc == nil {
			acc = &usageAcc{}
			c.accs[r.Agent] = acc
		}
		acc.observeRow(r)
		if r.ID > c.lastID {
			c.lastID = r.ID
		}
	}
}

// totals renders one agent's accumulated sums.
func (c *usageCache) totals(agentID string) AgentSnap {
	acc := c.accs[agentID]
	if acc == nil {
		return AgentSnap{}
	}
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

// memoryLogPath is one agent's memory log in the archive under its run.
func memoryLogPath(stateDir, run, agentID string) string {
	return filepath.Join(stateDir, "archive", sanitizeArchive(run), sanitizeArchive(agentID), "memory.jsonl")
}
