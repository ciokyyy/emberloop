package tools

import (
	"context"
	"encoding/json"
)

// renamed serves an inner tool under a new name. Description and schema stay
// the original's, so agents see the same capability under the name settings
// chose. The read-only mark follows the inner tool, so the MCP server keeps
// advertising batchable tools correctly.
type renamed struct {
	Tool
	name string
}

// Renamed serves t under name.
func Renamed(t Tool, name string) Tool { return renamed{Tool: t, name: name} }

func (r renamed) Name() string { return r.name }

func (r renamed) ReadOnly() bool {
	if ro, ok := r.Tool.(ReadOnly); ok {
		return ro.ReadOnly()
	}
	return false
}

// DaemonProxy follows the inner tool, so a renamed Daemon proxy stays out of
// execute scripts like the original.
func (r renamed) DaemonProxy() bool {
	if p, ok := r.Tool.(DaemonProxy); ok {
		return p.DaemonProxy()
	}
	return false
}

// OriginalName is the inner tool's server-side name, or "" for a native
// tool: the execute stub and the role resolution read the original through
// a rename instead of losing the MCP marker.
func (r renamed) OriginalName() string {
	if o, ok := r.Tool.(interface{ OriginalName() string }); ok {
		return o.OriginalName()
	}
	return ""
}

// MCPServer is the inner tool's configured MCP server, or "" for a native
// tool: a renamed MCP tool keeps its tools.<server>.<name> namespace.
func (r renamed) MCPServer() string {
	if s, ok := r.Tool.(interface{ MCPServer() string }); ok {
		return s.MCPServer()
	}
	return ""
}

// ExecuteMulti forwards images from the inner tool.
func (r renamed) ExecuteMulti(ctx context.Context, root string, args json.RawMessage) (string, []Image, error) {
	return ExecuteMulti(ctx, r.Tool, root, args)
}
