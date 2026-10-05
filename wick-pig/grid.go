package wick

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// The agent grid lays agents out in a tab side by side, in rows of three,
// filling left to right and top to bottom: `1 2 3` / `4 5 6` / `7 8 9`. A tab
// holds gridMax of them (the caller is pane 1); the next one opens in a tab
// labelled by the caller.
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
// its slot in the tab's fill order (1 is the caller).
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

// placeInGrid puts one new pane in tab and returns it. When tab already holds
// gridMax panes the pane opens in a tab labelled overflow in the same
// workspace, and the overflow tab is created under tab's lock. Sizes are evened
// out best effort: an uneven tab is still a working tab.
func placeInGrid(run Exec, tab, overflow, cwd string) (gridPane, error) {
	lock := gridLock(tab)
	lock.Lock()
	defer lock.Unlock()

	ids, err := tabPanes(run, tab)
	if err != nil {
		return gridPane{}, err
	}
	if len(ids) < gridMax {
		return splitForGrid(run, tab, len(ids)+1, ids, cwd)
	}

	over, overIDs, fresh, err := overflowTab(run, tab, overflow, cwd)
	if err != nil {
		return gridPane{}, err
	}
	if fresh {
		// A fresh overflow tab's root pane is the agent's, the way the
		// caller's pane is pane 1 of the tab it opened.
		return gridPane{Pane: overIDs[0], Tab: over, Slot: 1}, nil
	}
	if len(overIDs) >= gridMax {
		return gridPane{}, fmt.Errorf("tab %s and its overflow %q are both full", tab, overflow)
	}
	return splitForGrid(run, over, len(overIDs)+1, overIDs, cwd)
}

// replaceInGrid puts a new pane in the slot a dead pane left, so the rest of
// the grid keeps its positions and only the sizes are evened out. slot is the
// dead pane's slot, 2..gridMax; slot 1 is the caller, which has no slot to
// rebuild.
func replaceInGrid(run Exec, tab string, slot int, cwd string) (gridPane, error) {
	if slot < 2 || slot > gridMax {
		return gridPane{}, fmt.Errorf("slot %d is not a grid slot", slot)
	}
	lock := gridLock(tab)
	lock.Lock()
	defer lock.Unlock()

	ids, err := tabPanes(run, tab)
	if err != nil {
		return gridPane{}, err
	}
	return splitForGrid(run, tab, slot, ids, cwd)
}

// splitForGrid splits the pane a new pane at slot grows from and evens the tab
// out.
func splitForGrid(run Exec, tab string, slot int, ids []string, cwd string) (gridPane, error) {
	target, dir, ok := gridTarget(slot, ids)
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
	// Tiling is cosmetic: never fail a launch over a tab that stays uneven.
	_ = tilePanes(run, landed)
	return gridPane{Pane: pane, Tab: landed, Slot: slot}, nil
}

// gridTarget is the pane a new pane at slot splits from and the way it splits:
// the first row grows to the right of the pane before it, later rows split
// down from the pane a row above.
func gridTarget(slot int, ids []string) (pane, dir string, ok bool) {
	if slot < 2 {
		return "", "", false
	}
	source := slot - 1
	dir = "right"
	if slot > gridCols {
		source = slot - gridCols
		dir = "down"
	}
	if source < 1 || source > len(ids) {
		return "", "", false
	}
	return ids[source-1], dir, true
}

// overflowTab is the tab labelled label in tab's workspace that still has room
// for a pane, creating one when none does. fresh says it was just created, so
// its root pane is the pane the agent takes.
func overflowTab(run Exec, tab, label, cwd string) (id string, ids []string, fresh bool, err error) {
	ws := workspaceOf(tab)
	reply, err := herdrJSON(run, []string{"tab", "list", "--workspace", ws}, 15*time.Second)
	if err != nil {
		return "", nil, false, err
	}
	for _, t := range reply.Result.Tabs {
		if t.Label != label {
			continue
		}
		panes, err := tabPanes(run, t.TabID)
		if err != nil {
			return "", nil, false, err
		}
		if len(panes) < gridMax {
			return t.TabID, panes, false, nil
		}
	}
	reply, err = herdrJSON(run, []string{"tab", "create",
		"--workspace", ws,
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
	return tid, []string{pane}, true, nil
}

// workspaceOf is the workspace a tab id belongs to, from the id herdr qualifies
// it with, else the caller's own workspace.
func workspaceOf(tab string) string {
	if i := strings.IndexByte(tab, ':'); i > 0 {
		return tab[:i]
	}
	return os.Getenv("HERDR_WORKSPACE_ID")
}
