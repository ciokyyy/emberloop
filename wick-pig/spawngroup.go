package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// spawnGroup is the results of one spawn call: every agent it started, in
// order. Each member's submit is recorded as it comes; when the last member
// has submitted (or is lost), one notice carrying every result goes to the
// parent pane. A reviewer already judged at the exact state is reported in
// the spawn reply itself and is never a member, so a group always completes
// through submits.
type spawnGroup struct {
	ID string `json:"id"`
	// Parent is the spawning session's herdr pane, where the grouped notice
	// is typed. ParentID names the spawning agent, for debugging.
	Parent   string   `json:"parent_pane,omitempty"`
	ParentID string   `json:"parent_id,omitempty"`
	Members  []string `json:"members"`
	// Delivered marks a group whose notice already went out, so two members
	// submitting at the same moment deliver exactly once.
	Delivered bool   `json:"delivered,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// groupIdleProbeTimeout is how long one parent-idle probe may take: herdr
// agent wait --until idle --timeout <ms>. A var so tests can shrink it.
var groupIdleProbeTimeout = 2 * time.Second

// groupNoticeHoldTimeout is how long a grouped notice waits for its parent
// pane to go idle before it is delivered anyway, with a warning. Typed text
// cancels a running tool call, so a notice is held while the parent works
// one and delivered when it goes idle.
var groupNoticeHoldTimeout = 5 * time.Minute

func groupDir() string { return filepath.Join(filepath.Dir(statePath), "groups") }

func groupPath(id string) string { return filepath.Join(groupDir(), id+".json") }

// writeSpawnGroup records a group atomically, so a concurrent submit never
// reads a half-written file.
func writeSpawnGroup(g spawnGroup) error {
	if err := os.MkdirAll(groupDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	tmp := groupPath(g.ID) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, groupPath(g.ID))
}

// readSpawnGroup returns a group, and false when there is none.
func readSpawnGroup(id string) (spawnGroup, bool, error) {
	raw, err := os.ReadFile(groupPath(id))
	if os.IsNotExist(err) {
		return spawnGroup{}, false, nil
	}
	if err != nil {
		return spawnGroup{}, false, err
	}
	var g spawnGroup
	if err := json.Unmarshal(raw, &g); err != nil {
		return spawnGroup{}, false, fmt.Errorf("spawn group %s is corrupt: %w", id, err)
	}
	return g, true, nil
}

// groupNoticeText is the one grouped notice for a complete group: every
// member's result, in spawn order, inside a single system notice. A member
// that submitted contributes its report (renderSubmit: a full report for a
// blocking type, a one-line summary for a non-blocking one); a member whose
// pane vanished contributes a lost line, so the group always completes.
func groupNoticeText(g spawnGroup, submits map[string]AgentSubmit, lost map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<system-notice> spawn complete (%d of %d agents). Resume your work using the results below.\n", len(submits)+len(lost), len(g.Members))
	for i, id := range g.Members {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		if s, ok := submits[id]; ok {
			b.WriteString(renderSubmit(s))
		} else if line, ok := lost[id]; ok {
			b.WriteString(line)
		}
	}
	b.WriteString("\n</system-notice>")
	return b.String()
}

// lostLine reports a member that cannot submit: its pane is gone.
func lostLine(id, typ string) string {
	if typ == "" {
		typ = "agent"
	}
	return fmt.Sprintf("%s %s lost: its pane is gone before it could submit.", typ, id)
}

// isPaneLost reports whether pane is gone from herdr: a pane herdr no longer
// knows. Only the explicit pane_not_found counts, the way deliverNotice
// drops a notice for a session that is gone but still types into one that
// is only booting.
func isPaneLost(run Exec, pane string) bool {
	if pane == "" {
		return false
	}
	res, err := run(herdrBin(), []string{"pane", "get", pane}, ExecOpts{Timeout: 15 * time.Second})
	if err != nil {
		return false
	}
	if res.Code == 0 {
		return false
	}
	return paneNotFound(res)
}

// parentIdle reports whether the parent pane is idle: herdr's agent wait for
// it returns at once. A pane herdr no longer knows counts as idle, so the
// delivery that follows drops the notice the way any notice to a gone
// session drops. Any other failure (a timeout: the agent is still working)
// counts as busy.
func parentIdle(run Exec, parent string) bool {
	if parent == "" {
		return true
	}
	ms := strconv.Itoa(int(groupIdleProbeTimeout / time.Millisecond))
	res, err := run(herdrBin(), []string{"agent", "wait", parent, "--until", "idle", "--timeout", ms}, ExecOpts{Timeout: groupIdleProbeTimeout + 5*time.Second})
	if err != nil {
		return false
	}
	if res.Code == 0 {
		return true
	}
	if paneNotFound(res) {
		return true
	}
	return false
}

// waitForParentIdle holds a grouped notice while its parent pane works a tool
// call, because typed text cancels a running call. It returns true once the
// parent is idle, or false when the parent never goes idle within the hold
// bound: the caller then delivers anyway, with a warning.
func waitForParentIdle(run Exec, parent string) bool {
	if parentIdle(run, parent) {
		return true
	}
	deadline := time.Now().Add(groupNoticeHoldTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollEvery)
		if parentIdle(run, parent) {
			return true
		}
	}
	return parentIdle(run, parent)
}

// maybeDeliverGroup delivers the grouped notice for the group agentID
// belongs to, when its last member has submitted. Each submit records first;
// only the submit that completes the group delivers, under the group's lock,
// so two reviewers submitting at once deliver exactly one notice carrying
// both results. A member whose pane vanished counts as lost, so the group
// always completes. A group with no parent pane still completes: there is
// nowhere to deliver to, so it is marked delivered without typing. A member
// a tell claimed has left the group (claimNextSubmit clears its GroupID),
// so the group completes without it and its answer goes to the waiting tell.
func maybeDeliverGroup(agentID string) error {
	st, ok, err := ReadAgentState(agentID)
	if err != nil || !ok || st.GroupID == "" {
		return err
	}
	return deliverGroupIfComplete(st.GroupID)
}

// spawnGroupDelivered reports whether groupID's notice already went out. A
// group with no file yet is still open: members that submit synchronously
// during the launch wait for the spawn-side check, so missing counts as
// undelivered. Once the notice has gone out, a member's later submit falls
// back to an individual notice instead of joining the closed group, so a
// second submit after delivery is never silently dropped.
func spawnGroupDelivered(groupID string) (bool, error) {
	if groupID == "" {
		return true, nil
	}
	g, ok, err := readSpawnGroup(groupID)
	if err != nil || !ok {
		return false, err
	}
	return g.Delivered, nil
}

// deliverGroupIfComplete delivers the grouped notice for groupID when every
// remaining member has submitted or is lost. A member's submit calls it
// synchronously: the child's submit waits for the parent to go idle (held,
// then delivered anyway with a warning past the bound). Every other caller
// dispatches it asynchronously (dispatchGroupDelivery), because those callers
// run inside the parent's own tool call: waiting there would stall the call,
// and typing there would cancel it.
func deliverGroupIfComplete(groupID string) error {
	var deliverErr error
	err := withFileLock("group-"+groupID, func() error {
		g, ok, err := readSpawnGroup(groupID)
		if err != nil || !ok || g.Delivered {
			return err
		}
		submits := map[string]AgentSubmit{}
		lost := map[string]string{}
		remaining := 0
		for _, mid := range g.Members {
			mst, mok, err := ReadAgentState(mid)
			if err != nil {
				return err
			}
			if mok && mst.GroupID != groupID {
				// Left this group after the spawn: claimed by a tell (its
				// GroupID is cleared and its answer goes to the waiting
				// tell), or re-grouped by a later spawn. Either way it no
				// longer blocks this group.
				continue
			}
			remaining++
			if s, ok, err := ReadAgentSubmit(mid); err != nil {
				return err
			} else if ok {
				submits[mid] = s
				continue
			}
			if !mok || mst.Pane == "" {
				// No state yet: a queued worker that has not started.
				// The group waits for it.
				return nil
			}
			if isPaneLost(deliverExec, mst.Pane) {
				typ := mst.Type
				lost[mid] = lostLine(mid, typ)
				continue
			}
			return nil
		}
		if remaining == 0 {
			g.Delivered = true
			return writeSpawnGroup(g)
		}
		notice := groupNoticeText(g, submits, lost)
		idle := waitForParentIdle(deliverExec, g.Parent)
		if !idle {
			notice = "<system-notice> warning: the parent was busy for a long time; delivering anyway. The result that follows may have interrupted a tool call.</system-notice>\n" + notice
		}
		if g.Parent == "" {
			g.Delivered = true
			return writeSpawnGroup(g)
		}
		if err := deliverToParent(AgentState{Parent: g.Parent}, notice); err != nil {
			deliverErr = err
			return err
		}
		g.Delivered = true
		return writeSpawnGroup(g)
	})
	if deliverErr != nil {
		return deliverErr
	}
	return err
}

// dispatchGroupDelivery runs deliverGroupIfComplete in the background and
// returns at once. Callers that run inside the parent's own tool call
// (the spawn return path, the sweep inside spawn/tell/finish, the
// tell-failure finalizer) must use it: waiting there would stall the call
// they are part of, and typing there would cancel it. The background
// delivery still holds for the parent to go idle (warning past the bound),
// so the hold rule is kept; only the triggering call returns at once.
func dispatchGroupDelivery(groupID string) {
	go func() {
		_ = deliverGroupIfComplete(groupID)
	}()
}

// sweepOpenGroups dispatches a background delivery for every undelivered
// group of the calling parent that is already complete (all remaining
// members submitted or lost). A pane that vanishes without any further
// submit leaves no other trigger, so each parent action sweeps first: a
// stranded group whose members are all accounted for is finalized after
// the triggering call ends, instead of waiting forever. Dispatch is
// asynchronous, so the sweep never stalls or cancels its own call.
func sweepOpenGroups(parentPane, parentID string) {
	entries, err := os.ReadDir(groupDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		g, ok, err := readSpawnGroup(id)
		if err != nil || !ok || g.Delivered {
			continue
		}
		match := false
		if parentPane != "" && g.Parent == parentPane {
			match = true
		} else if parentID != "" && g.ParentID == parentID {
			match = true
		}
		if !match {
			continue
		}
		dispatchGroupDelivery(id)
	}
}
