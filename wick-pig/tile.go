package wick

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// A tab is a tree of splits. Each split divides a region in two and holds the
// share of it its first side keeps; a pane's rectangle is the region left over
// once every split above it has taken its share.
//
// herdr has no equalize action, only `pane split --ratio` and `pane resize
// --direction --amount`, so splitting the last pane again and again leaves a
// tab at 1/2, 1/4, 1/8. tilePanes reads that tree with `pane list` and `pane
// layout` and moves each divider until every pane holds 1/N of the tab.
//
// --ratio is the share the *original* pane keeps, not the share the new pane
// gets. A scratch pane 229 columns wide split at 0.3 measured 69 columns on
// the pane that was split and 160 on the new one, and the same held splitting
// down: 0.25 off a 70-row pane left it 18 rows and gave the new pane 52. So
// readonly.go's split hands the reviewer 0.7 of the pane and keeps 0.3 for the
// caller.
//
// --direction names the way the divider moves, and --amount is a share of the
// region the divider splits: 0.007 on a 70-row region moved a ratio of 0.53 to
// 0.537 and the divider with it. A pane's own geometry rounds that ratio, so a
// divider sits at round(ratio * region).

// tileTolerance is how far a pane's share of its tab may sit from 1/N.
const tileTolerance = 0.02

// paneRect is a rectangle of cells, in herdr's field names.
type paneRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// paneBox is one pane of a tab and where it sits.
type paneBox struct {
	PaneID string   `json:"pane_id"`
	Rect   paneRect `json:"rect"`
}

// paneSplit is one divider of a tab: the direction it runs, the share of its
// region its first side holds, and the region itself.
type paneSplit struct {
	Direction string   `json:"direction"`
	Ratio     float64  `json:"ratio"`
	Rect      paneRect `json:"rect"`
}

// paneLayout is the geometry of one tab, as `pane layout` reports it.
type paneLayout struct {
	Area   paneRect    `json:"area"`
	TabID  string      `json:"tab_id"`
	Panes  []paneBox   `json:"panes"`
	Splits []paneSplit `json:"splits"`
}

// tileMove is one divider move: the pane herdr has to be aimed at for the move
// to land on the right split, the way its divider goes, and how far.
type tileMove struct {
	pane      string
	direction string
	amount    float64
	cells     int // how many cells the divider moves; the biggest one wins
}

// tilePanes gives every pane in tab the same share of it.
//
// It is best effort by design: a tab that cannot be evened out is still a
// working tab, so spawn_worker reports its error as a warning rather than
// failing a spawn over an uneven pane, and the reviewer split ignores it.
func tilePanes(run Exec, tab string) error {
	if tab == "" {
		return nil
	}
	ids, err := tabPanes(run, tab)
	if err != nil {
		return err
	}
	if len(ids) < 2 {
		// Nothing to share with. The common case for spawn_worker, whose
		// worktree opens in a workspace of its own.
		return nil
	}

	// One divider per round, so every move is measured against the layout the
	// last one left. A move can change the region of a nested split even
	// though it leaves that split's ratio alone, which is why a single
	// snapshot cannot drive them all.
	for round := 0; round < 3*len(ids)+6; round++ {
		reply, err := herdrJSON(run, []string{"pane", "layout", "--pane", ids[0]}, 15*time.Second)
		if err != nil {
			return err
		}
		lay := reply.Result.Layout
		boxes := inTab(lay, ids)
		if len(boxes) < 2 {
			return nil
		}

		total := float64(lay.Area.Width * lay.Area.Height)
		if evenShare(boxes, total/float64(len(boxes)), tileTolerance*total) {
			return nil
		}
		move, ok := widestMove(lay, boxes)
		if !ok {
			return fmt.Errorf("panes in %s are not even and no divider can move", tab)
		}
		if _, err := herdrJSON(run, []string{"pane", "resize",
			"--pane", move.pane,
			"--direction", move.direction,
			"--amount", strconv.FormatFloat(move.amount, 'f', 4, 64),
		}, 15*time.Second); err != nil {
			return err
		}
	}
	return fmt.Errorf("panes in %s never evened out", tab)
}

// tabPanes lists the panes herdr reports in tab, in the order it reports them.
func tabPanes(run Exec, tab string) ([]string, error) {
	reply, err := herdrJSON(run, []string{"pane", "list"}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, p := range reply.Result.Panes {
		if p.TabID == tab {
			ids = append(ids, p.PaneID)
		}
	}
	return ids, nil
}

// inTab keeps the layout's panes that pane list put in the tab.
func inTab(lay paneLayout, ids []string) []paneBox {
	in := make(map[string]bool, len(ids))
	for _, id := range ids {
		in[id] = true
	}
	var out []paneBox
	for _, b := range lay.Panes {
		if in[b.PaneID] {
			out = append(out, b)
		}
	}
	return out
}

// widestMove picks the split with the most cells to move, so each round of the
// loop lands the biggest correction there is to make.
func widestMove(lay paneLayout, boxes []paneBox) (tileMove, bool) {
	var best tileMove
	found := false
	for _, s := range lay.Splits {
		m, ok := moveFor(s, boxes)
		if !ok {
			continue
		}
		if !found || math.Abs(float64(m.cells)) > math.Abs(float64(best.cells)) {
			best, found = m, true
		}
	}
	return best, found
}

// moveFor says how to move one split's divider. Each side should hold the share
// of the region that matches how many panes it holds -- one pane of four takes
// a quarter -- which is what leaves every pane with 1/N of the tab.
func moveFor(s paneSplit, boxes []paneBox) (tileMove, bool) {
	if s.Rect.Width <= 0 || s.Rect.Height <= 0 {
		return tileMove{}, false
	}
	down := s.Direction == "down"
	dim, origin := s.Rect.Width, s.Rect.X
	if down {
		dim, origin = s.Rect.Height, s.Rect.Y
	}

	first, second, divider := sidesOf(s, boxes, down)
	if len(first) == 0 || len(second) == 0 {
		return tileMove{}, false
	}

	want := float64(len(first)) / float64(len(first)+len(second))
	ideal := origin + int(math.Round(want*float64(dim)))
	m := tileMove{cells: ideal - divider}
	if m.cells == 0 {
		return tileMove{}, false
	}
	m.amount = math.Abs(float64(m.cells)) / float64(dim)

	// Name a pane that sits against the divider, on the side that is to gain:
	// herdr moves the split nearest the named pane in the named direction, so
	// a pane one split away would move the wrong divider.
	if m.cells > 0 {
		m.pane, m.direction = touching(first, down, true), "right"
		if down {
			m.direction = "down"
		}
	} else {
		m.pane, m.direction = touching(second, down, false), "left"
		if down {
			m.direction = "up"
		}
	}
	if m.pane == "" {
		return tileMove{}, false
	}
	return m, true
}

// sidesOf splits the panes inside a split's region by which side of its divider
// they lie on. A pane lies wholly on one side, so the edges against the divider
// give the divider's real position: herdr rounds the ratio into cells as it
// lays panes out, and that rounding is one cell of drift either way.
func sidesOf(s paneSplit, boxes []paneBox, down bool) (first, second []paneBox, divider int) {
	divider = s.Rect.X + int(math.Round(s.Ratio*float64(s.Rect.Width)))
	if down {
		divider = s.Rect.Y + int(math.Round(s.Ratio*float64(s.Rect.Height)))
	}
	for _, b := range boxes {
		if !inside(s.Rect, b.Rect) {
			continue
		}
		if start(b, down)+extent(b, down)/2 < divider {
			first = append(first, b)
		} else {
			second = append(second, b)
		}
	}
	for _, b := range first {
		if far := start(b, down) + extent(b, down); far > divider {
			divider = far
		}
	}
	return first, second, divider
}

// start and extent give a pane's near edge and length along an axis: the rows
// it spans for a split that runs down, its columns for one that runs right.
func start(b paneBox, down bool) int {
	if down {
		return b.Rect.Y
	}
	return b.Rect.X
}

func extent(b paneBox, down bool) int {
	if down {
		return b.Rect.Height
	}
	return b.Rect.Width
}

// touching names the pane on one side of a divider that rests against it: the
// furthest edge on the first side, the nearest one on the second.
func touching(side []paneBox, down, first bool) string {
	name := ""
	best := 0
	for _, b := range side {
		edge := start(b, down)
		if first {
			edge += extent(b, down)
		}
		if name == "" || (first && edge > best) || (!first && edge < best) {
			name, best = b.PaneID, edge
		}
	}
	return name
}

// inside says whether pane lies wholly in region.
func inside(region, pane paneRect) bool {
	return pane.X >= region.X && pane.Y >= region.Y &&
		pane.X+pane.Width <= region.X+region.Width &&
		pane.Y+pane.Height <= region.Y+region.Height
}

// evenShare says whether every pane holds its share of the tab.
func evenShare(boxes []paneBox, target, tol float64) bool {
	for _, b := range boxes {
		if math.Abs(float64(b.Rect.Width*b.Rect.Height)-target) > tol {
			return false
		}
	}
	return true
}
