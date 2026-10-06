package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeCheckFile puts body in <dir>/.wick/check for the check tests.
func writeCheckFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".wick"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".wick", "check"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIsRegisteredReadOnlyWithSchema(t *testing.T) {
	var found Tool
	for _, tl := range All() {
		if tl.Name() == "check" {
			found = tl
		}
	}
	if found == nil {
		t.Fatal("check is not registered in All()")
	}
	ro, ok := found.(ReadOnly)
	if !ok || !ro.ReadOnly() {
		t.Fatal("check must be ReadOnly")
	}
	schema := found.Schema()
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"name", "args"} {
		if _, ok := props[key]; !ok {
			t.Errorf("check schema missing %q", key)
		}
	}
	if required, _ := schema["required"].([]string); len(required) != 0 {
		t.Errorf("check schema required = %v, want none (name defaults to default, args to empty)", required)
	}
}

func TestCheckRunsAPlainCommand(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "echo checking; echo 3 passed\n")
	out, err := run(t, checkTool{}, dir, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"passed (exit 0)", "checking", "3 passed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestCheckReportsAFailureTail(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "echo checking; echo 3 passed; exit 1\n")
	out, err := run(t, checkTool{}, dir, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FAILED (exit 1)", "checking", "3 passed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestCheckTimesOut(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "sleep 5\n")
	old := CheckTimeout
	CheckTimeout = 200 * time.Millisecond
	t.Cleanup(func() { CheckTimeout = old })
	out, err := run(t, checkTool{}, dir, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "TIMED OUT") {
		t.Errorf("a timed-out check must say so, got:\n%s", out)
	}
}

func TestCheckKeepsASixtyLineTail(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "i=1; while [ $i -le 100 ]; do echo line$i; i=$((i+1)); done\n")
	out, err := run(t, checkTool{}, dir, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "passed (exit 0)") {
		t.Fatalf("want a pass, got:\n%s", out)
	}
	if strings.Contains(out, "line1\n") || strings.Contains(out, "line40\n") {
		t.Errorf("early lines must be tailed off, got:\n%s", out)
	}
	if !strings.Contains(out, "line100") || !strings.Contains(out, "line41") {
		t.Errorf("the last 60 lines must stay, got:\n%s", out)
	}
}
func TestCheckRunsANamedCheck(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"default": "echo default-ok", "test": "echo test-ok"}`+"\n")
	out, err := run(t, checkTool{}, dir, map[string]any{"name": "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"passed (exit 0)", "test-ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "default-ok") {
		t.Errorf("the wrong check ran, got:\n%s", out)
	}
	if !strings.Contains(out, "test") {
		t.Errorf("named output must say which check ran, got:\n%s", out)
	}
}

func TestCheckRunsDefaultByDefault(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"default": "echo default-ok", "test": "echo test-ok"}`+"\n")
	out, err := run(t, checkTool{}, dir, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "default-ok") {
		t.Errorf("empty name must run default, got:\n%s", out)
	}
}

func TestCheckUnknownNameListsAvailable(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"default": "echo default-ok", "test": "echo test-ok"}`+"\n")
	_, err := run(t, checkTool{}, dir, map[string]any{"name": "nope"})
	if err == nil {
		t.Fatal("unknown check name accepted")
	}
	for _, want := range []string{"nope", "default", "test"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q, got: %v", want, err)
		}
	}
}

func TestCheckPlainFileUnknownNameListsDefault(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, "echo hi\n")
	_, err := run(t, checkTool{}, dir, map[string]any{"name": "nope"})
	if err == nil {
		t.Fatal("unknown check name accepted on a plain file")
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error must list default, got: %v", err)
	}
}

func TestCheckSubstitutesArgs(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"test": "echo ran with {args}"}`+"\n")
	out, err := run(t, checkTool{}, dir, map[string]any{"name": "test", "args": "this-package"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ran with this-package") {
		t.Errorf("args were not substituted, got:\n%s", out)
	}
}

func TestCheckRefusesShellMetacharactersInArgs(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"test": "echo ran with {args}"}`+"\n")
	for _, bad := range []string{
		"a;b", "a|b", "a&b", "a$b", "a`b", "a\nb", "a\rb", "a<b", "a>b",
		"a(b", "a)b", `a\b`, "a'b", `a"b`, "a*b", "a?b", "a~b", "a#b", "a!b",
		"$(touch /tmp/pwned)", "; rm -rf /",
	} {
		if _, err := run(t, checkTool{}, dir, map[string]any{"name": "test", "args": bad}); err == nil {
			t.Errorf("args %q accepted", bad)
		}
	}
}

func TestCheckAcceptsPlainArgs(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"test": "echo ran with {args}"}`+"\n")
	for _, ok := range []string{"", "this-package", "./wick-pig/...", "-run TestFoo ./...", "a:b=c,d+e%f@g"} {
		out, err := run(t, checkTool{}, dir, map[string]any{"name": "test", "args": ok})
		if err != nil {
			t.Errorf("args %q refused: %v", ok, err)
		}
		if !strings.Contains(out, "passed (exit 0)") {
			t.Errorf("args %q did not pass, got:\n%s", ok, out)
		}
	}
}

func TestCheckRejectsArgsWithoutASlot(t *testing.T) {
	dir := t.TempDir()
	writeCheckFile(t, dir, `{"test": "echo no slot here"}`+"\n")
	if _, err := run(t, checkTool{}, dir, map[string]any{"name": "test", "args": "extra"}); err == nil {
		t.Fatal("non-empty args accepted for a command without a slot")
	}
}

func TestCheckWithoutAFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	_, err := run(t, checkTool{}, dir, map[string]any{})
	if err == nil {
		t.Fatal("missing .wick/check accepted")
	}
	if !strings.Contains(err.Error(), "no checks configured") {
		t.Errorf("error must say no checks are configured, got: %v", err)
	}
}
