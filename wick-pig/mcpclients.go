package wick

import (
	"context"

	"example.com/wick/tools"
)

// MCPSnapshot lists every configured MCP server behind reg for one
// resolution. A server that fails to start or answer is warned about once
// and skipped, so one broken server costs its own tools, not the session.
// The registry outlives the snapshot: the listed tools call through it, and
// its owner closes it when the process ends.
func MCPSnapshot(reg *tools.Registry) map[string][]tools.Tool {
	listed, errs := reg.Snapshot(context.Background())
	for _, err := range errs {
		warnSettingsOnce(err.Error())
	}
	return listed
}

// RoleToolsWithMCP returns the tools a role serves in its main list with an
// explicit MCP listing, the way wick-mcp serves them. It is the main list of
// ResolvedToolsWithMCP: the MCP server serves exactly this.
func RoleToolsWithMCP(role string, mcp map[string][]tools.Tool) ([]tools.Tool, error) {
	main, _, err := ResolvedToolsWithMCP(role, mcp)
	return main, err
}
