// Cursor tracks how far the miner has read one session log file.
//
// The Daemon miner reads events.jsonl only through FilterMinerEvents (see
// emberloop-web/injection.go); this cursor only slices bytes, never decides
// what is minable.
package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// cursorHeadBytes is how much of the file head the cursor hashes. A changed
// head means the file was rewritten, so the offset is dropped.
const cursorHeadBytes = 4096

// Cursor is the miner's read position in one events.jsonl file: the byte
// offset already mined, the file size seen then, and the hash of the first
// 4 KB then. A changed head or a shrunk file resets the offset.
type Cursor struct {
	Offset   int64  `json:"offset"`
	Size     int64  `json:"size"`
	HeadHash string `json:"headHash"`
}

// headHash hashes the first cursorHeadBytes of data for the cursor.
func headHash(data []byte) string {
	if len(data) > cursorHeadBytes {
		data = data[:cursorHeadBytes]
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// LoadCursor reads the cursor at path. A missing file is a zero cursor, not
// an error, so a first mine starts at the beginning.
func LoadCursor(path string) (Cursor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Cursor{}, nil
		}
		return Cursor{}, fmt.Errorf("memory: load cursor: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		// A corrupt cursor mines from the start rather than stopping the job.
		return Cursor{}, nil
	}
	if c.Offset < 0 || c.Size < 0 {
		return Cursor{}, nil
	}
	return c, nil
}

func SaveCursor(path string, c Cursor) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("memory: save cursor: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memory: save cursor: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "cursor-*.json")
	if err != nil {
		return fmt.Errorf("memory: save cursor: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("memory: save cursor: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("memory: save cursor: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("memory: save cursor: %w", err)
	}
	return nil
}

// ReadNew returns the bytes after cur.Offset up to the last newline, with the
// cursor for them. A changed head or a shrunk file resets the offset and
// reports reset. A torn last line (no trailing newline) is not returned: the
// offset stays while the size advances, so the line is read once complete.
// An empty file returns no bytes and no reset.
func ReadNew(path string, cur Cursor) (chunk []byte, next Cursor, reset bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, cur, false, fmt.Errorf("memory: read log: %w", err)
	}
	head := headHash(raw)
	size := int64(len(raw))
	off := cur.Offset
	// The head check compares only the prefix the cursor saw: an append
	// inside the first 4 KB grows the head without changing its prefix, so
	// growth alone never resets.
	if cur.HeadHash != "" {
		n := cur.Size
		if n > cursorHeadBytes {
			n = cursorHeadBytes
		}
		if n > size {
			n = size
		}
		if n < 0 {
			n = 0
		}
		if headHash(raw[:n]) != cur.HeadHash {
			off, reset = 0, true
		}
	}
	if size < cur.Size || off > size {
		off, reset = 0, true
	}
	if off < 0 {
		off, reset = 0, true
	}
	next = Cursor{Offset: off, Size: size, HeadHash: head}
	if off >= size {
		return nil, next, reset, nil
	}
	fresh := raw[off:]
	end := bytes.LastIndexByte(fresh, '\n')
	if end < 0 {
		return nil, next, reset, nil
	}
	chunk = fresh[:end+1]
	next.Offset = off + int64(end+1)
	return chunk, next, reset, nil
}
