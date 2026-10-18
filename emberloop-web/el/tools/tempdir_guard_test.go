package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func chmodTree(root string) {
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Mode().Perm()&0o200 == 0 {
			_ = os.Chmod(p, info.Mode().Perm()|0o200)
		}
		return nil
	})
}

func removeRetry(root string) {
	chmodTree(root)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := os.RemoveAll(root); err == nil {
			if _, statErr := os.Stat(root); os.IsNotExist(statErr) {
				return
			}
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestMain removes the process-shared git history template after the run so
// the package leaves nothing behind in the temp dir.
func TestMain(m *testing.M) {
	// A host DEEPSEEK_API_KEY must never turn a fake-backed test into a live call.
	_ = os.Unsetenv("DEEPSEEK_API_KEY")
	code := m.Run()
	if gitHistoryTemplateDir != "" {
		removeRetry(gitHistoryTemplateDir)
	}
	os.Exit(code)
}
