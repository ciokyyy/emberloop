package el

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"example.com/emberloop/tools"
)

// oversizeBashClause is the clause the read tool's oversized-file refusal
// carries: it tells the caller to pull the needed part with bash.
const oversizeBashClause = "pull the part you need with bash"

// oversizeNoBashClause is the same refusal for a caller with no bash: it
// names tools.grep in execute or read with offset and limit instead.
const oversizeNoBashClause = "search it with tools.grep in execute or pull the part you need with read with offset and limit"

// readHintTool wraps the served read tool so an oversized-file refusal names
// what the caller can actually use: bash for roles that serve it, tools.grep
// in execute or read with offset and limit for roles that do not. Name,
// description, schema and the read-only mark follow the inner tool, so the
// MCP listing and the execute stub agree with the unwrapped tool.
type readHintTool struct {
	inner   tools.Tool
	hasBash bool
}

func (t readHintTool) Name() string           { return t.inner.Name() }
func (t readHintTool) Description() string    { return t.inner.Description() }
func (t readHintTool) Schema() map[string]any { return t.inner.Schema() }
func (t readHintTool) ReadOnly() bool {
	if ro, ok := t.inner.(tools.ReadOnly); ok {
		return ro.ReadOnly()
	}
	return false
}
func (t readHintTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	out, err := t.inner.Execute(ctx, root, args)
	if err == nil || t.hasBash {
		return out, err
	}
	if msg := err.Error(); strings.Contains(msg, oversizeBashClause) {
		return "", errors.New(strings.Replace(msg, oversizeBashClause, oversizeNoBashClause, 1))
	}
	return "", err
}

// roleServesBash reports whether the role's resolved main list will carry
// bash: the base list must offer it and settings must leave it enabled and in
// the main list. It mirrors the resolution loop's enable and placement logic
// without warning; the loop itself still warns on an invalid placement.
func roleServesBash(role string, base []tools.Tool, s tools.Settings) bool {
	found := false
	for _, t := range base {
		if t.Name() == "bash" {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	enabled, placement, _ := true, tools.DefaultPlacement("bash"), ""
	if e, ok := s.Tools["bash"]; ok {
		enabled, placement, _ = effectiveToolSetting("bash", e, role)
	}
	if placement != tools.PlacementMain && placement != tools.PlacementExecute {
		placement = tools.PlacementMain
	}
	return enabled && placement == tools.PlacementMain
}

// ExecuteMulti forwards images from the inner read tool, with the same
// oversize-refusal rewrite as Execute.
func (t readHintTool) ExecuteMulti(ctx context.Context, root string, args json.RawMessage) (string, []tools.Image, error) {
	out, imgs, err := tools.ExecuteMulti(ctx, t.inner, root, args)
	if err == nil || t.hasBash {
		return out, imgs, err
	}
	if msg := err.Error(); strings.Contains(msg, oversizeBashClause) {
		return "", nil, errors.New(strings.Replace(msg, oversizeBashClause, oversizeNoBashClause, 1))
	}
	return "", nil, err
}
