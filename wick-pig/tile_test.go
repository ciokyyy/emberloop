package wick

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// fakeTab is a herdr tab of panes stacked one above the next, the shape the
// reviewer splits leave behind. It answers pane list, pane layout and pane
// resize the way herdr does: the tab is a tree of ratios, a pane's rows are the
// ratio applied to the region it sits in, and a resize moves the ratio of the
// split nearest the named pane in the named direction by the amount given.
//
// The arithmetic is herdr's, measured on a scratch pane, not tilePanes': a
// 229-column pane split at 0.3 kept 69 columns, a 70-row pane split at 0.25
// kept 18 rows, and a resize of 0.007 on a 70-row region moved a ratio of 0.53
// to 0.537 and its divider from 37 rows to 38.
type fakeTab struct {
	tab    string
	width  int
	height int
	ratios []float64 // one per split: a tab of n panes has n-1 of them
	calls  []string
}

func newFakeTab(tab string, ratios []float64) *fakeTab {
	return &fakeTab{tab: tab, width: 100, height: 100, ratios: ratios}
}

func (f *fakeTab) id(i int) string { return fmt.Sprintf("p%d", i) }

// rows is each pane's height, derived from the ratios the way herdr derives it.
func (f *fakeTab) rows() []int {
	rows := make([]int, len(f.ratios)+1)
	top := 0
	for i, r := range f.ratios {
		rows[i] = int(math.Round(r * float64(f.height-top)))
		top += rows[i]
	}
	rows[len(rows)-1] = f.height - top
	return rows
}

func rect(x, y, w, h int) map[string]any {
	return map[string]any{"x": x, "y": y, "width": w, "height": h}
}

func (f *fakeTab) layout() map[string]any {
	rows := f.rows()
	panes := []any{}
	for i, h := range rows {
		panes = append(panes, map[string]any{"pane_id": f.id(i), "rect": rect(0, f.top(i, rows), f.width, h)})
	}
	splits := []any{}
	top := 0
	for i, r := range f.ratios {
		splits = append(splits, map[string]any{
			"id": fmt.Sprintf("split_%d", i), "direction": "down", "ratio": r,
			"rect": rect(0, top, f.width, f.height-top),
		})
		top += rows[i]
	}
	return map[string]any{"area": rect(0, 0, f.width, f.height), "tab_id": f.tab, "panes": panes, "splits": splits}
}

func (f *fakeTab) top(i int, rows []int) int {
	y := 0
	for _, h := range rows[:i] {
		y += h
	}
	return y
}

// resize moves a divider and records the call. herdr aims at the split nearest
// the named pane in the named direction: down is the split below the pane, up
// the one above it.
func (f *fakeTab) resize(pane, direction string, amount float64) {
	f.calls = append(f.calls, fmt.Sprintf("%s %s %.4f", pane, direction, amount))
	i := 0
	if _, err := fmt.Sscanf(pane, "p%d", &i); err != nil {
		return
	}
	switch direction {
	case "down":
		if i < len(f.ratios) {
			f.ratios[i] += amount
		}
	case "up":
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
	for i := range f.rows() {
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

// A tab reachable from a split gets evened out: every pane ends with 1/N of it,
// and the resizes that got it there are the ones herdr needs, aimed at the pane
// touching each divider.
func TestTilePanesEvensATab(t *testing.T) {
	cases := []struct {
		name      string
		ratios    []float64
		wantCalls []string
	}{
		{"two panes from a 0.3 split", []float64{0.3}, []string{"p0 down 0.2000"}},
		{"three panes from two halvings", []float64{0.5, 0.5}, []string{"p1 up 0.1700"}},
		{"four panes from three halvings", []float64{0.5, 0.5, 0.5}, []string{"p1 up 0.2500", "p2 up 0.1733"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := newFakeTab("w1:t1", c.ratios)
			if err := tilePanes(tab.run, tab.tab); err != nil {
				t.Fatalf("tilePanes: %v", err)
			}
			if got := strings.Join(tab.calls, ", "); got != strings.Join(c.wantCalls, ", ") {
				t.Errorf("resize calls = [%s], want [%s]", got, strings.Join(c.wantCalls, ", "))
			}
			n := float64(len(c.ratios) + 1)
			rows := tab.rows()
			for i, h := range rows {
				share := float64(h) / float64(tab.height)
				if math.Abs(share-1/n) > 0.02 {
					t.Errorf("pane %d holds %d of %d rows: share %.3f, want %.3f", i, h, tab.height, share, 1/n)
				}
			}
		})
	}
}

// With one pane there is nothing to share, so nothing is asked of herdr.
func TestTilePanesLeavesASinglePaneAlone(t *testing.T) {
	tab := newFakeTab("w1:t1", nil)
	if err := tilePanes(tab.run, tab.tab); err != nil {
		t.Fatal(err)
	}
	if len(tab.calls) != 0 {
		t.Errorf("a one-pane tab was resized: %v", tab.calls)
	}
}
