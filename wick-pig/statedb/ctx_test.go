package statedb

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Seam: overflow store schema — first use creates the ctx tables at version 3.
func TestOpenCreatesCtxTablesAtV3(t *testing.T) {
	db, _ := openTemp(t)

	if v, err := SchemaVersion(db); err != nil || v != 3 {
		t.Fatalf("version = %d, %v; want 3", v, err)
	}
	for _, table := range []string{"ctx_blobs", "ctx_fts"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s present = %d, %v; want it created", table, n, err)
		}
	}
}

// Seam: schema upgrade — a v2 database opens into v3 with its rows intact.
func TestMigrateUpgradesV2ToV3KeepingRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	db, err := sql.Open("sqlite", dsn(path, writeParams()))
	if err != nil {
		t.Fatal(err)
	}
	if err := applyV1(db); err != nil {
		t.Fatal(err)
	}
	if err := applyV2(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(1,?),(2,?)`,
		"2026-10-06T00:00:00Z", "2026-10-06T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO usage(agent, at, harness, provider, model, kind, input_tokens, dedupe_key)
		VALUES('w1','2026-10-06T10:00:00Z','pig','p','m','request',100,'pig:m1')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("Open for upgrade: %v", err)
	}
	defer db2.Close()
	if v, err := SchemaVersion(db2); err != nil || v != 3 {
		t.Fatalf("version = %d, %v; want 3", v, err)
	}
	rows, err := UsageRowsForAgent(db2, "w1")
	if err != nil || len(rows) != 1 || rows[0].Input != 100 {
		t.Fatalf("v2 row after upgrade = %+v, %v; want the 100-token request kept", rows, err)
	}
	var n int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('ctx_blobs','ctx_fts')`).Scan(&n); err != nil || n != 2 {
		t.Errorf("ctx tables after upgrade = %d, %v; want both created", n, err)
	}
}

// Seam: insert and read back — a stored blob keeps its session, source,
// content type and bytes with its chunk line ranges.
func TestCtxBlobRoundTripKeepsChunks(t *testing.T) {
	db, _ := openTemp(t)

	content := "# Caching notes\n\nthe query cache speeds up caching\n\n```go\nline one\nline two\n```\n"
	id, err := InsertCtxBlob(db, CtxBlob{Session: "s1", Source: "bash", ContentType: "text/plain", Bytes: content}, ChunkText(content))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, ok, err := GetCtxBlob(db, id)
	if err != nil || !ok {
		t.Fatalf("get = %+v, %v, %v; want the blob", got, ok, err)
	}
	if got.Session != "s1" || got.Source != "bash" || got.Bytes != content {
		t.Errorf("blob = %+v, want session s1, source bash and the stored bytes", got)
	}
	if got.LineCount != 8 {
		t.Errorf("line count = %d, want 8", got.LineCount)
	}
}

// Seam: BM25 ranking — the chunk holding the exact term ranks first, and
// porter stemming matches caching with cached.
func TestCtxSearchRanksExactFirstWithStemming(t *testing.T) {
	db, _ := openTemp(t)

	id, err := InsertCtxBlob(db, CtxBlob{Session: "s1", Source: "t", Bytes: "unrelated words here\n\nexactmatchterm alpha beta\n"}, ChunkText("unrelated words here\n\nexactmatchterm alpha beta\n"))
	if err != nil {
		t.Fatal(err)
	}
	hits, err := SearchCtx(db, "exactmatchterm", nil, 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0].BlobID != id {
		t.Fatalf("hits = %+v, %v; want the blob first", hits, err)
	}
	if !containsLineRange(hits[0].StartLine, hits[0].EndLine, 3) {
		t.Errorf("hit lines = %d-%d, want the chunk at line 3", hits[0].StartLine, hits[0].EndLine)
	}

	stemID, err := InsertCtxBlob(db, CtxBlob{Session: "s1", Source: "t", Bytes: "the query cache speeds up caching\n"}, ChunkText("the query cache speeds up caching\n"))
	if err != nil {
		t.Fatal(err)
	}
	stemHits, err := SearchCtx(db, "cached", nil, 5)
	if err != nil {
		t.Fatalf("stemmed search: %v", err)
	}
	if !hitHasBlob(stemHits, stemID) {
		t.Errorf("porter MATCH 'cached' misses the caching chunk: %+v", stemHits)
	}
}

// Seam: handle scoping — a search scoped to one blob never returns another.
func TestCtxSearchScopesToSourceHandle(t *testing.T) {
	db, _ := openTemp(t)

	a, err := InsertCtxBlob(db, CtxBlob{Session: "s1", Source: "a", Bytes: "sharedword in blob a\n"}, ChunkText("sharedword in blob a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InsertCtxBlob(db, CtxBlob{Session: "s1", Source: "b", Bytes: "sharedword in blob b\n"}, ChunkText("sharedword in blob b\n")); err != nil {
		t.Fatal(err)
	}
	hits, err := SearchCtx(db, "sharedword", &a, 5)
	if err != nil {
		t.Fatalf("scoped search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("scoped search found nothing")
	}
	for _, h := range hits {
		if h.BlobID != a {
			t.Errorf("scoped hit blob = %d, want only %d: %+v", h.BlobID, a, hits)
		}
	}
}

// Seam: lifetime — rows older than 24 hours are purged on open; fresh rows stay.
func TestOpenPurgesCtxRowsOlderThanADay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-25 * time.Hour).Unix()
	fresh := time.Now().UTC().Unix()
	var oldID int64
	if err := db.QueryRow(`INSERT INTO ctx_blobs(session, source, content_type, bytes, created_at, size_bytes, line_count)
		VALUES('s','old','text/plain','old body',?,?,8) RETURNING id`, old, 8).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ctx_fts(blob_id, chunk_idx, heading, body, start_line, end_line)
		VALUES(?,0,'','old body',1,1)`, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertCtxBlob(db, CtxBlob{Session: "s", Source: "new", Bytes: "fresh body\n", CreatedAt: fresh}, ChunkText("fresh body\n")); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	if _, ok, _ := GetCtxBlob(db2, oldID); ok {
		t.Errorf("blob %d older than 24h survived reopen", oldID)
	}
	var ftsLeft int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM ctx_fts WHERE blob_id=?`, oldID).Scan(&ftsLeft); err != nil || ftsLeft != 0 {
		t.Errorf("fts rows for purged blob = %d, %v; want 0", ftsLeft, err)
	}
	var kept int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM ctx_blobs WHERE source='new'`).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("fresh blobs = %d, %v; want 1", kept, err)
	}
}

func hitHasBlob(hits []CtxHit, id int64) bool {
	for _, h := range hits {
		if h.BlobID == id {
			return true
		}
	}
	return false
}

// Seam: chunking — fenced code blocks stay whole across blank lines, and
// every non-blank line belongs to exactly one chunk with exact line ranges.
func TestChunkTextKeepsCodeBlocksWhole(t *testing.T) {
	content := "# Title\n\npara one line1\npara one line2\n\n```go\ncode line1\n\ncode line2\n```\n\n## Sub\n\ntail\n"
	chunks := ChunkText(content)
	codeChunks := 0
	for _, c := range chunks {
		if strings.Contains(c.Body, "code line1") {
			codeChunks++
			if !strings.Contains(c.Body, "code line2") {
				t.Errorf("code block split across chunks: %+v", c)
			}
		}
	}
	if codeChunks != 1 {
		t.Fatalf("code lines land in %d chunks, want 1: %+v", codeChunks, chunks)
	}
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(content, "\r\n", "\n"), "\n"), "\n")
	covered := map[int]int{}
	for _, c := range chunks {
		if c.StartLine < 1 || c.EndLine > len(lines) || c.StartLine > c.EndLine {
			t.Errorf("chunk range %d-%d outside 1-%d: %+v", c.StartLine, c.EndLine, len(lines), c)
		}
		for n := c.StartLine; n <= c.EndLine; n++ {
			covered[n]++
		}
	}
	for i, line := range lines {
		no := i + 1
		if strings.TrimSpace(line) == "" {
			continue
		}
		if covered[no] != 1 {
			t.Errorf("line %d covered %d times, want exactly once: %q", no, covered[no], line)
		}
	}
}

// Seam: session delete — dropping sessions removes their blobs and chunks
// while other sessions keep theirs.
func TestDeleteCtxForSessionsKeepsOthers(t *testing.T) {
	db, _ := openTemp(t)

	old, err := InsertCtxBlob(db, CtxBlob{Session: "s-old", Source: "a", Bytes: "old body\n"}, ChunkText("old body\n"))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := InsertCtxBlob(db, CtxBlob{Session: "s-new", Source: "b", Bytes: "new body\n"}, ChunkText("new body\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := DeleteCtxForSessions(db, []string{"s-old"}); err != nil || n != 1 {
		t.Fatalf("deleted = %d, %v; want 1", n, err)
	}
	if _, ok, _ := GetCtxBlob(db, old); ok {
		t.Errorf("blob %d of a dropped session survived", old)
	}
	var ftsLeft int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ctx_fts WHERE blob_id=?`, old).Scan(&ftsLeft); err != nil || ftsLeft != 0 {
		t.Errorf("chunks of dropped blob = %d, %v; want 0", ftsLeft, err)
	}
	if _, ok, _ := GetCtxBlob(db, kept); !ok {
		t.Errorf("blob %d of a kept session is gone", kept)
	}
}

func containsLineRange(start, end, line int) bool { return start <= line && line <= end }

// Seam: concurrent opens with a purge due — repeated waves of concurrent
// opens stay clean (regression: a purge write racing setup made
// journal_mode fail with SQLITE_BUSY), and the expired row is collected.
func TestConcurrentOpensWithExpiredRowsStayClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBFileName)
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	oldID, err := InsertCtxBlob(seed, CtxBlob{Session: "s", Source: "old", Bytes: "old body\n", CreatedAt: time.Now().UTC().Add(-25 * time.Hour).Unix()}, ChunkText("old body\n"))
	if err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()

	for round := 0; round < 3; round++ {
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
			t.Fatalf("round %d concurrent open: %v", round, err)
		}
	}
	check, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	if v, err := SchemaVersion(check); err != nil || v != CurrentVersion {
		t.Errorf("version = %d, %v; want %d", v, err, CurrentVersion)
	}
	if _, ok, _ := GetCtxBlob(check, oldID); ok {
		t.Errorf("expired blob %d survived concurrent opens", oldID)
	}
}
