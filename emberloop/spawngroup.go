package el

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// Replaced maps a dead member to the fresh agent that took over its
	// seat: the group resolves with the replacement's verdict, never with
	// the dead member's stale answer or a lost line for it.
	Replaced map[string]string `json:"replaced,omitempty"`
	// Delivered marks a group whose notice already went out, so two members
	// submitting at the same moment deliver exactly once.
	Delivered bool   `json:"delivered,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

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
// pane vanished contributes a lost line, and a member whose last turn ended
// in a provider error contributes a stopped-with-error line, so the group
// always completes.
func groupNoticeText(g spawnGroup, submits map[string]AgentSubmit, lost map[string]string, errored map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<system-notice> spawn complete (%d of %d agents). Resume your work using the results below.\n", len(submits)+len(lost)+len(errored), len(g.Members))
	for i, id := range g.Members {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		if s, ok := submits[id]; ok {
			b.WriteString(renderSubmit(s))
		} else if line, ok := lost[id]; ok {
			b.WriteString(line)
		} else if line, ok := errored[id]; ok {
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

// workerLostLine reports a worktree agent that cannot submit: its pane was
// closed by hand. It keeps the lost shape, so the sweep's exactly-once rule
// and the tests that match "lost:" hold.
func workerLostLine(id, typ string) string {
	if typ == "" {
		typ = "agent"
	}
	return fmt.Sprintf("%s %s lost: its pane closed manually before it could submit.", typ, id)
}

// errorLine reports a member whose last turn ended in a provider error, naming
// the agent and the error.
func errorLine(id, typ, detail string) string {
	if typ == "" {
		typ = "agent"
	}
	detail = strings.Join(strings.Fields(detail), " ")
	if detail == "" {
		detail = "unknown error"
	}
	return fmt.Sprintf("%s %s stopped with error: %s", typ, id, detail)
}

// paneDead reports whether no agent runs in pane: herdr no longer knows it,
// or knows it but only its shell is left and the launch is past the boot
// grace. A pane herdr answers pane_not_found for is dead at once, no grace:
// only a pane that still exists but looks empty may still be starting its
// child. launchedAt is the child's start time (RFC 3339); empty means no
// grace. One process-info call answers it, judged the way the stop gate
// judges it.
func paneDead(run Exec, pane, launchedAt string) bool {
	if pane == "" {
		return false
	}
	res, err := run(herdrBin(), []string{"pane", "process-info", "--pane", pane}, ExecOpts{Timeout: 15 * time.Second})
	if err != nil {
		return false
	}
	if res.Code != 0 {
		return paneNotFound(res)
	}
	proc, ok := decodePaneProcess(res.Stdout)
	if !ok {
		return false
	}
	return proc.stopped() && !booting(launchedAt)
}

// replaceGroupMember records a dead member's replacement in every still-open
// group holding it, so the group resolves with the fresh agent's verdict
// instead of the dead member's stale answer or a lost line for it. Members
// stay as spawned; the replacement is read through the map at delivery, so
// a tell-claimed member (GroupID cleared, still listed) keeps its old
// meaning. Delivered groups keep their record. It returns the groups it
// changed, so the caller can deliver the ones already complete.
func replaceGroupMember(oldID, newID string) []string {
	if oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	entries, err := os.ReadDir(groupDir())
	if err != nil {
		return nil
	}
	var changed []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		_ = withFileLock("group-"+id, func() error {
			g, ok, err := readSpawnGroup(id)
			if err != nil || !ok || g.Delivered {
				return err
			}
			held := false
			for _, m := range g.Members {
				if m == oldID {
					held = true
					break
				}
			}
			if !held {
				return nil
			}
			if g.Replaced == nil {
				g.Replaced = map[string]string{}
			}
			g.Replaced[oldID] = newID
			if err := writeSpawnGroup(g); err != nil {
				return err
			}
			changed = append(changed, id)
			return nil
		})
	}
	return changed
}

// groupsWaitingOn lists the still-open groups waiting on agentID: the ones
// naming it a member, and the ones naming it as some dead member's
// replacement, so one submit completes both its own group and the older one
// it took over.
func groupsWaitingOn(agentID string) []string {
	if agentID == "" {
		return nil
	}
	entries, err := os.ReadDir(groupDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		g, ok, err := readSpawnGroup(id)
		if err != nil || !ok || g.Delivered {
			continue
		}
		for _, m := range g.Members {
			if m == agentID || g.Replaced[m] == agentID {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

// maybeDeliverGroup delivers every still-open group waiting on agentID, when
// its last member has submitted. Each submit records first; only the submit
// that completes the group delivers, under the group's lock, so two
// reviewers submitting at once deliver exactly one notice carrying both
// results. A member whose pane vanished counts as lost, so the group always
// completes. A group with no parent pane still completes: there is nowhere
// to deliver to, so it is marked delivered without typing. A member a tell
// claimed left the group (claimNextSubmit clears its GroupID, so the member
// loop skips it), and its answer goes to the waiting tell.
func maybeDeliverGroup(agentID string) error {
	if _, ok, err := ReadAgentState(agentID); err != nil || !ok {
		return err
	}
	for _, gid := range groupsWaitingOn(agentID) {
		if err := deliverGroupIfComplete(gid); err != nil {
			return err
		}
	}
	return nil
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

// replacementComplete folds a replaced member's outcome into the group's
// maps: the replacement's submit under the dead member's seat, its error
// line when its turn failed, or a lost line with its own exit trace when it
// died too. It reports whether the seat is settled; while the replacement
// still runs (or still boots) the group waits.
func replacementComplete(submits map[string]AgentSubmit, lost, errored map[string]string, mid, rep string) (bool, error) {
	if s, ok, err := ReadAgentSubmit(rep); err != nil {
		return false, err
	} else if ok {
		submits[mid] = s
		return true, nil
	}
	rst, rok, err := ReadAgentState(rep)
	if err != nil {
		return false, err
	}
	if !rok || rst.Pane == "" {
		return false, nil
	}
	if rst.Errored {
		errored[mid] = errorLine(rep, rst.Type, rst.ErrorDetail)
		return true, nil
	}
	if !paneDead(deliverExec, rst.Pane, rst.LaunchedAt) {
		return false, nil
	}
	lost[mid] = lostLine(rep, rst.Type) + exitSuffix(rst)
	return true, nil
}

// deliverGroupIfComplete delivers the grouped notice for groupID when every
// remaining member has submitted, is lost, or is errored — except a dead
// worktree agent, which the worker prune cancels and the dead-child sweep
// reports, so the group waits for it. A replaced member counts through its
// replacement (replacementComplete), so the notice carries the fresh
// verdict, never a lost line for the seat it took. A member's submit
// calls it synchronously. The notice goes out at once, busy parent or not:
// herdr queues typed text as steering, and a working agent reads it after its
// next tool call, so nothing is held and no tool call is cancelled. Every
// other caller dispatches it asynchronously (dispatchGroupDelivery), because
// those callers run inside the parent's own tool call and need not wait.
func deliverGroupIfComplete(groupID string) error {
	var deliverErr error
	err := withFileLock("group-"+groupID, func() error {
		g, ok, err := readSpawnGroup(groupID)
		if err != nil || !ok || g.Delivered {
			return err
		}
		submits := map[string]AgentSubmit{}
		lost := map[string]string{}
		errored := map[string]string{}
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
			// A replaced member resolves only through its replacement:
			// the dead member's own submit, if one is on disk, answered
			// an earlier turn, never this group's wait.
			if rep, ok := g.Replaced[mid]; ok && rep != "" {
				complete, err := replacementComplete(submits, lost, errored, mid, rep)
				if err != nil {
					return err
				}
				if !complete {
					return nil
				}
				continue
			}
			if s, ok, err := ReadAgentSubmit(mid); err != nil {
				return err
			} else if ok {
				submits[mid] = s
				continue
			}
			if mok && mst.Errored {
				errored[mid] = errorLine(mid, mst.Type, mst.ErrorDetail)
				continue
			}
			if !mok || mst.Pane == "" {
				// No state yet: a queued worker that has not started.
				// The group waits for it.
				return nil
			}
			if paneDead(deliverExec, mst.Pane, mst.LaunchedAt) {
				if mst.Ticket != "" {
					// A dead worktree agent is cancelled by the worker prune
					// and reported by the dead-child sweep, never here: the
					// group waits for it, so the parent gets exactly one
					// notice, worded "pane closed manually".
					return nil
				}
				typ := mst.Type
				lost[mid] = lostLine(mid, typ) + exitSuffix(mst)
				continue
			}
			return nil
		}
		if remaining == 0 {
			g.Delivered = true
			return writeSpawnGroup(g)
		}
		notice := groupNoticeText(g, submits, lost, errored)
		if g.Parent == "" {
			g.Delivered = true
			if err := writeSpawnGroup(g); err != nil {
				return err
			}
			closeGroupScouts(deliverExec, submits, lost, errored)
			return nil
		}
		if err := deliverToParent(AgentState{Parent: g.Parent}, notice); err != nil {
			deliverErr = err
			return err
		}
		g.Delivered = true
		if err := writeSpawnGroup(g); err != nil {
			return err
		}
		closeGroupScouts(deliverExec, submits, lost, errored)
		return nil
	})
	if deliverErr != nil {
		return deliverErr
	}
	return err
}

// closeGroupScouts closes every scout whose result the group just delivered,
// and cleans up its launch file. Workers own worktrees and reviewers stay
// open; only scouts close. Lost and errored members have no live turn left,
// but their launch file still goes, so a vanished pane leaves nothing behind.
// It runs inside the group's lock, so concurrent submits for one group cannot
// interleave closes with the delivery they belong to.
func closeGroupScouts(run Exec, submits map[string]AgentSubmit, lost map[string]string, errored map[string]string) {
	for id := range submits {
		closeDeliveredScout(run, id)
	}
	for id := range lost {
		closeDeliveredScout(run, id)
	}
	for id := range errored {
		closeDeliveredScout(run, id)
	}
}

// dispatchGroupDelivery runs deliverGroupIfComplete in the background and
// returns at once. Callers that run inside the parent's own tool call
// (the spawn return path, the sweep inside spawn/tell/finish, the
// tell-failure finalizer) use it so they never wait on the delivery they are
// part of. The notice is delivered at once, as steering.
func dispatchGroupDelivery(groupID string) {
	pendingDeliveries.Add(1)
	go func() {
		defer pendingDeliveries.Done()
		_ = deliverGroupIfComplete(groupID)
	}()
}

// pendingDeliveries counts background group deliveries still running, so a
// test can wait for them before it swaps or removes the state directory they
// write into.
var pendingDeliveries sync.WaitGroup

// waitForDeliveries blocks until every background delivery has ended or the
// bound passes, whichever comes first, and says whether they all ended.
func waitForDeliveries(bound time.Duration) bool {
	done := make(chan struct{})
	go func() {
		pendingDeliveries.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(bound):
		return false
	}
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
