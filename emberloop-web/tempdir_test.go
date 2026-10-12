package web

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop-web/testsupport"
)

// bannedCall is the call the guard forbids, assembled from parts so this
// guard file itself — which TestNoTempDir scans — contains no literal
// occurrence of it.
const bannedCall = "t." + "TempDir()"

// tempDir is the only temporary directory for tests that start Daemon
// goroutines (Run drivers, Session log writers, Worker turns). Those can
// still write into the directory after the test returns, and the standard
// per-test cleanup fails the test with "directory not empty" when that
// races it. It delegates to the shared testsupport helper so every package
// cleans up the same way.
func tempDir(t testing.TB) string {
	t.Helper()
	return testsupport.TempDir(t)
}

// checkTempDirSource parses one Go test source file and reports the banned
// temporary-directory calls in it: any .TempDir() method call except the
// shared testsupport helper and os.TempDir. Comments never count (the
// parser drops them), and a parse failure is itself a report so a broken
// file cannot slip through silently.
func checkTempDirSource(filename, src string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: unparsable: %v", filename, err)}
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "TempDir" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && (id.Name == "testsupport" || id.Name == "os") {
			return true
		}
		pos := fset.Position(call.Pos())
		out = append(out, fmt.Sprintf("%s:%d: use tempDir(t) or testsupport.TempDir(t) instead", filename, pos.Line))
		return true
	})
	return out
}

// TestNoTempDir enforces the TempDir guard: no test file in this module may
// call the banned temporary-directory method (comments excluded — the
// parser drops them). Daemon goroutines can write after the test returns,
// so the standard cleanup flakes with "directory not empty"; tempDir and
// testsupport.TempDir retry removal instead. Add the wrapper at the call
// site, never an exception.
func TestNoTempDir(t *testing.T) {
	t.Parallel()
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("tempdir scan found no test files")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, report := range checkTempDirSource(f, string(raw)) {
			t.Error(report)
		}
	}
}

func TestCheckTempDirSource(t *testing.T) {
	t.Parallel()
	call := "package p\n\nimport \"testing\"\n\nfunc f(t *testing.T) { _ = " + bannedCall + " }\n"
	if reports := checkTempDirSource("x_test.go", call); len(reports) != 1 {
		t.Fatalf("call reported %d times, want 1: %v", len(reports), reports)
	}
	comment := "package p\n\n// use tempDir(t), not " + bannedCall + "\nfunc f() {}\n"
	if reports := checkTempDirSource("x_test.go", comment); len(reports) != 0 {
		t.Errorf("comment-only file reported: %v", reports)
	}
	inline := "package p\n\nfunc f() {}\n// _ = " + bannedCall + "\n"
	if reports := checkTempDirSource("x_test.go", inline); len(reports) != 0 {
		t.Errorf("commented-out call reported: %v", reports)
	}
	helper := "package p\n\nimport \"example.com/emberloop-web/testsupport\"\n\nfunc f(t *testing.T) { _ = testsupport.TempDir(t) }\n"
	if reports := checkTempDirSource("x_test.go", helper); len(reports) != 0 {
		t.Errorf("shared helper reported: %v", reports)
	}
	wrapper := "package p\n\nfunc f(t *testing.T) { _ = tempDir(t) }\n"
	if reports := checkTempDirSource("x_test.go", wrapper); len(reports) != 0 {
		t.Errorf("wrapper call reported: %v", reports)
	}
	osdir := "package p\n\nimport \"os\"\n\nfunc f() string { return os.TempDir() }\n"
	if reports := checkTempDirSource("x_test.go", osdir); len(reports) != 0 {
		t.Errorf("os.TempDir reported: %v", reports)
	}
	if reports := checkTempDirSource("bad_test.go", "package p\nfunc {"); len(reports) != 1 {
		t.Fatalf("unparsable source reported %d times, want 1: %v", len(reports), reports)
	}
}
