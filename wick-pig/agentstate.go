package wick

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AgentState is a spawned agent's own small record, kept beside the submit
// store and found by the agent id. It is what lets an agent's submit find its
// way home: the parent pane recorded at spawn, the spawn group it belongs
// to, and whether this submit goes to the parent as a notice.
type AgentState struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
	// Ticket is the ticket file a worktree agent was spawned from, recorded
	// beside the worker entry that finish merges.
	Ticket string `json:"ticket,omitempty"`
	// Pane is where the agent runs, so a tell can reach it.
	Pane string `json:"pane,omitempty"`
	// ParentID is the spawning agent's id, when there was one. The children an
	// agent is still waiting on are the agent states that name it as ParentID,
	// are set to Deliver, and have not submitted: the stop gate reads them.
	ParentID string `json:"parent_id,omitempty"`
	// Parent is the spawning session's herdr pane, where a notice is typed.
	Parent string `json:"parent_pane,omitempty"`
	// GroupID is the spawn group the agent belongs to: the results of one
	// spawn call. A member's submit never delivers alone; when the last
	// member of the group has submitted, one notice carrying every result
	// goes to the parent pane.
	GroupID string `json:"group_id,omitempty"`
	// Deliver reports that this agent's submit goes to Parent as a notice
	// rather than back to a waiting caller. Every spawn member starts this
	// way, grouped; a tell takes over its next submit by clearing it.
	Deliver bool `json:"deliver,omitempty"`
	// Review, on a reviewer agent, is where its submit belongs: the worktree,
	// the reviewer type, and the commit the review is tied to. A non-reviewer
	// has none, and its submit is just recorded.
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

// claimNextSubmit takes a blocking child's next submit for a waiting tell: it
// points the child's result at the caller rather than the notice path, takes
// the child out of its spawn group so the group completes without it, and
// clears its last submit's this-turn and give-up flags so only a fresh turn's
// submit answers the tell. All writes happen under the child's own lock, so
// a submit arriving at the same instant is either claimed here or delivered
// as a notice, never both. It returns an undo, so a tell whose delivery
// then fails puts the child back rather than leaving its result owned by
// nobody.
func claimNextSubmit(id string) (undo func() error, err error) {
	var (
		prevState  AgentState
		hadState   bool
		prevSubmit AgentSubmit
		hadSubmit  bool
	)
	err = withFileLock("state-"+id, func() error {
		prevState, hadState, err = ReadAgentState(id)
		if err != nil || !hadState {
			return err
		}
		s := prevState
		s.Deliver = false
		s.GroupID = ""
		if err := WriteAgentState(s); err != nil {
			return err
		}
		prevSubmit, hadSubmit, err = ReadAgentSubmit(id)
		if err != nil || !hadSubmit || !(prevSubmit.ThisTurn || prevSubmit.Stopped) {
			return err
		}
		sub := prevSubmit
		sub.ThisTurn, sub.Stopped = false, false
		return WriteAgentSubmit(sub)
	})
	if err != nil {
		return nil, err
	}
	return func() error {
		return withFileLock("state-"+id, func() error {
			if hadState {
				if err := WriteAgentState(prevState); err != nil {
					return err
				}
			}
			if hadSubmit {
				return WriteAgentSubmit(prevSubmit)
			}
			return nil
		})
	}, nil
}

// backgroundToldAgent switches a blocking agent that a tell waited on to the
// notice path, unless its answer arrived at the deadline. It is the tell-side
// counterpart to backgroundAgent: a told agent already has a submit on disk, so
// it is the submit's flags, not the file's existence, that say whether there is
// a new result to own.
func backgroundToldAgent(id string) (bool, error) {
	backgrounded := false
	err := withFileLock("state-"+id, func() error {
		if s, ok, err := ReadAgentSubmit(id); err != nil || (ok && answersTell(s)) {
			return err
		}
		s, _, err := ReadAgentState(id)
		if err != nil {
			return err
		}
		s.ID = id
		s.Deliver = true
		backgrounded = true
		return WriteAgentState(s)
	})
	return backgrounded, err
}

// submitDelivers reports whether an agent's submit is to be delivered as a
// notice, decided under the child's lock against backgroundAgent. It returns
// the state the notice is sent to.
func submitDelivers(id string) (AgentState, bool, error) {
	var st AgentState
	deliver := false
	err := withFileLock("state-"+id, func() error {
		s, ok, err := ReadAgentState(id)
		if err != nil || !ok {
			return err
		}
		st, deliver = s, s.Deliver
		return nil
	})
	return st, deliver, err
}

// PendingChildren is the children an agent is still waiting on: those that name
// it as their parent, have been set to deliver as notices, and have not
// submitted yet. A submit clears the child from the list by existing, so no
// cross-process bookkeeping can leave a delivered child behind.
func PendingChildren(parentID string) []string {
	if parentID == "" {
		return nil
	}
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, ok, err := ReadAgentState(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || !ok || s.ParentID != parentID || !s.Deliver {
			continue
		}
		if _, submitted, _ := ReadAgentSubmit(s.ID); submitted {
			continue
		}
		out = append(out, s.ID)
	}
	return out
}
