package tools

import (
	"os"
	"testing"

	"example.com/emberloop/statedb"
	"example.com/emberloop/tools/systemone"
)

// readFindRows returns the agent's find rows from the state database, in row
// order: LogFindCall writes rows there now, not JSONL lines.
func readFindRows(t *testing.T, agent string) []statedb.UsageRow {
	t.Helper()
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatalf("open state database: %v", err)
	}
	defer db.Close()
	rows, err := statedb.UsageRowsForAgent(db, agent)
	if err != nil {
		t.Fatalf("read usage rows: %v", err)
	}
	var out []statedb.UsageRow
	for _, r := range rows {
		if r.Kind != statedb.KindFind {
			t.Fatalf("row kind = %q, want find", r.Kind)
		}
		out = append(out, r)
	}
	return out
}

func TestLogFindCallWritesOneRowPerCall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EMBERLOOP_STATE_DIR", dir)

	rec := FindUsage{
		Agent:      "agent-1",
		Model:      systemone.JevModelPinned,
		FindCallID: "find-abc",
		Questions:  16,
		Generation: "gen-dec-1",
		Provider:   "TypeSafe",
		Usage:      &systemone.Usage{InputTokens: 278, OutputTokens: 21, Cost: 0.000011676},
	}
	if err := LogFindCall(rec); err != nil {
		t.Fatalf("LogFindCall: %v", err)
	}

	rows := readFindRows(t, "agent-1")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Agent != "agent-1" || row.Kind != statedb.KindFind {
		t.Errorf("row identity = %+v", row)
	}
	if row.Model != systemone.JevModelPinned || row.FindCallID != "find-abc" {
		t.Errorf("row attribution = %+v", row)
	}
	if row.FindQuestions != 16 {
		t.Errorf("row questions = %d, want 16", row.FindQuestions)
	}
	if row.Input != 278 || row.Output != 21 {
		t.Errorf("row usage = %d/%d, want 278/21", row.Input, row.Output)
	}
	if row.ReportedCost == nil || *row.ReportedCost != 0.000011676 {
		t.Errorf("row cost = %v, want 0.000011676", row.ReportedCost)
	}
	if row.Generation != "gen-dec-1" || row.Provider != "TypeSafe" {
		t.Errorf("row provenance = %+v", row)
	}
	if row.At == "" {
		t.Errorf("row has no timestamp: %+v", row)
	}
}

func TestLogFindCallWithoutUsageStillLogsTheCall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EMBERLOOP_STATE_DIR", dir)

	if err := LogFindCall(FindUsage{Agent: "agent-2", Model: "m", FindCallID: "f", Questions: 3}); err != nil {
		t.Fatalf("LogFindCall: %v", err)
	}
	rows := readFindRows(t, "agent-2")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ReportedCost != nil {
		t.Errorf("row carries cost = %v, want none", *row.ReportedCost)
	}
	if row.Agent != "agent-2" || row.Kind != statedb.KindFind {
		t.Errorf("row = %+v", row)
	}
}

func TestLogFindCallWithoutAgentWritesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EMBERLOOP_STATE_DIR", dir)

	if err := LogFindCall(FindUsage{Model: "m", FindCallID: "f", Questions: 1}); err != nil {
		t.Fatalf("LogFindCall: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) > 0 {
		t.Errorf("wrote %v without an agent id", entries)
	}
}
