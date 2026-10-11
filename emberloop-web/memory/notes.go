// Project notes: repo files recalled like Rules.
//
// Project knowledge lives in Markdown files with frontmatter (id, anchors,
// observed_commit, author, subject_key) under .emberloop/notes/ in the repo.
// Because they are files they are branch-aware (each worktree sees its own
// branch's notes), reviewed by the existing Reviewer loop, versioned by git
// and attributed by git blame. The Daemon only reads them; it never commits.
//
// At Session start the Daemon loads the worktree's notes with LoadNotes and
// indexes them into a temporary per-worktree FTS5 index (NotesIndex, an
// in-memory SQLite database). Recall searches that index with the same
// Query and SplitText rules as bank Rules, then verifies anchors like Rules:
// a note with deleted anchors is skipped (dormant), a changed or otherwise
// uncheckable note keeps its verify flag. Merging User Rules and Project
// notes uses reciprocal rank fusion (FuseRecall, k=60): only ranks fuse,
// BM25 scores from the two tables are never compared.
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// NotesSubdir is the repo-relative directory holding project notes.
const NotesSubdir = ".emberloop/notes"

// RRFK is the reciprocal-rank-fusion constant for merging User Rules and
// Project notes. Score = 1/(k+rank); k=60 per the spec.
const RRFK = 60

// Merge sources for MergedHit.
const (
	MergeRule = "rule"
	MergeNote = "note"
)

// Note is one project note file: the body text plus its frontmatter.
type Note struct {
	// ID is the frontmatter id. Unique per worktree; duplicates load once.
	ID string
	// Text is the Markdown body after the frontmatter.
	Text string
	// Anchors pins the note to code, like Rule anchors.
	Anchors []string
	// ObservedCommit is the commit the note was observed at.
	ObservedCommit string
	// Author is the approving User.
	Author string
	// SubjectKey groups notes like Rules (e.g. go.errors.wrapping).
	SubjectKey string
	// Path is the source file, for notices and debugging. Never injected.
	Path string
}

// VerifiedNote pairs a Note with its anchor verification outcome.
type VerifiedNote struct {
	Note   Note
	Result VerifyResult
}

// MergedHit is one entry of a fused Rule-plus-notes recall. Exactly one of
// Rule and Note is set, per Source.
type MergedHit struct {
	Source string
	Rule   Rule
	Note   Note
	Score  float64
	// RuleResult and NoteResult carry the verification outcome of the set
	// side; the other side is zero. A hit that NeedsFlag injects with
	// FlagNote, like Rules.
	RuleResult VerifyResult
	NoteResult VerifyResult
}

// ParseNote parses one note file. Path names the source for the returned
// Note and for error text; data is the file content.
//
// The file is frontmatter between two --- lines, then the body:
//
//	---
//	id: upload-retries
//	anchors: ["emberloop-web/memory/bank.go"]
//	observed_commit: <sha>
//	author: u1
//	subject_key: go.errors.wrapping
//	---
//	Use the upload handler for uploads.
//
// Anchors also accept a comma-separated scalar or a multiline list:
//
//	anchors:
//	  - a.go
//	  - b.go
//
// Only id and a non-empty body are required; every other field may be
// empty. Anything else malformed (missing delimiters, missing id, empty
// body) is an error the caller turns into a skip notice.
func ParseNote(path string, data []byte) (Note, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Note{}, fmt.Errorf("memory: note %s: missing frontmatter", path)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Note{}, fmt.Errorf("memory: note %s: unclosed frontmatter", path)
	}
	fields, lists := parseFrontmatter(lines[1:end])
	id := stripQuotes(fields["id"])
	if strings.TrimSpace(id) == "" {
		return Note{}, fmt.Errorf("memory: note %s: id missing", path)
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if body == "" {
		return Note{}, fmt.Errorf("memory: note %s: empty body", path)
	}
	anchors := parseAnchorsField(fields["anchors"], lists["anchors"])
	return Note{
		ID:             id,
		Text:           body,
		Anchors:        anchors,
		ObservedCommit: stripQuotes(fields["observed_commit"]),
		Author:         stripQuotes(fields["author"]),
		SubjectKey:     stripQuotes(fields["subject_key"]),
		Path:           path,
	}, nil
}

// parseFrontmatter reads key: value lines plus multiline "- item" lists.
// Keys are lowercased. A "- item" line appends to the most recent key.
func parseFrontmatter(lines []string) (map[string]string, map[string][]string) {
	fields := make(map[string]string)
	lists := make(map[string][]string)
	last := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if last == "" {
				continue
			}
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			lists[last] = append(lists[last], stripQuotes(item))
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		if key == "" {
			continue
		}
		fields[key] = strings.TrimSpace(line[i+1:])
		last = key
	}
	return fields, lists
}

// parseAnchorsField combines the scalar and list forms into a sorted,
// deduplicated anchor list. Empty means the note verifies clean.
func parseAnchorsField(scalar string, list []string) []string {
	var raw []string
	if s := strings.TrimSpace(scalar); s != "" {
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			s = s[1 : len(s)-1]
		}
		for _, part := range strings.Split(s, ",") {
			if a := stripQuotes(strings.TrimSpace(part)); a != "" {
				raw = append(raw, a)
			}
		}
	}
	for _, item := range list {
		if a := strings.TrimSpace(item); a != "" {
			raw = append(raw, a)
		}
	}
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(raw))
	var out []string
	for _, a := range raw {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// stripQuotes removes one pair of matching single or double quotes.
func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

// notesDirFor resolves the notes directory for a Session worktree. A
// worktree rooted at a subdirectory still reads the notes of its toplevel,
// so branch-awareness follows the worktree, not the start directory. Outside
// git the worktree directory itself is the root. Empty workDir gives "".
func notesDirFor(workDir string) string {
	if strings.TrimSpace(workDir) == "" {
		return ""
	}
	if top, err := runGit(context.Background(), workDir, "rev-parse", "--show-toplevel"); err == nil && top != "" {
		return filepath.Join(top, filepath.FromSlash(NotesSubdir))
	}
	return filepath.Join(workDir, filepath.FromSlash(NotesSubdir))
}

// LoadNotes reads the worktree's .emberloop/notes/*.md files, sorted by
// file name. A malformed file is skipped with a notice, never an error; a
// duplicate id keeps the first file and notices the rest. A missing notes
// directory is empty, not an error. Only directory read failures return an
// error. LoadNotes never writes.
func LoadNotes(workDir string) ([]Note, []string, error) {
	dir := notesDirFor(workDir)
	if dir == "" {
		return nil, nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("memory: read notes: %w", err)
	}
	var notes []Note
	var notices []string
	seen := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			notices = append(notices, fmt.Sprintf("note %s: %v, skipped", e.Name(), err))
			continue
		}
		n, err := ParseNote(path, data)
		if err != nil {
			notices = append(notices, fmt.Sprintf("note %s: %v, skipped", e.Name(), err))
			continue
		}
		if first, dup := seen[n.ID]; dup {
			notices = append(notices, fmt.Sprintf("note %s: duplicate id %q (first in %s), skipped", e.Name(), n.ID, first))
			continue
		}
		seen[n.ID] = e.Name()
		notes = append(notes, n)
	}
	return notes, notices, nil
}

// NotesIndex is the temporary per-worktree FTS5 index over one Session's
// notes. It is an in-memory SQLite database built at Session start and
// discarded after; Close frees it. Like bank Rules it indexes the text and
// the identifier-split copy, and searches through Query, so
// TestUploadHandler also matches upload handler.
type NotesIndex struct {
	db    *sql.DB
	notes []Note
	byID  map[string]Note
}

// OpenNotesIndex builds the temporary index over notes. It copies the
// slice; later changes to the argument are not indexed.
func OpenNotesIndex(notes []Note) (*NotesIndex, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("memory: notes index: %w", err)
	}
	// An in-memory database is per connection; one connection keeps one index.
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE notes (id TEXT PRIMARY KEY, text TEXT NOT NULL, split_text TEXT NOT NULL DEFAULT '', anchors_json TEXT NOT NULL DEFAULT '[]', observed_commit TEXT NOT NULL DEFAULT '', author TEXT NOT NULL DEFAULT '', subject_key TEXT NOT NULL DEFAULT '', path TEXT NOT NULL DEFAULT '')`,
		`CREATE VIRTUAL TABLE notes_fts USING fts5(text, split_text, content='notes', content_rowid='rowid')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("memory: notes index: %w", err)
		}
	}
	idx := &NotesIndex{db: db, byID: make(map[string]Note, len(notes))}
	for _, n := range notes {
		anchors, err := json.Marshal(nonNilStrings(n.Anchors))
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("memory: notes index: %w", err)
		}
		res, err := db.Exec(`INSERT INTO notes (id, text, split_text, anchors_json, observed_commit, author, subject_key, path) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.Text, SplitText(n.Text), string(anchors), n.ObservedCommit, n.Author, n.SubjectKey, n.Path)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("memory: notes index: %w", err)
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("memory: notes index: %w", err)
		}
		if _, err := db.Exec(`INSERT INTO notes_fts (rowid, text, split_text) VALUES (?, ?, ?)`, rowid, n.Text, SplitText(n.Text)); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("memory: notes index: %w", err)
		}
		idx.notes = append(idx.notes, n)
		idx.byID[n.ID] = n
	}
	return idx, nil
}

// Close discards the temporary index.
func (idx *NotesIndex) Close() error {
	if idx == nil || idx.db == nil {
		return nil
	}
	return idx.db.Close()
}

// Len reports how many notes the index holds.
func (idx *NotesIndex) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.notes)
}

// Search returns the notes matching text, best match first, at most limit
// (clamped to MaxRecall like bank Recall). Hostile input never errors;
// text with no term returns nil.
func (idx *NotesIndex) Search(ctx context.Context, text string, limit int) ([]Note, error) {
	if idx == nil {
		return nil, nil
	}
	q := Query(text)
	if q == "" {
		return nil, nil
	}
	rows, err := idx.db.QueryContext(ctx, `SELECT n.id, n.text, n.anchors_json, n.observed_commit, n.author, n.subject_key, n.path
		FROM notes_fts JOIN notes AS n ON n.rowid = notes_fts.rowid
		WHERE notes_fts MATCH ? ORDER BY bm25(notes_fts) LIMIT ?`, q, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("memory: search notes: %w", err)
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		var n Note
		var anchors string
		if err := rows.Scan(&n.ID, &n.Text, &anchors, &n.ObservedCommit, &n.Author, &n.SubjectKey, &n.Path); err != nil {
			return nil, fmt.Errorf("memory: search notes: %w", err)
		}
		if err := json.Unmarshal([]byte(anchors), &n.Anchors); err != nil {
			return nil, fmt.Errorf("memory: notes anchors of %s: %w", n.ID, err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// VerifyNote checks one note against the git worktree at workDir, like a
// Rule: unchanged injects as-is, changed or otherwise uncheckable injects
// with the flag, and a note whose anchor path is gone at HEAD is dormant
// (skipped). Anchorless and commitless notes are clean without git. Notes
// have no preference kind; every anchored note verifies.
func VerifyNote(ctx context.Context, workDir string, n Note) VerifyResult {
	return VerifyRule(ctx, workDir, Rule{Kind: KindFact, Anchors: n.Anchors, ObservedCommit: n.ObservedCommit})
}

// VerifyNotes checks every note against workDir in order. It never returns
// an error: per-note failures are VerifyStale results.
func VerifyNotes(ctx context.Context, workDir string, notes []Note) []VerifiedNote {
	out := make([]VerifiedNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, VerifiedNote{Note: n, Result: VerifyNote(ctx, workDir, n)})
	}
	return out
}

// RecallNotes searches the worktree's temporary notes index and verifies
// the hits, like bank recall plus anchor verification. Dormant notes
// (deleted anchors) are skipped; changed and stale notes stay with their
// flag. A nil index or a text with no term recalls nothing.
func RecallNotes(ctx context.Context, workDir string, idx *NotesIndex, text string, limit int) ([]VerifiedNote, error) {
	if idx == nil {
		return nil, nil
	}
	hits, err := idx.Search(ctx, text, limit)
	if err != nil {
		return nil, err
	}
	verified := VerifyNotes(ctx, workDir, hits)
	out := verified[:0]
	for _, v := range verified {
		if v.Result.Status == VerifyDormant {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// FuseRecall merges ranked User Rules and ranked Project notes into one
// recall order by reciprocal rank fusion with k=RRFK: each hit scores
// 1/(k+rank) from its own list's rank (1-based), and the merged list sorts
// by that score. BM25 scores from the two FTS tables are never compared;
// only ranks fuse. Ties break deterministically: Rules before notes, then
// by id. The inputs stay unmodified.
func FuseRecall(rules []VerifiedRule, notes []VerifiedNote) []MergedHit {
	out := make([]MergedHit, 0, len(rules)+len(notes))
	for i, r := range rules {
		out = append(out, MergedHit{
			Source:     MergeRule,
			Rule:       r.Rule,
			Score:      1.0 / (float64(RRFK) + float64(i+1)),
			RuleResult: r.Result,
		})
	}
	for j, n := range notes {
		out = append(out, MergedHit{
			Source:     MergeNote,
			Note:       n.Note,
			Score:      1.0 / (float64(RRFK) + float64(j+1)),
			NoteResult: n.Result,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Source != out[j].Source {
			// User Rules sort before Project notes on tied ranks.
			return out[i].Source == MergeRule
		}
		return mergedID(out[i]) < mergedID(out[j])
	})
	return out
}

// mergedID is the deterministic tie-break key of one fused hit.
func mergedID(h MergedHit) string {
	if h.Source == MergeNote {
		return h.Note.ID
	}
	return h.Rule.RuleID
}
