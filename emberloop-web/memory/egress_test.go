package memory

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

// egressRoles name the memory egresses the redactor guards. A function that
// names one (in an identifier or a string literal; comments never count) and
// sends is a memory egress call site.
var egressRoles = []string{"distill", "decider", "jev", "embedder"}

// egressPackages are package or directory names whose sends are memory
// egress by location: a future deciders/ or jev/ package, and memory itself,
// which must never send raw text anywhere.
var egressPackages = []string{"distill", "decider", "deciders", "jev", "memory"}

// egressTransports are the HTTP sends a guarded function may perform: plain
// calls by name. Do is deliberately absent: sync.Once.Do and retry.Do take a
// func literal while a transport Do takes a request (see the CallExpr case),
// so only the call shape below counts a Do as a send.
var egressTransports = []string{"Post", "Get", "Head", "PostForm", "NewRequest", "NewRequestWithContext"}

var egressModelCalls = []string{"summariz", "complete", "chatcompletion"}

// egressEndpoints are Jev request paths: sending to one is a send whatever
// helper performs it.
var egressEndpoints = []string{"/api/v1/decisions", "/v1/systemone"}

// egressGuards are the only accepted paths to an egress: a call site must
// call one of them in the same function as the send.
var egressGuards = []string{"Redact", "RedactedFor", "ForDistill", "ForDecider", "ForJev", "ForEmbedder"}

// checkEgressSource parses one Go source file and reports the unguarded
// memory-egress sends in it: a function that names a memory egress role (or
// lives in a memory-egress package), performs a send, and calls the redactor
// nowhere in the same function. Comments never count, and a guard in one
// function never silences a send in another. A parse failure is itself a
// report, so a broken file cannot slip through silently.
func checkEgressSource(filename, src string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: unparsable: %v", filename, err)}
	}
	inEgressPackage := false
	pkg := strings.ToLower(f.Name.Name)
	dir := strings.ToLower(filepath.Base(filepath.Dir(filename)))
	for _, want := range egressPackages {
		if strings.Contains(pkg, want) || strings.Contains(dir, want) {
			inEgressPackage = true
			break
		}
	}
	var out []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var role, send, guard string
		// The walk covers the signature and the body but never the doc
		// comment: a role or guard mentioned only in prose never counts.
		noteIdent := func(name string) {
			lowered := strings.ToLower(name)
			for _, want := range egressRoles {
				if strings.Contains(lowered, want) {
					role = name
				}
			}
			for _, want := range egressModelCalls {
				if strings.Contains(lowered, want) {
					send = name
				}
			}
			for _, want := range egressGuards {
				if name == want {
					guard = name
				}
			}
		}
		noteIdent(fn.Name.Name)
		walk := func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				noteIdent(n.Name)
			case *ast.SelectorExpr:
				for _, want := range egressTransports {
					if n.Sel.Name == want {
						send = want
					}
				}
				for _, want := range egressModelCalls {
					if strings.Contains(strings.ToLower(n.Sel.Name), want) {
						send = n.Sel.Name
					}
				}
				for _, want := range egressGuards {
					if n.Sel.Name == want {
						guard = n.Sel.Name
					}
				}
			case *ast.CallExpr:
				// once.Do(func...) and retry.Do(func...) take a func
				// literal: they schedule work, they send nothing. A
				// transport Do takes a request value instead.
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" &&
					len(n.Args) == 1 {
					if _, ok := n.Args[0].(*ast.FuncLit); ok {
						break
					}
				}
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
					send = "Do"
				}
			case *ast.BasicLit:
				if n.Kind.String() != "STRING" {
					break
				}
				lowered := strings.ToLower(n.Value)
				for _, want := range egressRoles {
					if strings.Contains(lowered, want) {
						role = n.Value
					}
				}
				for _, want := range egressEndpoints {
					if strings.Contains(lowered, want) {
						send = want
					}
				}
			}
			return true
		}
		ast.Inspect(fn.Type, walk)
		ast.Inspect(fn.Body, walk)
		if (role != "" || inEgressPackage) && send != "" && guard == "" {
			pos := fset.Position(fn.Pos())
			what := role
			if what == "" {
				what = "package " + f.Name.Name
			}
			out = append(out, fmt.Sprintf("%s:%d: func %s (%s) sends via %s without the redactor: call an egress wrapper",
				filename, pos.Line, fn.Name.Name, what, send))
		}
	}
	return out
}

// TestEgressGoesThroughRedactor enforces the spec's redaction rule: the
// redactor runs before any text leaves the Daemon for the distill LLM, a
// Decider or the Jev API. It walks every Go source file of this module
// (including cmd/ and any future subpackage) and fails an egress call site
// that bypasses the redactor. The Daemon's memory egress clients live in
// this module; add the wrapper at the call site, never an exception.
func TestEgressGoesThroughRedactor(t *testing.T) {
	root := ".."
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("egress scan found no Go files")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, report := range checkEgressSource(f, string(raw)) {
			t.Error(report)
		}
	}
}

func TestCheckEgressSource(t *testing.T) {
	guarded := `package d

import "net/http"

func askDecider(q string) string {
	body := RedactedFor("decider", q)
	resp, _ := http.Post("http://x", "text/plain", nil)
	_ = resp
	return body
}
`
	if reports := checkEgressSource("decider.go", guarded); len(reports) != 0 {
		t.Errorf("guarded call site reported: %v", reports)
	}
	bypass := `package d

import "net/http"

func askDecider(q string) string {
	resp, _ := http.Post("http://x", "text/plain", nil)
	_ = resp
	return q
}
`
	if reports := checkEgressSource("decider.go", bypass); len(reports) != 1 {
		t.Fatalf("bypass reported %d times, want 1: %v", len(reports), reports)
	}
	// A guard in one function never silences a send in another.
	sibling := `package d

import "net/http"

func redactIt(q string) string { return ForDistill(q) }

func distill(q string) string {
	resp, _ := http.Post("http://x", "text/plain", nil)
	_ = resp
	return q
}
`
	if reports := checkEgressSource("distill.go", sibling); len(reports) != 1 {
		t.Fatalf("sibling guard silenced the send: %v", reports)
	}
	// Comments never count, either as a role or as a guard.
	comment := `package d

// distill: this helper only builds a prompt, it never sends it.
func buildPrompt(q string) string { return q }

// ForDistill would go here if this sent anything.
func helper(q string) string { return q }
`
	if reports := checkEgressSource("note.go", comment); len(reports) != 0 {
		t.Errorf("comment-only file reported: %v", reports)
	}
	// A Jev endpoint send is a send whatever helper performs it.
	endpoint := `package d

func askJev(q string) string {
	return post("/api/v1/decisions", q)
}
`
	if reports := checkEgressSource("jevclient.go", endpoint); len(reports) != 1 {
		t.Fatalf("endpoint send reported %d times, want 1: %v", len(reports), reports)
	}
	// A file in a memory-egress package must guard every send, even one
	// that names no role.
	pkg := `package jev

import "net/http"

func health() bool {
	resp, err := http.Get("http://x/health")
	_ = resp
	return err == nil
}
`
	if reports := checkEgressSource("jev/health.go", pkg); len(reports) != 1 {
		t.Fatalf("egress-package send reported %d times, want 1: %v", len(reports), reports)
	}
	// A plain transport call with no role and no egress package passes.
	plain := `package quota

import "net/http"

func refresh() bool {
	resp, err := http.Get("http://x/quota")
	_ = resp
	return err == nil
}
`
	if reports := checkEgressSource("quota.go", plain); len(reports) != 0 {
		t.Errorf("unrelated transport call reported: %v", reports)
	}
	// A broken file cannot slip through silently.
	if reports := checkEgressSource("bad.go", "package d\nfunc {"); len(reports) != 1 {
		t.Fatalf("unparsable source reported %d times, want 1: %v", len(reports), reports)
	}
}
