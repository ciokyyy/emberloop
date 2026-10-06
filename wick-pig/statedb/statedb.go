// Package statedb owns wick's SQLite state database: one file in the wick
// state directory holding usage stats today and the search cache, quota and
// overflow stores follow-up tickets build on. Both the harness (usage tailer,
// prune) and the agent tools (find logging) write through here, and the TUI
// reads through here, so the schema has one home. It is a leaf package: it
// imports nothing from wick or its tools, so both can import it without a
// cycle.
package statedb

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DBFileName is the database file beside workers.json, history and the
// archive: one source of truth to back up and read.
const DBFileName = "wick.db"

// CurrentVersion is the schema Open migrates to. v1 carries the usage table;
// v2 adds the search cache and quota tables the websearch ticket fills; v3
// adds the overflow store (ctx_blobs plus the FTS5 chunk index).
const CurrentVersion = 3

// BusyTimeoutMillis is how long a writer waits on a locked database before
// failing: long enough that the TUI's read-only polls and a tailer's inserts
// never trip over each other.
const BusyTimeoutMillis = 5000

// StateDir is wick's state directory: WICK_STATE_DIR redirects it, the way
// WICK_SETTINGS redirects settings. Callers that already hold a state dir
// (the TUI's Snapshot seam) join DBFileName onto it with DBPathForDir
// instead.
func StateDir() string {
	if dir := os.Getenv("WICK_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick")
}

// DBPath is the default database file.
func DBPath() string { return DBPathForDir(StateDir()) }

// DBPathForDir is the database file inside an explicit state dir.
func DBPathForDir(stateDir string) string { return filepath.Join(stateDir, DBFileName) }

// dsn builds a file: URI for path with the given query parameters.
func dsn(path string, params url.Values) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: params.Encode()}
	return u.String()
}

func writeParams() url.Values {
	return url.Values{
		"_pragma": []string{
			fmt.Sprintf("busy_timeout(%d)", BusyTimeoutMillis),
			"foreign_keys(1)",
		},
	}
}

// Open opens (creating) the database at path, sets WAL mode and the busy
// timeout, migrates the schema to CurrentVersion, imports legacy JSONL usage
// logs sitting beside it, and purges overflow rows older than 24 hours.
// Import runs only while the usage table is empty, and inserts idempotently,
// so re-opening never double counts. Migration holds a cross-process file
// lock, so sessions starting together (a tailer, the MCP server, a prune)
// migrate exactly once.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(path, writeParams()))
	if err != nil {
		return nil, err
	}
	if err := tune(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateLocked(db, filepath.Dir(path)); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := EnsureImport(db, filepath.Dir(path)); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := PurgeExpiredCtx(db, time.Now().UTC().Unix()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// OpenReadOnly opens the database for reads that never block writers: the
// TUI's polls. It neither migrates nor writes, so opening a database others
// hold open for writing is safe. The file must already exist: unlike Open it
// creates nothing, refusing a missing file instead (the driver would
// otherwise create an empty, unmigrated file).
func OpenReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("statedb: read-only open: %w", err)
	}
	params := writeParams()
	params.Set("_query_only", "1")
	db, err := sql.Open("sqlite", dsn(path, params))
	if err != nil {
		return nil, err
	}
	var one int
	if err := db.QueryRow(fmt.Sprintf("PRAGMA busy_timeout=%d", BusyTimeoutMillis)).Scan(&one); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// migrateLocked runs Migrate under a cross-process file lock in the state
// dir's locks directory (the same directory the harness's withFileLock
// uses), so concurrent first opens migrate exactly once instead of racing
// on the version row.
func migrateLocked(db *sql.DB, stateDir string) error {
	dir := filepath.Join(stateDir, "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "migrate.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := flock(f); err != nil {
		return err
	}
	defer funlock(f)
	return Migrate(db)
}

// tune sets the pragmas every writer needs. WAL lets readers work while a
// writer holds the database; the synchronous level keeps WAL fast without
// risking corruption on a crash.
func tune(db *sql.DB) error {
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return fmt.Errorf("statedb: journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return fmt.Errorf("statedb: synchronous: %w", err)
	}
	return nil
}

// SchemaVersion reports the migrated version: 0 when the database was never
// migrated.
func SchemaVersion(db *sql.DB) (int, error) {
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, nil
	}
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

// Migrate brings the database to CurrentVersion, recording each version with
// its apply time. Fresh databases migrate from 0; a database already there
// is untouched.
func Migrate(db *sql.DB) error {
	v, err := SchemaVersion(db)
	if err != nil {
		return err
	}
	for v < CurrentVersion {
		switch v {
		case 0:
			if err := applyV1(db); err != nil {
				return err
			}
			v = 1
		case 1:
			if err := applyV2(db); err != nil {
				return err
			}
			v = 2
		case 2:
			if err := applyV3(db); err != nil {
				return err
			}
			v = 3
		default:
			return fmt.Errorf("statedb: cannot migrate from version %d", v)
		}
		if _, err := db.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(?,?)`,
			v, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("statedb: record version %d: %w", v, err)
		}
	}
	return nil
}

// MaxUsageID returns the highest usage row id, or 0 when the table is empty.
// Readers compare it against their high-water mark: a maximum below the mark
// means the database was recreated (row ids restart), and the reader must
// start over instead of skipping every row.
func MaxUsageID(db *sql.DB) (int64, error) {
	var id sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(id) FROM usage`).Scan(&id); err != nil {
		return 0, fmt.Errorf("statedb: max usage id: %w", err)
	}
	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
}

// applyV1 creates the schema version table and the usage table: one row per
// usage record (kinds request, find, cost, and the later search kind), with
// the tailer's dedupe key carried so a re-tailed line never inserts twice.
// Token columns serve every kind, read back by kind: request rows sum input,
// output, cache and reasoning with their reported cost; find rows carry the
// decision call's tokens and cost with their call id; cost rows hold one
// cumulative claude snapshot per model.
func applyV1(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent TEXT NOT NULL,
			at TEXT NOT NULL DEFAULT '',
			harness TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read INTEGER NOT NULL DEFAULT 0,
			cache_write INTEGER NOT NULL DEFAULT 0,
			reasoning INTEGER NOT NULL DEFAULT 0,
			reported_cost REAL,
			dedupe_key TEXT,
			find_call_id TEXT NOT NULL DEFAULT '',
			find_questions INTEGER NOT NULL DEFAULT 0,
			generation TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedupe
			ON usage(dedupe_key) WHERE dedupe_key IS NOT NULL AND dedupe_key != ''`,
		`CREATE INDEX IF NOT EXISTS idx_usage_agent_id ON usage(agent, id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("statedb: v1: %w", err)
		}
	}
	return nil
}

// applyV2 adds the search cache and quota tables the websearch ticket fills;
// this ticket only proves the shape exists and stays empty.
//
// Search cache shape: one row per answered search, keyed by the normalised
// query, the resolved provider and the call arguments, so an identical search
// within 15 minutes answers from results without calling the provider again.
// key is the cache key (query + provider + arguments hashed); query, provider
// and args (the count, freshness and domains arguments as JSON) name what was
// asked; results holds the rendered answer text; created_at is unix seconds,
// and a row older than 15 minutes reads as a miss.
//
// Search quota shape: one row per live provider call, so the TUI can show
// what each provider last reported. provider names the engine; cost holds
// Exa's reported cost when it reports one; remaining holds Brave's rate-limit
// remaining when its headers carry one; reset_at holds Brave's reset time;
// created_at is when the call happened. Nothing is enforced and nothing is
// estimated when a provider reports nothing: absent stays NULL. The TUI reads
// the latest row per provider.
func applyV2(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS search_cache (
			key TEXT PRIMARY KEY,
			query TEXT NOT NULL,
			provider TEXT NOT NULL,
			args TEXT NOT NULL DEFAULT '{}',
			results TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_search_cache_lookup
			ON search_cache(query, provider)`,
		`CREATE TABLE IF NOT EXISTS search_quota (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			cost REAL,
			remaining INTEGER,
			reset_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_search_quota_provider
			ON search_quota(provider, id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("statedb: v2: %w", err)
		}
	}
	return nil
}
