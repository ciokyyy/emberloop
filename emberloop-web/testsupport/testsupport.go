// Package testsupport holds helpers shared by the test packages of this
// module. Tests must use TempDir, never t.TempDir: Daemon goroutines can
// still write into the directory after the test returns, and t.TempDir
// fails the test with "directory not empty" when that races its cleanup.
package testsupport

import (
	"os"
	"testing"
	"time"
)

// TempDir returns a fresh empty directory that cleans up with retries:
// removal repeats for a few seconds and never fails the test, so a late
// Daemon write cannot flake it the way t.TempDir does.
func TempDir(t testing.TB) string {
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
