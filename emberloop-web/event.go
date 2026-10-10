package web

import (
	"strings"
	"time"
)

// Event is the normalized record shared by Backends, the Event log and the UI.
// Kinds: prompt, assistant_chunk, thinking_chunk, tool_call_started,
// tool_call_updated, diff, permission, status, subagent_spawned,
// subagent_finished, sudo_request, sudo_resolved, error.
// Mode marks which delivery a prompt Event used: "" (a plain first prompt),
// "steer" (next tool boundary) or "followup" (after the turn). A claude
// Steer degrades to a followup and is stored so.
type Event struct {
	Version   int    `json:"v"`
	Seq       int64  `json:"seq"`
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	Status    string `json:"status,omitempty"`
	Mode      string `json:"mode,omitempty"`
	// Images counts the attachments of a prompt; the bytes stay out of the log.
	Images int `json:"images,omitempty"`
	// ImageIDs names the Session-stored images attached to a prompt: the
	// transcript keeps the stable ids, the bytes live under the Session's
	// images directory, and the agent receives their file paths at send
	// time. Inline attachments stay counted in Images; the two ride together.
	ImageIDs  []string  `json:"imageIds,omitempty"`
	CreatedAt time.Time `json:"at"`
}

// QueuedMessage is one not-yet-delivered Steer or Follow-up (or a plain
// prompt that arrived while the Session worked).
type QueuedMessage struct {
	Mode     string   `json:"mode"`
	Text     string   `json:"text"`
	ImageIDs []string `json:"imageIds,omitempty"`
	Degraded bool     `json:"degraded,omitempty"`
}

// SessionMeta is the small metadata file per Session. ParentID names the
// spawning Session for Subagents; GroupID names the spawn call that created
// them (one Group per call). Scout marks a read-only Subagent: user sends
// and nested spawns are refused, and its tool set carries no write tools.
type SessionMeta struct {
	ID string `json:"id"`
	// Name is the Subagent display name: the spawn `name` field, or the first
	// four words of its prompt when none was given. The sidebar and the Group
	// card show it; the raw id stays a tooltip. Empty on root Sessions.
	Name    string `json:"name,omitempty"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
	// Effort is the reasoning effort chosen at creation (claude: low,
	// medium, high, xhigh, max; pig: off, minimal, low, medium, high,
	// xhigh, max); empty keeps the model default.
	Effort  string `json:"effort,omitempty"`
	WorkDir string `json:"workDir"`
	// ProjectID names the recorded Project the Session belongs to, also for
	// Subagents and Workers whose WorkDir is a worktree.
	ProjectID string    `json:"projectId,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	ParentID  string    `json:"parentId,omitempty"`
	GroupID   string    `json:"groupId,omitempty"`
	Scout     bool      `json:"scout,omitempty"`
	// Title is the short generated name the session list shows; empty
	// keeps the harness and id label.
	Title string `json:"title,omitempty"`
	// ResumeToken is the harness handle needed to resume: the claude ACP
	// session id, the pig Session id, or an opaque marker for fake.
	ResumeToken string `json:"resumeToken,omitempty"`
}

// ToolCall is the one tool-call shape every Backend normalizes to. It rides
// in the Text of tool_call_started and tool_call_updated Events as compact
// JSON, so the UI parses a single format whatever harness produced it.
// Server is the MCP server that owns the tool, empty for native tools.
type ToolCall struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Server string `json:"server,omitempty"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Status string `json:"status"`
}

// Text renders the call as the Event text.
func (t ToolCall) Text() string {
	t.Server, t.Name = splitMCPToolName(t.Server, t.Name)
	return compactJSON(t)
}

// splitMCPToolName turns "mcp__server__tool" into its server and tool name.
// Any other name stays whole and keeps the given server.
func splitMCPToolName(server, name string) (string, string) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return server, name
	}
	srv, tool, ok := strings.Cut(rest, "__")
	if !ok || srv == "" || tool == "" {
		return server, name
	}
	return srv, tool
}
