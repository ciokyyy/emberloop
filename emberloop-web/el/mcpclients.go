package el

import (
	"context"
	"encoding/json"

	"example.com/emberloop-web/el/tools"
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

// MCPSnapshotWithGraph snapshots reg and gates the listed graph tools behind
// the index-status wait for root's project, so the first graph query of the
// session waits on the worktree index instead of answering from a missing
// graph. Without a graph server the snapshot is unchanged.
func MCPSnapshotWithGraph(reg *tools.Registry, root string) map[string][]tools.Tool {
	listed := MCPSnapshot(reg)
	call := func(ctx context.Context, tool string, args json.RawMessage) (string, error) {
		return reg.Call(ctx, tools.GraphServerName, tool, args)
	}
	return GateListedGraph(listed, call, root)
}

// GateListedGraph gates a snapshot's graph tools behind the index-status wait
// for root's project. Split from MCPSnapshotWithGraph so tests drive it with
// a recording call.
func GateListedGraph(listed map[string][]tools.Tool, call tools.GraphStatusCall, root string) map[string][]tools.Tool {
	graphed, ok := listed[tools.GraphServerName]
	if !ok || len(graphed) == 0 {
		return listed
	}
	listed[tools.GraphServerName] = tools.GateGraphTools(graphed, call, root, tools.DefaultGraphGateWait)
	return listed
}

// RoleToolsWithMCP returns the tools a role serves in its main list with an
// explicit MCP listing, the way el-mcp serves them. It is the main list of
// ResolvedToolsWithMCP: the MCP server serves exactly this.
func RoleToolsWithMCP(role string, mcp map[string][]tools.Tool) ([]tools.Tool, error) {
	main, _, err := ResolvedToolsWithMCP(role, mcp)
	return main, err
}
