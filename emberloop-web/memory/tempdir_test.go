package memory

import (
	"testing"

	"example.com/emberloop-web/testsupport"
)

// tempDir is the temporary directory for memory tests, the way the root
// package helper is: engine and git helpers can leave writes racing the
// test return, so cleanup retries removal instead of failing the test (see
// tempdir_test.go in the root package and the TestNoTempDir guard).
func tempDir(t testing.TB) string {
	t.Helper()
	return testsupport.TempDir(t)
}
