package wick

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// fakeTab is a herdr tab of panes laid end to end along one axis -- the stack
// the reviewer splits leave behind, or a row -- which is all a tab of splits
// made one at a time ever is. It answers pane list, pane layout and pane
// resize the way herdr does: the tab is a tree of ratios, a pane's size along
// the axis is the ratio applied to the region it sits in, and a resize moves
// the ratio of the split nearest the named pane in the named direction by the
// amount given.
//
// The arithmetic is herdr's, measured on a scratch pane, not tilePanes': a
// 229-column pane split at 0.3 kept 69 columns, a 70-row pane split at 0.25
// kept 18 rows, and a resize of 0.007 on a 70-row region moved a ratio of 0.53
// to 0.537 and its divider from 37 rows to 38.
type fakeTab struct {
	tab    string
	axis   string // "down" for a stack, "right" for a row
	size   int    // cells along the axis
	cross  int    // cells across it
	ratios []float64
	calls  []string
}

func newFakeTab(axis string, ratios []float64) *fakeTab {
	return &fakeTab{tab: "w1:t1", axis: axis, size: 100, cross: 100, ratios: ratios}
}

func (f *fakeTab) id(i int) string { return fmt.Sprintf("p%d", i) }

func (f *fakeTab) down() bool { return f.axis == "down" }

// cells is each pane's size along the axis, derived from the ratios the way
// herdr derives it: each split holds its share of the region it divides.
func (f *fakeTab) cells() []int {
	cells := make([]int, len(f.ratios)+1)
	at := 0
	for i, r := range f.ratios {
		cells[i] = int(math.Round(r * float64(f.size-at)))
		at += cells[i]
	}
	cells[len(cells)-1] = f.size - at
	return cells
}

// spans is each pane's offset along the axis.
func (f *fakeTab) spans() []int {
	cells, at, out := f.cells(), 0, make([]int, len(f.ratios)+1)
	for i, c := range cells {
		out[i] = at
		at += c
	}
	return out
}

func (f *fakeTab) box(at, length int) map[string]any {
	if f.down() {
		return rect(0, at, f.cross, length)
	}
	return rect(at, 0, length, f.cross)
}

func rect(x, y, w, h int) map[string]any {
	return map[string]any{"x": x, "y": y, "width": w, "height": h}
}

func (f *fakeTab) layout() map[string]any {
	cells, spans := f.cells(), f.spans()
	panes := []any{}
	for i, c := range cells {
		panes = append(panes, map[string]any{"pane_id": f.id(i), "rect": f.box(spans[i], c)})
	}
	splits := []any{}
	for i, r := range f.ratios {
		splits = append(splits, map[string]any{
			"id": fmt.Sprintf("split_%d", i), "direction": f.axis, "ratio": r,
			"rect": f.box(spans[i], f.size-spans[i]),
		})
	}
	area := rect(0, 0, f.cross, f.size)
	if !f.down() {
		area = rect(0, 0, f.size, f.cross)
	}
	return map[string]any{"area": area, "tab_id": f.tab, "panes": panes, "splits": splits}
}

// resize moves a divider and records the call. herdr aims at the split nearest
// the named pane in the named direction: down or right is the split after the
// pane, up or left the one before it.
func (f *fakeTab) resize(pane, direction string, amount float64) {
	f.calls = append(f.calls, fmt.Sprintf("%s %s %.4f", pane, direction, amount))
	i := 0
	if _, err := fmt.Sscanf(pane, "p%d", &i); err != nil {
		return
	}
	switch direction {
	case "down", "right":
		if i < len(f.ratios) {
			f.ratios[i] += amount
		}
	case "up", "left":
		if i > 0 {
			f.ratios[i-1] -= amount
		}
	}
	for i, r := range f.ratios {
		f.ratios[i] = math.Max(0.05, math.Min(0.95, r))
	}
}

func (f *fakeTab) run(_ string, args []string, _ ExecOpts) (Result, error) {
	panes := []any{}
	for i := range f.cells() {
		panes = append(panes, map[string]any{"pane_id": f.id(i), "tab_id": f.tab, "workspace_id": "w1"})
	}
	switch args[1] {
	case "list":
		return jsonReply(map[string]any{"panes": panes}), nil
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
			n := float64(len(c.ratios) + 1)
			for i, c := range tab.cells() {
				share := float64(c) / float64(tab.size)
				if math.Abs(share-1/n) > 0.02 {
					t.Errorf("pane %d holds %d of %d cells: share %.3f, want %.3f", i, c, tab.size, share, 1/n)
				}
			}
		})
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
