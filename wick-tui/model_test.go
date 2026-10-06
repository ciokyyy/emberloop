package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// cannedSnapshot is a fixed Runs tree: two repos, three specs.
func cannedSnapshot(repoA, repoB string) Snap {
	return Snap{Repos: []RepoSnap{
		{Path: repoA, Specs: []SpecSnap{
			{Slug: "alpha", Spec: repoA + "/.scratch/alpha/spec.md", Branch: "spec/alpha", HasRun: true, Running: true, Merged: 1, Total: 2},
			{Slug: "beta", Spec: repoA + "/.scratch/beta/spec.md", Branch: "spec/beta", Merged: 0, Total: 1},
		}},
		{Path: repoB, Specs: []SpecSnap{
			{Slug: "gamma", Spec: repoB + "/.scratch/gamma/spec.md", Branch: "spec/gamma", HasRun: true, Merged: 2, Total: 2},
		}},
	}}
}

// cannedLoader serves snap and counts calls, so polling tests see reloads.
func cannedLoader(snap *Snap, calls *int) Loader {
	return func() (Snap, error) {
		*calls++
		return *snap, nil
	}
}

// loadModel builds a model over the canned tree and runs its initial load
// the way the runtime would: Init's batch, then the load leg's message.
func loadModel(t *testing.T, snap *Snap, calls *int) Model {
	t.Helper()
	m := NewModel(cannedLoader(snap, calls))
	loaded, err := m.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	updated, _ := m.Update(snapMsg{snap: loaded})
	m = updated.(Model)
	if len(m.list.Items()) != 3 {
		t.Fatalf("items = %d, want 3", len(m.list.Items()))
	}
	return m
}

// runCmds drives one command's data messages back through the model the way
// the runtime does, draining nested commands too (a reload's SetItems filter
// recompute, for example). Only data messages are forwarded — ticks stay
// with the runtime's rescheduling, and cursor blinks and spinner ticks are
// animation the tests never need (and which reschedule forever). The step
// cap fails a runaway instead of hanging the suite.
func runCmds(m Model, cmd tea.Cmd) Model {
	pending := []tea.Cmd{cmd}
	for steps := 0; len(pending) > 0 && steps < 100; steps++ {
		c := pending[0]
		pending = pending[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			pending = append(pending, batch...)
			continue
		}
		switch msg.(type) {
		case snapMsg, errMsg, list.FilterMatchesMsg:
			updated, next := m.Update(msg)
			m = updated.(Model)
			pending = append(pending, next)
		}
	}
	return m
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func selectedSlug(m Model) string {
	item, ok := m.list.SelectedItem().(specItem)
	if !ok {
		return ""
	}
	return item.spec.Slug
}

func pressKey(m Model, s string) (Model, tea.Cmd) {
	updated, cmd := m.Update(keyMsg(s))
	return updated.(Model), cmd
}

func TestModelKeyboardMovesSelectionVimAndArrows(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	if got := selectedSlug(m); got != "alpha" {
		t.Fatalf("initial selection = %q, want alpha", got)
	}
	m, _ = pressKey(m, "j")
	if got := selectedSlug(m); got != "beta" {
		t.Fatalf("after j selection = %q, want beta", got)
	}
	m, _ = pressKey(m, "down")
	if got := selectedSlug(m); got != "gamma" {
		t.Fatalf("after down selection = %q, want gamma", got)
	}
	m, _ = pressKey(m, "k")
	if got := selectedSlug(m); got != "beta" {
		t.Fatalf("after k selection = %q, want beta", got)
	}
	m, _ = pressKey(m, "up")
	if got := selectedSlug(m); got != "alpha" {
		t.Fatalf("after up selection = %q, want alpha", got)
	}
}

// waitZone polls the model's zone manager until the zone registers, the way
// a real terminal Scan settles before any click can land.
func waitZone(t *testing.T, m Model, id string) (x, y int) {
	t.Helper()
	for i := 0; i < 500; i++ {
		z := m.zone.Get(id)
		if !z.IsZero() {
			return (z.StartX + z.EndX) / 2, (z.StartY + z.EndY) / 2
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("zone %q never registered", id)
	return 0, 0
}

func TestModelClickSelectsASpec(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	_ = m.View()
	x, y := waitZone(t, m, specZoneID("/repo-a", "beta"))
	updated, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if got := selectedSlug(m); got != "beta" {
		t.Fatalf("after click selection = %q, want beta", got)
	}
}

func TestModelTabSwitchesTabsAndQQuits(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	m, _ = pressKey(m, "tab")
	if m.tab != tabSettings {
		t.Fatalf("after tab = %v, want settings", m.tab)
	}
	if !strings.Contains(m.View(), "Settings") {
		t.Fatalf("settings view must name itself, got %q", m.View())
	}
	m, _ = pressKey(m, "tab")
	if m.tab != tabRuns {
		t.Fatalf("after tab tab = %v, want runs", m.tab)
	}

	m, cmd := pressKey(m, "q")
	if cmd == nil {
		t.Fatal("q must return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q must quit")
	}
}

func TestModelPollingReloadsEverySecond(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)
	loaded := calls

	updated, cmd := m.Update(tickMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("a tick must schedule a load")
	}
	m = runCmds(m, cmd)
	if calls != loaded+1 {
		t.Fatalf("loads = %d, want one more than %d", calls, loaded)
	}

	// A new spec arriving keeps the old selection when it still exists.
	snap.Repos[0].Specs = append(snap.Repos[0].Specs,
		SpecSnap{Slug: "aardvark", Spec: "/repo-a/.scratch/aardvark/spec.md", Branch: "spec/aardvark", Total: 1})
	m, _ = pressKey(m, "j") // beta
	updated, cmd = m.Update(tickMsg{})
	m = updated.(Model)
	m = runCmds(m, cmd)
	if got := selectedSlug(m); got != "beta" {
		t.Fatalf("after reload selection = %q, want beta kept", got)
	}
	if len(m.list.Items()) != 4 {
		t.Fatalf("items = %d, want 4 after the new spec", len(m.list.Items()))
	}

}

// A poll while filtered must keep the filtered rows: SetItems recomputes
// the filter asynchronously, and its command drives that recompute.
func TestModelPollingWhileFilteredKeepsTheFilter(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	m, _ = pressKey(m, "/")
	// Every keystroke recomputes the filter asynchronously; the runtime runs
	// each command, so the last keystroke's message populates the rows
	// before enter accepts them.
	query := "beta"
	for i, r := range query {
		if i == len(query)-1 {
			var cmd tea.Cmd
			m, cmd = pressKey(m, string(r))
			m = runCmds(m, cmd)
		} else {
			m, _ = pressKey(m, string(r))
		}
	}
	var cmd tea.Cmd
	m, cmd = pressKey(m, "enter")
	m = runCmds(m, cmd)
	if got := len(m.list.VisibleItems()); got != 1 {
		t.Fatalf("visible = %d, want only beta", got)
	}

	updated, cmd := m.Update(tickMsg{})
	m = updated.(Model)
	m = runCmds(m, cmd)
	visible := m.list.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("visible after poll = %d, want only beta", len(visible))
	}
	if item, ok := visible[0].(specItem); !ok || item.spec.Slug != "beta" {
		t.Fatalf("visible after poll = %+v, want beta", visible)
	}
	if got := selectedSlug(m); got != "beta" {
		t.Fatalf("selection after poll = %q, want beta", got)
	}

}

// typeFilter applies a list filter the way the runtime does: every
// keystroke's recompute command runs, and the last one populates the rows
// before enter accepts them.
func typeFilter(m Model, query string) Model {
	var cmd tea.Cmd
	m, _ = pressKey(m, "/")
	for i, r := range query {
		if i == len(query)-1 {
			m, cmd = pressKey(m, string(r))
			m = runCmds(m, cmd)
		} else {
			m, _ = pressKey(m, string(r))
		}
	}
	m, cmd = pressKey(m, "enter")
	return runCmds(m, cmd)
}

// A poll that inserts a matching row under an active filter keeps the
// selection on the same spec by identity, not by row index.
func TestModelPollInsertsRowUnderActiveFilterKeepsSelection(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)
	m = typeFilter(m, "gamm")
	if got := selectedSlug(m); got != "gamma" {
		t.Fatalf("filtered selection = %q, want gamma", got)
	}

	// gamma-0 sorts before gamma and matches the same filter.
	snap.Repos[1].Specs = append([]SpecSnap{
		{Slug: "gamma-0", Spec: "/repo-b/.scratch/gamma-0/spec.md", Branch: "spec/gamma-0", Total: 1},
	}, snap.Repos[1].Specs...)
	updated, cmd := m.Update(tickMsg{})
	m = updated.(Model)
	m = runCmds(m, cmd)
	visible := m.list.VisibleItems()
	if len(visible) != 2 {
		t.Fatalf("visible after poll = %d, want gamma-0 and gamma", len(visible))
	}
	if got := selectedSlug(m); got != "gamma" {
		t.Fatalf("selection after poll = %q, want gamma kept by identity, not row 0", got)
	}
}
