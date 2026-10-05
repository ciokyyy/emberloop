package wick

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AgentState is a spawned agent's own small record, kept beside the submit
// store and found by the agent id. It is what lets an agent's submit find its
// way home: the parent pane recorded at spawn, whether this submit goes to the
// parent as a notice, and, on the parent's own record, the children that were
// auto-backgrounded and still owe a result.
type AgentState struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
	// ParentID is the spawning agent's id, when there was one: a notice that
	// arrives clears the child from that agent's Pending list.
	ParentID string `json:"parent_id,omitempty"`
	// Parent is the spawning session's herdr pane, where a notice is typed.
	Parent string `json:"parent_pane,omitempty"`
	// Deliver reports that this agent's submit goes to Parent as a notice
	// rather than back to a waiting spawn. Non-blocking agents start this way;
	// a blocking agent is switched over when its spawn auto-backgrounds it.
	Deliver bool `json:"deliver,omitempty"`
	// Pending lists children this agent auto-backgrounded whose results have
	// not arrived yet. The stop gate reads it to allow a parent to wait.
	Pending []string `json:"pending,omitempty"`
}

// agentStateDir holds one file per spawned agent.
func agentStateDir() string { return filepath.Join(filepath.Dir(statePath), "agents") }

func agentStatePath(id string) string { return filepath.Join(agentStateDir(), id+".json") }

// WriteAgentState records an agent's state, temp file and rename, so a reader
// never sees a half-written file.
func WriteAgentState(s AgentState) error {
	if err := os.MkdirAll(agentStateDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := agentStatePath(s.ID) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, agentStatePath(s.ID))
}

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

// MarkDeliver switches an agent's submits to the notice path. A blocking agent
// whose spawn ran out of budget is switched this way, so its eventual submit
// reaches the parent without anyone polling.
func MarkDeliver(id string) error {
	s, ok, err := ReadAgentState(id)
	if err != nil {
		return err
	}
	if !ok {
		s = AgentState{ID: id}
	}
	s.Deliver = true
	return WriteAgentState(s)
}

// AddPending records children on their parent's state as still owing a result.
// A parent with no state of its own (a top-level session) has no stop gate to
// satisfy, so there is nothing to record and no error.
func AddPending(parentID string, children []string) error {
	if parentID == "" || len(children) == 0 {
		return nil
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	s, _, err := ReadAgentState(parentID)
	if err != nil {
		return err
	}
	s.ID = parentID
	for _, c := range children {
		if !contains(s.Pending, c) {
			s.Pending = append(s.Pending, c)
		}
	}
	return WriteAgentState(s)
}

// RemovePending clears one child from its parent's pending list, once its
// notice has been delivered.
func RemovePending(parentID, childID string) error {
	if parentID == "" {
		return nil
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	s, ok, err := ReadAgentState(parentID)
	if err != nil || !ok {
		return err
	}
	kept := s.Pending[:0]
	for _, c := range s.Pending {
		if c != childID {
			kept = append(kept, c)
		}
	}
	s.Pending = kept
	return WriteAgentState(s)
}

// PendingChildren is the children an agent has auto-backgrounded and is still
// waiting on.
func PendingChildren(parentID string) []string {
	s, ok, err := ReadAgentState(parentID)
	if err != nil || !ok {
		return nil
	}
	return s.Pending
}
