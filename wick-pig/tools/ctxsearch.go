package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"example.com/wick/statedb"
)

// openCtxDB opens the state database for overflow reads. Both tools here are
// read-only, so a read-only handle is preferred: it never migrates, imports
// or purges and never blocks a writer. It falls back to a migrating open
// only when the file is missing or predates the overflow store.
//
// Handles are bearer references into this shared database: any session
// holding a ctx: handle can read it. The session column attributes storage
// so prune can drop a run's blobs; it is not an access boundary.
func openCtxDB() (*sql.DB, error) {
	path := statedb.DBPath()
	if ro, err := statedb.OpenReadOnly(path); err == nil {
		var n int
		if cerr := ro.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='ctx_blobs'`).Scan(&n); cerr == nil && n == 1 {
			return ro, nil
		}
		_ = ro.Close()
	}
	return statedb.Open(path)
}

// ctx_search rifles through stored tool output: BM25-ranked snippets over
// the overflow store's FTS5 chunk index. Every role that can read serves it.
type ctxSearchTool struct{}

func (ctxSearchTool) Name() string   { return "ctx_search" }
func (ctxSearchTool) ReadOnly() bool { return true }
func (ctxSearchTool) Description() string {
	return "Search stored tool output (the overflow store): BM25-ranked snippets focused on the match, each with its ctx: handle and line range. " +
		"Takes one or more queries, an optional source handle to scope the search, and a per-query limit (default 3). " +
		"Page a hit with read ctx:<id>:<start>-<end>."
}
func (ctxSearchTool) Schema() map[string]any {
	return obj(map[string]any{
		"queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "One or more search strings."},
		"source":  map[string]any{"type": "string", "description": "Optional ctx: handle to scope the search to one stored output."},
		"limit":   map[string]any{"type": "integer", "description": "Matches per query (default 3)."},
	}, "queries")
}

func (ctxSearchTool) Execute(_ context.Context, root string, args json.RawMessage) (string, error) {
	_ = root
	var p struct {
		Queries []string `json:"queries"`
		Source  string   `json:"source"`
		Limit   int      `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid ctx_search parameters: %w", err)
	}
	if len(p.Queries) == 0 {
		return "", fmt.Errorf("queries is required: give one or more search strings")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 3
	}
	if limit > 20 {
		limit = 20
	}
	var scope *int64
	if strings.TrimSpace(p.Source) != "" {
		id, _, ok := ParseCtxHandle(p.Source)
		if !ok {
			return "", fmt.Errorf("invalid source handle %q: want ctx:<id>", p.Source)
		}
		scope = &id
	}
	db, err := openCtxDB()
	if err != nil {
		return "", err
	}
	defer db.Close()
	var b strings.Builder
	for _, q := range p.Queries {
		hits, err := statedb.SearchCtx(db, q, scope, limit)
		if err != nil {
			return "", err
		}
		if len(hits) == 0 {
			fmt.Fprintf(&b, "query %q: nothing matched\n", q)
			continue
		}
		fmt.Fprintf(&b, "query %q: %d match(es)\n", q, len(hits))
		for _, h := range hits {
			if h.Heading != "" {
				fmt.Fprintf(&b, "ctx:%d:%d-%d (%s): %s\n", h.BlobID, h.StartLine, h.EndLine, h.Heading, h.Snippet)
			} else {
				fmt.Fprintf(&b, "ctx:%d:%d-%d: %s\n", h.BlobID, h.StartLine, h.EndLine, h.Snippet)
			}
		}
	}
	return b.String(), nil
}

// readCtxHandle pages one stored output: ctx:<id> reads from the top (with
// the read offset and limit), ctx:<id>:<range> reads that line range.
func readCtxHandle(path string, offset, limit int) (string, error) {
	id, spec, ok := ParseCtxHandle(path)
	if !ok {
		return "", fmt.Errorf("invalid ctx handle %q: want ctx:<id> or ctx:<id>:<start>-<end>", path)
	}
	db, err := openCtxDB()
	if err != nil {
		return "", err
	}
	defer db.Close()
	blob, found, err := statedb.GetCtxBlob(db, id)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("unknown ctx handle %q: nothing stored under it (rows older than 24 hours are purged)", path)
	}
	lines := splitOverflowLines(blob.Bytes)
	var b strings.Builder
	if spec != "" {
		ranges, err := parseLineSpec(spec)
		if err != nil {
			return "", err
		}
		selected := selectURLLines(lines, ranges)
		fmt.Fprintf(&b, "[ctx:%d] lines %s of %d (%d bytes) from %s as %s\n",
			id, spec, len(lines), len(blob.Bytes), blob.Source, contentLabel(blob.ContentType))
		for _, line := range selected {
			b.WriteString(line + "\n")
		}
		return b.String(), nil
	}
	start, end := window(len(lines), offset, limit)
	fmt.Fprintf(&b, "[ctx:%d] lines %d-%d of %d (%d bytes) from %s as %s\n",
		id, start+1, end, len(lines), len(blob.Bytes), blob.Source, contentLabel(blob.ContentType))
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "... [%d more lines; read ctx:%d:%d-%d to page]\n", len(lines)-end, id, end+1, min(end+100, len(lines)))
	}
	return b.String(), nil
}

func contentLabel(ct string) string {
	if strings.TrimSpace(ct) == "" {
		return "text/plain"
	}
	return ct
}
