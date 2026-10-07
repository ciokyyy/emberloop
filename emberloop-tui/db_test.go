package tui

import (
	"testing"

	"example.com/emberloop/statedb"
)

// costp boxes a reported cost for fixture rows: the endpoint's zero is data,
// not a missing value, so fixtures that saw a usage object always report.
func costp(v float64) *float64 { return &v }

// writeUsageRows inserts fixture rows into the state dir's database — the
// fixture-database form of the per-agent JSONL logs the TUI used to tail.
// Rows land in insertion order, so row ids reflect the slice order.
func writeUsageRows(t *testing.T, stateDir string, rows []statedb.UsageRow) {
	t.Helper()
	db, err := statedb.Open(statedb.DBPathForDir(stateDir))
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	defer db.Close()
	for _, r := range rows {
		if _, err := statedb.InsertUsageRow(db, r); err != nil {
			t.Fatalf("insert fixture row %+v: %v", r, err)
		}
	}
}
