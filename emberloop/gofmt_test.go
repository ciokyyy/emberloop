package emberloop

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

// gofmtBinary locates the gofmt binary the gate checks with: first on PATH,
// then beside the toolchain running the test. A missing binary is an error,
// never a skip: skipping would pass unformatted code silently.
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

// goFiles walks dir in-process and returns every Go file under it. The walk
// and the read exist so the gate stays cache-proof: go test attributes only
// in-process file reads to a test's cache key, so without this an unformatted
// file added in a package this one does not import replays a stale "ok"
// instead of re-running the gate. gofmt below judges; the read registers.
func goFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
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
		return nil, err
	}
	return files, nil
}

// unformattedFiles runs gofmt -l over the Go files under dir and returns the
// listed paths. A gofmt failure (a file that does not parse) is an error
// carrying gofmt's own output, so syntax errors never read as a formatting
// fix. The gate uses the gofmt binary rather than go/format because
// formatting varies with the toolchain; the binary is the check.
func unformattedFiles(dir string) ([]string, error) {
	files, err := goFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	bin, err := gofmtBinary()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(bin, append([]string{"-l"}, files...)...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gofmt -l %s: %v\n%s", dir, err, out)
	}
	var bad []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			bad = append(bad, line)
		}
	}
	return bad, nil
}

// TestGofmtGateFlagsUnformattedFile pins the check's detector on an
// independent source of truth: a temp dir holding one unformatted and one
// clean file must report exactly the unformatted one.
func TestGofmtGateFlagsUnformattedFile(t *testing.T) {
	dir := t.TempDir()
	writeGateFile(t, dir, "bad.go", "package p\n\nfunc F() {}\n// c\n")
	writeGateFile(t, dir, "good.go", "package p\n\nfunc F() {}\n\n// c\n")
	bad, err := unformattedFiles(dir)
	if err != nil {
		t.Fatalf("gate on a temp dir: %v", err)
	}
	if len(bad) != 1 || filepath.Base(bad[0]) != "bad.go" {
		t.Fatalf("gate must flag exactly bad.go, got %q", bad)
	}
}

// TestGofmtGateReportsSyntaxError pins the failure split: a file that does
// not parse must surface gofmt's own error, never a "run gofmt -w" line that
// gofmt -w cannot fix.
func TestGofmtGateReportsSyntaxError(t *testing.T) {
	dir := t.TempDir()
	writeGateFile(t, dir, "broken.go", "package p\n\nfunc (\n")
	_, err := unformattedFiles(dir)
	if err == nil {
		t.Fatal("gate must report a file that does not parse")
	}
	if strings.Contains(err.Error(), "gofmt -w") {
		t.Fatalf("a parse failure must not read as a formatting fix: %v", err)
	}
}

// TestGofmtGateEmptyDir pins the empty case: a dir with no Go files reports
// nothing and no error.
func TestGofmtGateEmptyDir(t *testing.T) {
	bad, err := unformattedFiles(t.TempDir())
	if err != nil {
		t.Fatalf("gate on an empty dir: %v", err)
	}
	if len(bad) != 0 {
		t.Fatalf("gate on an empty dir must report nothing, got %q", bad)
	}
}

func writeGateFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestTreeIsGofmtClean is the repo check's formatting gate: every Go file
// under the module must already be gofmt-clean, so a failing format check
// is always the worker's own to fix. It fails naming the files; run
// gofmt -w on them.
func TestTreeIsGofmtClean(t *testing.T) {
	bad, err := unformattedFiles(".")
	if err != nil {
		t.Fatalf("gofmt check failed:\n%v", err)
	}
	if len(bad) > 0 {
		t.Fatalf("unformatted Go files; run gofmt -w on them:\n%s", strings.Join(bad, "\n"))
	}
}
