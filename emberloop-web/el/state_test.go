package el

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolateState points the state store at a temp dir, so tests never touch
// the real home.
func isolateState(t *testing.T) {
	t.Helper()
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("EL_AGENT_ID", "")
}

// resolverSettings points the settings store at a temp file and clears the
// env overrides, so a test reads only the file it writes. An empty body
// leaves the file missing.
func resolverSettings(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("EL_SETTINGS", p)
	t.Setenv("EL_DECISION_MODEL", "")
	t.Setenv("EL_FIND_THRESHOLD", "")
	t.Setenv("EL_MAX_WORKERS", "")
	if body != "" {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// writeFile writes body to path, creating its directory.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitOut runs git in dir and returns its trimmed combined output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// chmodTree makes every file under root writable, so temp git trees clean up
// on read-only checkouts.
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

// removeRetry removes root, retrying past transient locks.
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
