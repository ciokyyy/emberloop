package wick

import "fmt"

// backgroundPane places a background command's pane through the grid: the tab
// this session runs in, overflowing into `scouts` when the grid is full.
func backgroundPane(cwd string) (string, error) {
	return backgroundPaneWith(OSExec)(cwd)
}

// backgroundPaneWith is backgroundPane over an exec seam, so a test can drive
// the real grid against a fake herdr.
func backgroundPaneWith(run Exec) func(string) (string, error) {
	return func(cwd string) (string, error) {
		tab := callerTab(run)
		if tab == "" {
			return "", fmt.Errorf("not inside a herdr tab, so there is no tab to place a pane in")
		}
		placed, err := placeInGrid(run, tab, "scouts", cwd)
		if err != nil {
			return "", err
		}
		return placed.Pane, nil
	}
}
