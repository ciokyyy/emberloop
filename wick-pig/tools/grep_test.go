package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// grepTree builds a small fixture tree for grep tests.
func grepTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.txt":     "hello world\nfoo bar\nhello again\n",
		"sub/b.txt": "say hello\nnothing here\n",
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGrepIsRegisteredReadOnlyWithSchema(t *testing.T) {
	var found Tool
	for _, tl := range All() {
		if tl.Name() == "grep" {
			found = tl
		}
	}
	if found == nil {
		t.Fatal("grep is not registered in All()")
	}
	ro, ok := found.(ReadOnly)
	if !ok || !ro.ReadOnly() {
		t.Fatal("grep must be ReadOnly")
	}
	schema := found.Schema()
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"pattern", "fixed", "path", "glob", "context", "max_results", "ignore_case"} {
		if _, ok := props[key]; !ok {
			t.Errorf("grep schema missing %q", key)
		}
	}
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "pattern" {
		t.Errorf("grep schema required = %v, want [pattern]", required)
	}
}

func TestGrepFindsRegexMatches(t *testing.T) {
	dir := grepTree(t)
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"a.txt:1:hello world", "a.txt:3:hello again", "sub/b.txt:1:say hello"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "foo bar") || strings.Contains(out, "nothing here") {
		t.Errorf("non-matching lines leaked into:\n%s", out)
	}
	if !strings.Contains(out, "3 match") {
		t.Errorf("output must carry the total count, got:\n%s", out)
	}
}

func TestGrepRequiresAPattern(t *testing.T) {
	if _, err := run(t, grepTool{}, grepTree(t), map[string]any{"pattern": "  "}); err == nil {
		t.Fatal("empty pattern accepted")
	}
}

func TestGrepRefusesAPathOutsideTheWorkspace(t *testing.T) {
	if _, err := run(t, grepTool{}, grepTree(t), map[string]any{"pattern": "hello", "path": "../.."}); err == nil {
		t.Fatal("searched outside the workspace")
	}
}
func TestGrepFixedStringSearch(t *testing.T) {
	dir := grepTree(t)
	// "hello." as a regex matches nothing; as a fixed string it must not be
	// treated as a pattern either, so only the literal line matches.
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "hello (world|again)", "fixed": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no matches") {
		t.Errorf("fixed metacharacters must not act as a regex, got:\n%s", out)
	}
	out, err = run(t, grepTool{}, dir, map[string]any{"pattern": "hello world", "fixed": true})
	if err != nil || !strings.Contains(out, "a.txt:1:hello world") {
		t.Fatalf("fixed search = %q, %v", out, err)
	}
	if strings.Contains(out, "hello again") {
		t.Errorf("fixed \"hello world\" must not match \"hello again\":\n%s", out)
	}
}

func TestGrepIgnoreCase(t *testing.T) {
	dir := grepTree(t)
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "HELLO", "ignore_case": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "3 match") {
		t.Errorf("case-insensitive HELLO must find 3 matches, got:\n%s", out)
	}
	out, err = run(t, grepTool{}, dir, map[string]any{"pattern": "HELLO"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no matches") {
		t.Errorf("case-sensitive HELLO must match nothing, got:\n%s", out)
	}
}

func TestGrepGlobNarrowsFiles(t *testing.T) {
	dir := grepTree(t)
	if err := os.WriteFile(filepath.Join(dir, "c.md"), []byte("hello md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "hello", "glob": "*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "3 match") || strings.Contains(out, "c.md") {
		t.Errorf("glob *.txt must find only the txt matches, got:\n%s", out)
	}
}

func TestGrepContextShowsSurroundingLines(t *testing.T) {
	dir := grepTree(t)
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "foo bar", "context": 1})
	if err != nil {
		t.Fatal(err)
	}
	// The match keeps `:`, context lines use `-`, and the total counts the
	// match alone.
	for _, want := range []string{"1 match(es)", "a.txt:2:foo bar", "a.txt-1-hello world", "a.txt-3-hello again"} {
		if !strings.Contains(out, want) {
			t.Errorf("context output missing %q, got:\n%s", want, out)
		}
	}
}

func TestGrepCapCountsMatchesNotContextLines(t *testing.T) {
	dir := t.TempDir()
	body := "line one\nfoo bar\nline three\nfoo again\nline five\n"
	if err := os.WriteFile(filepath.Join(dir, "deep.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "foo", "context": 1, "max_results": 1})
	if err != nil {
		t.Fatal(err)
	}
	// Two matches, one shown with its context; the second match stays out.
	for _, want := range []string{"2 match(es)", "more results", "deep.txt:2:foo bar", "deep.txt-1-line one", "deep.txt-3-line three"} {
		if !strings.Contains(out, want) {
			t.Errorf("capped context output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "deep.txt:4:") {
		t.Errorf("the capped-off match must stay out, got:\n%s", out)
	}
}

func TestGrepGlobDoubleStarCrossesZeroDirectories(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"sub/x.txt":   "hello shallow\n",
		"sub/a/x.txt": "hello deep\n",
		"other.txt":   "hello elsewhere\n",
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rg, fallback := grepBoth(t, dir, map[string]any{"pattern": "hello", "glob": "sub/**/x.txt"})
	for name, out := range map[string]string{"rg": rg, "go": fallback} {
		if !strings.Contains(out, "sub/x.txt") || !strings.Contains(out, "sub/a/x.txt") || strings.Contains(out, "elsewhere") {
			t.Errorf("%s: interior ** must reach zero and more directories, got:\n%s", name, out)
		}
	}
	if rg != fallback {
		t.Errorf("backends disagree:\nrg:\n%s\ngo:\n%s", rg, fallback)
	}
}

func TestGrepPathAndSlashGlobAgree(t *testing.T) {
	dir := grepTree(t)
	for _, args := range []map[string]any{
		{"pattern": "hello", "path": "sub", "glob": "*.txt"},
		{"pattern": "hello", "path": "sub", "glob": "sub/*.txt"},
	} {
		rg, fallback := grepBoth(t, dir, args)
		if rg != fallback {
			t.Errorf("query %v:\nrg:\n%s\ngo:\n%s", args, rg, fallback)
		}
		if !strings.Contains(rg, "sub/b.txt:1:say hello") {
			t.Errorf("query %v lost the match, got:\n%s", args, rg)
		}
	}
}
func TestGrepCapsResultsWithAMoreNote(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, "needle here")
	}
	if err := os.WriteFile(filepath.Join(dir, "hay.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, grepTool{}, dir, map[string]any{"pattern": "needle", "max_results": 4})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "10 match") {
		t.Errorf("capped output must still carry the total, got:\n%s", out)
	}
	if !strings.Contains(out, "more results") {
		t.Errorf("capped output must note there are more results, got:\n%s", out)
	}
	if strings.Contains(out, "hay.txt:5:") {
		t.Errorf("capped output must stop at 4 lines, got:\n%s", out)
	}
}

// grepBoth runs the same call through the rg backend and the Go fallback and
// returns both outputs. Without rg installed there is only one backend, so
// parity is skipped rather than passed vacuously: on such a host the whole
// suite already exercises the fallback through the plain grepTool{} calls.
func grepBoth(t *testing.T, dir string, args map[string]any) (rg, fallback string) {
	t.Helper()
	if rgPath() == "" {
		t.Skip("rg not installed: backend parity needs both backends")
	}
	var err error
	rg, err = run(t, grepTool{}, dir, args)
	if err != nil {
		t.Fatalf("rg backend: %v", err)
	}
	fallback, err = run(t, grepTool{fallbackOnly: true}, dir, args)
	if err != nil {
		t.Fatalf("go fallback: %v", err)
	}
	return rg, fallback
}

func TestGrepRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		// `\#hash.txt` is a literal file name, not a comment.
		".gitignore":    "ignored.txt\nbuild/\n*.log\n\\#hash.txt\n",
		"kept.txt":      "hello kept\n",
		"ignored.txt":   "hello ignored\n",
		"#hash.txt":     "hello hash\n",
		"build/out.txt": "hello built\n",
		"debug.log":     "hello logged\n",
		"sub/kept.txt":  "hello sub\n",
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rg, fallback := grepBoth(t, dir, map[string]any{"pattern": "hello"})
	for name, out := range map[string]string{"rg": rg, "go": fallback} {
		for _, want := range []string{"kept.txt:1:hello kept", "sub/kept.txt:1:hello sub"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: output missing %q, got:\n%s", name, want, out)
			}
		}
		for _, banned := range []string{"ignored", "built", "logged", "hash"} {
			if strings.Contains(out, banned) {
				t.Errorf("%s: gitignored content leaked (%q) into:\n%s", name, banned, out)
			}
		}
	}
}

func TestGrepSkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte("hello\x00binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("hello text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rg, fallback := grepBoth(t, dir, map[string]any{"pattern": "hello"})
	for name, out := range map[string]string{"rg": rg, "go": fallback} {
		if strings.Contains(out, "blob.bin") {
			t.Errorf("%s: binary file searched, got:\n%s", name, out)
		}
		if !strings.Contains(out, "plain.txt:1:hello text") {
			t.Errorf("%s: text match missing, got:\n%s", name, out)
		}
	}
}

func TestGrepInvalidPatternErrors(t *testing.T) {
	for _, tool := range []Tool{grepTool{}, grepTool{fallbackOnly: true}} {
		if _, err := run(t, tool, grepTree(t), map[string]any{"pattern": "hello("}); err == nil {
			t.Errorf("%T accepted an invalid regex", tool)
		}
	}
}

func TestGrepFallbackAgreesWithRipgrep(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".gitignore":    "ignored.txt\n",
		"main.go":       "package main\n\nfunc Hello() string {\n\treturn \"Hello, World\"\n}\n",
		"main_test.go":  "package main\n\nfunc TestHello(t *testing.T) {\n\tif Hello() == \"\" {\n\t\tt.Fail()\n\t}\n}\n",
		"sub/util.go":   "// hello helper\npackage sub\n",
		"ignored.txt":   "Hello ignored\n",
		"notes.md":      "HELLO LOUD\nhello quiet\n",
		"sub/deep.txt":  "line one\nfoo bar\nline three\nfoo again\nline five\n",
		"we-ird.go":     "hello dash file\n",
		"with:colon.go": "hello colon file\n",
		// Paths carrying a digit run: the rg line parser must not mistake
		// them for the line-number field.
		"report-2024-01-15.md": "hello dated\n",
		"a-1-b.go":             "foo dashnum\n",
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	queries := []map[string]any{
		{"pattern": "Hello"},
		{"pattern": "hello", "ignore_case": true},
		{"pattern": "Hello", "fixed": true},
		{"pattern": "Hello.*", "glob": "*.go"},
		{"pattern": "foo", "context": 1},
		{"pattern": "hello", "ignore_case": true, "max_results": 3},
		{"pattern": "hello", "ignore_case": true, "path": "sub"},
		{"pattern": "hello", "ignore_case": true, "glob": "sub/*.txt"},
		{"pattern": "hello", "ignore_case": true, "glob": "**/*.go"},
	}
	for _, q := range queries {
		rg, fallback := grepBoth(t, dir, q)
		if rg != fallback {
			t.Errorf("query %v:\nrg:\n%s\ngo:\n%s", q, rg, fallback)
		}
	}
}

func TestGrepCappedOffMatchStaysOutOfSharedContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("top\nfoo\nfoo\nbottom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"pattern": "foo", "context": 1, "max_results": 1}
	rg, fallback := grepBoth(t, dir, args)
	for name, out := range map[string]string{"rg": rg, "go": fallback} {
		// Two matches, one shown: the capped-off match is its own hit, never
		// the shown hit's context, so line 3 stays out either way.
		for _, want := range []string{"2 match(es)", "more results", "f.txt:2:foo", "f.txt-1-top"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: output missing %q, got:\n%s", name, want, out)
			}
		}
		if strings.Contains(out, "f.txt:3:") || strings.Contains(out, "f.txt-3-") {
			t.Errorf("%s: capped-off match leaked into shared context, got:\n%s", name, out)
		}
	}
	if rg != fallback {
		t.Errorf("backends disagree:\nrg:\n%s\ngo:\n%s", rg, fallback)
	}
}
