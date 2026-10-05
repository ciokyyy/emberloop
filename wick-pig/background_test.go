package wick

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"example.com/wick/tools"
)

// The background tool lives in the tools package, but wick wires its grid, so
// its placement is tested here, beside the grid fake.

// gridBackground is the background tool wick serves, driven by g: wick installs
// its grid at startup, and the test points that grid and the tool's herdr at
// the fake. Both seams go back when the test ends.
func gridBackground(t *testing.T, g *fakeGrid) tools.Tool {
	t.Helper()
	oldRun, oldGrid := tools.HerdrRun, tools.Grid
	tools.HerdrRun = func(_ context.Context, args ...string) (string, error) {
		res, err := g.run(herdrBin(), args, ExecOpts{})
		if err != nil {
			return "", err
		}
		if res.Code != 0 {
			return res.Stderr, fmt.Errorf("herdr %s exited %d: %s", args[0], res.Code, res.Stderr)
		}
		return res.Stdout, nil
	}
	tools.Grid = backgroundPaneWith(g.run)
	t.Cleanup(func() { tools.HerdrRun, tools.Grid = oldRun, oldGrid })

	for _, tool := range AllTools() {
		if tool.Name() == "background" {
			return tool
		}
	}
	t.Fatal("wick serves no background tool")
	return nil
}

// backgroundStart runs the background tool's start action in root.
func backgroundStart(t *testing.T, tool tools.Tool, root, command string) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"action": "start", "command": command})
	return tool.Execute(context.Background(), root, raw)
}

// A background start opens its pane through the grid in the caller's tab, not
// by splitting the caller's own pane: the pane lands beside the agents, and no
// `--direction down` split is issued.
func TestBackgroundStartPlacesItsPaneThroughTheGrid(t *testing.T) {
	g := newFakeGrid()
	caller := g.tabs[0].order[0]
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", caller)
	tool := gridBackground(t, g)

	out, err := backgroundStart(t, tool, "/repo", "sleep 999")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pane := "w1:p2" // the second pane of the caller's tab
	if !strings.Contains(out, pane) {
		t.Fatalf("start output %q does not name the pane %s", out, pane)
	}
	if tab := g.tabWith(pane); tab == nil || tab.tab != g.tabs[0].tab {
		t.Fatalf("the pane landed in %v, want the caller's tab %s", tab, g.tabs[0].tab)
	}
	ran := false
	for _, c := range g.calls {
		if strings.Contains(c, "pane run "+pane) {
			ran = true
		}
		if strings.Contains(c, "--direction down") || strings.Contains(c, "--current") {
			t.Errorf("background split its own pane instead of using the grid: %s", c)
		}
	}
	if !ran {
		t.Error("start never ran the command in the placed pane")
	}
}

// Past nine panes the grid overflows into a tab labelled scouts, and a
// background pane goes there rather than failing.
func TestBackgroundOverflowsIntoScouts(t *testing.T) {
	g := newFakeGrid()
	caller := g.tabs[0].order[0]
	for i := 0; i < 8; i++ {
		if _, err := placeInGrid(g.run, g.tabs[0].tab, "scouts", "/repo"); err != nil {
			t.Fatalf("placement %d: %v", i+2, err)
		}
	}
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", caller)
	tool := gridBackground(t, g)

	if _, err := backgroundStart(t, tool, "/repo", "sleep 999"); err != nil {
		t.Fatalf("start: %v", err)
	}
	pane := "w1:p10" // the tenth pane opens the overflow tab
	tab := g.tabWith(pane)
	if tab == nil {
		t.Fatalf("the tenth pane %s is nowhere", pane)
	}
	if got := g.label[tab.tab]; got != "scouts" {
		t.Errorf("overflow tab label = %q, want scouts", got)
	}
}
