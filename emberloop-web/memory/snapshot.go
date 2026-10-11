package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// snapLayout names a snapshot file by its UTC time. The fixed width keeps
// names in time order when sorted.
const snapLayout = "20060102T150405.000000000"

// Snapshot copies the bank with VACUUM INTO. It skips when the newest
// snapshot is under SnapshotInterval old. After a snapshot it keeps the newest
// SnapshotKeep files and removes the rest.
func (b *Bank) Snapshot(ctx context.Context) (SnapshotResult, error) {
	var res SnapshotResult
	err := b.write(ctx, func(db *sql.DB) error {
		files, err := snapshotFiles(b.snapDir)
		if err != nil {
			return err
		}
		now := b.now()
		var newest time.Time
		if n := len(files); n > 0 {
			newest = snapshotTime(files[n-1])
		}
		if !snapshotDue(newest, now) {
			res.Kept = len(files)
			return nil
		}
		if err := os.MkdirAll(b.snapDir, 0o755); err != nil {
			return fmt.Errorf("memory: snapshot dir: %w", err)
		}
		path := filepath.Join(b.snapDir, now.Format(snapLayout)+".db")
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("memory: snapshot %s already exists", filepath.Base(path))
		}
		if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
			_ = os.Remove(path)
			return fmt.Errorf("memory: snapshot: %w", err)
		}
		files = append(files, path)
		pruned, err := pruneSnapshots(files, SnapshotKeep)
		res.Taken = true
		res.Path = path
		res.Pruned = pruned
		res.Kept = len(files) - pruned
		return err
	})
	return res, err
}

// snapshotFiles lists a bank's snapshot files, oldest first. A missing
// directory holds none.
func snapshotFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

// snapshotTime reads a snapshot's time from its file name. Zero when the name
// does not parse.
func snapshotTime(path string) time.Time {
	name := strings.TrimSuffix(filepath.Base(path), ".db")
	t, err := time.ParseInLocation(snapLayout, name, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
}

// pruneSnapshots removes the oldest files beyond keep. files is oldest first.
// It returns how many files it removed.
func pruneSnapshots(files []string, keep int) (int, error) {
	var errs []error
	removed := 0
	for i := 0; i < len(files)-keep; i++ {
		if err := os.Remove(files[i]); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}
