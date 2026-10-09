package el

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// fakeTab is a herdr tab of panes laid out as a tree of splits, which is what
// `pane layout` reports. A node is a leaf (a pane) or a split: the direction it
// runs, the share of its region its first side keeps, and the two sides. The
// common case -- the stack the reviewer splits leave behind, or a row -- is the
// cascade newFakeTab builds; newFakeTree is for the mixed down/right layouts a
// split of the first pane produces.
//
// It answers pane list, pane layout and pane resize the way herdr does: a
// pane's rectangle is the region left once every split above it has taken its
// share, and a resize moves the ratio of the split nearest the named pane in
// the named direction by the amount given.
//
// The arithmetic is herdr's, measured on a scratch pane, not tilePanes': a
// 229-column pane split at 0.3 kept 69 columns, a 70-row pane split at 0.25
// kept 18 rows, and a resize of 0.007 on a 70-row region moved a ratio of 0.53
// to 0.537 and its divider from 37 rows to 38.
type fakeTab struct {
	tab   string
	area  paneRect
	root  *fakeNode
	calls []string
	// order is the panes in the order herdr created them, which is the order
	// pane list reports and the grid fills. next is the counter new panes are
	// named from, and name overrides it when a workspace hands out the ids.
	order []string
	next  int
	name  func() string
}

type fakeNode struct {
	pane   string // set on a leaf
	dir    string // "down" for a stack, "right" for a row
	ratio  float64
	first  *fakeNode // keeps ratio of the region
	second *fakeNode // gets the rest
}

func leaf(id string) *fakeNode { return &fakeNode{pane: id} }

func (n *fakeNode) isLeaf() bool { return n.pane != "" }

// newFakeTab builds the stack or row an N-pane cascade leaves behind: each
// split takes its share and hands the rest on, so pane i is the first child of
// split i. A one-axis tab made one split at a time is exactly this shape.
func newFakeTab(axis string, ratios []float64) *fakeTab {
	root := leaf(fmt.Sprintf("p%d", len(ratios)))
	for i := len(ratios) - 1; i >= 0; i-- {
		root = &fakeNode{dir: axis, ratio: ratios[i], first: leaf(fmt.Sprintf("p%d", i)), second: root}
	}
	f := &fakeTab{tab: "w1:t1", area: paneRect{Width: 100, Height: 100}, root: root}
	for i := 0; i <= len(ratios); i++ {
		f.order = append(f.order, fmt.Sprintf("p%d", i))
	}
	f.next = len(ratios) + 1
	return f
}

// hydrate fills order from the tree, for a hand-built fake whose panes were
// never created by a split.
func (f *fakeTab) hydrate() {
	if f.order != nil {
		return
	}
	for _, b := range f.boxes() {
		f.order = append(f.order, b.PaneID)
	}
	f.next = len(f.order)
}

// split replaces pane with a split of it: the pane keeps the first side of
// dir and a new pane takes the second, exactly as herdr splits one pane. The
// new pane's id is one the fake has not handed out.
func (f *fakeTab) split(pane, dir string, ratio float64) (string, error) {
	f.hydrate()
	id := fmt.Sprintf("p%d", f.next)
	if f.name != nil {
		id = f.name()
	} else {
		f.next++
	}
	repl := &fakeNode{dir: dir, ratio: ratio, first: leaf(pane), second: leaf(id)}
	if f.root.isLeaf() && f.root.pane == pane {
		f.root = repl
	} else if !replaceLeaf(f.root, pane, repl) {
		return "", fmt.Errorf("no pane %s in %s", pane, f.tab)
	}
	f.order = append(f.order, id)
	return id, nil
}

// replaceLeaf swaps the leaf named pane for repl somewhere under n.
func replaceLeaf(n *fakeNode, pane string, repl *fakeNode) bool {
	if n.isLeaf() {
		return false
	}
	if n.first.isLeaf() && n.first.pane == pane {
		n.first = repl
		return true
	}
	if n.second.isLeaf() && n.second.pane == pane {
		n.second = repl
		return true
	}
	return replaceLeaf(n.first, pane, repl) || replaceLeaf(n.second, pane, repl)
}

// close removes a pane the way herdr does: its split collapses and the other
// side keeps the whole region.
func (f *fakeTab) close(pane string) {
	f.root = collapse(f.root, pane)
	for i, id := range f.order {
		if id == pane {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
}

func collapse(n *fakeNode, pane string) *fakeNode {
	if n == nil || n.isLeaf() {
		return n
	}
	if n.first.isLeaf() && n.first.pane == pane {
		return n.second
	}
	if n.second.isLeaf() && n.second.pane == pane {
		return n.first
	}
	n.first = collapse(n.first, pane)
	n.second = collapse(n.second, pane)
	return n
}

// newFakeTree wraps a hand-built node, for a layout that is not one axis.
func newFakeTree(root *fakeNode) *fakeTab {
	return &fakeTab{tab: "w1:t1", area: paneRect{Width: 100, Height: 100}, root: root}
}

// divide cuts a region in two the way herdr does: the first side keeps ratio of
// the axis the split runs along, rounded to cells.
func divide(r paneRect, dir string, ratio float64) (paneRect, paneRect) {
	if dir == "down" {
		h := int(math.Round(ratio * float64(r.Height)))
		return paneRect{X: r.X, Y: r.Y, Width: r.Width, Height: h},
			paneRect{X: r.X, Y: r.Y + h, Width: r.Width, Height: r.Height - h}
	}
	w := int(math.Round(ratio * float64(r.Width)))
	return paneRect{X: r.X, Y: r.Y, Width: w, Height: r.Height},
		paneRect{X: r.X + w, Y: r.Y, Width: r.Width - w, Height: r.Height}
}

func rectMap(p paneRect) map[string]any {
	return map[string]any{"x": p.X, "y": p.Y, "width": p.Width, "height": p.Height}
}

func (f *fakeTab) id(i int) string { return fmt.Sprintf("p%d", i) }

// boxes is every pane and the rectangle it holds, in layout order.
func (f *fakeTab) boxes() []paneBox {
	var out []paneBox
	var walk func(*fakeNode, paneRect)
	walk = func(n *fakeNode, r paneRect) {
		if n.isLeaf() {
			out = append(out, paneBox{PaneID: n.pane, Rect: r})
			return
		}
		first, second := divide(r, n.dir, n.ratio)
		walk(n.first, first)
		walk(n.second, second)
	}
	walk(f.root, f.area)
	return out
}

// shares is each pane's share of the tab, for the evenness check.
func (f *fakeTab) shares() []float64 {
	boxes := f.boxes()
	total := float64(f.area.Width * f.area.Height)
	out := make([]float64, len(boxes))
	for i, b := range boxes {
		out[i] = float64(b.Rect.Width*b.Rect.Height) / total
	}
	return out
}

func (f *fakeTab) layout() map[string]any {
	panes := []any{}
	splits := []any{}
	var walk func(*fakeNode, paneRect, string)
	walk = func(n *fakeNode, r paneRect, id string) {
		if n.isLeaf() {
			panes = append(panes, map[string]any{"pane_id": n.pane, "rect": rectMap(r)})
			return
		}
		splits = append(splits, map[string]any{
			"id": id, "direction": n.dir, "ratio": n.ratio, "rect": rectMap(r),
		})
		first, second := divide(r, n.dir, n.ratio)
		walk(n.first, first, id+"_a")
		walk(n.second, second, id+"_b")
	}
	walk(f.root, f.area, "split_root")
	return map[string]any{"area": rectMap(f.area), "tab_id": f.tab, "panes": panes, "splits": splits}
}

// resize moves the divider nearest the named pane and records the call. herdr
// aims at the split nearest the pane in the named direction: down or right is
// a split the pane is the first side of, up or left one it is the second side
// of.
func (f *fakeTab) resize(pane, direction string, amount float64) {
	f.calls = append(f.calls, fmt.Sprintf("%s %s %.4f", pane, direction, amount))
	f.adjust(f.root, pane, direction, amount)
	f.clamp(f.root)
}

func (f *fakeTab) adjust(n *fakeNode, target, direction string, amount float64) bool {
	if n.isLeaf() {
		return false
	}
	first, second := holds(n.first, target), holds(n.second, target)
	if !first && !second {
		return false
	}
	if f.adjust(n.first, target, direction, amount) || f.adjust(n.second, target, direction, amount) {
		return true
	}
	if axis(direction) != n.dir {
		return false
	}
	// The nearest split on the pane's path wins, so this is only reached when
	// neither side below already handled the move.
	if first && forward(direction) {
		n.ratio += amount
		return true
	}
	if second && !forward(direction) {
		n.ratio -= amount
		return true
	}
	return false
}

func (f *fakeTab) clamp(n *fakeNode) {
	if n.isLeaf() {
		return
	}
	n.ratio = math.Max(0.05, math.Min(0.95, n.ratio))
	f.clamp(n.first)
	f.clamp(n.second)
}

func holds(n *fakeNode, target string) bool {
	if n.isLeaf() {
		return n.pane == target
	}
	return holds(n.first, target) || holds(n.second, target)
}

func axis(direction string) string {
	if direction == "up" || direction == "down" {
		return "down"
	}
	return "right"
}

func forward(direction string) bool { return direction == "down" || direction == "right" }

func (f *fakeTab) run(_ string, args []string, _ ExecOpts) (Result, error) {
	switch args[1] {
	case "list":
		f.hydrate()
		panes := []any{}
		for _, id := range f.order {
			panes = append(panes, map[string]any{"pane_id": id, "tab_id": f.tab, "workspace_id": "w1"})
		}
		return jsonReply(map[string]any{"panes": panes}), nil
	case "split":
		var pane, direction string
		var ratio float64
		for i := 2; i+1 < len(args); i += 2 {
			switch args[i] {
			case "--pane":
				pane = args[i+1]
			case "--direction":
				direction = args[i+1]
			case "--ratio":
				_, _ = fmt.Sscanf(args[i+1], "%g", &ratio)
			}
		}
		id, err := f.split(pane, direction, ratio)
		if err != nil {
			return Result{Code: 1, Stderr: err.Error()}, nil
		}
		return jsonReply(map[string]any{"pane": map[string]any{"pane_id": id, "tab_id": f.tab}}), nil
	case "close":
		f.close(args[2])
		return Result{Stdout: `{"result":{}}`}, nil
	case "layout":
		return jsonReply(map[string]any{"layout": f.layout()}), nil
	case "resize":
		var pane, direction string
		var amount float64
		for i := 2; i+1 < len(args); i += 2 {
			switch args[i] {
			case "--pane":
				pane = args[i+1]
			case "--direction":
				direction = args[i+1]
			case "--amount":
				_, _ = fmt.Sscanf(args[i+1], "%g", &amount)
			}
		}
		f.resize(pane, direction, amount)
		return Result{Stdout: `{"result":{"resize":{"changed":true}}}`}, nil
	}
	return Result{Stdout: "{}"}, nil
}

func jsonReply(result map[string]any) Result {
	b, err := json.Marshal(map[string]any{"id": "cli:pane", "result": result})
	if err != nil {
		panic(err)
	}
	return Result{Stdout: string(b)}
}

// A tab gets evened out whichever way it was split: every pane ends with 1/N of
// it, and the resizes that got it there are the ones herdr needs, aimed at the
// pane touching each divider.
func TestTilePanesEvensATab(t *testing.T) {
	cases := []struct {
		name      string
		axis      string
		ratios    []float64
		wantCalls []string
	}{
		{"a stack of two from a 0.3 split", "down", []float64{0.3}, []string{"p0 down 0.2000"}},
		{"a stack of three from two halvings", "down", []float64{0.5, 0.5}, []string{"p1 up 0.1700"}},
		{"a stack of four from three halvings", "down", []float64{0.5, 0.5, 0.5}, []string{"p1 up 0.2500", "p2 up 0.1733"}},
		{"a row of three from two halvings", "right", []float64{0.5, 0.5}, []string{"p1 left 0.1700"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := newFakeTab(c.axis, c.ratios)
			if err := tilePanes(tab.run, tab.tab); err != nil {
				t.Fatalf("tilePanes: %v", err)
			}
			if got := strings.Join(tab.calls, ", "); got != strings.Join(c.wantCalls, ", ") {
				t.Errorf("resize calls = [%s], want [%s]", got, strings.Join(c.wantCalls, ", "))
			}
			// The expected shares come from the call list the case states, not
			// from recomputing what tilePanes does.
			assertEven(t, tab)
		})
	}
}

// A layout that is not one axis is tiled too: here the first pane was split
// again, so the left side holds two panes and the right side one, and the root
// divider has to move to give each half its third.
func TestTilePanesEvensAMixedTree(t *testing.T) {
	tab := newFakeTree(&fakeNode{
		dir: "right", ratio: 0.5,
		first:  &fakeNode{dir: "down", ratio: 0.5, first: leaf("p0"), second: leaf("p1")},
		second: leaf("p2"),
	})
	if err := tilePanes(tab.run, tab.tab); err != nil {
		t.Fatalf("tilePanes: %v", err)
	}
	if got := strings.Join(tab.calls, ", "); got != "p0 right 0.1700" {
		t.Errorf("resize calls = [%s], want [p0 right 0.1700]", got)
	}
	assertEven(t, tab)
}

// A read-only tab that herdr reports as uneven but holds no split to move
// leaves tilePanes with nothing to do, which is an error the caller must see.
func TestTilePanesReportsATabItCannotMove(t *testing.T) {
	tab := "w1:t1"
	boxes := []paneBox{
		{PaneID: "p0", Rect: paneRect{X: 0, Y: 0, Width: 80, Height: 100}},
		{PaneID: "p1", Rect: paneRect{X: 80, Y: 0, Width: 20, Height: 100}},
	}
	err := tilePanes(staticHerdr(tab, boxes, nil), tab)
	if err == nil {
		t.Fatal("tilePanes returned nil for a tab with no divider to move")
	}
	if !strings.Contains(err.Error(), "no divider can move") {
		t.Errorf("error = %q, want the no-divider case", err)
	}
}

// A tab whose divider will not stay put after a resize is given a bounded
// number of rounds and then surrendered rather than looped over for ever.
func TestTilePanesReportsATabThatNeverEvens(t *testing.T) {
	tab := "w1:t1"
	boxes := []paneBox{
		{PaneID: "p0", Rect: paneRect{X: 0, Y: 0, Width: 80, Height: 100}},
		{PaneID: "p1", Rect: paneRect{X: 80, Y: 0, Width: 20, Height: 100}},
	}
	splits := []paneSplit{{Direction: "right", Ratio: 0.8, Rect: paneRect{Width: 100, Height: 100}}}
	err := tilePanes(staticHerdr(tab, boxes, splits), tab)
	if err == nil {
		t.Fatal("tilePanes returned nil for a tab that never evens")
	}
	if !strings.Contains(err.Error(), "never evened out") {
		t.Errorf("error = %q, want the never-evened case", err)
	}
}

// With one pane there is nothing to share, so nothing is asked of herdr.
func TestTilePanesLeavesASinglePaneAlone(t *testing.T) {
	tab := newFakeTab("down", nil)
	if err := tilePanes(tab.run, tab.tab); err != nil {
		t.Fatal(err)
	}
	if len(tab.calls) != 0 {
		t.Errorf("a one-pane tab was resized: %v", tab.calls)
	}
}

func assertEven(t *testing.T, tab *fakeTab) {
	t.Helper()
	shares := tab.shares()
	want := 1 / float64(len(shares))
	for i, share := range shares {
		if math.Abs(share-want) > tileTolerance {
			t.Errorf("pane %d holds share %.3f, want %.3f", i, share, want)
		}
	}
}

// staticHerdr answers pane list and pane layout from a fixed table and treats
// every resize as a no-op, so a layout that cannot be evened stays as it is.
func staticHerdr(tab string, boxes []paneBox, splits []paneSplit) Exec {
	panes := []any{}
	for _, b := range boxes {
		panes = append(panes, map[string]any{"pane_id": b.PaneID, "tab_id": tab, "workspace_id": "w1"})
	}
	layout := map[string]any{
		"area": rectMap(paneRect{Width: 100, Height: 100}), "tab_id": tab,
		"panes": []any{}, "splits": []any{},
	}
	for _, b := range boxes {
		layout["panes"] = append(layout["panes"].([]any), map[string]any{"pane_id": b.PaneID, "rect": rectMap(b.Rect)})
	}
	for _, s := range splits {
		layout["splits"] = append(layout["splits"].([]any), map[string]any{
			"id": "split", "direction": s.Direction, "ratio": s.Ratio, "rect": rectMap(s.Rect),
		})
	}
	return func(_ string, args []string, _ ExecOpts) (Result, error) {
		switch args[1] {
		case "list":
			return jsonReply(map[string]any{"panes": panes}), nil
		case "layout":
			return jsonReply(map[string]any{"layout": layout}), nil
		}
		return Result{Stdout: `{"result":{}}`}, nil
	}
}
