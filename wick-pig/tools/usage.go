package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"example.com/wick/tools/systemone"
)

// This file owns the per-agent usage log: an append-only JSONL file per
// agent, one line per request or per find call, beside the worker store in
// wick's state directory. Ticket 03's harness tailer appends token-usage
// lines here; ticket 04's find cascade appends decision-call lines. The TUI
// and stats read the files by polling; nothing is ever rewritten.
//
// Layout: <state>/usage/<agent-id>.jsonl. A line is either a find line
// (kind "find", FindUsage below) or a harness request line from ticket 03's
// tailer. Readers tell them apart by kind — ticket 03's format will need its
// own kind value (the spec's request record has none yet) — and count lines
// for call counts when the endpoint returned no usage.

// stateDir is wick's state directory: the worker store's directory. Tests
// point it at a temp dir through WICK_STATE_DIR, the way WICK_SETTINGS
// redirects settings.
func stateDir() string {
	if dir := os.Getenv("WICK_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick")
}

// UsagePath is the log file of one agent.
func UsagePath(agentID string) string {
	return filepath.Join(stateDir(), "usage", agentID+".jsonl")
}

// FindUsage is one decision-model call made by find, logged against the
// calling agent. Each line is one call, so per-agent call counts come from
// counting lines (grouped by FindCallID); Usage and its cost are present
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

// LogFindCall appends one find line to the agent's usage log. An empty agent
// means the call ran outside any session (bare wick-mcp, tests that chose
// not to set one): with no agent to attribute to, it writes nothing and
// reports success.
func LogFindCall(rec FindUsage) error {
	if rec.Agent == "" {
		return nil
	}
	rec.Kind = "find"
	if rec.At == "" {
		rec.At = time.Now().UTC().Format(time.RFC3339)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	path := UsagePath(rec.Agent)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}
