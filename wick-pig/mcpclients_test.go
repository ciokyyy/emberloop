package wick

import (
	"encoding/json"
	"testing"

	"example.com/wick/tools"
	"example.com/wick/tools/mcpfake"
)

// TestMcpFakeHelperProcess is the fake MCP server's entry point in this
// binary, for the registry wiring tests below and ticket 20's graph tests.
func TestMcpFakeHelperProcess(t *testing.T) {
	mcpfake.RunIfHelper(t)
}

// fakeServerSettings writes a global settings file with one mcp_servers
// entry running the fake, merged with entry, and returns nothing: the
// registry is built from the effective settings like production does.
func fakeServerSettings(t *testing.T, cfg mcpfake.Config, entry map[string]any) {
	t.Helper()
	command, args, env := mcpfake.Helper(t, cfg)
	base := map[string]any{"command": command, "args": args, "env": env}
	for k, v := range entry {
		base[k] = v
	}
	body, err := json.Marshal(map[string]any{"mcp_servers": map[string]any{"gh": base}})
	if err != nil {
		t.Fatal(err)
	}
	writeSettingsFile(t, string(body))
}

// A configured MCP server plugs into the same resolution as native tools:
// the renamed tool serves under its new name, in main for the role whose
// placement says main and behind execute for the rest.
func TestRoleToolsWithMCPServesRenamedFakeTools(t *testing.T) {
	fakeServerSettings(t,
		mcpfake.Config{Tools: []mcpfake.ToolDef{{Name: "search_repos", Description: "search"}}},
		map[string]any{
			"rename": map[string]string{"search_repos": "gh_search"},
			"roles":  map[string]any{"review": map[string]string{"placement": "main"}},
		})
	s := tools.EffectiveSettings(t.TempDir())
	reg := tools.NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed := MCPSnapshot(reg)

	main, execute, err := ResolvedToolsWithMCP("review", listed)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(toolNames(main), "gh_search") {
		t.Fatalf("review main = %v, want gh_search promoted to main", toolNames(main))
	}

	main, execute, err = ResolvedToolsWithMCP("work", listed)
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(main), "gh_search") || !containsName(toolNames(execute), "gh_search") {
		t.Fatalf("work must keep gh_search behind execute: main=%v execute=%v", toolNames(main), toolNames(execute))
	}
}

// A server that cannot start costs a log line, not the session: the snapshot
// skips it and the good server still serves.
func TestMCPSnapshotSkipsAnUnstartableServer(t *testing.T) {
	fakeServerSettings(t,
		mcpfake.Config{Tools: []mcpfake.ToolDef{{Name: "search_repos"}}}, nil)
	s := tools.EffectiveSettings(t.TempDir())
	s.MCPServers["bad"] = tools.MCPServerSetting{Command: "/nonexistent-server"}
	reg := tools.NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed := MCPSnapshot(reg)
	gh, ok := listed["gh"]
	if !ok || !containsName(toolNames(gh), "search_repos") {
		t.Fatalf("good server lost: %v", listed)
	}
	if _, ok := listed["bad"]; ok {
		t.Fatalf("bad server listed tools: %v", listed)
	}
}
