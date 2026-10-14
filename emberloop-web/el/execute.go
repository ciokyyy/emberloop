package el

import (
	"sort"

	"example.com/emberloop-web/el/tools"
)

// executeSandboxed reports whether a role's execute Python runs under the
// bubblewrap sandbox: the read-only scout role. Every other role already
// holds bash or a worktree, so its script runs directly.
func executeSandboxed(role string) bool {
	return role == "scout"
}

// isMCPTool reports whether t came from a configured MCP server: only those
// tools carry a server-side original name. Renamed wrappers forward it, so a
// rename keeps the marker.
func isMCPTool(t tools.Tool) bool {
	if o, ok := t.(interface{ OriginalName() string }); ok {
		return o.OriginalName() != ""
	}
	return false
}

// mcpServerOf names the configured MCP server a resolved tool was listed
// from, or "" for a native tool. Renamed wrappers forward it, so a rename
// keeps the tools.<server>.<name> namespace.
func mcpServerOf(t tools.Tool) string {
	if s, ok := t.(interface{ MCPServer() string }); ok {
		return s.MCPServer()
	}
	return ""
}

// boundExecuteTool builds one role's execute tool over its resolved buckets:
// the script may call exactly the role's resolved toolset minus execute
// itself and the orchestration tools (NewExecute excludes those), native and
// MCP alike, under their configured names and, for MCP tools, as
// tools.<server>.<name>. The description lists the execute bucket by name
// plus one line per MCP server behind execute. Disabled settings serve
// nothing; a rename serves under the new name; any other placement warns and
// stays in main, since execute behind execute would be unreachable. taken
// tracks claimed names so a colliding rename keeps the first, like the rest
// of the resolution.
func boundExecuteTool(role string, s tools.Settings, main, execute []tools.Tool, taken map[string]bool, warn func(string)) []tools.Tool {
	enabled, placement, rename := true, tools.PlacementMain, ""
	if e, ok := s.Tools["execute"]; ok {
		enabled, placement, rename = effectiveToolSetting("execute", e, role)
	}
	if !enabled {
		return nil
	}
	if placement != tools.PlacementMain {
		warnf(warn, "execute cannot live behind execute; serving it in the main list")
	}
	final := "execute"
	if rename != "" {
		final = rename
	}
	if taken[final] {
		warnf(warn, "settings rename %q collides with a served tool; keeping the first", final)
		return nil
	}
	// A script calls the plain bash: the filter-in-execute tip is for the
	// caller outside execute.
	var allowed []tools.Tool
	for _, t := range main {
		if h, ok := t.(bashHintTool); ok {
			t = h.inner
		}
		allowed = append(allowed, t)
	}
	allowed = append(allowed, execute...)
	var names []string
	for _, t := range execute {
		if !isMCPTool(t) {
			names = append(names, t.Name())
		}
	}
	var mainNames []string
	for _, t := range main {
		if !isMCPTool(t) && !tools.ExecuteExcludedTool(t) {
			mainNames = append(mainNames, t.Name())
		}
	}
	bound := tools.NewExecute(allowed, tools.ExecuteDescription(names, mainNames, mcpServerLines(s, execute)), executeSandboxed(role))
	if final != "execute" {
		bound = tools.Renamed(bound, final)
	}
	return []tools.Tool{bound}
}

// mcpServerLines renders one description line per MCP server with tools
// behind execute, in server name order: the configured name, the short
// summary from settings, and the tool count. Only the execute bucket counts:
// a tool promoted to the main list is described by the tool list itself.
func mcpServerLines(s tools.Settings, execute []tools.Tool) []tools.MCPServerLine {
	counts := map[string]int{}
	for _, t := range execute {
		if server := mcpServerOf(t); server != "" {
			counts[server]++
		}
	}
	var servers []string
	for server := range counts {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	var out []tools.MCPServerLine
	for _, server := range servers {
		out = append(out, tools.MCPServerLine{
			Name:    server,
			Summary: s.MCPServers[server].Summary,
			Tools:   counts[server],
		})
	}
	return out
}
