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

// tempDirAllowFile reports test files that may create process-shared temp
// dirs outside the per-test helper: TestMain's own home dir, this guard file
// itself, and shared run-repo templates that register for TestMain removal
// via registerSharedTemp (cleaned with chmod+retry after m.Run).
func tempDirAllowFile(filename, src string) bool {
	base := filepath.Base(filename)
	if base == "main_test.go" || base == "tempdir_test.go" {
		return true
	}
	return strings.Contains(src, "registerSharedTemp")
}

func containsOSTempDir(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && sel.Sel.Name == "TempDir" {
			found = true
			return false
		}
		return true
	})
	return found
}

// checkTempDirSource parses one Go test source file and reports the banned
// temporary-directory calls in it: any .TempDir() method call except the
// shared testsupport helper and os.TempDir, any os.MkdirTemp outside the
// shared helper (TestMain and registered shared templates excepted), and
// any os.MkdirAll rooted at os.TempDir(). Comments never count (the
// parser drops them), and a parse failure is itself a report so a broken
// file cannot slip through silently.
func checkTempDirSource(filename, src string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: unparsable: %v", filename, err)}
	}
	allow := tempDirAllowFile(filename, src)
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, isIdent := sel.X.(*ast.Ident)
		pos := fset.Position(call.Pos())
		switch sel.Sel.Name {
		case "TempDir":
			if isIdent && (id.Name == "testsupport" || id.Name == "os") {
				return true
			}
			out = append(out, fmt.Sprintf("%s:%d: use tempDir(t) or testsupport.TempDir(t) instead", filename, pos.Line))
		case "MkdirTemp":
			if isIdent && id.Name == "os" && !allow {
				out = append(out, fmt.Sprintf("%s:%d: use tempDir(t) or testsupport.TempDir(t) instead of os.MkdirTemp", filename, pos.Line))
			}
		case "MkdirAll":
			if isIdent && id.Name == "os" && !allow {
				for _, arg := range call.Args {
					if containsOSTempDir(arg) {
						out = append(out, fmt.Sprintf("%s:%d: use tempDir(t) instead of os.MkdirAll under os.TempDir()", filename, pos.Line))
						break
					}
				}
			}
		}
		return true
	})
	return out
}

// TestNoTempDir enforces the TempDir guard: no test file in this module may
// call the banned temporary-directory method (comments excluded — the
// parser drops them), except under el/, which keeps its own tempdir guard
// (el/tempdir_guard_test.go) over the Exec seam's fake-exec policy. Daemon
// goroutines can write after the test returns, so the standard cleanup
// flakes with "directory not empty"; tempDir and testsupport.TempDir retry
// removal instead. Add the wrapper at the call site, never an exception.
func TestNoTempDir(t *testing.T) {
	t.Parallel()
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", "node_modules", "dist", "el":
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
	mk := "package p\n\nimport \"os\"\n\nfunc f() string { s, _ := os.MkdirTemp(\"\", \"el-x-\"); return s }\n"
	if reports := checkTempDirSource("x_test.go", mk); len(reports) != 1 {
		t.Errorf("os.MkdirTemp reported %d times, want 1: %v", len(reports), reports)
	}
	mkAllow := "package p\n\nimport \"os\"\n\nfunc f() string { s, _ := os.MkdirTemp(\"\", \"el-x-\"); registerSharedTemp(s); return s }\n"
	if reports := checkTempDirSource("shared_test.go", mkAllow); len(reports) != 0 {
		t.Errorf("registered shared template reported: %v", reports)
	}
	mkAll := "package p\n\nimport \"os\"\n\nfunc f() { _ = os.MkdirAll(os.TempDir()+\"/x\", 0o755) }\n"
	if reports := checkTempDirSource("x_test.go", mkAll); len(reports) != 1 {
		t.Errorf("os.MkdirAll under TempDir reported %d times, want 1: %v", len(reports), reports)
	}
	nested := "package p\n\nimport (\"os\";\"path/filepath\")\n\nfunc f(base string) { _ = os.MkdirAll(filepath.Join(base, \"x\"), 0o755) }\n"
	if reports := checkTempDirSource("x_test.go", nested); len(reports) != 0 {
		t.Errorf("nested MkdirAll reported: %v", reports)
	}
	if reports := checkTempDirSource("bad_test.go", "package p\nfunc {"); len(reports) != 1 {
		t.Fatalf("unparsable source reported %d times, want 1: %v", len(reports), reports)
	}
}
