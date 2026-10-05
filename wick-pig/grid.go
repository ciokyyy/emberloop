package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The agent grid lays agents out in a tab side by side, in rows of three,
// filling left to right and top to bottom: `1 2 3` / `4 5 6` / `7 8 9`. A tab
// holds gridMax of them (the caller is the root pane); the next one opens in a
// tab labelled by the caller.
//
// herdr can only split a pane, never a region, so the grid is three columns
// that are each split into rows. A new pane at slot k splits `right` from the
// pane before it while the first row is still filling (k is 2 or 3), and
// `down` from the pane one row above it (k-3) after that. The split ratio is
// arbitrary: tilePanes is what makes the panes equal.
const (
	gridCols = 3
	gridMax  = gridCols * gridCols
)

// gridPane is where the grid put a pane: the pane and tab it landed in, and
// its slot in the tab's reading order (1 is the tab's root pane: the caller in
// the caller's tab, the first pane of an overflow tab).
type gridPane struct {
	Pane string
	Tab  string
	Slot int
}

var (
	gridLocksMu sync.Mutex
	gridLocks   = map[string]*sync.Mutex{}
)

// gridLock is the in-process lock for one tab. Splits in different tabs run
// at the same time; splits in one tab are serialized, so a second launch
// never splits a half-built layout.
func gridLock(tab string) *sync.Mutex {
	gridLocksMu.Lock()
	defer gridLocksMu.Unlock()
	m := gridLocks[tab]
	if m == nil {
		m = &sync.Mutex{}
		gridLocks[tab] = m
	}
	return m
}

// withGridLock holds one tab's grid lock while fn runs. Sessions are separate
// wick-mcp processes, so the in-process mutex alone cannot serialize what
// they do to shared state: the file lock holds across all of them. One lock
// per tab, so splits in different tabs still run at the same time.
func withGridLock(tab string, fn func() error) error {
	lock := gridLock(tab)
	lock.Lock()
	defer lock.Unlock()
	return withFileLock("grid-"+tab, fn)
}

// gridDir holds the grid's slot record: one locked file per tab.
func gridDir() string { return filepath.Join(filepath.Dir(statePath), "grid") }

// gridFile is the slot record of one tab: which pane holds which slot.
func gridFile(tab string) string { return filepath.Join(gridDir(), lockName(tab)+".json") }

// loadGridRecord reads one tab's slot record. A missing file is an empty tab.
func loadGridRecord(tab string) (map[int]string, error) {
	raw, err := os.ReadFile(gridFile(tab))
	if os.IsNotExist(err) {
		return map[int]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[int]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("grid record for %s is corrupt: %w", tab, err)
	}
	if out == nil {
		out = map[int]string{}
	}
	return out, nil
}

// saveGridRecord writes one tab's slot record through a temp file and a
// rename, so a concurrent reader never sees a half-written file. The caller
// holds the tab's grid lock.
func saveGridRecord(tab string, slots map[int]string) error {
	if err := os.MkdirAll(gridDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(slots)
	if err != nil {
		return err
	}
	tmp := gridFile(tab) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, gridFile(tab)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// placeInGrid puts one new pane in tab and returns it. When tab already holds
// gridMax panes the pane opens in a tab labelled overflow in the same
// workspace. Sizes are evened out best effort: an uneven tab is still a
// working tab.
//
// The whole placement holds the tab's grid lock, and the slot record is read
// and written under it, so two sessions placing at the same moment get
// distinct slots. The overflow branch releases the tab's lock first: every
// grid lock is taken alone, so two sessions can never hold each other's tab
// locks, and a claim that resolves to the caller's own tab cannot deadlock
// on its own lock. A claim that fills up before the placement lands looks at
// the source tab again before claiming again.
func placeInGrid(run Exec, tab, overflow, cwd string) (gridPane, error) {
	for {
		var out gridPane
		full := false
		err := withGridLock(tab, func() error {
			slots, err := gridSlotsInLock(run, tab)
			if err != nil {
				return err
			}
			if nextSlot(slots) > gridMax {
				full = true
				return nil
			}
			gp, err := splitForGrid(run, tab, nextSlot(slots), slots, cwd)
			if err != nil {
				return err
			}
			out = gp
			return nil
		})
		if err != nil {
			return gridPane{}, err
		}
		if !full {
			return out, nil
		}
		gp, placed, err := placeInOverflowOnce(run, tab, overflow, cwd)
		if err != nil {
			return gridPane{}, err
		}
		if placed {
			return gp, nil
		}
	}
}

// placeInOverflowOnce claims one overflow tab and places one pane into it. It
// takes each lock alone — first the workspace overflow lock for the claim,
// then the claimed tab's own grid lock for the placement — so sessions never
// hold one tab's lock while waiting on another's. The caller holds no grid
// lock. placed is false when the claimed tab filled up after the claim, so
// the caller looks at its source tab again before claiming again.
func placeInOverflowOnce(run Exec, tab, label, cwd string) (gridPane, bool, error) {
	over, overSlots, fresh, err := claimOverflowTab(run, tab, label, cwd)
	if err != nil {
		return gridPane{}, false, err
	}
	if fresh {
		// A fresh overflow tab's root pane is the agent's, the way the
		// caller's pane is the root of the tab it opened.
		if err := recordGridSlot(over, 1, overSlots[1]); err != nil {
			return gridPane{}, false, err
		}
		return gridPane{Pane: overSlots[1], Tab: over, Slot: 1}, true, nil
	}
	return placeInExistingOverflow(run, over, cwd)
}

// claimOverflowTab finds or creates the overflow tab a placement takes, under
// the workspace overflow lock, so two sessions never create the same label
// twice. The returned slots are advisory: the placement re-reads the tab's
// record under that tab's own lock, which may find it full after all.
func claimOverflowTab(run Exec, tab, label, cwd string) (over string, slots map[int]string, fresh bool, err error) {
	err = withFileLock("grid-overflow-"+workspaceOf(tab), func() error {
		o, s, f, err := overflowTab(run, tab, label, cwd)
		if err != nil {
			return err
		}
		over, slots, fresh = o, s, f
		return nil
	})
	return over, slots, fresh, err
}

// placeInExistingOverflow places one pane in over under over's own grid lock,
// re-reading its record there. placed is false when the tab filled up after
// it was claimed, so the caller claims again.
func placeInExistingOverflow(run Exec, tab, cwd string) (gridPane, bool, error) {
	var out gridPane
	placed := false
	err := withGridLock(tab, func() error {
		slots, err := gridSlotsInLock(run, tab)
		if err != nil {
			return err
		}
		if nextSlot(slots) > gridMax {
			return nil
		}
		gp, err := splitForGrid(run, tab, nextSlot(slots), slots, cwd)
		if err != nil {
			return err
		}
		out, placed = gp, true
		return nil
	})
	return out, placed, err
}

// replaceInGrid puts a new pane in the slot a dead pane left, so the rest of
// the grid keeps its positions and only the sizes are evened out. slot is the
// dead pane's slot, 2..gridMax; slot 1 is the tab's root pane, which has no
// pane before it to split and cannot be rebuilt.
func replaceInGrid(run Exec, tab string, slot int, cwd string) (gridPane, error) {
	if slot < 2 || slot > gridMax {
		return gridPane{}, fmt.Errorf("slot %d is not a grid slot", slot)
	}
	var out gridPane
	err := withGridLock(tab, func() error {
		slots, err := gridSlotsInLock(run, tab)
		if err != nil {
			return err
		}
		gp, err := splitForGrid(run, tab, slot, slots, cwd)
		if err != nil {
			return err
		}
		out = gp
		return nil
	})
	return out, err
}

// splitForGrid splits the pane a new pane at slot grows from, evens the tab
// out, and remembers the slot. The caller holds the tab's grid lock, and the
// slot is recorded under it.
func splitForGrid(run Exec, tab string, slot int, slots map[int]string, cwd string) (gridPane, error) {
	target, dir, ok := gridTarget(slot, slots)
	if !ok {
		return gridPane{}, fmt.Errorf("no pane in %s to split for slot %d", tab, slot)
	}
	reply, err := herdrJSON(run, []string{"pane", "split",
		"--pane", target,
		"--direction", dir,
		"--ratio", "0.5",
		"--cwd", cwd,
		"--no-focus",
	}, 15*time.Second)
	if err != nil {
		return gridPane{}, err
	}
	pane := reply.Result.Pane.PaneID
	if pane == "" {
		return gridPane{}, fmt.Errorf("herdr split returned no pane id")
	}
	landed := reply.Result.Pane.TabID
	if landed == "" {
		landed = tab
	}
	if err := recordGridSlotInLock(landed, slot, pane); err != nil {
		return gridPane{}, err
	}
	// Tiling is cosmetic: never fail a launch over a tab that stays uneven.
	_ = tilePanes(run, landed)
	return gridPane{Pane: pane, Tab: landed, Slot: slot}, nil
}

// gridTarget is the pane a new pane at slot splits from and the way it splits:
// the first row grows to the right of the pane before it, later rows split
// down from the pane a row above.
func gridTarget(slot int, slots map[int]string) (pane, dir string, ok bool) {
	if slot < 2 {
		return "", "", false
	}
	source := slot - 1
	dir = "right"
	if slot > gridCols {
		source = slot - gridCols
		dir = "down"
	}
	pane, ok = slots[source]
	return pane, dir, ok
}

// gridSlots is the pane in each grid slot of tab. Slots the grid has placed
// come from its record, kept only while that pane is still live; a pane the
// grid never placed (before a restart, say) fills the first free slot in pane
// list order, which is the fill order until a pane has been replaced.
func gridSlots(run Exec, tab string) (map[int]string, error) {
	var out map[int]string
	err := withGridLock(tab, func() error {
		slots, err := gridSlotsInLock(run, tab)
		if err != nil {
			return err
		}
		out = slots
		return nil
	})
	return out, err
}

// gridSlotsInLock is gridSlots with the tab's grid lock held: the file record
// is read under the lock, pruned against the live pane list, and written
// back, so the next session under the same lock sees this one's placements.
func gridSlotsInLock(run Exec, tab string) (map[int]string, error) {
	live, err := tabPanes(run, tab)
	if err != nil {
		return nil, err
	}
	rec, err := loadGridRecord(tab)
	if err != nil {
		return nil, err
	}
	slots := assignGridSlots(rec, live)
	if err := saveGridRecord(tab, slots); err != nil {
		return nil, err
	}
	return slots, nil
}

// peekGridSlots reads what gridSlotsInLock would return, without holding the
// tab's lock and without writing anything back. The overflow claim uses it to
// tell a tab with room from a full one; the placement then re-reads under
// the tab's lock, so a stale peek only costs a second claim, never a wrong
// slot. A file rename is atomic, so a concurrent write is never half seen.
func peekGridSlots(run Exec, tab string) (map[int]string, error) {
	live, err := tabPanes(run, tab)
	if err != nil {
		return nil, err
	}
	rec, err := loadGridRecord(tab)
	if err != nil {
		return nil, err
	}
	return assignGridSlots(rec, live), nil
}

// assignGridSlots packs live panes into slots: recorded panes keep theirs
// while live, and the rest fill the lowest free slots in pane list order.
func assignGridSlots(rec map[int]string, live []string) map[int]string {
	liveSet := make(map[string]bool, len(live))
	for _, id := range live {
		liveSet[id] = true
	}
	slots := map[int]string{}
	taken := map[string]bool{}
	for s, pane := range rec {
		if liveSet[pane] {
			slots[s] = pane
			taken[pane] = true
		}
	}
	next := 1
	for _, pane := range live {
		if taken[pane] {
			continue
		}
		for slots[next] != "" {
			next++
		}
		slots[next] = pane
		taken[pane] = true
	}
	return slots
}

// recordGridSlot remembers that pane holds slot of tab, across sessions.
func recordGridSlot(tab string, slot int, pane string) error {
	return withGridLock(tab, func() error {
		return recordGridSlotInLock(tab, slot, pane)
	})
}

// recordGridSlotInLock is recordGridSlot with the tab's grid lock held.
func recordGridSlotInLock(tab string, slot int, pane string) error {
	rec, err := loadGridRecord(tab)
	if err != nil {
		return err
	}
	rec[slot] = pane
	return saveGridRecord(tab, rec)
}

// nextSlot is the lowest slot a new pane can take: one that is free and whose
// source pane (the one before it, or the one a row above) is still there to
// split. Slot 1 is the tab's root and has no source, so a tab whose root died
// fills its next splittable slot instead of failing. gridMax+1 means no slot
// is left.
func nextSlot(slots map[int]string) int {
	for s := 2; s <= gridMax; s++ {
		if slots[s] != "" {
			continue
		}
		source := s - 1
		if s > gridCols {
			source = s - gridCols
		}
		if slots[source] != "" {
			return s
		}
	}
	return gridMax + 1
}

// overflowTab is the tab labelled label in tab's workspace that still has room
// for a pane, creating one when none does. A full overflow tab moves on to the
// next label in its family: `scouts` then `scouts (2)`, `<ticket> (2)` then
// `<ticket> (3)`. fresh says the tab was just created, so its root pane is the
// pane the agent takes. The caller holds the workspace overflow lock, which
// serializes creation between sessions no matter which tab they place from.
// The returned slots are a peek, read without the tab's lock and never
// written back, so they cannot clobber a concurrent placement's record: the
// placement re-reads under the tab's own lock.
func overflowTab(run Exec, tab, label, cwd string) (id string, slots map[int]string, fresh bool, err error) {
	ws := workspaceOf(tab)
	reply, err := herdrJSON(run, []string{"tab", "list", "--workspace", ws}, 15*time.Second)
	if err != nil {
		return "", nil, false, err
	}
	byLabel := map[string]string{}
	for _, t := range reply.Result.Tabs {
		byLabel[t.Label] = t.TabID
	}
	for lbl := label; ; lbl = nextOverflow(lbl) {
		tid, ok := byLabel[lbl]
		if !ok {
			return createOverflowTab(run, ws, lbl, cwd)
		}
		panes, err := peekGridSlots(run, tid)
		if err != nil {
			return "", nil, false, err
		}
		if nextSlot(panes) <= gridMax {
			return tid, panes, false, nil
		}
	}
}

// nextOverflow is the label after one in an overflow family: a trailing " (n)"
// counts up, otherwise the label takes a " (2)".
func nextOverflow(label string) string {
	if i := strings.LastIndex(label, " ("); i >= 0 && strings.HasSuffix(label, ")") {
		if n, err := strconv.Atoi(label[i+2 : len(label)-1]); err == nil {
			return fmt.Sprintf("%s (%d)", label[:i], n+1)
		}
	}
	return label + " (2)"
}

// createOverflowTab opens a fresh tab labelled label in workspace, with cwd as
// its root pane's directory.
func createOverflowTab(run Exec, workspace, label, cwd string) (id string, slots map[int]string, fresh bool, err error) {
	reply, err := herdrJSON(run, []string{"tab", "create",
		"--workspace", workspace,
		"--cwd", cwd,
		"--label", label,
		"--no-focus",
	}, 30*time.Second)
	if err != nil {
		return "", nil, false, err
	}
	tid := reply.Result.Tab.TabID
	if tid == "" {
		tid = reply.Result.RootPane.TabID
	}
	pane := reply.Result.RootPane.PaneID
	if tid == "" || pane == "" {
		return "", nil, false, fmt.Errorf("herdr tab create returned no tab or pane for %q", label)
	}
	return tid, map[int]string{1: pane}, true, nil
}

// workspaceOf is the workspace a tab id belongs to, from the id herdr qualifies
// it with, else the caller's own workspace.
func workspaceOf(tab string) string {
	if i := strings.IndexByte(tab, ':'); i > 0 {
		return tab[:i]
	}
	return os.Getenv("HERDR_WORKSPACE_ID")
}

// closeOverflowTabs closes every overflow tab that belongs to the tab labelled
// base: base (2), base (3), and any higher number, even when one in between is
// already gone. The base tab is the caller's to close. wick owns its tabs, but
// a missing one is not an error.
func closeOverflowTabs(run Exec, tab, base string) {
	if tab == "" || base == "" {
		return
	}
	reply, err := herdrJSON(run, []string{"tab", "list", "--workspace", workspaceOf(tab)}, 30*time.Second)
	if err != nil {
		return
	}
	for _, t := range reply.Result.Tabs {
		if overflowNumber(t.Label, base) == 0 {
			continue
		}
		_, _ = herdrJSON(run, []string{"tab", "close", t.TabID}, 30*time.Second)
	}
}

// overflowNumber is the n in the overflow label "base (n)", or 0 when label is
// not one of base's overflow tabs.
func overflowNumber(label, base string) int {
	if !strings.HasPrefix(label, base+" (") || !strings.HasSuffix(label, ")") {
		return 0
	}
	n, err := strconv.Atoi(label[len(base)+2 : len(label)-1])
	if err != nil || n < 2 {
		return 0
	}
	return n
}
