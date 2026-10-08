package web

import "time"

// Event is the normalized record shared by Backends, the Event log and the UI.
// Kinds: prompt, assistant_chunk, thinking_chunk, tool_call_started,
// tool_call_updated, diff, permission, status, subagent_spawned,
// subagent_finished, error.
// Mode marks which delivery a prompt Event used: "" (a plain first prompt),
// "steer" (next tool boundary) or "followup" (after the turn). A claude
// Steer degrades to a followup and is stored so.
type Event struct {
	Version   int       `json:"v"`
	Seq       int64     `json:"seq"`
	SessionID string    `json:"sessionId"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text,omitempty"`
	Status    string    `json:"status,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	CreatedAt time.Time `json:"at"`
}

// QueuedMessage is one not-yet-delivered Steer or Follow-up (or a plain
// prompt that arrived while the Session worked).
type QueuedMessage struct {
	Mode     string `json:"mode"`
	Text     string `json:"text"`
	Degraded bool   `json:"degraded,omitempty"`
}

// SessionMeta is the small metadata file per Session. ParentID names the
// spawning Session for Subagents; GroupID names the spawn call that created
// them (one Group per call). Scout marks a read-only Subagent: user sends
// and nested spawns are refused, and its tool set carries no write tools.
type SessionMeta struct {
	ID        string    `json:"id"`
	Harness   string    `json:"harness"`
	Model     string    `json:"model"`
	WorkDir   string    `json:"workDir"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	ParentID  string    `json:"parentId,omitempty"`
	GroupID   string    `json:"groupId,omitempty"`
	Scout     bool      `json:"scout,omitempty"`
	// ResumeToken is the harness handle needed to resume: the claude ACP
	// session id, the pig Session id, or an opaque marker for fake.
	ResumeToken string `json:"resumeToken,omitempty"`
}
