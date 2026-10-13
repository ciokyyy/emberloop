package web

import (
	"strings"
	"time"
)

// Event is the normalized record shared by Backends, the Event log and the UI.
// Kinds: prompt, injection, assistant_chunk, thinking_chunk,
// tool_call_started, tool_call_updated, diff, permission, status,
// subagent_spawned, subagent_finished, sudo_request, sudo_resolved, error.
// An injection Event records one memory injection (Profile version and Rule
// IDs with versions) with sender=system; the miner never reads it.
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
	// Handoff flags the prompt Event that carries a compress Handoff; the UI
	// renders it as a collapsed block, not a user message.
	Handoff bool `json:"handoff,omitempty"`
	// Sender types a prompt Event: "user" for a message a signed-in User
	// sent, "system" for one the Daemon or an agent made. An injection
	// Event is always "system". Empty on other kinds.
	Sender string `json:"sender,omitempty"`
	// User is the ID of the User who sent a "user" prompt; empty otherwise.
	User string `json:"user,omitempty"`
	// Images counts the attachments of a prompt; the bytes stay out of the log.
	Images int `json:"images,omitempty"`
	// ImageIDs names the Session-stored images attached to a prompt: the
	// transcript keeps the stable ids, the bytes live under the Session's
	// images directory, and the agent receives their file paths at send
	// time. Inline attachments stay counted in Images; the two ride together.
	ImageIDs  []string  `json:"imageIds,omitempty"`
	CreatedAt time.Time `json:"at"`
}

// Sender values of a prompt Event. A User's own message is "user", with the
// User ID in User; every message the Daemon or an agent makes is "system".
const (
	senderUser   = "user"
	senderSystem = "system"
)

// promptSender types one prompt Event. The zero value is the system sender.
type promptSender struct {
	kind string // senderUser or senderSystem
	user string // the User ID, set only for senderUser
}

// systemSender types a prompt the Daemon or an agent made.
var systemSender = promptSender{kind: senderSystem}

// userSender types a prompt the signed-in User u sent.
func userSender(u User) promptSender {
	return promptSender{kind: senderUser, user: u.ID}
}

// stamp writes the sender onto a prompt Event.
func (p promptSender) stamp(e *Event) {
	if p.kind != senderUser {
		e.Sender, e.User = senderSystem, ""
		return
	}
	e.Sender, e.User = senderUser, p.user
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
	ProjectID string `json:"projectId,omitempty"`
	Status    string `json:"status"`
	// Capped marks a failed turn classified as a provider session limit. It
	// is exposed to the Session list while Status remains failed internally.
	Capped    bool      `json:"capped,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ParentID  string    `json:"parentId,omitempty"`
	GroupID   string    `json:"groupId,omitempty"`
	Scout     bool      `json:"scout,omitempty"`
	// RunWorker marks a Run ticket Worker: it runs the run-worker el role,
	// which always serves submit.
	RunWorker bool `json:"runWorker,omitempty"`
	// NoRecall marks a Run A/B Worker that runs with Rule recall off (ticket
	// 30): create and compress inject the Profile but no recalled Rules, so
	// reviewer verdicts give a paired comparison against recall-on Workers.
	// Children the Worker spawns recall normally.
	NoRecall bool `json:"noRecall,omitempty"`
	// Reviewer marks a Run Reviewer Session with its type
	// (reviewer-spec or reviewer-standards). Reviewers get the filtered
	// Profile: workflow and style lines only, never barred lines.
	Reviewer string `json:"reviewer,omitempty"`
	// Title is the short generated name the session list shows; empty
	// keeps the harness and id label.
	Title string `json:"title,omitempty"`
	// ResumeToken is the harness handle needed to resume: the claude ACP
	// session id, the pig Session id, or an opaque marker for fake.
	ResumeToken string `json:"resumeToken,omitempty"`
	// Owner is the ID of the User who created the Session. Subagents, Workers
	// and Reviewers inherit it. A Session with no Owner is hidden everywhere.
	Owner string `json:"owner,omitempty"`
	// CompressBlock is the error of a compress that stopped on a credit or auth
	// failure. Non-empty means the Session is blocked: its messages queue until
	// Retry succeeds or the block is dismissed.
	CompressBlock string `json:"compressBlock,omitempty"`
	// RestartStopped marks a Run Worker or Reviewer the Daemon stopped at a
	// restart, right after its current tool call finished. The next Daemon
	// continues it in the same Session and worktree and clears the mark.
	RestartStopped bool `json:"restartStopped,omitempty"`
	// Carry holds the messages that sat in the Daemon queue when the restart
	// stopped the Session, one "- " bullet each. They reach the Session in its
	// continue prompt.
	Carry string `json:"carry,omitempty"`
	// Dirs are the owner's harness config dirs for the current start. The
	// Daemon sets them from Owner on every create, resume and Handoff, and
	// they are never persisted.
	Dirs ownerDirs `json:"-"`
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
