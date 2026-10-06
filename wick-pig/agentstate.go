package wick

import (
	"encoding/json"
	"fmt"
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
	// Harness, Model and Effort name how the agent runs. Set for scouts at
	// spawn, so readers without their own store (the TUI) can list them;
	// workers and reviewers keep theirs in their own stores.
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	// Ticket is the ticket file a worktree agent was spawned from, recorded
	// beside the worker entry that finish merges.
	Ticket string `json:"ticket,omitempty"`
	// Pane is where the agent runs, so a tell can reach it.
	Pane string `json:"pane,omitempty"`
	// Tab is the herdr tab the pane lives in, so a close can take its grid
	// lock. Old records carry none; a close without it still closes.
	Tab string `json:"tab,omitempty"`
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
	// Errored marks a child whose last turn ended in a provider error, with
	// the error text in ErrorDetail. An errored child owes no result until
	// its next turn starts (fresh clears it), so the stop gate does not wait
	// on it. Set by the child's own stop gate when it allows on a failure.
	Errored bool `json:"errored,omitempty"`
	// ErrorDetail is the provider error that ended the last turn, named in
	// the stopped-with-error notice the child's gate sends its parent.
	ErrorDetail string `json:"error_detail,omitempty"`
	// LostNotified marks a dead child whose lost notice already went out, so
	// the dead-child sweep reports it exactly once.
	LostNotified bool `json:"lost_notified,omitempty"`
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
// it as their parent, have been set to deliver as notices, have not
// submitted, and are still alive. A child with no submit but a dead pane, or
// whose last turn ended in a provider error, is lost or errored, not pending:
// the sweep reports it and the stop gate does not wait on it.
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
		if s.Errored {
			continue
		}
		if isPaneLost(deliverExec, s.Pane) {
			continue
		}
		out = append(out, s.ID)
	}
	return out
}

// resolvePaneToAgent turns a herdr pane id into the agent that runs in it,
// through the agent record that stores its pane. An unknown pane, a pane
// shared by several records, or one whose agent is no longer live is a
// clear error that acts on nothing.
func resolvePaneToAgent(pane string, run Exec) (string, error) {
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return "", fmt.Errorf("unknown pane %q: no agent runs in it", pane)
	}
	var matches []AgentState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		s, ok, err := ReadAgentState(id)
		if err != nil || !ok || s.Pane != pane {
			continue
		}
		s.ID = id
		matches = append(matches, s)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("unknown pane %q: no agent runs in it", pane)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return "", fmt.Errorf("pane %q matches multiple agents (%s): tell or finish one agent id instead", pane, strings.Join(ids, ", "))
	}
	if isPaneLost(run, matches[0].Pane) {
		return "", fmt.Errorf("pane %q matches no live agent (agent %q is gone)", pane, matches[0].ID)
	}
	return matches[0].ID, nil
}

// resolveAgentParam takes the agent/pane pair of tell and finish: exactly
// one is required, and a pane resolves to its agent. It returns the agent
// id to act on.
func resolveAgentParam(params map[string]any, run Exec) (string, error) {
	agent := str(params, "agent")
	pane := str(params, "pane")
	switch {
	case agent == "" && pane == "":
		return "", fmt.Errorf("agent or pane is required (exactly one)")
	case agent != "" && pane != "":
		return "", fmt.Errorf("agent and pane are exclusive: give exactly one")
	case pane != "":
		return resolvePaneToAgent(pane, run)
	default:
		return agent, nil
	}
}

// isScoutState reports whether an agent's record is a scout's: its type's
// checkout is none (read-only, runs in the repo). Unknown types fall back
// to the name, so a scout outlives its definition.
func isScoutState(st AgentState) bool {
	if t, ok := mustLoadAgentTypes(cwd())[st.Type]; ok {
		return t.Checkout == "none"
	}
	return st.Type == "scout"
}

// sweepDeadChildren reports every dead child of one parent that has no submit
// and no live pane, exactly once. The parent is named by pane or id: a
// top-level orchestrator has no agent id, so its children match by its pane.
// Members of any group are skipped: the group reports them in its single
// notice, exactly once through its delivered flag, whether the group is still
// open or already delivered. Any other dead child gets the lost notice as an
// individual system notice — "pane closed manually" for a worktree agent —
// then stops counting as pending through the pane check above. It runs
// synchronously, so the stop gate blocks on what is left after it.
func sweepDeadChildren(parentPane, parentID string) {
	if parentPane == "" && parentID == "" {
		return
	}
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		_ = withFileLock("state-"+id, func() error {
			s, ok, err := ReadAgentState(id)
			if err != nil || !ok || !s.Deliver || s.LostNotified {
				return nil
			}
			mine := parentID != "" && s.ParentID == parentID
			mine = mine || (parentPane != "" && s.Parent == parentPane)
			if !mine {
				return nil
			}
			if _, submitted, _ := ReadAgentSubmit(id); submitted {
				return nil
			}
			if s.Errored {
				return nil
			}
			if !isPaneLost(deliverExec, s.Pane) {
				return nil
			}
			if s.GroupID != "" {
				return nil
			}
			s.LostNotified = true
			if err := WriteAgentState(s); err != nil {
				return nil
			}
			_ = deliverToParent(s, lostNoticeFor(s, id))
			return nil
		})
	}
}

// lostNotice is the individual lost notice for a dead child outside any
// group: the existing lost line inside a system notice, the shape of every
// other individual notice.
func lostNotice(typ, id string) string {
	return "<system-notice> " + lostLine(id, typ) + "</system-notice>"
}

// lostNoticeFor is the individual lost notice for one dead child: a worktree
// agent carries the ticket it was spawned from, and its pane went away
// because the user closed it by hand, so its line says so; every other
// child keeps the existing lost line.
func lostNoticeFor(s AgentState, id string) string {
	if s.Ticket != "" {
		return "<system-notice> " + workerLostLine(id, s.Type) + "</system-notice>"
	}
	return lostNotice(s.Type, id)
}

// errorNotice is the stopped-with-error notice a turn that ends in a provider
// error sends its parent, naming the agent and the error.
func errorNotice(typ, id, detail string) string {
	if typ == "" {
		typ = "agent"
	}
	detail = strings.Join(strings.Fields(detail), " ")
	if detail == "" {
		detail = "unknown error"
	}
	return "<system-notice> " + typ + " " + id + " stopped with error: " + detail + "</system-notice>"
}

// setChildErrored marks id as errored with detail, and reports whether this
// call is the one that owns the notice: only the first set per detail sends.
func setChildErrored(id, detail string) bool {
	owned := false
	_ = withFileLock("state-"+id, func() error {
		s, ok, err := ReadAgentState(id)
		if err != nil {
			return err
		}
		if !ok {
			s = AgentState{ID: id}
		}
		if s.Errored && s.ErrorDetail == detail {
			return nil
		}
		s.ID = id
		s.Errored, s.ErrorDetail = true, detail
		if err := WriteAgentState(s); err != nil {
			return err
		}
		owned = true
		return nil
	})
	return owned
}

// clearChildError clears the errored mark, so a new turn starts pending
// afresh. A real submit still excludes through its file; this only drops the
// transient failed-turn mark.
func clearChildError(id string) {
	_ = withFileLock("state-"+id, func() error {
		s, ok, err := ReadAgentState(id)
		if err != nil || !ok || !s.Errored {
			return err
		}
		s.Errored, s.ErrorDetail = false, ""
		return WriteAgentState(s)
	})
}
