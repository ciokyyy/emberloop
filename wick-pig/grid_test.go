package wick

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGrid is a herdr workspace of tabs, each a split-tree fakeTab, that answers
// pane list, pane split, pane close, pane layout, pane resize, tab list and tab
// create. It is the tiling fake with a workspace around it, so a grid test can
// watch a whole grid of panes appear, overflow into a labelled tab and take a
// pane's slot when one dies.
type fakeGrid struct {
	mu    sync.Mutex
	tabs  []*fakeTab
	label map[string]string // tab id -> label
	next  int

	// delay holds every split open for this long, so a test can tell an
	// overlapping pair of splits from a serialized one.
	delay       time.Duration
	splitActive int
	maxSplit    int
	calls       []string
}

func newFakeGrid() *fakeGrid {
	resetGrid()
	g := &fakeGrid{label: map[string]string{}}
	g.newTab("1")
	return g
}

// resetGrid forgets the slots of every earlier test: the registry is
// process-wide, and the fakes reuse tab and pane ids.
func resetGrid() {
	gridPlacedMu.Lock()
	gridPlaced = map[string]map[int]string{}
	gridPlacedMu.Unlock()
}

// newTab adds a tab with one pane, the caller's, and returns it.
func (g *fakeGrid) newTab(label string) *fakeTab {
	id := fmt.Sprintf("w1:t%d", len(g.tabs)+1)
	root := g.paneID()
	t := &fakeTab{tab: id, area: paneRect{Width: 100, Height: 100},
		root: leaf(root), order: []string{root}, name: g.paneID}
	g.tabs = append(g.tabs, t)
	g.label[id] = label
	return t
}

func (g *fakeGrid) paneID() string {
	g.next++
	return fmt.Sprintf("w1:p%d", g.next)
}

func (g *fakeGrid) tabWith(pane string) *fakeTab {
	for _, t := range g.tabs {
		t.hydrate()
		for _, id := range t.order {
			if id == pane {
				return t
			}
		}
	}
	return nil
}

// panes is every pane of every tab, in tab then creation order.
func (g *fakeGrid) panes() []any {
	out := []any{}
	for _, t := range g.tabs {
		t.hydrate()
		for _, id := range t.order {
			out = append(out, map[string]any{"pane_id": id, "tab_id": t.tab, "workspace_id": "w1"})
		}
	}
	return out
}

func argValue(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func (g *fakeGrid) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	if cmd != herdrBin() || len(args) < 2 {
		return Result{Stdout: "{}"}, nil
	}
	g.mu.Lock()
	g.calls = append(g.calls, strings.Join(append([]string{cmd}, args...), " "))
	g.mu.Unlock()
	switch args[0] {
	case "pane":
		switch args[1] {
		case "list":
			g.mu.Lock()
			defer g.mu.Unlock()
			return jsonReply(map[string]any{"panes": g.panes()}), nil
		case "split":
			return g.splitCall(args)
		case "close":
			g.mu.Lock()
			defer g.mu.Unlock()
			if t := g.tabWith(args[2]); t != nil {
				t.close(args[2])
			}
			return Result{Stdout: `{"result":{}}`}, nil
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if t := g.tabWith(argValue(args, "--pane")); t != nil {
			return t.run(cmd, args, ExecOpts{})
		}
		return Result{Stdout: "{}"}, nil
	case "tab":
		g.mu.Lock()
		defer g.mu.Unlock()
		switch args[1] {
		case "list":
			tabs := []any{}
			for _, t := range g.tabs {
				tabs = append(tabs, map[string]any{"tab_id": t.tab, "label": g.label[t.tab], "pane_count": len(t.order)})
			}
			return jsonReply(map[string]any{"tabs": tabs}), nil
		case "create":
			t := g.newTab(argValue(args, "--label"))
			return jsonReply(map[string]any{
				"tab":       map[string]any{"tab_id": t.tab},
				"root_pane": map[string]any{"pane_id": t.order[0], "tab_id": t.tab},
			}), nil
		}
	}
	return Result{Stdout: "{}"}, nil
}

// splitCall opens the split, records whether another was in flight, and only
// then mutates the tree, so overlapping splits show up as maxSplit > 1.
func (g *fakeGrid) splitCall(args []string) (Result, error) {
	pane := argValue(args, "--pane")
	g.mu.Lock()
	g.splitActive++
	if g.splitActive > g.maxSplit {
		g.maxSplit = g.splitActive
	}
	t := g.tabWith(pane)
	d := g.delay
	g.mu.Unlock()

	if d > 0 {
		time.Sleep(d)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.splitActive--
	if t == nil {
		return Result{Code: 1, Stderr: "no pane " + pane}, nil
	}
	id, err := t.split(pane, argValue(args, "--direction"), parseRatio(args))
	if err != nil {
		return Result{Code: 1, Stderr: err.Error()}, nil
	}
	return jsonReply(map[string]any{"pane": map[string]any{"pane_id": id, "tab_id": t.tab}}), nil
}

func parseRatio(args []string) float64 {
	var r float64
	_, _ = fmt.Sscanf(argValue(args, "--ratio"), "%g", &r)
	return r
}

// rowsOf reads a tab's panes in reading order (top to bottom, left to right)
// and returns their ids.
func rowsOf(t *fakeTab) []string {
	boxes := t.boxes()
	sort.SliceStable(boxes, func(i, j int) bool {
		if boxes[i].Rect.Y != boxes[j].Rect.Y {
			return boxes[i].Rect.Y < boxes[j].Rect.Y
		}
		return boxes[i].Rect.X < boxes[j].Rect.X
	})
	ids := make([]string, len(boxes))
	for i, b := range boxes {
		ids[i] = b.PaneID
	}
	return ids
}

func equalSizes(t *testing.T, tab *fakeTab) {
	t.Helper()
	boxes := tab.boxes()
	total := float64(tab.area.Width * tab.area.Height)
	want := total / float64(len(boxes))
	for _, b := range boxes {
		got := float64(b.Rect.Width * b.Rect.Height)
		if math.Abs(got-want) > tileTolerance*total {
			t.Errorf("pane %s holds %.0f cells, want %.0f", b.PaneID, got, want)
		}
	}
}

// Nine panes in one tab sit in rows of three, in reading order, with the
// caller as pane 1 and every pane the same size.
func TestGridFillsRowsOfThree(t *testing.T) {
	g := newFakeGrid()
	tab := g.tabs[0]
	for i := 0; i < 8; i++ {
		if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
			t.Fatalf("placement %d: %v", i+2, err)
		}
	}
	if len(tab.boxes()) != 9 {
		t.Fatalf("tab holds %d panes, want 9", len(tab.boxes()))
	}
	want := []string{"w1:p1", "w1:p2", "w1:p3", "w1:p4", "w1:p5", "w1:p6", "w1:p7", "w1:p8", "w1:p9"}
	if got := rowsOf(tab); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("reading order = %v, want %v", got, want)
	}
	// Three columns each three deep: a single vertical stack would also read
	// 1..9, so count the columns too.
	cols, rows := map[int]int{}, map[int]int{}
	for _, b := range tab.boxes() {
		cols[b.Rect.X]++
		rows[b.Rect.Y]++
	}
	if len(cols) != 3 || len(rows) != 3 {
		t.Errorf("the grid is %d columns by %d rows, want 3 by 3", len(cols), len(rows))
	}
	for x, n := range cols {
		if n != 3 {
			t.Errorf("column at x=%d holds %d panes, want 3", x, n)
		}
	}
	equalSizes(t, tab)
}

// The pane after the ninth opens a tab labelled for the overflow, in the same
// workspace, and further panes fill that tab by the same grid rules.
func TestGridOverflowsIntoALabelledTab(t *testing.T) {
	g := newFakeGrid()
	tab := g.tabs[0]
	for i := 0; i < 8; i++ {
		if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
			t.Fatalf("placement %d: %v", i+2, err)
		}
	}
	if len(tab.boxes()) != 9 {
		t.Fatalf("the caller's tab holds %d panes, want 9", len(tab.boxes()))
	}

	first, err := placeInGrid(g.run, tab.tab, "scouts", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if first.Tab == tab.tab {
		t.Fatalf("the tenth pane landed in the caller's tab")
	}
	if g.label[first.Tab] != "scouts" {
		t.Errorf("overflow tab label = %q, want scouts", g.label[first.Tab])
	}
	over := g.tabWith(first.Pane)
	if over == nil || over.tab != first.Tab {
		t.Fatalf("overflow pane %s is not in tab %s", first.Pane, first.Tab)
	}
	if len(over.boxes()) != 1 {
		t.Fatalf("the first overflow pane is not alone in its tab: %d panes", len(over.boxes()))
	}

	second, err := placeInGrid(g.run, tab.tab, "scouts", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if second.Tab != first.Tab {
		t.Errorf("the next pane opened a new tab %s, want %s", second.Tab, first.Tab)
	}
	if len(over.boxes()) != 2 {
		t.Fatalf("the overflow tab holds %d panes, want 2", len(over.boxes()))
	}
	if got := rowsOf(over); got[0] != first.Pane || got[1] != second.Pane {
		t.Errorf("overflow reading order = %v, want %s then %s", got, first.Pane, second.Pane)
	}
	equalSizes(t, over)
}

func rects(tab *fakeTab) map[string]paneRect {
	out := map[string]paneRect{}
	for _, b := range tab.boxes() {
		out[b.PaneID] = b.Rect
	}
	return out
}

func closeRect(a, b paneRect) bool {
	return abs(a.X-b.X) <= 1 && abs(a.Y-b.Y) <= 1 &&
		abs(a.Width-b.Width) <= 1 && abs(a.Height-b.Height) <= 1
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// A replacement goes into the slot the dead pane left, so the rest of the grid
// keeps its positions and only the sizes are evened out.
func TestGridReplacesInTheDeadPaneSlot(t *testing.T) {
	g := newFakeGrid()
	tab := g.tabs[0]
	for i := 0; i < 4; i++ {
		if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
			t.Fatalf("placement %d: %v", i+2, err)
		}
	}
	before := rects(tab)
	dead := tab.order[3] // slot 4

	if _, err := g.run(herdrBin(), []string{"pane", "close", dead}, ExecOpts{}); err != nil {
		t.Fatal(err)
	}
	spot, err := replaceInGrid(g.run, tab.tab, 4, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if spot.Slot != 4 {
		t.Errorf("replacement slot = %d, want 4", spot.Slot)
	}

	after := rects(tab)
	if _, gone := after[dead]; gone {
		t.Errorf("the dead pane %s is still in the layout", dead)
	}
	if !closeRect(after[spot.Pane], before[dead]) {
		t.Errorf("replacement %s sits at %+v, the dead pane held %+v", spot.Pane, after[spot.Pane], before[dead])
	}
	for pane, r := range before {
		if pane == dead {
			continue
		}
		if !closeRect(after[pane], r) {
			t.Errorf("pane %s moved from %+v to %+v", pane, r, after[pane])
		}
	}
	if got, want := rowsOf(tab), []string{"w1:p1", "w1:p2", "w1:p3", spot.Pane, "w1:p5"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("reading order = %v, want %v", got, want)
	}
}

// Splits in different tabs run at the same time; splits in one tab are
// serialized so a second launch never splits a half-built layout.
func TestGridSplitsDifferentTabsAtOnceAndOneTabInTurn(t *testing.T) {
	g := newFakeGrid()
	g.newTab("2")
	g.delay = 40 * time.Millisecond

	var wg sync.WaitGroup
	for _, tab := range g.tabs {
		wg.Add(1)
		go func(tab *fakeTab) {
			defer wg.Done()
			if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
				t.Errorf("placement in %s: %v", tab.tab, err)
			}
		}(tab)
	}
	wg.Wait()
	if g.maxSplit < 2 {
		t.Errorf("splits in two tabs did not overlap: %d at once", g.maxSplit)
	}

	one := newFakeGrid()
	one.delay = 40 * time.Millisecond
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := placeInGrid(one.run, one.tabs[0].tab, "scouts", "/repo"); err != nil {
				t.Errorf("placement in one tab: %v", err)
			}
		}()
	}
	wg.Wait()
	if one.maxSplit != 1 {
		t.Errorf("splits in one tab overlapped: %d at once", one.maxSplit)
	}
	if len(one.tabs[0].boxes()) != 3 {
		t.Errorf("one tab holds %d panes after two placements, want 3", len(one.tabs[0].boxes()))
	}
}

// Tiling is cosmetic: a tab that will not even out still takes the pane, and
// the placement succeeds.
func TestGridPlacementSurvivesATabThatWillNotEven(t *testing.T) {
	resetGrid()
	// Two unequal panes with no divider between them: tilePanes has nothing to
	// move, so it returns an error the grid must swallow.
	static := `{"result":{"panes":[{"pane_id":"w1:p1","tab_id":"w1:t1"},` +
		`{"pane_id":"w1:p2","tab_id":"w1:t1"}]}}`
	layout := `{"result":{"layout":{"area":{"width":100,"height":100},"tab_id":"w1:t1",` +
		`"panes":[{"pane_id":"w1:p1","rect":{"x":0,"y":0,"width":80,"height":100}},` +
		`{"pane_id":"w1:p2","rect":{"x":80,"y":0,"width":20,"height":100}}],"splits":[]}}}`
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		switch {
		case args[0] == "pane" && args[1] == "split":
			return Result{Stdout: `{"result":{"pane":{"pane_id":"w1:p3","tab_id":"w1:t1"}}}`}, nil
		case args[0] == "pane" && args[1] == "list":
			return Result{Stdout: static}, nil
		case args[0] == "pane" && args[1] == "layout":
			return Result{Stdout: layout}, nil
		}
		return Result{Stdout: "{}"}, nil
	}
	spot, err := placeInGrid(run, "w1:t1", "scouts", "/repo")
	if err != nil {
		t.Fatalf("an uneven tab failed the placement: %v", err)
	}
	if spot.Pane != "w1:p3" {
		t.Errorf("pane = %q, want the split pane", spot.Pane)
	}
}

// A replacement after an earlier one still lands in the dead pane's slot: the
// grid orders panes by where they sit, not by the order herdr made them, which
// an earlier replacement has already disturbed.
func TestGridReplacesAfterAnEarlierReplacement(t *testing.T) {
	g := newFakeGrid()
	tab := g.tabs[0]
	for i := 0; i < 8; i++ {
		if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
			t.Fatalf("placement %d: %v", i+2, err)
		}
	}
	order := rowsOf(tab)
	dead4, dead7 := order[3], order[6]

	if _, err := g.run(herdrBin(), []string{"pane", "close", dead4}, ExecOpts{}); err != nil {
		t.Fatal(err)
	}
	first, err := replaceInGrid(g.run, tab.tab, 4, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	before7 := rects(tab)[dead7]

	if _, err := g.run(herdrBin(), []string{"pane", "close", dead7}, ExecOpts{}); err != nil {
		t.Fatal(err)
	}
	second, err := replaceInGrid(g.run, tab.tab, 7, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !closeRect(rects(tab)[second.Pane], before7) {
		t.Errorf("the second replacement sits at %+v, the dead slot held %+v", rects(tab)[second.Pane], before7)
	}
	want := []string{order[0], order[1], order[2], first.Pane, order[4], order[5], second.Pane, order[7], order[8]}
	if got := rowsOf(tab); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("reading order = %v, want %v", got, want)
	}
	equalSizes(t, tab)
}

// A read-only pane that finished frees its slot, and the next agent takes it
// rather than a slot that is still occupied.
func TestGridReusesAFreedSlot(t *testing.T) {
	g := newFakeGrid()
	tab := g.tabs[0]
	caller := tab.order[0]
	first, err := placeInGrid(g.run, tab.tab, "scouts", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.run(herdrBin(), []string{"pane", "close", first.Pane}, ExecOpts{}); err != nil {
		t.Fatal(err)
	}
	second, err := placeInGrid(g.run, tab.tab, "scouts", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if second.Slot != 2 {
		t.Errorf("the next agent took slot %d, want the freed slot 2", second.Slot)
	}
	if got, want := rowsOf(tab), []string{caller, second.Pane}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("reading order = %v, want %v", got, want)
	}
	equalSizes(t, tab)
}

// An overflow tab whose root pane died still takes the next agent: the grid
// skips the root slot it cannot split and grows from a pane that is still
// there, rather than failing while the caller's tab stays full.
func TestGridPlacesAfterAnOverflowRootDies(t *testing.T) {
	g := newFakeGrid()
	tab := g.tabs[0]
	for i := 0; i < 8; i++ {
		if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
			t.Fatalf("placement %d: %v", i+2, err)
		}
	}
	root, err := placeInGrid(g.run, tab.tab, "scouts", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := placeInGrid(g.run, tab.tab, "scouts", "/repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.run(herdrBin(), []string{"pane", "close", root.Pane}, ExecOpts{}); err != nil {
		t.Fatal(err)
	}

	spot, err := placeInGrid(g.run, tab.tab, "scouts", "/repo")
	if err != nil {
		t.Fatalf("the next overflow launch failed: %v", err)
	}
	if spot.Tab != root.Tab {
		t.Errorf("the pane landed in %s, want the overflow tab %s", spot.Tab, root.Tab)
	}
}
