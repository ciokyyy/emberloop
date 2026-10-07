package statedb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Usage kinds. Request rows sum per-request tokens; cost rows hold one
// cumulative claude snapshot per model (the latest snapshot per model wins);
// find rows hold one decision-model call each; search rows arrive with the
// websearch ticket.
const (
	KindRequest = "request"
	KindCost    = "cost"
	KindFind    = "find"
	KindSearch  = "search"
)

// UsageRow is one row of the usage table: the database form of the JSONL
// records the tailer and find logging used to append. ReportedCost is nil
// exactly when the harness reported no cost, so a missing cost stays missing;
// DedupeKey is empty exactly when the row needs no idempotence (find calls
// log every call). Find columns apply to kind "find" only.
type UsageRow struct {
	ID            int64
	Agent         string
	At            string
	Harness       string
	Provider      string
	Model         string
	Kind          string
	Input         int64
	Output        int64
	CacheRead     int64
	CacheWrite    int64
	Reasoning     int64
	ReportedCost  *float64
	DedupeKey     string
	FindCallID    string
	FindQuestions int64
	Generation    string
}

// costPtr boxes a reported cost: the endpoint's zero is data, not a missing
// value, so callers that saw a usage object always report, even at zero.
func costPtr(c float64) *float64 { return &c }

// InsertUsageRow inserts one usage row. A non-empty dedupe key inserts
// idempotently (a row with the same key already there wins and inserted is
// false); an empty key always inserts. Callers hold the agent's file lock
// across their check-then-write passes where they need line atomicity; the
// unique index is the backstop for races outside it.
func InsertUsageRow(db *sql.DB, row UsageRow) (inserted bool, err error) {
	stmt := `INSERT INTO usage(agent, at, harness, provider, model, kind,
		input_tokens, output_tokens, cache_read, cache_write, reasoning,
		reported_cost, dedupe_key, find_call_id, find_questions, generation)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	args := []any{row.Agent, row.At, row.Harness, row.Provider, row.Model, row.Kind,
		row.Input, row.Output, row.CacheRead, row.CacheWrite, row.Reasoning,
		row.ReportedCost, nullIfEmpty(row.DedupeKey), row.FindCallID, row.FindQuestions, row.Generation}
	if row.DedupeKey != "" {
		stmt = `INSERT OR IGNORE INTO usage(agent, at, harness, provider, model, kind,
			input_tokens, output_tokens, cache_read, cache_write, reasoning,
			reported_cost, dedupe_key, find_call_id, find_questions, generation)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	}
	res, err := db.Exec(stmt, args...)
	if err != nil {
		return false, fmt.Errorf("statedb: insert usage: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("statedb: insert usage: %w", err)
	}
	return n > 0, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// scanUsageRow reads one usage row from a query over the usage table's
// columns in canonical order (see usageColumns).
const usageColumns = `id, agent, at, harness, provider, model, kind,
	input_tokens, output_tokens, cache_read, cache_write, reasoning,
	reported_cost, dedupe_key, find_call_id, find_questions, generation`

func scanUsageRow(scanner interface{ Scan(...any) error }) (UsageRow, error) {
	var r UsageRow
	var dedupe sql.NullString
	if err := scanner.Scan(&r.ID, &r.Agent, &r.At, &r.Harness, &r.Provider,
		&r.Model, &r.Kind, &r.Input, &r.Output, &r.CacheRead, &r.CacheWrite,
		&r.Reasoning, &r.ReportedCost, &dedupe, &r.FindCallID,
		&r.FindQuestions, &r.Generation); err != nil {
		return UsageRow{}, err
	}
	if dedupe.Valid {
		r.DedupeKey = dedupe.String
	}
	return r, nil
}

// UsageRowsSince returns every usage row past id, in row order: the
// incremental poll the TUI runs each tick. A fresh reader starts at 0 and
// sees everything.
func UsageRowsSince(db *sql.DB, sinceID int64) ([]UsageRow, error) {
	rows, err := db.Query(`SELECT `+usageColumns+` FROM usage WHERE id > ? ORDER BY id`, sinceID)
	if err != nil {
		return nil, fmt.Errorf("statedb: usage since %d: %w", sinceID, err)
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		r, err := scanUsageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("statedb: scan usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UsageRowsForAgent returns every row for one agent, in row order.
func UsageRowsForAgent(db *sql.DB, agent string) ([]UsageRow, error) {
	rows, err := db.Query(`SELECT `+usageColumns+` FROM usage WHERE agent = ? ORDER BY id`, agent)
	if err != nil {
		return nil, fmt.Errorf("statedb: usage for %q: %w", agent, err)
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		r, err := scanUsageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("statedb: scan usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountUsage returns how many usage rows the database holds.
func CountUsage(db *sql.DB) (int64, error) {
	var n int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage`).Scan(&n); err != nil {
		return 0, fmt.Errorf("statedb: count usage: %w", err)
	}
	return n, nil
}

// DeleteUsageForAgents deletes every usage row for the named agents,
// returning how many rows went. Prune calls this for the agents that belong
// only to pruned runs.
func DeleteUsageForAgents(db *sql.DB, agents []string) (int64, error) {
	if len(agents) == 0 {
		return 0, nil
	}
	placeholders := strings.Repeat("?,", len(agents))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(agents))
	for i, a := range agents {
		args[i] = a
	}
	res, err := db.Exec(`DELETE FROM usage WHERE agent IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("statedb: delete usage: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("statedb: delete usage: %w", err)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// one-time import
// ---------------------------------------------------------------------------

// importLine is the union of the legacy JSONL shapes sitting in the usage
// directory: request and cost rows from the harness tailer (flat token
// fields) and find rows from find logging (a nested usage object with its
// call id). Readers tell them apart by kind.
type importLine struct {
	Agent        string   `json:"agent"`
	At           string   `json:"at"`
	Harness      string   `json:"harness"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Kind         string   `json:"kind"`
	Input        int64    `json:"input"`
	Output       int64    `json:"output"`
	CacheRead    int64    `json:"cacheRead"`
	CacheWrite   int64    `json:"cacheWrite"`
	Reasoning    int64    `json:"reasoning"`
	ReportedCost *float64 `json:"reportedCost"`
	FindCallID   string   `json:"find_call_id"`
	Questions    int64    `json:"questions"`
	Generation   string   `json:"generation"`
	Usage        *struct {
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		Cost         float64 `json:"cost"`
	} `json:"usage"`
}

// lineDedupeKey fingerprints one imported line, so re-importing the same
// files inserts nothing: the same line always maps to the same key, and the
// unique index ignores the repeat.
func lineDedupeKey(agent, line string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(line))
	return "import:" + agent + ":" + strconv.FormatUint(h.Sum64(), 16)
}

// EnsureImport reads the legacy per-agent JSONL logs in the state dir's usage
// directory into the database. It runs only while the usage table is empty —
// the first open after the migration — and inserts idempotently, so calling
// it again (or opening again) never double counts. The old files stay where
// they are until prune removes them.
func EnsureImport(db *sql.DB, stateDir string) (int64, error) {
	n, err := CountUsage(db)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, nil
	}
	return ImportUsageDir(db, filepath.Join(stateDir, "usage"))
}

// ImportUsageDir reads every *.jsonl file in dir into the database,
// idempotently by line fingerprint. It returns how many rows were newly
// inserted.
func ImportUsageDir(db *sql.DB, dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("statedb: import: %w", err)
	}
	var inserted int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		agent := strings.TrimSuffix(e.Name(), ".jsonl")
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return inserted, fmt.Errorf("statedb: import: %w", err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			row, ok := importRow(agent, line)
			if !ok {
				continue
			}
			yes, err := InsertUsageRow(db, row)
			if err != nil {
				return inserted, err
			}
			if yes {
				inserted++
			}
		}
	}
	return inserted, nil
}

// importRow maps one legacy JSONL line to its usage row. ok is false for
// lines that carry no usage (blank, corrupt, or a kind nobody logs).
func importRow(fileAgent, line string) (row UsageRow, ok bool) {
	var l importLine
	if err := json.Unmarshal([]byte(line), &l); err != nil {
		return UsageRow{}, false
	}
	agent := l.Agent
	if agent == "" {
		agent = fileAgent
	}
	switch l.Kind {
	case KindRequest, KindCost:
		return UsageRow{Agent: agent, At: l.At, Harness: l.Harness,
			Provider: l.Provider, Model: l.Model, Kind: l.Kind,
			Input: l.Input, Output: l.Output,
			CacheRead: l.CacheRead, CacheWrite: l.CacheWrite,
			Reasoning: l.Reasoning, ReportedCost: l.ReportedCost,
			DedupeKey: lineDedupeKey(agent, line)}, true
	case KindFind:
		r := UsageRow{Agent: agent, At: l.At, Harness: l.Harness,
			Provider: l.Provider, Model: l.Model, Kind: KindFind,
			FindCallID: l.FindCallID, FindQuestions: l.Questions,
			Generation: l.Generation,
			DedupeKey:  lineDedupeKey(agent, line)}
		if l.Usage != nil {
			r.Input, r.Output = l.Usage.InputTokens, l.Usage.OutputTokens
			r.ReportedCost = costPtr(l.Usage.Cost)
		}
		return r, true
	}
	return UsageRow{}, false
}
