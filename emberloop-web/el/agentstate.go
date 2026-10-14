package el

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// statePath follows PiG's own convention for extension-owned state.
var statePath = filepath.Join(os.Getenv("HOME"), ".pig", "state", "emberloop", "workers.json")

// AgentState is one agent's own small record, kept beside the state store
// and found by the agent id. The usage tailer reads it for the harness
// session id currently in use, which changes at each restart while the id
// stays stable.
type AgentState struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
	// Harness, Model and Effort name how the agent runs.
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	// Ticket is the ticket file a worktree agent was spawned from.
	Ticket string `json:"ticket,omitempty"`
	// Pane is where the agent runs.
	Pane string `json:"pane,omitempty"`
	// Tab is the tab the pane lives in.
	Tab string `json:"tab,omitempty"`
	// ParentID is the spawning agent's id, when there was one.
	ParentID string `json:"parent_id,omitempty"`
	// Parent is the spawning session's pane.
	Parent string `json:"parent_pane,omitempty"`
	// GroupID is the spawn group the agent belongs to.
	GroupID string `json:"group_id,omitempty"`
	// Deliver reports that this agent's submit goes to Parent as a notice
	// rather than back to a waiting caller.
	Deliver bool `json:"deliver,omitempty"`
	// Errored marks a child whose last turn ended in a provider error, with
	// the error text in ErrorDetail.
	Errored bool `json:"errored,omitempty"`
	// ErrorDetail is the provider error that ended the last turn.
	ErrorDetail string `json:"error_detail,omitempty"`
	// LostNotified marks a dead child whose lost notice already went out.
	LostNotified bool `json:"lost_notified,omitempty"`
	// SessionID is the harness session id currently in use: the id the
	// transcript file carries. It is set at launch and changes at each
	// restart, while ID stays the stable el agent id every other record
	// (usage rows, submits, gate state, pane mapping) is keyed by.
	SessionID string `json:"session_id,omitempty"`
	// LaunchedAt is when the agent's current child process started.
	LaunchedAt string `json:"launched_at,omitempty"`
	// Compressed counts how many handoffs this agent went through.
	Compressed int `json:"compressed,omitempty"`
	// Role is the session role the agent runs as.
	Role string `json:"role,omitempty"`
	// Cwd is the directory the session runs in.
	Cwd string `json:"cwd,omitempty"`
	// ExitCode is how the agent's child process ended: nil while the child
	// still runs or ended cleanly.
	ExitCode *int `json:"exit_code,omitempty"`
	// ExitSignal is the signal that killed the child, when one did.
	ExitSignal string `json:"exit_signal,omitempty"`
	// ExitStderr is the last lines of the child's stderr.
	ExitStderr string `json:"exit_stderr,omitempty"`
	// Compressing marks an agent whose session is being summarized and
	// restarted right now.
	Compressing bool `json:"compressing,omitempty"`
	// CompressingSince is when the mark was claimed, unix seconds.
	CompressingSince int64 `json:"compressing_since,omitempty"`
	// CompressFailed is the failure or refusal reason from the last
	// auto-compress attempt.
	CompressFailed string `json:"compress_failed,omitempty"`
	// Review, on a reviewer agent, is where its submit belongs.
	Review *ReviewTarget `json:"review,omitempty"`
}

// ReviewTarget is what a reviewer's submit is folded into: the worktree whose
// reviewers it updates, the reviewer type it is keyed by, and the commit the
// verdict is tied to.
type ReviewTarget struct {
	Tree string `json:"tree"`
	Type string `json:"type"`
	Head string `json:"head"`
	At   string `json:"at"`
	// TreeID is the git tree of the exact content the reviewer judged,
	// uncommitted and new files included.
	TreeID string `json:"tree_id,omitempty"`
	// PatchID is the hash of the worker's own diff from its base at the same
	// moment, so a clean rebase keeps the pass.
	PatchID string `json:"patch_id,omitempty"`
}

// agentStateDir holds one file per agent.
func agentStateDir() string { return filepath.Join(filepath.Dir(statePath), "agents") }

func agentStatePath(id string) string { return filepath.Join(agentStateDir(), id+".json") }

// ReadAgentState returns an agent's state, and false when there is none.
func ReadAgentState(id string) (AgentState, bool, error) {
	raw, err := os.ReadFile(agentStatePath(id))
	if os.IsNotExist(err) {
		return AgentState{}, false, nil
	}
	if err != nil {
		return AgentState{}, false, err
	}
	var s AgentState
	if err := json.Unmarshal(raw, &s); err != nil {
		return AgentState{}, false, err
	}
	return s, true, nil
}
