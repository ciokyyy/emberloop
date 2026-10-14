package web

import (
	"context"
	"encoding/json"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"

	"example.com/emberloop-web/el"
	"example.com/emberloop-web/el/tools"
)

// elTool adapts one shared tools.Tool, the same value el-mcp
// serves to claude, into a pig AgentTool bound to one Session's workspace.
type elTool struct {
	tool tools.Tool
	root string
	// gate, when set, takes over bash commands that invoke sudo (see
	// tools.SudoGate); the Session's pending approval replaces the run.
	gate func(ctx context.Context, root, command string) (string, bool)
}

func (t elTool) Name() string  { return t.tool.Name() }
func (t elTool) Label() string { return t.tool.Name() }
func (t elTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{Name: t.tool.Name(), Description: t.tool.Description(), Parameters: t.tool.Schema()}
}

func (t elTool) Execute(ctx context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	if t.gate != nil && t.tool.Name() == "bash" {
		var p struct{ Command string }
		if json.Unmarshal(params, &p) == nil && tools.UsesSudo(p.Command) {
			if msg, taken := t.gate(ctx, t.root, p.Command); taken {
				return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: msg}}}, nil
			}
		}
	}
	out, images, err := tools.ExecuteMulti(ctx, t.tool, t.root, params)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	content := []ai.ToolResultMessageContent{ai.TextContent{Text: out}}
	for _, img := range images {
		content = append(content, ai.ImageContent{Data: img.Data, MimeType: img.MimeType})
	}
	return agent.AgentToolResult{Content: content}, nil
}

func (t elTool) ExecutionMode() agent.ToolExecutionMode {
	if ro, ok := t.tool.(tools.ReadOnly); ok && ro.ReadOnly() {
		return agent.ToolModeParallel
	}
	return agent.ToolModeSequential
}

// hostOnlyTools are el orchestration tools the Daemon replaces with
// its own spawn: serving them would start herdr-era panes, not Sessions.
var hostOnlyTools = map[string]bool{"spawn": true, "finish": true, "tell": true, "submit": true}

// elToolsFor returns the shared tools of one Session's el role (elRoleFor):
// file tools for web, web-worker and run-worker, read-only tools for scouts.
func elToolsFor(meta SessionMeta, gate func(ctx context.Context, root, command string) (string, bool)) ([]agent.AgentTool, error) {
	role := elRoleFor(meta)
	served, err := el.RoleTools(role)
	if err != nil {
		return nil, err
	}
	var out []agent.AgentTool
	for _, t := range served {
		if hostOnlyTools[t.Name()] {
			continue
		}
		out = append(out, elTool{tool: t, root: meta.WorkDir, gate: gate})
	}
	return out, nil
}
