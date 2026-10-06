package wick

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/wick/tools"
)

// executeToolByName finds the bound execute in a main list, failing the test
// when it is absent.
func executeToolByName(t *testing.T, main []tools.Tool, name string) tools.Tool {
	t.Helper()
	for _, tl := range main {
		if tl.Name() == name {
			return tl
		}
	}
	t.Fatalf("no %q tool served, have %v", name, toolNames(main))
	return nil
}

func requireExecutePython(t *testing.T) {
	t.Helper()
	if _, err := tools.ResolveBinary("python3"); err != nil {
		t.Skipf("python3 not installed, skipping execute test: %v", err)
	}
}

// Seam: roles offered execute serve it in main; implement does not.
func TestExecuteRoleGating(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"work", "plan", "full", "review", "scout"} {
		served, err := RoleTools(role)
		if err != nil {
			t.Fatal(err)
		}
		if !containsName(toolNames(served), "execute") {
			t.Errorf("%s must serve execute in main: %v", role, toolNames(served))
		}
	}
	served, err := RoleTools("implement")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(served), "execute") {
		t.Errorf("implement must not serve execute: %v", toolNames(served))
	}
	_, execute, err := ResolvedTools("implement")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(execute), "execute") {
		t.Errorf("implement must not offer execute behind execute either: %v", toolNames(execute))
	}
}

// Seam: a disabled tool is not callable from a script, and a renamed tool
// answers under its new name.
func TestExecuteHonoursDisabledAndRenamed(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"grep":{"enabled":false},"find":{"rename":"seek"}}}`)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	requireExecutePython(t)
	main, _, err := ResolvedTools("work")
	if err != nil {
		t.Fatal(err)
	}
	ex := executeToolByName(t, main, "execute")
	root := t.TempDir()
	out, err := ex.Execute(context.Background(), root, json.RawMessage(`{"code":"import tools\n`+
		`print(tools.seek(query=\"x\", grep_keywords=[\"x\"]))\n`+
		`try:\n    tools.grep(pattern=\"x\")\nexcept tools.ToolError as e:\n    print(\"grep refused:\", e)\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(out, "grep refused:") == false {
		t.Errorf("a disabled tool must raise ToolError: %q", out)
	}
}

// Seam: renamed tools appear under the new name in search and help.
func TestExecuteRenamedToolInStub(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"grep":{"rename":"rip"}}}`)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	requireExecutePython(t)
	main, execute, err := ResolvedTools("work")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(execute), "grep") || !containsName(toolNames(execute), "rip") {
		t.Fatalf("grep rename did not reach the execute bucket: %v", toolNames(execute))
	}
	ex := executeToolByName(t, main, "execute")
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\nprint(tools.search(\"rip\"))\nprint(tools.help(\"rip\"))"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "rip(") {
		t.Fatalf("search must list the renamed tool: %q", out)
	}
}

// Seam: orchestration tools and execute itself are not callable from a script.
func TestExecuteExcludesOrchestration(t *testing.T) {
	hermeticSettings(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	requireExecutePython(t)
	main, _, err := ResolvedTools("plan")
	if err != nil {
		t.Fatal(err)
	}
	ex := executeToolByName(t, main, "execute")
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\n`+
		`for name in [\"spawn\", \"execute\"]:\n`+
		`    try:\n        tools.call(name)\n        print(name, \"reached\")\n`+
		`    except tools.ToolError as e:\n        print(name, \"refused\")\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"spawn refused", "execute refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("orchestration must be refused inside execute: %q", out)
		}
	}
}

// Seam: Guard limits still apply to the tools a script calls.
func TestExecuteHonoursGuard(t *testing.T) {
	hermeticSettings(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	requireExecutePython(t)
	inner := tools.Named("write")
	if len(inner) != 1 {
		t.Fatal("need the write tool")
	}
	guarded := tools.Guard(inner[0], func(string) []string { return nil })
	ex := tools.NewExecute([]tools.Tool{guarded}, "test execute", false)
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\n`+
		`try:\n    tools.write(path=\"x.txt\", content=\"hi\")\n    print(\"wrote\")\n`+
		`except tools.ToolError as e:\n    print(\"refused:\", e)\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "refused:") || strings.Contains(out, "wrote") {
		t.Errorf("a Guarded write outside the allowlist must raise ToolError: %q", out)
	}
}

// Seam: disabling execute removes it from the role.
func TestExecuteDisabledServesNothing(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"execute":{"enabled":false}}}`)
	served, err := RoleTools("work")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(served), "execute") {
		t.Errorf("disabled execute still served: %v", toolNames(served))
	}
}

// Seam: a reviewer's execute runs sandboxed — the script cannot write files,
// yet tools.check runs outside the sandbox and can still write its own
// caches.
func TestReviewExecuteIsSandboxed(t *testing.T) {
	hermeticSettings(t)
	if _, err := tools.ResolveBinary("python3"); err != nil {
		t.Skipf("python3 not installed, skipping execute test: %v", err)
	}
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	root := t.TempDir()
	if err := tools.EnsureHealthy(root); err != nil {
		t.Skipf("bwrap not installed or unhealthy, skipping sandboxed execute test: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".wick"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".wick", "check"), []byte("echo cached > wrote-by-check.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	main, _, err := ResolvedTools("review")
	if err != nil {
		t.Fatal(err)
	}
	ex := executeToolByName(t, main, "execute")
	if !strings.Contains(ex.Description(), "grep") || !strings.Contains(ex.Description(), "tools.grep") {
		t.Fatalf("the bound description must list the tools and carry a grep example:\n%s", ex.Description())
	}
	out, err := ex.Execute(context.Background(), root, json.RawMessage(`{"code":"import tools\n`+
		`print(tools.search(\"check\"))\n`+
		`print(tools.check())\n`+
		`open(\"escape.txt\", \"w\").write(\"no\")\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "check(") {
		t.Errorf("a reviewer script must see check: %q", out)
	}
	if cached, err := os.ReadFile(filepath.Join(root, "wrote-by-check.txt")); err != nil || strings.TrimSpace(string(cached)) != "cached" {
		t.Errorf("check must still write its caches outside the sandbox: %q, %v", cached, err)
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Errorf("the script's own write must not land: %v", err)
	}
}
