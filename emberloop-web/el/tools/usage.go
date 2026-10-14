package tools

import (
	"path/filepath"
	"time"

	"example.com/emberloop-web/el/statedb"
	"example.com/emberloop-web/el/tools/systemone"
)

// This file owns find's usage attribution: one row per decision-model call
// against the calling agent, in the state database beside the harness
// tailer's request and cost rows. Ticket 03's tailer writes token-usage rows
// there through the same table; the TUI and stats read rows by kind, and
// prune deletes them per run. The legacy layout (<state>/usage/<agent-id>.jsonl,
// one line per call) stays on disk for the database's one-time import and
// until prune removes it.
//
// stateDir is el's state directory: the worker store's directory. Tests
// point it at a temp dir through EL_STATE_DIR, the way EL_SETTINGS
// redirects settings.
func stateDir() string {
	return statedb.StateDir()
}

// UsagePath is one agent's legacy usage log file.
func UsagePath(agentID string) string {
	return filepath.Join(stateDir(), "usage", agentID+".jsonl")
}

// FindUsage is one decision-model call made by find, logged against the
// calling agent. Each call is one row, so per-agent call counts come from
// counting rows (grouped by FindCallID); Usage and its cost are present
// only when the endpoint returned them.
type FindUsage struct {
	Agent string `json:"agent"`
	At    string `json:"at"`
	Kind  string `json:"kind"` // always "find"
	// Model is the decision model find asked for; the endpoint's resolved
	// release, when it reports one, is not stored separately.
	Model      string           `json:"model"`
	FindCallID string           `json:"find_call_id"`
	Questions  int              `json:"questions"`
	Generation string           `json:"generation,omitempty"`
	Provider   string           `json:"provider,omitempty"`
	Usage      *systemone.Usage `json:"usage,omitempty"`
}

// LogFindCall records one find call against the agent's usage rows. An empty
// agent means the call ran outside any session (bare el-mcp, tests that
// chose not to set one): with no agent to attribute to, it writes nothing
// and reports success.
func LogFindCall(rec FindUsage) error {
	if rec.Agent == "" {
		return nil
	}
	rec.Kind = "find"
	if rec.At == "" {
		rec.At = time.Now().UTC().Format(time.RFC3339)
	}
	row := statedb.UsageRow{
		Agent: rec.Agent, At: rec.At, Model: rec.Model, Kind: statedb.KindFind,
		Provider: rec.Provider, FindCallID: rec.FindCallID,
		FindQuestions: int64(rec.Questions), Generation: rec.Generation,
	}
	// A present usage object with a zero cost still counts as reported —
	// the endpoint's zero is data, not a missing value.
	if rec.Usage != nil {
		row.Input, row.Output = int64(rec.Usage.InputTokens), int64(rec.Usage.OutputTokens)
		cost := rec.Usage.Cost
		row.ReportedCost = &cost
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = statedb.InsertUsageRow(db, row)
	return err
}
