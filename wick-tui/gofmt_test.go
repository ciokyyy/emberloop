package tui

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Formatting gate for the dashboard module, mirroring wick-pig's: every Go
// file under this module must already be gofmt-clean. The detector itself
// (unformatted file, syntax error, empty dir) is pinned in wick-pig's
// gofmt_test.go; here only the tree check runs.

// gofmtBinary locates the gofmt binary: first on PATH, then beside the
// toolchain running the test.
func gofmtBinary() (string, error) {
	if p, err := exec.LookPath("gofmt"); err == nil {
		return p, nil
	}
	for _, name := range []string{"gofmt", "gofmt.exe"} {
		p := filepath.Join(runtime.GOROOT(), "bin", name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("gofmt not on PATH nor in %s", filepath.Join(runtime.GOROOT(), "bin"))
}

// TestTreeIsGofmtClean fails naming the files; run gofmt -w on them.
func TestTreeIsGofmtClean(t *testing.T) {
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if _, err := os.ReadFile(path); err != nil {
			return err
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		return
	}
	bin, err := gofmtBinary()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, append([]string{"-l"}, files...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("gofmt -l: %v\n%s", err, out)
	}
	if bad := strings.TrimSpace(string(out)); bad != "" {
		t.Fatalf("unformatted Go files; run gofmt -w on them:\n%s", bad)
	}
}
