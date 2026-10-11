package web

import (
	"os"
	"testing"
	"time"
)

// tempDir replaces t.TempDir for tests that start Daemon goroutines (Run
// drivers, Session log writers, Worker turns). Those can still write into
// the directory after the test returns, and t.TempDir fails the test with
// "directory not empty" when that races its cleanup. This cleanup retries
// removal for a few seconds and never fails the test.
func tempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "el-web-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if os.RemoveAll(dir) == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return dir
}
