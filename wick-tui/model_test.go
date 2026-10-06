package tui

import (
	"errors"
	"reflect"
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

// cannedTicketsSnapshot is a Runs tree with tickets: alpha carries two
// tickets (working, merged), beta one (not started).
func cannedTicketsSnapshot(repoA, repoB string) Snap {
	snap := cannedSnapshot(repoA, repoB)
	snap.Repos[0].Specs[0].Tickets = []TicketSnap{
		{Path: repoA + "/.scratch/alpha/issues/01-one.md", Name: "01-one.md", Status: TicketWorking},
		{Path: repoA + "/.scratch/alpha/issues/02-two.md", Name: "02-two.md", Status: TicketMerged},
	}
	snap.Repos[0].Specs[1].Tickets = []TicketSnap{
		{Path: repoA + "/.scratch/beta/issues/01-only.md", Name: "01-only.md", Status: TicketNotStarted},
	}
	return snap
}

func selectedTicket(m Model) string {
	item, ok := m.tickets.SelectedItem().(ticketItem)
	if !ok {
		return ""
	}
	return item.ticket.Name
}

func TestModelTicketsPaneShowsSelectedSpecTickets(t *testing.T) {
	snap := cannedTicketsSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)
	if len(m.tickets.Items()) != 2 {
		t.Fatalf("tickets = %d, want alpha's 2", len(m.tickets.Items()))
	}
	if got := selectedTicket(m); got != "01-one.md" {
		t.Fatalf("ticket selection = %q, want 01-one.md", got)
	}
}

func TestModelTicketKeyboardMovesSelection(t *testing.T) {
	snap := cannedTicketsSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	m, _ = pressKey(m, "l")
	if !m.focusTickets {
		t.Fatal("after l the tickets pane must have focus")
	}
	m, _ = pressKey(m, "j")
	if got := selectedTicket(m); got != "02-two.md" {
		t.Fatalf("after j ticket = %q, want 02-two.md", got)
	}
	m, _ = pressKey(m, "k")
	if got := selectedTicket(m); got != "01-one.md" {
		t.Fatalf("after k ticket = %q, want 01-one.md", got)
	}
	m, _ = pressKey(m, "h")
	if m.focusTickets {
		t.Fatal("after h the specs pane must have focus")
	}
}

func TestModelSpecSelectionDrivesTickets(t *testing.T) {
	snap := cannedTicketsSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	m, _ = pressKey(m, "j")
	if got := selectedSlug(m); got != "beta" {
		t.Fatalf("spec = %q, want beta", got)
	}
	if len(m.tickets.Items()) != 1 {
		t.Fatalf("tickets = %d, want beta's 1", len(m.tickets.Items()))
	}
	if got := selectedTicket(m); got != "01-only.md" {
		t.Fatalf("ticket = %q, want beta's 01-only.md", got)
	}
}

func TestModelTicketClickSelects(t *testing.T) {
	snap := cannedTicketsSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)

	_ = m.View()
	x, y := waitZone(t, m, ticketZoneID("/repo-a", "alpha", "02-two.md"))
	updated, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if got := selectedTicket(m); got != "02-two.md" {
		t.Fatalf("after click ticket = %q, want 02-two.md", got)
	}
	if !m.focusTickets {
		t.Fatal("clicking a ticket must focus the tickets pane")
	}
}

// cannedDetailSnapshot is alpha with one rich ticket: a costed worker and
// an uncosted reviewer, ticket totals, a spec total with an orchestrator,
// and a machine-wide RSS. The detail view renders all of it.
func cannedDetailSnapshot() Snap {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	snap.MachineRSS = 6500
	alpha := &snap.Repos[0].Specs[0]
	alpha.Orchestrator = &AgentSnap{ID: "orch", Kind: AgentOrchestrator,
		Harness: "claude", Model: "opus", Effort: "high",
		Pane: "pane-orch", Status: "working", Alive: true,
		Input: 2, Output: 10, RSS: 4000, HasRSS: true}
	alpha.Totals = Totals{Input: 312, Output: 135, CacheRead: 40, CacheWrite: 60,
		Cost: 0.03, HasCost: true, FindCalls: 2, RSS: 6500, HasRSS: true}
	alpha.Tickets = []TicketSnap{{
		Path: "/repo-a/.scratch/alpha/issues/01-one.md", Name: "01-one.md",
		Status: TicketWorking, Title: "My ticket", Body: "# My ticket\n\nDo things.\n",
		Agents: []AgentSnap{
			{ID: "01-one", Kind: AgentWorker, Harness: "pig", Model: "opus", Effort: "high",
				Pane: "pane-w1", Status: "working", Alive: true,
				Input: 300, Output: 120, CacheRead: 40, CacheWrite: 60, Reasoning: 12,
				Cost: 0.03, HasCost: true, FindCalls: 2, RSS: 2500, HasRSS: true},
			{ID: "rs1", Kind: AgentReviewerStandards, Harness: "pig", Model: "sonnet",
				Pane: "pane-rs", Status: "idle", Alive: true,
				Input: 10, Output: 5},
		},
		Totals: Totals{Input: 310, Output: 125, CacheRead: 40, CacheWrite: 60,
			Cost: 0.03, HasCost: true, FindCalls: 2, RSS: 2500, HasRSS: true},
	}}
	return snap
}

func openDetail(m Model) Model {
	m, _ = pressKey(m, "l")
	m, _ = pressKey(m, "enter")
	return m
}

func TestModelEnterOpensTicketDetailAndEscCloses(t *testing.T) {
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)

	m = openDetail(m)
	if m.detail == nil {
		t.Fatal("after enter the ticket detail must be open")
	}
	if m.detail.name != "01-one.md" {
		t.Fatalf("detail ticket = %q, want 01-one.md", m.detail.name)
	}
	view := m.View()
	for _, want := range []string{"My ticket", "Do things.", "opus · high", "sonnet",
		"$0.03", "no cost reported", "2.4 KB", "worker", "std", "300/120/100"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view must contain %q, got:\n%s", want, view)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.detail != nil {
		t.Fatal("after esc the detail must be closed")
	}
}

func TestModelDoubleClickOpensTicketDetail(t *testing.T) {
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)

	_ = m.View()
	x, y := waitZone(t, m, ticketZoneID("/repo-a", "alpha", "01-one.md"))
	click := tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	updated, _ := m.Update(click)
	m = updated.(Model)
	if m.detail != nil {
		t.Fatal("a single click only selects")
	}
	updated, _ = m.Update(click)
	m = updated.(Model)
	if m.detail == nil {
		t.Fatal("a double click opens the ticket detail")
	}
}

func TestModelRunsShowsTokensCostMemoryAndOrchestrator(t *testing.T) {
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)

	view := m.View()
	for _, want := range []string{"6.3 KB", "$0.03", "orchestrator", "claude"} {
		if !strings.Contains(view, want) {
			t.Errorf("runs view must contain %q, got:\n%s", want, view)
		}
	}
}

// The tickets pane renders every ticket name with its status glyph: an
// in-review ticket carries its code/spec verdicts in the row, and every
// status maps to its own glyph.
func TestModelTicketsViewShowsNamesAndVerdicts(t *testing.T) {
	snap := cannedSnapshot("/repo-a", "/repo-b")
	snap.Repos[0].Specs[0].Tickets = []TicketSnap{
		{Path: "/repo-a/.scratch/alpha/issues/01.md", Name: "01.md", Status: TicketNotStarted},
		{Path: "/repo-a/.scratch/alpha/issues/02.md", Name: "02.md", Status: TicketWorking},
		{Path: "/repo-a/.scratch/alpha/issues/03.md", Name: "03.md", Status: TicketRetrying},
		{Path: "/repo-a/.scratch/alpha/issues/04.md", Name: "04.md", Status: TicketIdle},
		{Path: "/repo-a/.scratch/alpha/issues/05.md", Name: "05.md", Status: TicketSubmittedDone},
		{Path: "/repo-a/.scratch/alpha/issues/06.md", Name: "06.md", Status: TicketSubmittedBlocked},
		{Path: "/repo-a/.scratch/alpha/issues/07.md", Name: "07.md", Status: TicketInReview, CodeVerdict: "pass", SpecVerdict: "changes"},
		{Path: "/repo-a/.scratch/alpha/issues/08.md", Name: "08.md", Status: TicketMerged},
		{Path: "/repo-a/.scratch/alpha/issues/09.md", Name: "09.md", Status: TicketDiscarded},
		{Path: "/repo-a/.scratch/alpha/issues/10.md", Name: "10.md", Status: TicketFailed},
	}
	var calls int
	m := loadModel(t, &snap, &calls)
	view := m.View()
	for _, want := range []string{"01.md", "10.md", "[pass/changes]", "○ 01.md", "● 02.md", "↻", "◌", "✓", "✗", "◐", "■", "□", "✖"} {
		if !strings.Contains(view, want) {
			t.Errorf("view must contain %q, got:\n%s", want, view)
		}
	}
}

// Jumping to an agent's pane: Enter on an agent (or a double-click, or
// Enter on a spec's orchestrator) focuses its herdr tab and pane through
// the jump CommandRunner; a pane herdr no longer lists shows a short
// message instead of any focus call.

// jumpExecWith serves a canned pane list mapping each pane to its tab and
// records every herdr invocation, so jump tests assert the focus sequence
// through the fake Exec.
func jumpExecWith(calls *[]string, tabs map[string]string) CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		*calls = append(*calls, strings.Join(append([]string{name}, args...), " "))
		if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
			panes := ""
			for pane, tab := range tabs {
				if panes != "" {
					panes += ","
				}
				panes += `{"pane_id":"` + pane + `","tab_id":"` + tab + `","agent_status":"working"}`
			}
			return []byte(`{"result":{"panes":[` + panes + `]}}`), nil
		}
		return []byte(`{}`), nil
	}
}

// detailPanes maps every pane the detail snapshot names to a tab.
func detailPanes() map[string]string {
	return map[string]string{"pane-w1": "tab-a", "pane-rs": "tab-a", "pane-orch": "tab-o"}
}

func TestModelEnterOnAgentJumpsToItsPane(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, detailPanes()))

	m = openDetail(m)
	m, _ = pressKey(m, "enter")
	want := []string{"herdr pane list", "herdr tab focus tab-a", "herdr agent focus pane-w1"}
	if !reflect.DeepEqual(jcalls, want) {
		t.Fatalf("jump calls = %q, want %q", jcalls, want)
	}
	noPaneChanges(t, jcalls)
	if m.notice != "" {
		t.Fatalf("notice = %q, want none after a clean jump", m.notice)
	}
}

func TestModelAgentKeysMoveSelectionAndEnterJumps(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, detailPanes()))

	m = openDetail(m)
	m, _ = pressKey(m, "j")
	if got := m.agentRows.Cursor(); got != 1 {
		t.Fatalf("agent cursor = %d, want the reviewer row", got)
	}
	m, _ = pressKey(m, "enter")
	want := []string{"herdr pane list", "herdr tab focus tab-a", "herdr agent focus pane-rs"}
	if !reflect.DeepEqual(jcalls, want) {
		t.Fatalf("jump calls = %q, want %q", jcalls, want)
	}
}

func TestModelEnterOnDeadAgentShowsGoneMessage(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, map[string]string{"pane-rs": "tab-a"}))

	m = openDetail(m)
	m, _ = pressKey(m, "enter")
	if !reflect.DeepEqual(jcalls, []string{"herdr pane list"}) {
		t.Fatalf("jump calls = %q, want only the lookup, never a focus", jcalls)
	}
	if m.notice != "agent 01-one's pane is gone" {
		t.Fatalf("notice = %q, want the gone message", m.notice)
	}
	if view := m.View(); !strings.Contains(view, "agent 01-one's pane is gone") {
		t.Fatalf("view must show the gone message, got:\n%s", view)
	}
}

func TestModelJumpFailureShowsFailedMessage(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(func(name string, args ...string) ([]byte, error) {
		jcalls = append(jcalls, strings.Join(append([]string{name}, args...), " "))
		return nil, errors.New("herdr down")
	})

	m = openDetail(m)
	m, _ = pressKey(m, "enter")
	if !reflect.DeepEqual(jcalls, []string{"herdr pane list"}) {
		t.Fatalf("jump calls = %q, want only the failed lookup", jcalls)
	}
	if m.notice != "jump failed: herdr down" {
		t.Fatalf("notice = %q, want the failure message", m.notice)
	}
	if view := m.View(); !strings.Contains(view, "jump failed: herdr down") {
		t.Fatalf("view must show the failure message, got:\n%s", view)
	}
}

func TestModelEnterOnSpecJumpsToOrchestrator(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, detailPanes()))

	m, _ = pressKey(m, "enter")
	want := []string{"herdr pane list", "herdr tab focus tab-o", "herdr agent focus pane-orch"}
	if !reflect.DeepEqual(jcalls, want) {
		t.Fatalf("jump calls = %q, want %q", jcalls, want)
	}
	noPaneChanges(t, jcalls)
}

func TestModelEnterOnSpecWithoutRunDoesNothing(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, detailPanes()))

	m, _ = pressKey(m, "j") // beta has no run and no orchestrator
	m, _ = pressKey(m, "enter")
	if len(jcalls) != 0 {
		t.Fatalf("jump calls = %q, want none without an orchestrator", jcalls)
	}
	if m.notice != "" {
		t.Fatalf("notice = %q, want none", m.notice)
	}
}

func TestModelDoubleClickAgentJumpsToItsPane(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, detailPanes()))

	m = openDetail(m)
	_ = m.View()
	x, y := waitZone(t, m, agentZoneID("/repo-a", "alpha", "01-one.md", "rs1"))
	click := tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	updated, _ := m.Update(click)
	m = updated.(Model)
	if m.detail == nil {
		t.Fatal("a single click on an agent only selects")
	}
	if got := m.agentRows.Cursor(); got != 1 {
		t.Fatalf("agent cursor = %d, want the clicked reviewer row", got)
	}
	updated, _ = m.Update(click)
	m = updated.(Model)
	want := []string{"herdr pane list", "herdr tab focus tab-a", "herdr agent focus pane-rs"}
	if !reflect.DeepEqual(jcalls, want) {
		t.Fatalf("jump calls = %q, want %q", jcalls, want)
	}
}

func TestModelDoubleClickOrchestratorJumpsToItsPane(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	snap := cannedDetailSnapshot()
	var calls int
	m := loadModel(t, &snap, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, detailPanes()))

	_ = m.View()
	x, y := waitZone(t, m, orchZoneID("/repo-a", "alpha"))
	click := tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	updated, _ := m.Update(click)
	m = updated.(Model)
	if len(jcalls) != 0 {
		t.Fatalf("a single click only selects, but ran %q", jcalls)
	}
	updated, _ = m.Update(click)
	m = updated.(Model)
	want := []string{"herdr pane list", "herdr tab focus tab-o", "herdr agent focus pane-orch"}
	if !reflect.DeepEqual(jcalls, want) {
		t.Fatalf("jump calls = %q, want %q", jcalls, want)
	}
	noPaneChanges(t, jcalls)
}
