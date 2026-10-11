package memory

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestSnapshotIsReadableCopyAndKeepsNewestTen(t *testing.T) {
	b, _, clock := openTestBank(t)
	ctx := context.Background()
	if _, err := b.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: testUser, Quote: "copied quote", Kind: KindFact}); err != nil {
		t.Fatal(err)
	}
	var taken []string
	for i := 0; i < SnapshotKeep+2; i++ {
		clock.Advance(SnapshotInterval)
		res, err := b.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Taken {
			t.Fatalf("snapshot %d skipped", i)
		}
		taken = append(taken, res.Path)
	}
	want := taken[len(taken)-SnapshotKeep:]

	files, err := snapshotFiles(b.snapDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("kept files = %v\nwant the newest ten = %v", files, want)
	}
	if !sort.StringsAreSorted(files) {
		t.Fatal("snapshot files not in time order")
	}

	db, err := sql.Open("sqlite", dsn(files[len(files)-1], nil))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM observations").Scan(&n); err != nil {
		t.Fatalf("snapshot unreadable: %v", err)
	}
	if n != 1 {
		t.Fatalf("snapshot has %d observations, want 1", n)
	}
}

func TestSnapshotSkipLeavesNoFile(t *testing.T) {
	b, _, clock := openTestBank(t)
	ctx := context.Background()
	if _, err := b.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	clock.Advance(SnapshotInterval - time.Minute)
	res, err := b.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken || res.Path != "" {
		t.Fatalf("snapshot under six hours taken: %+v", res)
	}
	files, _ := snapshotFiles(b.snapDir)
	if len(files) != 1 {
		t.Fatalf("snapshot files = %d, want 1", len(files))
	}
}

func TestSnapshotTimeRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 11, 3, 4, 5, 123456789, time.UTC)
	path := filepath.Join(tempDir(t), now.Format(snapLayout)+".db")
	if got := snapshotTime(path); !got.Equal(now) {
		t.Fatalf("snapshotTime(%s) = %v, want %v", path, got, now)
	}
	if got := snapshotTime(filepath.Join("x", "not-a-stamp.db")); !got.IsZero() {
		t.Fatalf("bad name parsed to %v", got)
	}
}

func TestPruneSnapshotsRemovesOldest(t *testing.T) {
	dir := tempDir(t)
	var files []string
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".db")
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	removed, err := pruneSnapshots(files, 2)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	for _, gone := range files[:2] {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s not removed", gone)
		}
	}
	for _, kept := range files[2:] {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s removed: %v", kept, err)
		}
	}
}
