// Overflow store: large tool output lives here instead of flooding context.
// One row in ctx_blobs holds the full bytes; ctx_fts (FTS5, porter tokenizer)
// holds one row per chunk with its line range, so ctx_search ranks with BM25
// and read pages by handle. Rows older than 24 hours are purged on open and
// by prune.
package statedb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CtxTTLSeconds is how long a stored output lives: rows older than this are
// purged on database open and by prune.
const CtxTTLSeconds = 24 * 3600

// CtxBlob is one stored tool output: the full bytes plus who stored it and
// when. SizeBytes is len(Bytes); LineCount counts \n plus a final partial
// line, the way the summary reports it.
type CtxBlob struct {
	ID          int64
	Session     string
	Source      string
	ContentType string
	Bytes       string
	CreatedAt   int64
	SizeBytes   int
	LineCount   int
}

// CtxChunk is one searchable piece of a blob. Heading is the markdown
// heading in force where the chunk starts (empty outside any heading);
// StartLine and EndLine are 1-based over the blob's lines.
type CtxChunk struct {
	Heading            string
	Body               string
	StartLine, EndLine int
}

// CtxHit is one BM25-ranked search hit: the blob, the chunk's line range,
// the snippet focused on the match, and the rank (lower ranks first).
type CtxHit struct {
	BlobID             int64
	ChunkIdx           int
	Heading            string
	Snippet            string
	StartLine, EndLine int
	Rank               float64
}

// ChunkText splits content into searchable chunks: a new chunk starts at a
// markdown heading and at every blank-line run, while fenced code blocks
// (``` or ~~~) stay whole even across blank lines. Every non-blank line
// belongs to exactly one chunk, so line ranges are exact.
func ChunkText(content string) []CtxChunk {
	s := strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	var out []CtxChunk
	var block []string
	blockStart := 0
	heading := ""
	inFence := false
	flush := func(end int) {
		hasText := false
		for _, l := range block {
			if strings.TrimSpace(l) != "" {
				hasText = true
				break
			}
		}
		if hasText {
			out = append(out, CtxChunk{
				Heading:   heading,
				Body:      strings.Join(block, "\n"),
				StartLine: blockStart,
				EndLine:   end,
			})
		}
		block = nil
	}
	for i, line := range lines {
		no := i + 1
		trimmed := strings.TrimSpace(line)
		if isFenceLine(trimmed) {
			if !inFence {
				if len(block) > 0 {
					flush(no - 1)
				}
				inFence = true
				block = []string{line}
				blockStart = no
			} else {
				block = append(block, line)
				inFence = false
				flush(no)
			}
			continue
		}
		if inFence {
			block = append(block, line)
			continue
		}
		if h, ok := headingText(trimmed); ok {
			if len(block) > 0 {
				flush(no - 1)
			}
			heading = h
			block = []string{line}
			blockStart = no
			continue
		}
		if trimmed == "" {
			if len(block) > 0 {
				flush(no - 1)
			}
			continue
		}
		if len(block) == 0 {
			blockStart = no
		}
		block = append(block, line)
	}
	if len(block) > 0 {
		flush(len(lines))
	}
	return out
}

func isFenceLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

// headingText reports the text of a markdown ATX heading line.
func headingText(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n > 6 || n == len(trimmed) || (trimmed[n] != ' ' && trimmed[n] != '\t') {
		return "", false
	}
	return strings.TrimSpace(trimmed[n:]), true
}

// applyV3 adds the overflow store the ctx_search ticket fills: ctx_blobs
// holds one row per stored tool output (id, session, source label, created
// at, content type, bytes with its size and line count), and ctx_fts is the
// FTS5 chunk index over it (porter tokenizer, headings a separate column so
// they weigh more in ranking). Chunk coordinates ride along as UNINDEXED
// columns: retrievable, never matched.
func applyV3(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS ctx_blobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT '',
			bytes TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			line_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ctx_blobs_session
			ON ctx_blobs(session, id)`,
		`CREATE INDEX IF NOT EXISTS idx_ctx_blobs_created
			ON ctx_blobs(created_at)`,
		`CREATE VIRTUAL TABLE ctx_fts USING fts5(
			blob_id UNINDEXED, chunk_idx UNINDEXED,
			heading, body,
			start_line UNINDEXED, end_line UNINDEXED,
			tokenize='porter')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("statedb: v3: %w", err)
		}
	}
	return nil
}

// InsertCtxBlob stores one tool output with its chunks, returning the blob
// id the ctx: handle names. CreatedAt defaults to now; size and line count
// are derived from the bytes so callers cannot skew the summary.
func InsertCtxBlob(db *sql.DB, blob CtxBlob, chunks []CtxChunk) (int64, error) {
	if blob.CreatedAt == 0 {
		blob.CreatedAt = time.Now().UTC().Unix()
	}
	blob.SizeBytes = len(blob.Bytes)
	blob.LineCount = countCtxLines(blob.Bytes)
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("statedb: insert ctx blob: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO ctx_blobs(session, source, content_type, bytes, created_at, size_bytes, line_count)
		VALUES(?,?,?,?,?,?,?)`, blob.Session, blob.Source, blob.ContentType, blob.Bytes, blob.CreatedAt, blob.SizeBytes, blob.LineCount)
	if err != nil {
		return 0, fmt.Errorf("statedb: insert ctx blob: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("statedb: insert ctx blob id: %w", err)
	}
	for i, c := range chunks {
		if _, err := tx.Exec(`INSERT INTO ctx_fts(blob_id, chunk_idx, heading, body, start_line, end_line)
			VALUES(?,?,?,?,?,?)`, id, i, c.Heading, c.Body, c.StartLine, c.EndLine); err != nil {
			return 0, fmt.Errorf("statedb: insert ctx chunk: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("statedb: insert ctx blob: %w", err)
	}
	return id, nil
}

// GetCtxBlob reads one stored output by id.
func GetCtxBlob(db *sql.DB, id int64) (CtxBlob, bool, error) {
	var b CtxBlob
	err := db.QueryRow(`SELECT id, session, source, content_type, bytes, created_at, size_bytes, line_count
		FROM ctx_blobs WHERE id=?`, id).Scan(
		&b.ID, &b.Session, &b.Source, &b.ContentType, &b.Bytes, &b.CreatedAt, &b.SizeBytes, &b.LineCount)
	if err == sql.ErrNoRows {
		return CtxBlob{}, false, nil
	}
	if err != nil {
		return CtxBlob{}, false, fmt.Errorf("statedb: get ctx blob %d: %w", id, err)
	}
	return b, true, nil
}

// ftsQuery builds the FTS5 MATCH expression for a user query: word tokens,
// each double-quoted, ORed together. Quoting keeps punctuation in the query
// from parsing as FTS5 syntax; OR keeps a multi-word query matching chunks
// that hold either word, with BM25 ranking the chunk holding every word
// first. No word tokens means no expression: the caller reports no match.
func ftsQuery(query string) string {
	var toks []string
	for _, f := range strings.FieldsFunc(query, func(r rune) bool {
		return !(r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9'))
	}) {
		if f == "" {
			continue
		}
		toks = append(toks, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(toks, " OR ")
}

// SearchCtx returns up to limit BM25-ranked hits for query, optionally scoped
// to one blob. Headings weigh five times a body match: the weight list runs
// over every FTS column in order (unindexed columns carry zero). A query with
// no word tokens matches nothing.
func SearchCtx(db *sql.DB, query string, scopeID *int64, limit int) ([]CtxHit, error) {
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	const cols = `blob_id, chunk_idx, heading,
		snippet(ctx_fts, 3, '[', ']', '...', 12),
		start_line, end_line, bm25(ctx_fts, 0.0, 0.0, 5.0, 1.0, 0.0, 0.0)`
	var rows *sql.Rows
	var err error
	if scopeID != nil {
		rows, err = db.Query(`SELECT `+cols+` FROM ctx_fts
			WHERE blob_id = ? AND ctx_fts MATCH ?
			ORDER BY bm25(ctx_fts, 0.0, 0.0, 5.0, 1.0, 0.0, 0.0) LIMIT ?`, *scopeID, match, limit)
	} else {
		rows, err = db.Query(`SELECT `+cols+` FROM ctx_fts
			WHERE ctx_fts MATCH ?
			ORDER BY bm25(ctx_fts, 0.0, 0.0, 5.0, 1.0, 0.0, 0.0) LIMIT ?`, match, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("statedb: search ctx: %w", err)
	}
	defer rows.Close()
	var out []CtxHit
	for rows.Next() {
		var h CtxHit
		if err := rows.Scan(&h.BlobID, &h.ChunkIdx, &h.Heading, &h.Snippet, &h.StartLine, &h.EndLine, &h.Rank); err != nil {
			return nil, fmt.Errorf("statedb: scan ctx hit: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// PurgeExpiredCtx deletes blobs (and their chunks) older than the 24-hour
// lifetime, returning how many blobs went.
func PurgeExpiredCtx(db *sql.DB, nowUnix int64) (int64, error) {
	cutoff := nowUnix - CtxTTLSeconds
	if _, err := db.Exec(`DELETE FROM ctx_fts WHERE blob_id IN
		(SELECT id FROM ctx_blobs WHERE created_at <= ?)`, cutoff); err != nil {
		return 0, fmt.Errorf("statedb: purge ctx chunks: %w", err)
	}
	res, err := db.Exec(`DELETE FROM ctx_blobs WHERE created_at <= ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("statedb: purge ctx blobs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("statedb: purge ctx blobs: %w", err)
	}
	return n, nil
}

// DeleteCtxForSessions deletes every blob of the named sessions (their
// chunks go with them), returning how many blobs went. Prune calls this for
// the sessions that belong only to pruned runs.
func DeleteCtxForSessions(db *sql.DB, sessions []string) (int64, error) {
	if len(sessions) == 0 {
		return 0, nil
	}
	placeholders := strings.Repeat("?,", len(sessions))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(sessions))
	for i, s := range sessions {
		args[i] = s
	}
	if _, err := db.Exec(`DELETE FROM ctx_fts WHERE blob_id IN
		(SELECT id FROM ctx_blobs WHERE session IN (`+placeholders+`))`, args...); err != nil {
		return 0, fmt.Errorf("statedb: delete ctx chunks: %w", err)
	}
	res, err := db.Exec(`DELETE FROM ctx_blobs WHERE session IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("statedb: delete ctx blobs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("statedb: delete ctx blobs: %w", err)
	}
	return n, nil
}

func countCtxLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
