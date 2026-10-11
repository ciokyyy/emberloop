package memory

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openTestEngine opens an engine on a fresh directory with a fixed clock. The
// engine is closed before the directory is removed.
func openTestEngine(t *testing.T) (*Engine, string, *testClock) {
	t.Helper()
	dir := t.TempDir()
	clock := newTestClock()
	eng, err := Open(dir, Options{Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng, dir, clock
}

func openTestBank(t *testing.T) (*Bank, string, *testClock) {
	t.Helper()
	eng, dir, clock := openTestEngine(t)
	b, err := eng.Bank(testUser)
	if err != nil {
		t.Fatal(err)
	}
	return b, dir, clock
}

func TestEngineRejectsBadUserIDs(t *testing.T) {
	eng, _, _ := openTestEngine(t)
	for _, id := range []string{"", "../escape", "a/b", "a b", "-lead"} {
		if _, err := eng.Bank(id); !errors.Is(err, ErrInvalid) {
			t.Errorf("Bank(%q) err = %v, want ErrInvalid", id, err)
		}
	}
}

func TestEngineBankFileAndReuse(t *testing.T) {
	eng, dir, _ := openTestEngine(t)
	b1, err := eng.Bank(testUser)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := eng.Bank(testUser)
	if err != nil {
		t.Fatal(err)
	}
	if b1 != b2 {
		t.Fatal("same user got two banks")
	}
	want := filepath.Join(dir, "users", testUser+".db")
	if b1.path != want {
		t.Fatalf("bank path = %s, want %s", b1.path, want)
	}
}

func TestEngineClosedRefusesWork(t *testing.T) {
	eng, _, _ := openTestEngine(t)
	b, err := eng.Bank(testUser)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := eng.Bank(testUser); !errors.Is(err, ErrClosed) {
		t.Fatalf("Bank after Close err = %v, want ErrClosed", err)
	}
	if _, err := b.AddRule(context.Background(), Rule{Text: "late", Kind: KindFact, ApprovedBy: "u1"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("write after Close err = %v, want ErrClosed", err)
	}
}

func TestBankSchemaVersionAndPragmas(t *testing.T) {
	b, _, _ := openTestBank(t)
	var version int
	if err := b.wdb.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d", version, len(migrations))
	}
	var mode string
	if err := b.wdb.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %s, want wal", mode)
	}
	var busy int
	if err := b.wdb.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busy)
	}
}

func TestBankReopenKeepsDataAndSchema(t *testing.T) {
	eng, dir, clock := openTestEngine(t)
	b, err := eng.Bank(testUser)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r, err := b.AddRule(ctx, Rule{Text: "survives reopen", Kind: KindFact, ApprovedBy: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	eng2, err := Open(dir, Options{Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer eng2.Close()
	b2, err := eng2.Bank(testUser)
	if err != nil {
		t.Fatalf("reopen bank (migrations must not rerun): %v", err)
	}
	got, err := b2.Recall(ctx, "reopen", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(got, r.RuleID) {
		t.Fatalf("rule lost on reopen: %v", ruleIDs(got))
	}
}

// seedEveryTable seeds one row per deletable table, through the API or raw
// SQL, so the DELETE tests have a row to try to remove.
func seedEveryTable(t *testing.T, b *Bank) {
	t.Helper()
	ctx := context.Background()
	if _, err := b.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "seed quote", Kind: KindFact}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddRule(ctx, Rule{Text: "seed rule", Kind: KindFact, ApprovedBy: "u1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.wdb.Exec(`INSERT INTO profile_versions (version, lines_json, approved_by, created_at)
		VALUES (1, '[]', 'u1', '2026-10-11T09:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := b.RecordLedger(ctx, LedgerRow{Stage: "mine", Status: LedgerSuccess}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteOutsidePurgeAborts(t *testing.T) {
	b, _, _ := openTestBank(t)
	seedEveryTable(t, b)
	for _, table := range deletableTables {
		_, err := b.wdb.Exec("DELETE FROM " + table)
		if err == nil || !strings.Contains(err.Error(), "use purge") {
			t.Errorf("DELETE FROM %s err = %v, want abort with %q", table, err, "use purge")
		}
		var n int
		if err := b.wdb.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("%s lost its row to a refused DELETE", table)
		}
	}
}

func TestRulesUpdateChangesOnlyStatus(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	if _, err := b.AddRule(ctx, Rule{RuleID: "r1", Text: "original", Kind: KindFact, ApprovedBy: "u1"}); err != nil {
		t.Fatal(err)
	}
	refused := []string{
		`UPDATE rules SET text = 'changed' WHERE rule_id = 'r1'`,
		`UPDATE rules SET anchors_json = '["x"]' WHERE rule_id = 'r1'`,
		`UPDATE rules SET approved_by = 'someone' WHERE rule_id = 'r1'`,
		`UPDATE rules SET status = 'dormant', text = 'changed' WHERE rule_id = 'r1'`,
	}
	for _, stmt := range refused {
		if _, err := b.wdb.Exec(stmt); err == nil || !strings.Contains(err.Error(), "only status may change") {
			t.Errorf("%s err = %v, want refusal", stmt, err)
		}
	}
	if _, err := b.wdb.Exec(`UPDATE rules SET status = 'dormant' WHERE rule_id = 'r1'`); err != nil {
		t.Fatalf("status-only update refused: %v", err)
	}
	versions, err := b.RuleVersions(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Text != "original" || versions[0].Status != StatusDormant {
		t.Fatalf("after refused and status updates: %+v", versions)
	}
}

// integrityCheck runs FTS5's own check: it fails when an index differs from
// its content table.
func integrityCheck(t *testing.T, b *Bank) {
	t.Helper()
	for _, fts := range []string{"observations_fts", "rules_fts"} {
		if _, err := b.wdb.Exec("INSERT INTO " + fts + "(" + fts + ", rank) VALUES ('integrity-check', 1)"); err != nil {
			t.Fatalf("%s out of sync: %v", fts, err)
		}
	}
}

func TestFTSStaysInSync(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()

	id, err := b.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "first zebras quote", Kind: KindFact})
	if err != nil {
		t.Fatal(err)
	}
	integrityCheck(t, b)
	if got, _ := b.SearchObservations(ctx, "zebras", 5); len(got) != 1 {
		t.Fatalf("insert not indexed: %d hits", len(got))
	}

	// The update trigger: an edit to the quote must leave the old words out of
	// the index and put the new words in.
	if _, err := b.wdb.Exec(`UPDATE observations SET quote = 'giraffes instead', split_text = 'giraffes instead' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	integrityCheck(t, b)
	if got, _ := b.SearchObservations(ctx, "zebras", 5); len(got) != 0 {
		t.Fatalf("update left old words indexed: %d hits", len(got))
	}
	if got, _ := b.SearchObservations(ctx, "giraffes", 5); len(got) != 1 {
		t.Fatalf("update not indexed: %d hits", len(got))
	}

	rule, err := b.AddRule(ctx, Rule{Text: "prefer zebras", Kind: KindFact, ApprovedBy: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	integrityCheck(t, b)
	if err := b.SetRuleStatus(ctx, rule.RuleID, StatusDormant); err != nil {
		t.Fatal(err)
	}
	integrityCheck(t, b)
	if _, err := b.ReviseRule(ctx, Rule{RuleID: rule.RuleID, Text: "prefer giraffes", Kind: KindFact, ApprovedBy: "u1"}); err != nil {
		t.Fatal(err)
	}
	integrityCheck(t, b)
	if got, _ := b.Recall(ctx, "giraffes", 5); len(got) != 1 {
		t.Fatalf("revised rule not recalled: %d hits", len(got))
	}
	if got, _ := b.Recall(ctx, "zebras", 5); len(got) != 0 {
		t.Fatalf("superseded text still recalled: %d hits", len(got))
	}

	if _, err := b.Purge(ctx, Selection{Observations: []int64{id}, Rules: []string{rule.RuleID}}); err != nil {
		t.Fatal(err)
	}
	integrityCheck(t, b)
	if got, _ := b.SearchObservations(ctx, "giraffes", 5); len(got) != 0 {
		t.Fatalf("purge left observation words indexed: %d hits", len(got))
	}
}

func TestSearchNeverErrorsOnHostileInput(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	if _, err := b.AddRule(ctx, Rule{Text: "alpha beta", Kind: KindFact, ApprovedBy: "u1"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range hostileQueries {
		if _, err := b.Recall(ctx, q, 20); err != nil {
			t.Errorf("Recall(%q) = %v", q, err)
		}
	}
	// A query containing FTS5 syntax must match literally, not as an operator.
	if got, err := b.Recall(ctx, `alpha NOT beta`, 20); err != nil || len(got) != 1 {
		t.Errorf("NOT is an operator, not a word: %d hits, err %v", len(got), err)
	}
}

func TestBankClosedStopsWriterGoroutine(t *testing.T) {
	b, _, _ := openTestBank(t)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-b.done:
	case <-time.After(5 * time.Second):
		t.Fatal("writer goroutine still running after Close")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
