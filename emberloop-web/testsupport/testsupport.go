// Package testsupport holds helpers shared by the test packages of this
// module. Tests must use TempDir, never t.TempDir: Daemon goroutines can
// still write into the directory after the test returns, and t.TempDir
// fails the test with "directory not empty" when that races its cleanup.
package testsupport

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var (
	leakMu sync.Mutex
	leaks  []string
	// Counts for the TestMain report: cleanups that removed everything
	// versus trees that resisted and were registered as leaks.
	removedOK  int
	removedBad int
)

// ChmodTree adds owner-write permission to every file under root so a
// read-only tree (for example the 0555 mirror in runs_exec.go) removes
// cleanly. It best-efforts: one unreadable entry never fails the cleanup.
func ChmodTree(root string) {
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Add u+w without clearing other bits; dirs need it to unlink.
		if info.Mode().Perm()&0o200 == 0 {
			_ = os.Chmod(p, info.Mode().Perm()|0o200)
		}
		return nil
	})
}

// RemoveDir removes root after making it writable, retrying briefly so a
// late Daemon write cannot flake it. It reports whether anything remains.
func RemoveDir(root string) bool {
	ChmodTree(root)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := os.RemoveAll(root); err == nil {
			if _, statErr := os.Stat(root); os.IsNotExist(statErr) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TempDir returns a fresh empty directory that cleans up with retries:
// removal repeats for a few seconds and never fails the test, so a late
// Daemon write cannot flake it the way t.TempDir does. A tree that still
// resists (read-only modes, a Daemon write past the deadline) is recorded
// in a process-level leak list that TestMain drains after m.Run, so a
// package run leaves nothing behind.
func TempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "el-web-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if RemoveDir(dir) {
			leakMu.Lock()
			removedOK++
			leakMu.Unlock()
			return
		}
		leakMu.Lock()
		removedBad++
		leaks = append(leaks, dir)
		leakMu.Unlock()
	})
	return dir
}

// Report counts finished cleanups by outcome, for the TestMain log line.
func Report() (ok, bad, pending int) {
	leakMu.Lock()
	defer leakMu.Unlock()
	return removedOK, removedBad, len(leaks)
}

// CleanupLeaks removes every TempDir that outlived its test's retry window.
// TestMain calls it after m.Run, before removing its own home dir. It
// returns the paths that still resist removal.
func CleanupLeaks() []string {
	leakMu.Lock()
	pending := leaks
	leaks = nil
	leakMu.Unlock()
	var left []string
	for _, dir := range pending {
		if !RemoveDir(dir) {
			left = append(left, dir)
		}
	}
	return left
}
