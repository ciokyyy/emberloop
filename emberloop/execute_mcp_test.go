package emberloop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop/tools"
	"example.com/emberloop/tools/mcpfake"
)

// resolveExecuteWithFake resolves role against one fake MCP server and
// returns the bound execute tool, both buckets, and the server's method log.
func resolveExecuteWithFake(t *testing.T, role string, cfg mcpfake.Config, entry map[string]any) (tools.Tool, []string, []string, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "methods.log")
	cfg.LogPath = log
	fakeServerSettings(t, cfg, entry)
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	s := tools.EffectiveSettings(t.TempDir())
	reg := tools.NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed := MCPSnapshot(reg)
	main, execute, err := ResolvedToolsWithMCP(role, listed)
	if err != nil {
		t.Fatal(err)
	}
	return executeToolByName(t, main, "execute"), toolNames(main), toolNames(execute), log
}

func readMethodLog(t *testing.T, log string) string {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Seam: a script calls an MCP tool from a fake server, plain and namespaced.
func TestExecuteMCPToolCallEndToEnd(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	ex, _, _, _ := resolveExecuteWithFake(t, "work",
		mcpfake.Config{Tools: []mcpfake.ToolDef{{Name: "search_repos", Description: "search repos"}}},
		map[string]any{"summary": "GitHub search"})
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\n`+
		`print(tools.search_repos(query=\"emberloop\"))\n`+
		`print(tools.gh.search_repos(query=\"emberloop\"))\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Count(out, "search_repos(") != 2 {
		t.Fatalf("both calls must reach the fake server: %q", out)
	}
}

// Seam: a failing server call raises ToolError naming the server.
func TestExecuteMCPToolErrorNamesServer(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	ex, _, _, _ := resolveExecuteWithFake(t, "work",
		mcpfake.Config{
			Tools: []mcpfake.ToolDef{{Name: "search_repos"}},
			Calls: map[string]mcpfake.Result{"search_repos": {Text: "quota spent", IsError: true}},
		}, nil)
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\n`+
		`try:\n    tools.search_repos()\nexcept tools.ToolError as e:\n    print(\"caught:\", e)\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"caught:", `mcp server "gh"`, "quota spent"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ToolError must name the server and its text: %q", out)
		}
	}
}

// Seam: the description lists servers one line each (name, summary, count)
// and carries no schemas.
func TestExecuteMCPDescriptionServerLines(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	ex, _, execute, _ := resolveExecuteWithFake(t, "work",
		mcpfake.Config{Tools: []mcpfake.ToolDef{
			{Name: "search_repos", Description: "search repos"},
			{Name: "get_repo", Description: "fetch a repo"},
		}},
		map[string]any{"summary": "GitHub search"})
	if !containsName(execute, "search_repos") || !containsName(execute, "get_repo") {
		t.Fatalf("MCP tools must resolve behind execute: %v", execute)
	}
	desc := ex.Description()
	for _, want := range []string{`"gh"`, "GitHub search", "2 tools"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description must contain %q:\n%s", want, desc)
		}
	}
	if strings.Contains(desc, `"type"`) || strings.Contains(desc, "inputSchema") || strings.Contains(desc, "search_repos(") {
		t.Fatalf("no schemas or per-tool signatures belong in the description:\n%s", desc)
	}
}

// Seam: search and help find MCP tools without calling the server; the first
// real call reaches it.
func TestExecuteMCPSearchHelpAreLazy(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	ex, _, _, log := resolveExecuteWithFake(t, "work",
		mcpfake.Config{Tools: []mcpfake.ToolDef{{Name: "search_repos", Description: "search repos"}}},
		nil)
	before := readMethodLog(t, log)
	if strings.Contains(before, "tools/call") {
		t.Fatalf("resolving must not call tools: %q", before)
	}
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\n`+
		`print(tools.search(\"repos\"))\n`+
		`print(tools.help(\"gh.search_repos\"))\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "search_repos(") || !strings.Contains(out, "search repos") {
		t.Fatalf("search and help must cover the MCP tool: %q", out)
	}
	if logged := readMethodLog(t, log); strings.Contains(logged, "tools/call") {
		t.Fatalf("search/help must not call the server: %q", logged)
	}
	out, err = ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\nprint(tools.search_repos())"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "search_repos(") {
		t.Fatalf("the call must reach the fake server: %q", out)
	}
	if logged := readMethodLog(t, log); !strings.Contains(logged, "tools/call\n") {
		t.Fatalf("the first call must reach the server: %q", logged)
	}
}

// Seam: include, exclude, rename, enabled and per-role placement all take
// effect inside execute.
func TestExecuteMCPSettingsTakeEffect(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	newFake := func(t *testing.T, role string, entry map[string]any) (tools.Tool, []string, []string) {
		t.Helper()
		log := filepath.Join(t.TempDir(), "methods.log")
		fakeServerSettings(t, mcpfake.Config{Tools: []mcpfake.ToolDef{
			{Name: "search_repos"}, {Name: "delete_repo"}, {Name: "get_repo"},
		}, LogPath: log}, entry)
		t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
		s := tools.EffectiveSettings(t.TempDir())
		reg := tools.NewRegistry(s.MCPServers)
		t.Cleanup(reg.Close)
		main, execute, err := ResolvedToolsWithMCP(role, MCPSnapshot(reg))
		if err != nil {
			t.Fatal(err)
		}
		return executeToolByName(t, main, "execute"), toolNames(main), toolNames(execute)
	}
	ex, _, execute := newFake(t, "work", map[string]any{
		"include": []string{"search_*", "get_*"},
		"exclude": []string{"get_repo"},
		"rename":  map[string]string{"search_repos": "gh_search"},
	})
	if !containsName(execute, "gh_search") || containsName(execute, "search_repos") ||
		containsName(execute, "delete_repo") || containsName(execute, "get_repo") {
		t.Fatalf("include/exclude/rename must shape the stub set: %v", execute)
	}
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\n`+
		`print(tools.search(\"gh\"))\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "gh_search(") || strings.Contains(out, "delete_repo(") {
		t.Fatalf("the stub search must reflect the shaped set: %q", out)
	}

	reviewEx, main, execute := newFake(t, "review", map[string]any{
		"roles": map[string]any{"review": map[string]string{"placement": "main"}},
	})
	if !containsName(main, "search_repos") || containsName(execute, "search_repos") {
		t.Fatalf("per-role placement must promote to main for review: main=%v execute=%v", main, execute)
	}
	if strings.Contains(reviewEx.Description(), `"gh"`) {
		t.Fatalf("main-placed MCP tools are in the tool list, not the execute description:\n%s", reviewEx.Description())
	}
	_, main, execute = newFake(t, "work", map[string]any{
		"roles": map[string]any{"review": map[string]string{"placement": "main"}},
	})
	if containsName(main, "search_repos") || !containsName(execute, "search_repos") {
		t.Fatalf("other roles keep the execute placement: main=%v execute=%v", main, execute)
	}

	_, main, execute = newFake(t, "scout", map[string]any{
		"roles": map[string]any{"scout": map[string]any{"enabled": false}},
	})
	for _, got := range append(main, execute...) {
		if got == "search_repos" || got == "delete_repo" || got == "get_repo" {
			t.Fatalf("per-role disable must hide MCP tools: main=%v execute=%v", main, execute)
		}
	}
}

// Seam: the tools/list output and the generated stub agree for the same
// settings, so the two cannot disagree.
func TestExecuteMCPListAndStubAgree(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	ex, main, execute, _ := resolveExecuteWithFake(t, "work",
		mcpfake.Config{Tools: []mcpfake.ToolDef{
			{Name: "search_repos", Description: "search repos"},
			{Name: "get_repo", Description: "fetch a repo"},
		}},
		map[string]any{
			"rename": map[string]string{"search_repos": "gh_search"},
			"roles":  map[string]any{"review": map[string]string{"placement": "main"}},
		})
	if containsName(main, "gh_search") || !containsName(execute, "gh_search") {
		t.Fatalf("work keeps gh_search behind execute: main=%v execute=%v", main, execute)
	}
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\nprint(tools.search(\"\"))"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, name := range execute {
		if name == "execute" {
			continue
		}
		if !strings.Contains(out, name+"(") {
			t.Fatalf("stub search must list execute-bucket %q:\n%s", name, out)
		}
	}
}

// Seam: MCP tools in a sandboxed read-only role work through the socket: the
// server runs outside the sandbox.
func TestExecuteMCPSandboxedReviewCallsThroughSocket(t *testing.T) {
	hermeticSettings(t)
	requireExecutePython(t)
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	root := t.TempDir()
	if err := tools.EnsureHealthy(root); err != nil {
		t.Skipf("bwrap not installed or unhealthy, skipping sandboxed execute test: %v", err)
	}
	ex, _, execute, _ := resolveExecuteWithFake(t, "review",
		mcpfake.Config{Tools: []mcpfake.ToolDef{{Name: "search_repos", Description: "search repos"}}},
		nil)
	if !containsName(execute, "search_repos") {
		t.Fatalf("review must offer the MCP tool behind execute: %v", execute)
	}
	out, err := ex.Execute(context.Background(), root, json.RawMessage(`{"code":"import tools\n`+
		`print(tools.search_repos(query=\"emberloop\"))\n"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "search_repos(") {
		t.Fatalf("a sandboxed reviewer script must reach the MCP server: %q", out)
	}
}
