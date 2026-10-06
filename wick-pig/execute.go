package wick

import (
	"os"
	"strings"

	"example.com/wick/tools"
)

// executeSandboxed reports whether a role's execute Python runs under the
// bubblewrap sandbox: the read-only roles review and scout, and spawned
// agents of the reviewer and scout types. Every other role already holds bash
// or a worktree, so its script runs directly. implement never reaches execute
// at all.
func executeSandboxed(role string) bool {
	switch role {
	case "review", "scout":
		return true
	case "agent":
		typ := os.Getenv("WICK_AGENT_TYPE")
		return typ == "scout" || strings.HasPrefix(typ, "reviewer-")
	}
	return false
}

// boundExecuteTool builds one role's execute tool over its resolved buckets:
// the script may call exactly the role's resolved toolset minus execute
// itself and the orchestration tools (NewExecute excludes those), and the
// description lists the execute bucket by name. Disabled settings serve
// nothing; a rename serves under the new name; any other placement warns and
// stays in main, since execute behind execute would be unreachable. taken
// tracks claimed names so a colliding rename keeps the first, like the rest
// of the resolution. Ticket 17 adds the resolved MCP tools to the same stub;
// native tools only here.
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
	taken[final] = true
	allowed := append(append([]tools.Tool{}, main...), execute...)
	var names []string
	for _, t := range execute {
		names = append(names, t.Name())
	}
	bound := tools.NewExecute(allowed, tools.ExecuteDescription(names), executeSandboxed(role))
	if final != "execute" {
		bound = tools.Renamed(bound, final)
	}
	return []tools.Tool{bound}
}
