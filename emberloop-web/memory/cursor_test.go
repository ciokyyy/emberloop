package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadNewReadsUpToLastNewline(t *testing.T) {
	path := filepath.Join(tempDir(t), "events.jsonl")
	writeLog(t, path, "{\"a\":1}\n{\"b\":2}\n{\"torn\":")
	chunk, next, reset, err := ReadNew(path, Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	if reset {
		t.Error("fresh cursor reported reset")
	}
	if string(chunk) != "{\"a\":1}\n{\"b\":2}\n" {
		t.Fatalf("chunk = %q, want the two full lines", chunk)
	}
	// The torn last line is not read: the offset stays behind the size.
	if next.Offset >= next.Size {
		t.Fatalf("cursor = %+v, want offset behind size while a line is torn", next)
	}
	// Nothing new: no bytes, same offset.
	chunk, next2, _, err := ReadNew(path, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk) != 0 || next2.Offset != next.Offset {
		t.Fatalf("second read = %q %+v, want nothing new", chunk, next2)
	}
	// Completing the line makes it readable.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("3\"}\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	chunk, next3, _, err := ReadNew(path, next)
	if err != nil {
		t.Fatal(err)
	}
	if string(chunk) != "{\"torn\":3\"}\n" {
		t.Fatalf("completed line chunk = %q", chunk)
	}
	if next3.Offset != next3.Size {
		t.Fatalf("cursor after full read = %+v, want offset at size", next3)
	}
}

func TestCursorResetsOnTruncate(t *testing.T) {
	path := filepath.Join(tempDir(t), "events.jsonl")
	writeLog(t, path, "{\"a\":1}\n{\"b\":2}\n{\"c\":3}\n")
	_, cur, _, err := ReadNew(path, Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	writeLog(t, path, "{\"a\":1}\n")
	chunk, next, reset, err := ReadNew(path, cur)
	if err != nil {
		t.Fatal(err)
	}
	if !reset {
		t.Error("shrunk file did not reset the cursor")
	}
	if string(chunk) != "{\"a\":1}\n" {
		t.Fatalf("chunk after truncate = %q, want from the start", chunk)
	}
	if next.Offset != int64(len(chunk)) {
		t.Fatalf("cursor after reset = %+v", next)
	}
}

func TestCursorResetsOnRewrite(t *testing.T) {
	path := filepath.Join(tempDir(t), "events.jsonl")
	writeLog(t, path, "{\"a\":1}\n{\"b\":2}\n")
	_, cur, _, err := ReadNew(path, Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	// Same size, different head: a rewrite, not an append.
	writeLog(t, path, "{\"a\":1}\n{\"X\":9}\n")
	chunk, _, reset, err := ReadNew(path, cur)
	if err != nil {
		t.Fatal(err)
	}
	if !reset {
		t.Error("rewritten head did not reset the cursor")
	}
	if string(chunk) != "{\"a\":1}\n{\"X\":9}\n" {
		t.Fatalf("chunk after rewrite = %q, want the whole file", chunk)
	}
}

func TestCursorSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(tempDir(t), "sub", "miner.cursor.json")
	c := Cursor{Offset: 41, Size: 100, HeadHash: "abc"}
	if err := SaveCursor(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCursor(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("loaded cursor = %+v, want %+v", got, c)
	}
	if _, err := LoadCursor(filepath.Join(tempDir(t), "missing.json")); err != nil {
		t.Fatalf("missing cursor err = %v, want nil", err)
	}
}
