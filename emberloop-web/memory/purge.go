package memory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// scrubColumn is one text field that purge clears. json marks a field holding
// a JSON document. Purged text is matched on the decoded strings there, so the
// document stays valid and escaping never lets the text survive.
type scrubColumn struct {
	table string
	col   string
	json  bool
}

// scrubColumns are the fields purge clears text from. Observations and Rules
// hold the purged records themselves and are deleted, not scrubbed.
var scrubColumns = []scrubColumn{
	{table: "queue", col: "payload", json: true},
	{table: "ledger", col: "detail"},
	{table: "profile_versions", col: "lines_json", json: true},
	{table: "profile_versions", col: "patch_json", json: true},
}

// Purge is the only path that deletes rows. It removes the selected records
// from the bank and from every snapshot, clears the same text from queue,
// ledger and Profile rows, rebuilds FTS, and runs VACUUM. It then truncates the
// WAL so the text leaves the files on disk.
func (b *Bank) Purge(ctx context.Context, sel Selection) (PurgeResult, error) {
	var res PurgeResult
	err := b.write(ctx, func(db *sql.DB) error {
		texts, err := selectedTexts(ctx, db, sel)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "PRAGMA secure_delete = ON"); err != nil {
			return err
		}
		counts, err := purgeDB(ctx, db, sel, texts)
		if err != nil {
			return err
		}
		res = counts
		// The purge is committed. Finish the rest even when one step fails, so
		// snapshots never keep text the live bank has lost.
		var errs []error
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			errs = append(errs, fmt.Errorf("memory: vacuum: %w", err))
		}
		if err := checkpointTruncate(ctx, db); err != nil {
			errs = append(errs, err)
		}
		files, err := snapshotFiles(b.snapDir)
		if err != nil {
			return errors.Join(append(errs, err)...)
		}
		for _, f := range files {
			if err := purgeFile(ctx, f, sel, texts); err != nil {
				errs = append(errs, err)
				continue
			}
			res.Snapshots++
		}
		return errors.Join(errs...)
	})
	return res, err
}

// selectedTexts returns the text of the selected observations and of every
// version of the selected rules. These are the strings purge clears elsewhere.
func selectedTexts(ctx context.Context, db *sql.DB, sel Selection) ([]string, error) {
	var texts []string
	if len(sel.Observations) > 0 {
		args := anyArgs(sel.Observations)
		got, err := queryAll(ctx, db, scanText,
			"SELECT quote FROM observations WHERE id IN ("+placeholders(len(args))+")", args...)
		if err != nil {
			return nil, err
		}
		texts = append(texts, got...)
	}
	if len(sel.Memories) > 0 {
		args := anyArgs(sel.Memories)
		got, err := queryAll(ctx, db, scanText,
			"SELECT text FROM memories WHERE id IN ("+placeholders(len(args))+")", args...)
		if err != nil {
			return nil, err
		}
		texts = append(texts, got...)
	}
	if len(sel.Episodes) > 0 {
		args := anyArgs(sel.Episodes)
		got, err := queryAll(ctx, db, scanText,
			"SELECT text FROM episodes WHERE id IN ("+placeholders(len(args))+")", args...)
		if err != nil {
			return nil, err
		}
		texts = append(texts, got...)
	}
	if len(sel.Rules) > 0 {
		args := anyArgs(sel.Rules)
		got, err := queryAll(ctx, db, scanText,
			"SELECT text FROM rules WHERE rule_id IN ("+placeholders(len(args))+")", args...)
		if err != nil {
			return nil, err
		}
		texts = append(texts, got...)
	}
	return texts, nil
}

func scanText(s rowScanner) (string, error) {
	var t string
	err := s.Scan(&t)
	return t, err
}

// purgeDB deletes the selected rows and scrubs the texts, in one transaction.
// It drops the DELETE aborts for that transaction and recreates them before
// commit, so the purge is the only window where a delete succeeds.
func purgeDB(ctx context.Context, db *sql.DB, sel Selection, texts []string) (PurgeResult, error) {
	var res PurgeResult
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, t := range purgeTables {
		if _, err := tx.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+noDeleteName(t)); err != nil {
			return res, err
		}
	}
	if res.Observations, err = deleteIDs(ctx, tx, "observations", sel.Observations); err != nil {
		return res, err
	}
	if res.Memories, err = deleteIDs(ctx, tx, "memories", sel.Memories); err != nil {
		return res, err
	}
	if res.Episodes, err = deleteIDs(ctx, tx, "episodes", sel.Episodes); err != nil {
		return res, err
	}
	if res.Rules, err = deleteRuleIDs(ctx, tx, sel.Rules); err != nil {
		return res, err
	}
	if res.Queue, err = deleteIDs(ctx, tx, "queue", sel.Queue); err != nil {
		return res, err
	}
	if res.Ledger, err = deleteIDs(ctx, tx, "ledger", sel.Ledger); err != nil {
		return res, err
	}
	for _, c := range scrubColumns {
		n, err := scrubColumnRows(ctx, tx, c, texts)
		if err != nil {
			return res, err
		}
		res.Scrubbed += n
	}
	for _, t := range purgeTables {
		if _, err := tx.ExecContext(ctx, noDeleteSQL(t)); err != nil {
			return res, err
		}
	}
	for _, t := range []string{"observations", "rules", "memories", "episodes"} {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s_fts (%s_fts) VALUES ('rebuild')", t, t)); err != nil {
			return res, err
		}
	}
	return res, tx.Commit()
}

// purgeFile applies a purge to one snapshot file and runs VACUUM on it, so the
// text leaves the file. The snapshot is opened in rollback-journal mode: it is
// a closed copy, not a live bank.
func purgeFile(ctx context.Context, path string, sel Selection, texts []string) error {
	name := filepath.Base(path)
	db, err := sql.Open("sqlite", dsn(path, url.Values{"_pragma": {busyPragma}}))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"PRAGMA journal_mode = DELETE", "PRAGMA secure_delete = ON"} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("memory: purge snapshot %s: %w", name, err)
		}
	}
	if _, err := purgeDB(ctx, db, sel, texts); err != nil {
		return fmt.Errorf("memory: purge snapshot %s: %w", name, err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("memory: vacuum snapshot %s: %w", name, err)
	}
	return nil
}

// checkpointTruncate moves WAL pages into the database and empties the WAL
// file. A busy checkpoint means a reader held the WAL; the purge is committed,
// but its old pages may stay in the WAL until the next checkpoint.
func checkpointTruncate(ctx context.Context, db *sql.DB) error {
	var busy, logged, moved int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logged, &moved); err != nil {
		return fmt.Errorf("memory: checkpoint: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("memory: checkpoint busy; purged text may remain in the WAL until the next checkpoint")
	}
	return nil
}

// scrubColumnRows clears the texts from one column and returns how many rows
// changed. Rows are read first and written after, so no read is open during a
// write.
func scrubColumnRows(ctx context.Context, tx *sql.Tx, c scrubColumn, texts []string) (int, error) {
	if len(texts) == 0 {
		return 0, nil
	}
	changes, err := scrubChanges(ctx, tx, c, texts)
	if err != nil {
		return 0, err
	}
	for _, ch := range changes {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", c.table, c.col), ch.value, ch.id); err != nil {
			return 0, err
		}
	}
	return len(changes), nil
}

type cellChange struct {
	id    int64
	value string
}

// scrubChanges returns the cells of one column that hold a purged text, with
// the text cleared.
func scrubChanges(ctx context.Context, tx *sql.Tx, c scrubColumn, texts []string) ([]cellChange, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT id, %s FROM %s", c.col, c.table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []cellChange
	for rows.Next() {
		var id int64
		var value string
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		var next string
		var changed bool
		if c.json {
			next, changed = scrubJSONText(value, texts)
		} else {
			next, changed = scrubPlain(value, texts)
		}
		if changed {
			out = append(out, cellChange{id: id, value: next})
		}
	}
	return out, rows.Err()
}

// scrubPlain replaces every occurrence of each text in s with PurgedText. It
// reports whether s changed. Empty texts are skipped.
func scrubPlain(s string, texts []string) (string, bool) {
	changed := false
	for _, text := range texts {
		if text != "" && strings.Contains(s, text) {
			s = strings.ReplaceAll(s, text, PurgedText)
			changed = true
		}
	}
	return s, changed
}

// scrubJSONText clears the texts from the strings and object keys of a JSON
// document. It matches decoded strings, so a text is found whatever escaping
// the document uses, and structure is never touched. A value that is not JSON
// gets the plain replace. Numbers pass through unchanged.
func scrubJSONText(doc string, texts []string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return scrubPlain(doc, texts)
	}
	changed := false
	v = scrubJSONValue(v, texts, &changed)
	if !changed {
		return doc, false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return scrubPlain(doc, texts)
	}
	return strings.TrimSuffix(buf.String(), "\n"), true
}

func scrubJSONValue(v any, texts []string, changed *bool) any {
	switch x := v.(type) {
	case string:
		s, c := scrubPlain(x, texts)
		*changed = *changed || c
		return s
	case []any:
		for i := range x {
			x[i] = scrubJSONValue(x[i], texts, changed)
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			nk, c := scrubPlain(k, texts)
			*changed = *changed || c
			out[nk] = scrubJSONValue(val, texts, changed)
		}
		return out
	default:
		return v
	}
}

// deleteIDs deletes rows of table whose id is in ids. table is a fixed name
// from this package, never input.
func deleteIDs(ctx context.Context, ex execer, table string, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := anyArgs(ids)
	return execRows(ctx, ex, fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", table, placeholders(len(args))), args...)
}

func deleteRuleIDs(ctx context.Context, ex execer, ruleIDs []string) (int, error) {
	if len(ruleIDs) == 0 {
		return 0, nil
	}
	args := anyArgs(ruleIDs)
	return execRows(ctx, ex, "DELETE FROM rules WHERE rule_id IN ("+placeholders(len(args))+")", args...)
}

func execRows(ctx context.Context, ex execer, query string, args ...any) (int, error) {
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// placeholders renders n bind markers, "?, ?, ?".
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func anyArgs[T any](vals []T) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}
