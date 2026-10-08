package web

import "time"

// Event is the normalized record shared by Backends, the Event log and the UI.
// Kinds: prompt, assistant_chunk, thinking_chunk, tool_call_started,
// tool_call_updated, diff, permission, status, subagent_spawned,
// subagent_finished, error.
type Event struct {
	Version   int       `json:"v"`
	Seq       int64     `json:"seq"`
	SessionID string    `json:"sessionId"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"at"`
}

// SessionMeta is the small metadata file per Session.
type SessionMeta struct {
	ID        string    `json:"id"`
	Harness   string    `json:"harness"`
	Model     string    `json:"model"`
	WorkDir   string    `json:"workDir"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	// ResumeToken is the harness handle needed to resume: the claude ACP
	// session id, the pig Session id, or an opaque marker for fake.
	ResumeToken string `json:"resumeToken,omitempty"`
}
