package el

import (
	"context"
	"encoding/json"
	"strings"

	"example.com/emberloop/tools"
)

// overflowMarker starts every stored-output summary tools.MaybeOverflow returns.
const overflowMarker = "[overflow] "

// bashOverflowTip is the line a stored bash output gains for a role that also
// serves execute: paging a big output costs more than filtering it in a script.
const bashOverflowTip = "Tip: next time filter big output in execute (tools.bash, then print only what you need) instead of paging it."

// bashHintTool wraps the served bash tool for a role that also serves execute.
// Only a stored (oversized) result changes: it gains bashOverflowTip. Name,
// description, schema and the read-only mark follow the inner tool.
type bashHintTool struct {
	inner tools.Tool
}

func (t bashHintTool) Name() string           { return t.inner.Name() }
func (t bashHintTool) Description() string    { return t.inner.Description() }
func (t bashHintTool) Schema() map[string]any { return t.inner.Schema() }
func (t bashHintTool) ReadOnly() bool {
	if ro, ok := t.inner.(tools.ReadOnly); ok {
		return ro.ReadOnly()
	}
	return false
}
func (t bashHintTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	out, err := t.inner.Execute(ctx, root, args)
	if err == nil && strings.HasPrefix(out, overflowMarker) {
		out = strings.TrimRight(out, "\n") + "\n" + bashOverflowTip + "\n"
	}
	return out, err
}

// roleServesExecute reports whether the role's resolved main list will carry
// execute: the base list must offer it and settings must leave it enabled. It
// mirrors boundExecuteTool's enable logic without warning.
func roleServesExecute(role string, base []tools.Tool, s tools.Settings) bool {
	found := false
	for _, t := range base {
		if t.Name() == "execute" {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if e, ok := s.Tools["execute"]; ok {
		enabled, _, _ := effectiveToolSetting("execute", e, role)
		return enabled
	}
	return true
}
