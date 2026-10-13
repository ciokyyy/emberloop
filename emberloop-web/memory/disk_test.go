package memory

import (
	"os"
	"testing"

	"example.com/emberloop-web/testsupport"
)

// TestMain drains TempDirs that outlived their test's retry window (late
// engine or git writes), so a package run leaves nothing behind.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, left := range testsupport.CleanupLeaks() {
		os.Stderr.WriteString("memory: leaked temp dir: " + left + "\n")
	}
	os.Exit(code)
}
