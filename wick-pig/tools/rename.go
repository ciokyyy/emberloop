package tools

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
