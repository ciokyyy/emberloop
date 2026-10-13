package el

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
)

// tempDirAllowFile reports test files that may keep process-shared temp
// dirs: TestMain's own herdr stub dir and the shared git templates cleaned
// by TestMain after m.Run (see cleanupGitTemplates).
func tempDirAllowFile(filename, src string) bool {
	base := filepath.Base(filename)
	if base == "main_test.go" || base == "tempdir_guard_test.go" {
		return true
	}
	return strings.Contains(src, "cleanupGitTemplates") ||
		strings.Contains(src, "gitConfigTemplateDir") ||
		strings.Contains(src, "gridIsolation")
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
		case "MkdirTemp":
			if isIdent && id.Name == "os" && !allow {
				out = append(out, fmt.Sprintf("%s:%d: register the dir for TestMain removal or use t.TempDir", filename, pos.Line))
			}
		case "MkdirAll":
			if isIdent && id.Name == "os" && !allow {
				for _, arg := range call.Args {
					if containsOSTempDir(arg) {
						out = append(out, fmt.Sprintf("%s:%d: do not MkdirAll under os.TempDir()", filename, pos.Line))
						break
					}
				}
			}
		}
		return true
	})
	return out
}

// TestNoTempDirLeak fails when a test file creates temp dirs outside the
// shared helpers: every os.MkdirTemp must live in a file TestMain cleans
// (main, templates, grid isolation), and no test may MkdirAll directly
// under os.TempDir().
func TestNoTempDirLeak(t *testing.T) {
	t.Parallel()
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "tools" || d.Name() == "mcp" || d.Name() == "agenttypes" || d.Name() == "statedb" || d.Name() == "cmd" {
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
