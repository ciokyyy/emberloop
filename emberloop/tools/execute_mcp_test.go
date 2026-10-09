package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// mcpStubFake is one MCP tool for the stub tests: it carries its server and
// its server-side name the way the registry's tools do.
type mcpStubFake struct {
	server, name, original, result string
	err                            error
}

func (f mcpStubFake) Name() string        { return f.name }
func (f mcpStubFake) Description() string { return "fake MCP " + f.original + " from " + f.server }
func (f mcpStubFake) Schema() map[string]any {
	return obj(map[string]any{
		"query": map[string]any{"type": "string", "description": "A query."},
	}, "query")
}
func (f mcpStubFake) Execute(context.Context, string, json.RawMessage) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.result, nil
}
func (f mcpStubFake) OriginalName() string { return f.original }
func (f mcpStubFake) MCPServer() string    { return f.server }

// Seam: MCP tools appear in the stub under their configured names and as
// tools.<server>.<name>.
func TestExecuteStubExposesMCPToolsByNameAndNamespace(t *testing.T) {
	stub, err := buildToolsModule("/tmp/sock", []Tool{
		fakeExecuteTool{name: "read", result: "x"},
		mcpStubFake{server: "gh", name: "gh_search", original: "search_repos", result: "hits"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stub, "def gh_search(") {
		t.Fatalf("the stub must define the configured name:\n%s", stub)
	}
	if !strings.Contains(stub, "gh_search") || !strings.Contains(stub, "_ALIASES") {
		t.Fatalf("the stub must carry the server namespace alias:\n%s", stub)
	}
}

// Seam: search and help cover MCP tools, matching names and descriptions.
func TestExecuteStubSearchAndHelpCoverMCPTools(t *testing.T) {
	requirePython(t)
	t.Setenv("EL_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{
		mcpStubFake{server: "gh", name: "gh_search", original: "search_repos", result: "hits"},
	}, "test execute", false)
	code := "import tools\nprint(tools.search(\"search_repos\"))\nprint(tools.search(\"gh\"))\nprint(tools.help(\"gh.gh_search\"))\n"
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "gh_search(") {
		t.Fatalf("search must find the MCP tool by its original name in the description: %q", out)
	}
	if strings.Contains(out, "(see ") {
		t.Fatalf("search must list the tool once, not as name and alias: %q", out)
	}
	if !strings.Contains(out, "fake MCP search_repos") {
		t.Fatalf("help through the server namespace must show the MCP description: %q", out)
	}
}

// Seam: a script calls an MCP tool through the namespace; a failing server
// call raises ToolError naming the server.
func TestExecuteScriptCallsMCPToolThroughNamespace(t *testing.T) {
	requirePython(t)
	t.Setenv("EL_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{
		mcpStubFake{server: "gh", name: "gh_search", original: "search_repos", result: "three hits"},
		mcpStubFake{server: "gh", name: "broken", original: "broken", err: errors.New(`mcp server "gh": tool "broken" failed: boom`)},
	}, "test execute", false)
	code := "import tools\n" +
		"print(tools.gh_search(query=\"x\"))\n" +
		"print(tools.gh.gh_search(query=\"x\"))\n" +
		"try:\n    tools.broken()\nexcept tools.ToolError as e:\n    print(\"caught:\", e)\n"
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"three hits", "caught:", `mcp server "gh"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("MCP output must contain %q: %q", want, out)
		}
	}
	if strings.Count(out, "three hits") != 2 {
		t.Fatalf("both the plain and the namespaced call must answer: %q", out)
	}
}

// Seam: the description carries one line per MCP server (name, summary,
// count) and no schemas.
func TestExecuteDescriptionListsMCPServers(t *testing.T) {
	desc := ExecuteDescription([]string{"grep"}, nil, []MCPServerLine{
		{Name: "gh", Summary: "GitHub search", Tools: 2},
		{Name: "zz", Tools: 1},
	})
	for _, want := range []string{`"gh"`, "GitHub search", "2 tools", `"zz"`, "1 tool"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description must contain %q:\n%s", want, desc)
		}
	}
	if strings.Contains(desc, `"type"`) || strings.Contains(desc, "inputSchema") {
		t.Fatalf("no schemas belong in the description:\n%s", desc)
	}
}

// Seam: a renamed MCP tool keeps its server and original name: the stub
// serves it under the new name with the server namespace intact.
func TestExecuteStubRenamedMCPToolKeepsServer(t *testing.T) {
	requirePython(t)
	t.Setenv("EL_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{
		Renamed(mcpStubFake{server: "gh", name: "search_repos", original: "search_repos", result: "hits"}, "gh_search"),
	}, "test execute", false)
	code := "import tools\nprint(tools.search(\"gh\"))\nprint(tools.gh.gh_search(query=\"x\"))\n"
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "gh_search(") || !strings.Contains(out, "hits") {
		t.Fatalf("a renamed MCP tool must stay namespaced and callable: %q", out)
	}
}
