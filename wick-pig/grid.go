package wick

import (
	"fmt"
	"os"
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

// gridLock is the lock for one tab. Splits in different tabs run at the same
// time; splits in one tab are serialized, so a second launch never splits a
// half-built layout.
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

// gridPlaced remembers which pane holds which slot of a tab. herdr cannot say:
// a pane's slot is gone with the pane, and a replacement lands at the end of
// pane list, not in the hole it filled. The grid is the only place that knows,
// because it chose every slot. It is pruned against pane list on every read,
// so a pane closed behind the grid's back still frees its slot.
var (
	gridPlacedMu sync.Mutex
	gridPlaced   = map[string]map[int]string{}
)

// placeInGrid puts one new pane in tab and returns it. When tab already holds
// gridMax panes the pane opens in a tab labelled overflow in the same
// workspace, and the overflow tab is created under tab's lock. Sizes are evened
// out best effort: an uneven tab is still a working tab.
func placeInGrid(run Exec, tab, overflow, cwd string) (gridPane, error) {
	lock := gridLock(tab)
	lock.Lock()
	defer lock.Unlock()

	slots, err := gridSlots(run, tab)
	if err != nil {
		return gridPane{}, err
	}
	if slot := nextSlot(slots); slot <= gridMax {
		return splitForGrid(run, tab, slot, slots, cwd)
	}

	over, overSlots, fresh, err := overflowTab(run, tab, overflow, cwd)
	if err != nil {
		return gridPane{}, err
	}
	if fresh {
		// A fresh overflow tab's root pane is the agent's, the way the
		// caller's pane is the root of the tab it opened.
		recordGridSlot(over, 1, overSlots[1])
		return gridPane{Pane: overSlots[1], Tab: over, Slot: 1}, nil
	}
	return splitForGrid(run, over, nextSlot(overSlots), overSlots, cwd)
}

// replaceInGrid puts a new pane in the slot a dead pane left, so the rest of
// the grid keeps its positions and only the sizes are evened out. slot is the
// dead pane's slot, 2..gridMax; slot 1 is the tab's root pane, which has no
// pane before it to split and cannot be rebuilt.
func replaceInGrid(run Exec, tab string, slot int, cwd string) (gridPane, error) {
	if slot < 2 || slot > gridMax {
		return gridPane{}, fmt.Errorf("slot %d is not a grid slot", slot)
	}
	lock := gridLock(tab)
	lock.Lock()
	defer lock.Unlock()

	slots, err := gridSlots(run, tab)
	if err != nil {
		return gridPane{}, err
	}
	return splitForGrid(run, tab, slot, slots, cwd)
}

// splitForGrid splits the pane a new pane at slot grows from, evens the tab
// out, and remembers the slot.
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
	recordGridSlot(landed, slot, pane)
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
	live, err := tabPanes(run, tab)
	if err != nil {
		return nil, err
	}
	liveSet := make(map[string]bool, len(live))
	for _, id := range live {
		liveSet[id] = true
	}

	gridPlacedMu.Lock()
	defer gridPlacedMu.Unlock()
	slots := map[int]string{}
	taken := map[string]bool{}
	for s, pane := range gridPlaced[tab] {
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
	gridPlaced[tab] = slots
	return slots, nil
}

// recordGridSlot remembers that pane holds slot of tab.
func recordGridSlot(tab string, slot int, pane string) {
	gridPlacedMu.Lock()
	defer gridPlacedMu.Unlock()
	m := gridPlaced[tab]
	if m == nil {
		m = map[int]string{}
		gridPlaced[tab] = m
	}
	m[slot] = pane
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
// pane the agent takes.
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
		panes, err := gridSlots(run, tid)
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

// closeOverflowTabs closes the overflow tabs that belong to the tab labelled
// base: base (2), base (3), .... The base tab is the caller's to close. A tab
// already gone is not an error: wick owns its tabs, but a human may have
// closed one.
func closeOverflowTabs(run Exec, tab, base string) {
	if tab == "" || base == "" {
		return
	}
	reply, err := herdrJSON(run, []string{"tab", "list", "--workspace", workspaceOf(tab)}, 30*time.Second)
	if err != nil {
		return
	}
	present := map[string]string{}
	for _, t := range reply.Result.Tabs {
		present[t.Label] = t.TabID
	}
	for lbl := base + " (2)"; ; lbl = nextOverflow(lbl) {
		tid, ok := present[lbl]
		if !ok {
			return
		}
		_, _ = herdrJSON(run, []string{"tab", "close", tid}, 30*time.Second)
	}
}
