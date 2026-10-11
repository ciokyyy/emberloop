package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// busyPragma is how long a statement waits on a locked bank before it fails.
const busyPragma = "busy_timeout(5000)"

// Bank is one User's SQLite bank. Every write runs on the bank's single writer
// goroutine, over one connection, so writers serialize in process. Reads use a
// separate read-only pool; WAL lets them run beside a write.
type Bank struct {
	path      string
	snapDir   string
	now       func() time.Time
	wdb       *sql.DB // writer connection, used only by loop
	rdb       *sql.DB // read-only pool
	jobs      chan writeJob
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// writeJob is one write, run on the writer goroutine. The result goes to reply.
type writeJob struct {
	run   func(db *sql.DB) error
	reply chan error
}

// execer is the part of *sql.DB and *sql.Tx that writes use.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// rowScanner is the part of *sql.Row and *sql.Rows that scanning uses.
type rowScanner interface {
	Scan(dest ...any) error
}

var _ Memory = (*Bank)(nil)

func openBank(path, snapDir string, opts Options) (*Bank, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("memory: bank dir: %w", err)
	}
	// _txlock=immediate makes every writer transaction, and so every
	// migration, take the write lock at BEGIN.
	wdb, err := sql.Open("sqlite", dsn(path, url.Values{
		"_pragma": {busyPragma, "synchronous(NORMAL)"},
		"_txlock": {"immediate"},
	}))
	if err != nil {
		return nil, err
	}
	wdb.SetMaxOpenConns(1)
	if err := tune(wdb); err != nil {
		_ = wdb.Close()
		return nil, err
	}
	if err := migrate(wdb); err != nil {
		_ = wdb.Close()
		return nil, err
	}
	rdb, err := sql.Open("sqlite", dsn(path, url.Values{
		"_pragma": {busyPragma, "query_only(1)"},
	}))
	if err != nil {
		_ = wdb.Close()
		return nil, err
	}
	rdb.SetMaxOpenConns(4)
	b := &Bank{
		path:    path,
		snapDir: snapDir,
		now:     opts.now,
		wdb:     wdb,
		rdb:     rdb,
		jobs:    make(chan writeJob),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go b.loop()
	return b, nil
}

// tune sets WAL, which lets readers run beside the writer. The mode is kept in
// the file, so it is set once per bank.
func tune(db *sql.DB) error {
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode); err != nil {
		return fmt.Errorf("memory: journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("memory: journal_mode is %q, want wal", mode)
	}
	return nil
}

// loop is the bank's one writer goroutine.
func (b *Bank) loop() {
	defer close(b.done)
	for {
		select {
		case j := <-b.jobs:
			j.reply <- j.run(b.wdb)
		case <-b.stop:
			return
		}
	}
}

// write queues run on the writer goroutine and waits for its result.
func (b *Bank) write(ctx context.Context, run func(db *sql.DB) error) error {
	j := writeJob{run: run, reply: make(chan error, 1)}
	select {
	case b.jobs <- j:
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return ErrClosed
	}
	return <-j.reply
}

// Close stops the writer and closes both pools. It is safe to call twice.
func (b *Bank) Close() error {
	b.closeOnce.Do(func() {
		close(b.stop)
		<-b.done
		b.closeErr = errors.Join(b.wdb.Close(), b.rdb.Close())
	})
	return b.closeErr
}

// AddObservation stores a grounded candidate and returns its ID.
func (b *Bank) AddObservation(ctx context.Context, o Observation) (int64, error) {
	if err := validObservation(o); err != nil {
		return 0, err
	}
	evidence := max(o.EvidenceCount, 1)
	var id int64
	err := b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `INSERT INTO observations
			(session_id, event_id, author, quote, split_text, kind, subject_key,
			 evidence_count, decider_json, observed_commit, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			o.SessionID, o.EventID, o.Author, o.Quote, SplitText(o.Quote), o.Kind,
			o.SubjectKey, evidence, o.DeciderJSON, o.ObservedCommit, timeText(b.now()))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

const observationCols = `o.id, o.session_id, o.event_id, o.author, o.quote, o.kind,
	o.subject_key, o.evidence_count, o.decider_json, o.observed_commit, o.created_at`

// SearchObservations returns observations matching text, best match first.
func (b *Bank) SearchObservations(ctx context.Context, text string, limit int) ([]Observation, error) {
	q := Query(text)
	if q == "" {
		return nil, nil
	}
	return queryAll(ctx, b.rdb, scanObservation, `SELECT `+observationCols+`
		FROM observations_fts
		JOIN observations AS o ON o.id = observations_fts.rowid
		WHERE observations_fts MATCH ?
		ORDER BY bm25(observations_fts), o.id
		LIMIT ?`, q, clampLimit(limit))
}

func scanObservation(s rowScanner) (Observation, error) {
	var o Observation
	var created string
	err := s.Scan(&o.ID, &o.SessionID, &o.EventID, &o.Author, &o.Quote, &o.Kind,
		&o.SubjectKey, &o.EvidenceCount, &o.DeciderJSON, &o.ObservedCommit, &created)
	o.CreatedAt = parseTimeText(created)
	return o, err
}

const ruleCols = `r.rule_id, r.version, r.text, r.kind, r.subject_key, r.anchors_json,
	r.observed_commit, r.status, r.source_observation_ids, r.approved_by, r.approved_at`

// AddRule stores version 1 of a new Rule. An empty RuleID gets a fresh ID. The
// bank sets Version, Status (default active) and ApprovedAt.
func (b *Bank) AddRule(ctx context.Context, r Rule) (Rule, error) {
	if r.RuleID == "" {
		r.RuleID = newID()
	}
	r.Version = 1
	if r.Status == "" {
		r.Status = StatusActive
	}
	r.ApprovedAt = b.now()
	if err := validRule(r); err != nil {
		return Rule{}, err
	}
	err := b.write(ctx, func(db *sql.DB) error {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rules WHERE rule_id = ?`, r.RuleID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: rule %s exists; revise it", ErrInvalid, r.RuleID)
		}
		return insertRule(ctx, db, r)
	})
	return r, err
}

// ReviseRule adds the next version of an existing Rule. The previous latest
// version becomes superseded in the same transaction.
func (b *Bank) ReviseRule(ctx context.Context, next Rule) (Rule, error) {
	if next.RuleID == "" {
		return Rule{}, fmt.Errorf("%w: rule id empty", ErrInvalid)
	}
	if next.Status == "" {
		next.Status = StatusActive
	}
	next.ApprovedAt = b.now()
	if err := validRule(next); err != nil {
		return Rule{}, err
	}
	err := b.write(ctx, func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var latest int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM rules WHERE rule_id = ?`, next.RuleID).Scan(&latest); err != nil {
			return err
		}
		if latest == 0 {
			return fmt.Errorf("%w: rule %s", ErrNotFound, next.RuleID)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE rules SET status = ? WHERE rule_id = ? AND version = ?`,
			StatusSuperseded, next.RuleID, latest); err != nil {
			return err
		}
		next.Version = latest + 1
		if err := insertRule(ctx, tx, next); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return Rule{}, err
	}
	return next, nil
}

func insertRule(ctx context.Context, ex execer, r Rule) error {
	anchors, err := json.Marshal(nonNilStrings(r.Anchors))
	if err != nil {
		return err
	}
	sources, err := json.Marshal(nonNilIDs(r.SourceObservationIDs))
	if err != nil {
		return err
	}
	_, err = ex.ExecContext(ctx, `INSERT INTO rules
		(rule_id, version, text, split_text, kind, subject_key, anchors_json,
		 observed_commit, status, source_observation_ids, approved_by, approved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RuleID, r.Version, r.Text, SplitText(r.Text), r.Kind, r.SubjectKey,
		string(anchors), r.ObservedCommit, r.Status, string(sources),
		r.ApprovedBy, timeText(r.ApprovedAt))
	return err
}

// SetRuleStatus changes the status of the latest version. Text and provenance
// stay as they are; the rules guard allows only this change.
func (b *Bank) SetRuleStatus(ctx context.Context, ruleID, status string) error {
	if !validStatus(status) {
		return fmt.Errorf("%w: status %q", ErrInvalid, status)
	}
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE rules SET status = ?
			WHERE rule_id = ? AND version = (SELECT MAX(version) FROM rules WHERE rule_id = ?)`,
			status, ruleID, ruleID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: rule %s", ErrNotFound, ruleID)
		}
		return nil
	})
}

// RuleVersions returns every version of a Rule, oldest first.
func (b *Bank) RuleVersions(ctx context.Context, ruleID string) ([]Rule, error) {
	return queryAll(ctx, b.rdb, scanRule, `SELECT `+ruleCols+`
		FROM rules AS r WHERE r.rule_id = ? ORDER BY r.version`, ruleID)
}

// Recall returns live Rules matching text, best match first, at most MaxRecall.
// The query is built by Query. Live means not superseded and not dormant.
func (b *Bank) Recall(ctx context.Context, text string, limit int) ([]Rule, error) {
	q := Query(text)
	if q == "" {
		return nil, nil
	}
	return queryAll(ctx, b.rdb, scanRule, `SELECT `+ruleCols+`
		FROM rules_fts
		JOIN rules AS r ON r.id = rules_fts.rowid
		WHERE rules_fts MATCH ?
		  AND EXISTS (SELECT 1 FROM rules AS live
		              WHERE live.id = rules_fts.rowid
		                AND live.status NOT IN ('superseded', 'dormant'))
		ORDER BY bm25(rules_fts), r.id
		LIMIT ?`, q, clampLimit(limit))
}

func scanRule(s rowScanner) (Rule, error) {
	var r Rule
	var anchors, sources, approved string
	err := s.Scan(&r.RuleID, &r.Version, &r.Text, &r.Kind, &r.SubjectKey, &anchors,
		&r.ObservedCommit, &r.Status, &sources, &r.ApprovedBy, &approved)
	if err != nil {
		return Rule{}, err
	}
	if err := json.Unmarshal([]byte(anchors), &r.Anchors); err != nil {
		return Rule{}, fmt.Errorf("memory: anchors of %s: %w", r.RuleID, err)
	}
	if err := json.Unmarshal([]byte(sources), &r.SourceObservationIDs); err != nil {
		return Rule{}, fmt.Errorf("memory: sources of %s: %w", r.RuleID, err)
	}
	r.ApprovedAt = parseTimeText(approved)
	return r, nil
}

// Enqueue adds a pending decision and returns its ID.
func (b *Bank) Enqueue(ctx context.Context, item QueueItem) (int64, error) {
	if err := validQueueItem(item); err != nil {
		return 0, err
	}
	var id int64
	err := b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `INSERT INTO queue (kind, payload, status, created_at)
			VALUES (?, ?, 'pending', ?)`, item.Kind, item.Payload, timeText(b.now()))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// Decide records a human decision on a pending queue item. Decided items
// keep their row; a second decision is refused.
func (b *Bank) Decide(ctx context.Context, id int64, status, decidedBy string) error {
	if status != QueueApproved && status != QueueRejected {
		return fmt.Errorf("%w: decision %q", ErrInvalid, status)
	}
	if decidedBy == "" {
		return fmt.Errorf("%w: decider empty", ErrInvalid)
	}
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE queue SET status = ?, decided_by = ?, decided_at = ?
			WHERE id = ? AND status = 'pending'`, status, decidedBy, timeText(b.now()), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return nil
		}
		var found int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue WHERE id = ?`, id).Scan(&found); err != nil {
			return err
		}
		if found == 0 {
			return fmt.Errorf("%w: queue item %d", ErrNotFound, id)
		}
		return fmt.Errorf("%w: queue item %d already decided", ErrInvalid, id)
	})
}

// Queue lists every queue item, oldest first.
func (b *Bank) Queue(ctx context.Context) ([]QueueItem, error) {
	return queryAll(ctx, b.rdb, scanQueueItem, `SELECT id, kind, payload, status,
		decided_by, decided_at, created_at FROM queue ORDER BY id`)
}

func scanQueueItem(s rowScanner) (QueueItem, error) {
	var q QueueItem
	var decided, created string
	err := s.Scan(&q.ID, &q.Kind, &q.Payload, &q.Status, &q.DecidedBy, &decided, &created)
	q.DecidedAt = parseTimeText(decided)
	q.CreatedAt = parseTimeText(created)
	return q, err
}

const profileCols = `version, lines_json, patch_json, approved_by, created_at`

// GetProfile returns the latest Profile version, or Version 0 with no lines
// when the User never saved one.
func (b *Bank) GetProfile(ctx context.Context) (ProfileVersion, error) {
	rows, err := queryAll(ctx, b.rdb, scanProfileVersion, `SELECT `+profileCols+`
		FROM profile_versions ORDER BY version DESC LIMIT 1`)
	if err != nil {
		return ProfileVersion{}, err
	}
	if len(rows) == 0 {
		return ProfileVersion{}, nil
	}
	return rows[0], nil
}

// ProfileHistory returns every Profile version, oldest first.
func (b *Bank) ProfileHistory(ctx context.Context) ([]ProfileVersion, error) {
	return queryAll(ctx, b.rdb, scanProfileVersion, `SELECT `+profileCols+`
		FROM profile_versions ORDER BY version`)
}

// AppendProfileVersion validates patch against the latest version, refuses an
// over-2 KB render with a condense message, and stores the next version. A
// retired line stays stored and hidden.
func (b *Bank) AppendProfileVersion(ctx context.Context, patch ProfilePatch, approvedBy string) (ProfileVersion, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return ProfileVersion{}, fmt.Errorf("%w: profile needs an approver", ErrInvalid)
	}
	var out ProfileVersion
	err := b.write(ctx, func(db *sql.DB) error {
		rows, err := loadProfileRows(ctx, db)
		if err != nil {
			return err
		}
		var current []ProfileLine
		latest := 0
		if n := len(rows); n > 0 {
			current = rows[n-1].Lines
			latest = rows[n-1].Version
		}
		next, err := ApplyProfilePatch(current, patch)
		if err != nil {
			return err
		}
		if err := checkProfileCap(next); err != nil {
			return err
		}
		out, err = insertProfileRow(ctx, db, latest, next, profilePatchJSON(patch), approvedBy, b.now())
		return err
	})
	return out, err
}

// loadProfileRows reads every Profile version, oldest first, for Append and
// Restore inside one writer transaction.
func loadProfileRows(ctx context.Context, db *sql.DB) ([]ProfileVersion, error) {
	return queryAll(ctx, db, scanProfileVersion, `SELECT `+profileCols+`
		FROM profile_versions ORDER BY version`)
}

// insertProfileRow stores lines as the version after latest and returns it.
// Callers hold the bank writer.
func insertProfileRow(ctx context.Context, db *sql.DB, latest int, lines []ProfileLine, patchDoc, approvedBy string, now time.Time) (ProfileVersion, error) {
	linesDoc, err := json.Marshal(nonNilProfileLines(lines))
	if err != nil {
		return ProfileVersion{}, err
	}
	out := ProfileVersion{
		Version:    latest + 1,
		Lines:      append([]ProfileLine(nil), lines...),
		Patch:      patchDoc,
		ApprovedBy: approvedBy,
		CreatedAt:  now,
	}
	_, err = db.ExecContext(ctx, `INSERT INTO profile_versions
		(version, lines_json, patch_json, approved_by, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		out.Version, string(linesDoc), out.Patch, out.ApprovedBy, timeText(out.CreatedAt))
	return out, err
}

func nonNilProfileLines(lines []ProfileLine) []ProfileLine {
	if lines == nil {
		return []ProfileLine{}
	}
	return lines
}

// RestoreProfileVersion copies an older version's lines as the next version,
// so any version can be restored without deleting history.
func (b *Bank) RestoreProfileVersion(ctx context.Context, version int, approvedBy string) (ProfileVersion, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return ProfileVersion{}, fmt.Errorf("%w: profile needs an approver", ErrInvalid)
	}
	if version < 1 {
		return ProfileVersion{}, fmt.Errorf("%w: profile version %d", ErrInvalid, version)
	}
	var out ProfileVersion
	err := b.write(ctx, func(db *sql.DB) error {
		rows, err := loadProfileRows(ctx, db)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("%w: profile has no versions", ErrNotFound)
		}
		var src *ProfileVersion
		for i := range rows {
			if rows[i].Version == version {
				src = &rows[i]
				break
			}
		}
		if src == nil {
			return fmt.Errorf("%w: profile version %d", ErrNotFound, version)
		}
		latest := rows[len(rows)-1].Version
		out, err = insertProfileRow(ctx, db, latest, src.Lines, fmt.Sprintf(`{"restore":%d}`, version), approvedBy, b.now())
		return err
	})
	return out, err
}

// RecordLedger appends one nightly stage row and returns its ID.
func (b *Bank) RecordLedger(ctx context.Context, row LedgerRow) (int64, error) {
	if row.Stage == "" || !validLedgerStatus(row.Status) {
		return 0, fmt.Errorf("%w: ledger stage %q status %q", ErrInvalid, row.Stage, row.Status)
	}
	counts := row.CountsJSON
	if counts == "" {
		counts = "{}"
	}
	if !json.Valid([]byte(counts)) {
		return 0, fmt.Errorf("%w: ledger counts are not JSON", ErrInvalid)
	}
	var id int64
	err := b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `INSERT INTO ledger
			(run_id, stage, status, counts_json, detail, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			row.RunID, row.Stage, row.Status, counts, row.Detail, timeText(b.now()))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// Ledger lists every ledger row, oldest first.
func (b *Bank) Ledger(ctx context.Context) ([]LedgerRow, error) {
	return queryAll(ctx, b.rdb, scanLedgerRow, `SELECT id, run_id, stage, status,
		counts_json, detail, created_at FROM ledger ORDER BY id`)
}

func scanLedgerRow(s rowScanner) (LedgerRow, error) {
	var l LedgerRow
	var created string
	err := s.Scan(&l.ID, &l.RunID, &l.Stage, &l.Status, &l.CountsJSON, &l.Detail, &created)
	l.CreatedAt = parseTimeText(created)
	return l, err
}

// queryAll runs a read and maps every row with scan.
func queryAll[T any](ctx context.Context, db *sql.DB, scan func(rowScanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilIDs(s []int64) []int64 {
	if s == nil {
		return []int64{}
	}
	return s
}
