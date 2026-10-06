package statedb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// openTemp opens a migrated database in a temp dir.
func openTemp(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, DBFileName))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

// Seam: database creation — first use creates the file at the schema version
// with every table present and WAL mode on.
func TestOpenCreatesVersionedDatabaseWithAllTables(t *testing.T) {
	db, dir := openTemp(t)

	if _, err := os.Stat(filepath.Join(dir, DBFileName)); err != nil {
		t.Fatalf("database file missing: %v", err)
	}
	if v, err := SchemaVersion(db); err != nil || v != CurrentVersion {
		t.Errorf("version = %d, %v; want %d", v, err, CurrentVersion)
	}
	for _, table := range []string{"schema_version", "usage", "search_cache", "search_quota"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s present = %d, %v; want it created", table, n, err)
		}
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal so readers never block writers", mode)
	}
}

// Seam: schema upgrade — a v1 database (usage only, as the first migration
// left it) opens into the current version with its rows intact and the new
// tables present.
func TestMigrateUpgradesV1ToCurrentKeepingRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	db, err := sql.Open("sqlite", dsn(path, writeParams()))
	if err != nil {
		t.Fatal(err)
	}
	if err := applyV1(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(1,?)`, "2026-10-06T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO usage(agent, at, harness, provider, model, kind, input_tokens, output_tokens, reported_cost, dedupe_key)
		VALUES('w1','2026-10-06T10:00:00Z','pig','p','m','request',100,50,0.01,'pig:m1')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("Open for upgrade: %v", err)
	}
	defer db2.Close()
	if v, err := SchemaVersion(db2); err != nil || v != CurrentVersion {
		t.Fatalf("version = %d, %v; want %d", v, err, CurrentVersion)
	}
	rows, err := UsageRowsForAgent(db2, "w1")
	if err != nil || len(rows) != 1 || rows[0].Input != 100 {
		t.Fatalf("v1 row after upgrade = %+v, %v; want the 100-token request kept", rows, err)
	}
	var n int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('search_cache','search_quota')`).Scan(&n); err != nil || n != 2 {
		t.Errorf("search tables after upgrade = %d, %v; want both created", n, err)
	}
}

// Seam: usage rows — what the tailer inserts is what the TUI reads back, and
// a missing cost stays missing.
func TestUsageRoundTripKeepsKindsAndMissingCost(t *testing.T) {
	db, _ := openTemp(t)

	cost := 0.01
	for _, row := range []UsageRow{
		{Agent: "w1", At: "2026-10-06T10:00:00Z", Harness: "pig", Provider: "p", Model: "m", Kind: KindRequest, Input: 100, Output: 50, CacheRead: 10, CacheWrite: 20, Reasoning: 5, ReportedCost: &cost, DedupeKey: "pig:m1"},
		{Agent: "w1", At: "2026-10-06T10:01:00Z", Harness: "pig", Provider: "p", Model: "m", Kind: KindRequest, Input: 200, Output: 70, DedupeKey: "pig:m2"},
		{Agent: "w1", At: "2026-10-06T09:00:00Z", Harness: "claude", Provider: "anthropic", Model: "opus", Kind: KindCost, ReportedCost: &cost, DedupeKey: "claude-cost:x\x00opus"},
		{Agent: "w1", At: "2026-10-06T10:02:00Z", Model: "jev", Kind: KindFind, Input: 278, Output: 21, ReportedCost: &cost, FindCallID: "f1", FindQuestions: 2, Generation: "gen-1", Provider: "TypeSafe"},
	} {
		if _, err := InsertUsageRow(db, row); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	got, err := UsageRowsForAgent(db, "w1")
	if err != nil || len(got) != 4 {
		t.Fatalf("rows = %d, %v; want 4 in row order", len(got), err)
	}
	if got[0].Input != 100 || got[0].ReportedCost == nil || *got[0].ReportedCost != 0.01 {
		t.Errorf("request row = %+v, want 100 tokens with its 0.01 cost", got[0])
	}
	if got[1].ReportedCost != nil {
		t.Errorf("unreported cost must stay missing, got %v", *got[1].ReportedCost)
	}
	if got[2].Kind != KindCost || got[2].DedupeKey != "claude-cost:x\x00opus" {
		t.Errorf("cost row = %+v, want the per-model snapshot key", got[2])
	}
	if got[3].Kind != KindFind || got[3].FindCallID != "f1" || got[3].FindQuestions != 2 || got[3].Generation != "gen-1" {
		t.Errorf("find row = %+v, want the call attribution kept", got[3])
	}
	// Incremental reads page by row id: past the first two rows only the
	// cost and find rows remain.
	since, err := UsageRowsSince(db, got[1].ID)
	if err != nil || len(since) != 2 || since[0].Kind != KindCost {
		t.Errorf("since %d = %+v, %v; want the last two rows", got[1].ID, since, err)
	}
}

// Seam: idempotent inserts — the same dedupe key twice logs once, so a
// re-tailed line never double counts even when the tail state was lost.
func TestInsertWithSameDedupeKeyLogsOnce(t *testing.T) {
	db, _ := openTemp(t)

	row := UsageRow{Agent: "w1", At: "2026-10-06T10:00:00Z", Harness: "pig", Kind: KindRequest, Input: 10, DedupeKey: "pig:m1"}
	if yes, err := InsertUsageRow(db, row); !yes || err != nil {
		t.Fatalf("first insert = %v, %v; want true, nil", yes, err)
	}
	if yes, err := InsertUsageRow(db, row); yes || err != nil {
		t.Fatalf("second insert = %v, %v; want false, nil", yes, err)
	}
	if n, err := CountUsage(db); err != nil || n != 1 {
		t.Errorf("count = %d, %v; want 1", n, err)
	}
}

// Seam: one-time import — legacy JSONL logs land in the database with their
// tokens and costs, and importing again inserts nothing.
func TestImportReadsLegacyJSONLOnce(t *testing.T) {
	dir := t.TempDir()
	usageDir := filepath.Join(dir, "usage")
	if err := os.MkdirAll(usageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"agent":"w1","at":"2026-10-06T10:00:00Z","harness":"pig","provider":"p","model":"m","input":100,"output":50,"cacheRead":10,"cacheWrite":20,"reasoning":5,"reportedCost":0.01,"kind":"request"}` + "\n" +
		`{"agent":"w1","at":"2026-10-06T10:02:00Z","kind":"find","model":"jev","find_call_id":"f1","questions":2,"usage":{"input_tokens":278,"output_tokens":21,"cost":0.000011676}}` + "\n" +
		`{"agent":"w1","at":"2026-10-06T09:00:00Z","harness":"claude","provider":"anthropic","model":"opus","input":2,"output":474,"cacheRead":0,"cacheWrite":17838,"reasoning":170,"reportedCost":5.0,"kind":"cost"}` + "\n"
	if err := os.WriteFile(filepath.Join(usageDir, "w1.jsonl"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(filepath.Join(dir, DBFileName))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	rows, err := UsageRowsForAgent(db, "w1")
	if err != nil || len(rows) != 3 {
		t.Fatalf("imported = %d, %v; want 3", len(rows), err)
	}
	if rows[0].Input != 100 || rows[0].ReportedCost == nil {
		t.Errorf("imported request = %+v, want 100 tokens costed", rows[0])
	}
	if rows[1].Kind != KindFind || rows[1].Input != 278 || rows[1].ReportedCost == nil {
		t.Errorf("imported find = %+v, want 278 tokens with the endpoint cost", rows[1])
	}
	// Re-opening (and an explicit second import) inserts nothing.
	again, err := EnsureImport(db, dir)
	if err != nil || again != 0 {
		t.Errorf("second import = %d, %v; want 0", again, err)
	}
	if n, err := CountUsage(db); err != nil || n != 3 {
		t.Errorf("count after re-import = %d, %v; want 3", n, err)
	}
}

// Seam: concurrent writers plus a read-only reader — separate connections
// inserting while one reads never see lock errors, and every row lands.
func TestConcurrentWritersWithReadOnlyReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	w0, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = w0.Close()

	const writers = 4
	const perWriter = 25
	var wg sync.WaitGroup
	errs := make(chan error, writers+1)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			db, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer db.Close()
			for i := 0; i < perWriter; i++ {
				_, err := InsertUsageRow(db, UsageRow{
					Agent: fmt.Sprintf("w%d", w), At: "2026-10-06T10:00:00Z",
					Harness: "pig", Kind: KindRequest, Input: 1,
					DedupeKey: fmt.Sprintf("pig:w%d:%d", w, i),
				})
				if err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		db, err := OpenReadOnly(path)
		if err != nil {
			errs <- err
			return
		}
		defer db.Close()
		for i := 0; i < 50; i++ {
			if _, err := UsageRowsSince(db, 0); err != nil {
				errs <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent use: %v", err)
	}
	check, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	if n, err := CountUsage(check); err != nil || n != writers*perWriter {
		t.Errorf("count = %d, %v; want %d", n, err, writers*perWriter)
	}
}

// Seam: prune — deleting a run's agents removes their rows and keeps the
// other run's.
func TestDeleteUsageForAgentsKeepsOtherAgents(t *testing.T) {
	db, _ := openTemp(t)

	for _, agent := range []string{"agent-old", "agent-new"} {
		if _, err := InsertUsageRow(db, UsageRow{Agent: agent, Kind: KindRequest, Input: 1, DedupeKey: "k:" + agent}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := DeleteUsageForAgents(db, []string{"agent-old"}); err != nil || n != 1 {
		t.Fatalf("deleted = %d, %v; want 1", n, err)
	}
	kept, err := UsageRowsForAgent(db, "agent-new")
	if err != nil || len(kept) != 1 {
		t.Errorf("kept rows = %+v, %v; want the surviving agent's row", kept, err)
	}
	if gone, err := UsageRowsForAgent(db, "agent-old"); err != nil || len(gone) != 0 {
		t.Errorf("pruned rows = %+v, %v; want none", gone, err)
	}
}

// Seam: search tables — the cache and quota tables accept the documented
// shape (this ticket only proves the shape; the websearch ticket fills them).
func TestSearchCacheAndQuotaTablesHoldDocumentedShape(t *testing.T) {
	db, _ := openTemp(t)

	if _, err := db.Exec(`INSERT INTO search_cache(key, query, provider, args, results, created_at)
		VALUES(?,?,?,?,?,?)`, "k1", "golang sqlite", "brave", `{"count":10}`, "1. title\nhttps://example.com", 1728211200); err != nil {
		t.Fatalf("search_cache insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO search_quota(provider, cost, remaining, reset_at, created_at)
		VALUES(?,?,?,?,?)`, "brave", nil, 1999, "2026-10-06T11:00:00Z", "2026-10-06T10:00:00Z"); err != nil {
		t.Fatalf("search_quota insert: %v", err)
	}
	var results string
	if err := db.QueryRow(`SELECT results FROM search_cache WHERE key='k1'`).Scan(&results); err != nil {
		t.Fatalf("search_cache read: %v", err)
	}
	var remaining sql.NullInt64
	if err := db.QueryRow(`SELECT remaining FROM search_quota WHERE provider='brave'`).Scan(&remaining); err != nil {
		t.Fatalf("search_quota read: %v", err)
	}
	if !remaining.Valid || remaining.Int64 != 1999 {
		t.Errorf("quota remaining = %+v, want 1999", remaining)
	}
}

// Seam: FTS5 with the porter tokenizer — the pure-Go driver must provide it.
// Ticket 09 builds the overflow store on this; a driver without it fails this
// test loudly instead of failing that ticket mysteriously.
func TestDriverSupportsFTS5PorterTokenizer(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, DBFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE VIRTUAL TABLE fts_probe USING fts5(body, tokenize='porter')`); err != nil {
		t.Fatalf("FTS5 porter table: %v (the driver must ship FTS5 with porter)", err)
	}
	defer db.Exec(`DROP TABLE fts_probe`)
	for _, body := range []string{"the query cache speeds up caching", "unrelated words here"} {
		if _, err := db.Exec(`INSERT INTO fts_probe(body) VALUES(?)`, body); err != nil {
			t.Fatal(err)
		}
	}
	var hits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fts_probe WHERE fts_probe MATCH 'cached'`).Scan(&hits); err != nil {
		t.Fatalf("porter MATCH: %v", err)
	}
	if hits != 1 {
		t.Errorf("porter MATCH 'cached' = %d, want 1 (stemming caching/cached together)", hits)
	}
}

// Seam: concurrent first opens — sessions starting together migrate exactly
// once instead of racing on the version row (regression: two opens both saw
// version 0 and both inserted version 1).
func TestConcurrentFirstOpensMigrateOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			_ = db.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent open: %v", err)
	}
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v, err := SchemaVersion(db); err != nil || v != CurrentVersion {
		t.Errorf("version = %d, %v; want %d", v, err, CurrentVersion)
	}
}

// Seam: read-only open — opening a missing database fails instead of
// creating an empty, unmigrated file behind the TUI's back.
func TestOpenReadOnlyRefusesAMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	if db, err := OpenReadOnly(path); err == nil {
		_ = db.Close()
		t.Fatal("read-only open of a missing file must fail")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a refused read-only open must create nothing, stat err = %v", err)
	}
}
