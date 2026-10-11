// Schema and FTS trigger shape are adapted from mnemopi (MIT). The upstream
// notice is kept in NOTICE.md beside this file. This package adds the FTS
// update trigger mnemopi lacks, the append-only guard on rules, and the
// identifier-split copy column.
package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations[i] upgrades PRAGMA user_version from i to i+1. Append only:
// a shipped migration is never edited.
var migrations = []string{
	schemaTables + ftsTables + guardTriggers + ftsTriggers + noDeleteTriggers(),
}

// deletableTables lists every table whose DELETE aborts outside purge.
var deletableTables = []string{"observations", "rules", "queue", "profile_versions", "ledger"}

const schemaTables = `
CREATE TABLE observations (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id      TEXT NOT NULL,
  event_id        TEXT NOT NULL,
  author          TEXT NOT NULL,
  quote           TEXT NOT NULL,
  split_text      TEXT NOT NULL DEFAULT '',
  kind            TEXT NOT NULL CHECK (kind IN ('preference', 'decision', 'antipattern', 'fact', 'procedure')),
  subject_key     TEXT NOT NULL DEFAULT '',
  evidence_count  INTEGER NOT NULL DEFAULT 1 CHECK (evidence_count >= 1),
  decider_json    TEXT NOT NULL DEFAULT '',
  observed_commit TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL
);

CREATE TABLE rules (
  id                     INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_id                TEXT NOT NULL,
  version                INTEGER NOT NULL CHECK (version >= 1),
  text                   TEXT NOT NULL CHECK (length(text) BETWEEN 1 AND 300),
  split_text             TEXT NOT NULL DEFAULT '',
  kind                   TEXT NOT NULL CHECK (kind IN ('preference', 'decision', 'antipattern', 'fact', 'procedure')),
  subject_key            TEXT NOT NULL DEFAULT '',
  anchors_json           TEXT NOT NULL DEFAULT '[]',
  observed_commit        TEXT NOT NULL DEFAULT '',
  status                 TEXT NOT NULL CHECK (status IN ('active', 'parked', 'dormant', 'superseded')),
  source_observation_ids TEXT NOT NULL DEFAULT '[]',
  approved_by            TEXT NOT NULL,
  approved_at            TEXT NOT NULL,
  UNIQUE (rule_id, version)
);

CREATE TABLE queue (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  kind       TEXT NOT NULL CHECK (kind IN ('rule', 'profile_patch', 'promotion')),
  payload    TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  decided_by TEXT NOT NULL DEFAULT '',
  decided_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE profile_versions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  version     INTEGER NOT NULL UNIQUE CHECK (version >= 1),
  lines_json  TEXT NOT NULL,
  patch_json  TEXT NOT NULL DEFAULT '',
  approved_by TEXT NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE TABLE ledger (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id      TEXT NOT NULL,
  stage       TEXT NOT NULL,
  status      TEXT NOT NULL CHECK (status IN ('SUCCESS', 'NO_DATA', 'FAILED')),
  counts_json TEXT NOT NULL DEFAULT '{}',
  detail      TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);
`

// ftsTables are external-content FTS5 indexes. Each reads the content
// table by rowid. split_text holds the identifier-split copy.
const ftsTables = `
CREATE VIRTUAL TABLE observations_fts USING fts5(
  quote, split_text, content = 'observations', content_rowid = 'id'
);

CREATE VIRTUAL TABLE rules_fts USING fts5(
  text, split_text, content = 'rules', content_rowid = 'id'
);
`

// guardTriggers make rules append-only: only status may change on UPDATE.
const guardTriggers = `
CREATE TRIGGER rules_guard BEFORE UPDATE ON rules
WHEN NEW.id IS NOT OLD.id
  OR NEW.rule_id IS NOT OLD.rule_id
  OR NEW.version IS NOT OLD.version
  OR NEW.text IS NOT OLD.text
  OR NEW.split_text IS NOT OLD.split_text
  OR NEW.kind IS NOT OLD.kind
  OR NEW.subject_key IS NOT OLD.subject_key
  OR NEW.anchors_json IS NOT OLD.anchors_json
  OR NEW.observed_commit IS NOT OLD.observed_commit
  OR NEW.source_observation_ids IS NOT OLD.source_observation_ids
  OR NEW.approved_by IS NOT OLD.approved_by
  OR NEW.approved_at IS NOT OLD.approved_at
BEGIN
  SELECT RAISE(ABORT, 'rules: only status may change');
END;
`

// ftsTriggers keep both FTS indexes in step with their content tables:
// insert, delete, and update of the indexed columns.
const ftsTriggers = `
CREATE TRIGGER observations_ai AFTER INSERT ON observations BEGIN
  INSERT INTO observations_fts (rowid, quote, split_text) VALUES (new.id, new.quote, new.split_text);
END;

CREATE TRIGGER observations_ad AFTER DELETE ON observations BEGIN
  INSERT INTO observations_fts (observations_fts, rowid, quote, split_text)
  VALUES ('delete', old.id, old.quote, old.split_text);
END;

CREATE TRIGGER observations_au AFTER UPDATE OF quote, split_text ON observations BEGIN
  INSERT INTO observations_fts (observations_fts, rowid, quote, split_text)
  VALUES ('delete', old.id, old.quote, old.split_text);
  INSERT INTO observations_fts (rowid, quote, split_text) VALUES (new.id, new.quote, new.split_text);
END;

CREATE TRIGGER rules_ai AFTER INSERT ON rules BEGIN
  INSERT INTO rules_fts (rowid, text, split_text) VALUES (new.id, new.text, new.split_text);
END;

CREATE TRIGGER rules_ad AFTER DELETE ON rules BEGIN
  INSERT INTO rules_fts (rules_fts, rowid, text, split_text)
  VALUES ('delete', old.id, old.text, old.split_text);
END;

CREATE TRIGGER rules_au AFTER UPDATE OF text, split_text ON rules BEGIN
  INSERT INTO rules_fts (rules_fts, rowid, text, split_text)
  VALUES ('delete', old.id, old.text, old.split_text);
  INSERT INTO rules_fts (rowid, text, split_text) VALUES (new.id, new.text, new.split_text);
END;
`

// noDeleteName is the name of a table's abort-on-DELETE trigger.
func noDeleteName(table string) string { return table + "_no_delete" }

// noDeleteSQL is the trigger that aborts DELETE on table with the purge hint.
func noDeleteSQL(table string) string {
	return fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE DELETE ON %s BEGIN SELECT RAISE(ABORT, 'use purge'); END;",
		noDeleteName(table), table)
}

func noDeleteTriggers() string {
	var out string
	for _, t := range deletableTables {
		out += noDeleteSQL(t) + ";\n"
	}
	return out
}

// migrate brings the bank to the latest schema. BEGIN IMMEDIATE (set by the
// DSN) serializes migrators across processes, and PRAGMA user_version moves
// in the same transaction as the DDL.
func migrate(db *sql.DB) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("memory: migrate begin: %w", err)
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("memory: user_version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("memory: bank schema v%d is newer than v%d", version, len(migrations))
	}
	for _, m := range migrations[version:] {
		if _, err := tx.Exec(m); err != nil {
			return fmt.Errorf("memory: migrate: %w", err)
		}
	}
	if version < len(migrations) {
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", len(migrations))); err != nil {
			return fmt.Errorf("memory: set user_version: %w", err)
		}
	}
	return tx.Commit()
}
