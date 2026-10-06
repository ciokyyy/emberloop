package tui

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// The tabs of the dashboard. Stats joins them under the stats spec; Settings
// is a placeholder until its ticket.
type Tab int

const (
	tabRuns Tab = iota
	tabSettings
)

// Loader reloads the read model. The live program passes a closure over
// Snapshot with the real state dir and herdr; tests pass a canned tree.
type Loader func() (Snap, error)

// pollEvery is the live-refresh interval both tabs share.
const pollEvery = time.Second

// Status glyphs and colours, defined once here in the view layer. Later
// tickets add the per-ticket glyphs beside these.
var (
	glyphRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	glyphEnded   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○")
	styleRepo    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleSel     = lipgloss.NewStyle().Bold(true)
	styleTab     = lipgloss.NewStyle().Bold(true).Underline(true)
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// specItem is one row of the Runs list: a spec with its repo, flattened so
// the list stays one-dimensional. showRepo renders the repo header above the
// first spec of each repo, which is what groups the flat list by repo.
type specItem struct {
	repo     string
	spec     SpecSnap
	showRepo bool
}

// FilterValue matches slug and repo, so `/` finds both.
func (i specItem) FilterValue() string { return i.repo + " " + i.spec.Slug }

// specZoneID names the bubblezone click region of a spec row. The repo is in
// the id because two repos can carry the same slug.
func specZoneID(repo, slug string) string { return "spec:" + repo + "\x00" + slug }

func (i specItem) zoneID() string { return specZoneID(i.repo, i.spec.Slug) }

// specDelegate renders Runs rows: a repo header line plus the spec line with
// its running/ended glyph, merged/total progress and progress bar. Height is
// two, always — the header line is blank past the first spec of a repo.
type specDelegate struct {
	zone *zone.Manager
	bar  progress.Model
}

func (d specDelegate) Height() int                         { return 2 }
func (d specDelegate) Spacing() int                        { return 0 }
func (d specDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d specDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(specItem)
	if !ok {
		return
	}
	head := ""
	if si.showRepo {
		head = styleRepo.Render(si.repo)
	}
	glyph := glyphEnded
	if si.spec.Running {
		glyph = glyphRunning
	}
	row := fmt.Sprintf("%s %s  %d/%d %s",
		glyph, si.spec.Slug, si.spec.Merged, si.spec.Total,
		d.bar.ViewAs(progressPercent(si.spec.Merged, si.spec.Total)))
	if index == m.Index() {
		row = styleSel.Render(row)
	}
	fmt.Fprint(w, d.zone.Mark(si.zoneID(), head+"\n"+row))
}

// progressPercent is the merged/total fraction one row's bar shows.
func progressPercent(merged, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(merged) / float64(total)
}

// Model is the dashboard: a Runs list over the Snapshot read model plus a
// Settings placeholder, refreshed every second. Mouse clicks land through
// bubblezone regions; double-click detection stays ours — the last click's
// zone and time are recorded for the tickets that open detail views.
type Model struct {
	load                Loader
	snap                Snap
	list                list.Model
	zone                *zone.Manager
	tab                 Tab
	width               int
	height              int
	lastClick           time.Time
	lastZone            string
	pendingSelect       string
	reselectAfterFilter bool
	err                 error
}

// NewModel builds the dashboard over load. The list starts 80x20; the first
// WindowSizeMsg resizes it.
func NewModel(load Loader) Model {
	zm := zone.New()
	bar := progress.New(progress.WithWidth(12), progress.WithoutPercentage())
	l := list.New(nil, specDelegate{zone: zm, bar: bar}, 80, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	return Model{load: load, list: l, zone: zm, width: 80, height: 24}
}

type tickMsg struct{}
type snapMsg struct{ snap Snap }
type errMsg struct{ err error }

func tickCmd() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		snap, err := m.load()
		if err != nil {
			return errMsg{err}
		}
		return snapMsg{snap: snap}
	}
}

// Init loads the first snapshot and starts the one-second poll.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(), tickCmd())
}

// Update drives the dashboard: tab switches tabs, q quits, arrows and vim
// keys move through the list, a left press selects the clicked spec row, and
// every tick reloads the snapshot. A herdr failure keeps the last snapshot
// and retries on the next tick.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(max(1, msg.Width), max(1, msg.Height-2))
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.loadCmd(), tickCmd())
	case snapMsg:
		m.snap, m.err = msg.snap, nil
		return m, m.rebuildItems()
	case errMsg:
		m.err = msg.err
		return m, nil
	case list.FilterMatchesMsg:
		lm, cmd := m.list.Update(msg)
		m.list = lm
		if m.reselectAfterFilter {
			m.reselectAfterFilter = false
			m.restoreSelection()
		}
		return m, cmd
	case tea.MouseMsg:
		if m.tab == tabRuns && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.click(msg)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyTab && m.list.FilterState() != list.Filtering {
			m.tab = (m.tab + 1) % 2
			return m, nil
		}
		if m.tab == tabSettings {
			if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
				return m, tea.Quit
			}
			if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
				return m, tea.Quit
			}
			return m, nil
		}
	}
	if m.tab == tabRuns {
		lm, cmd := m.list.Update(msg)
		m.list = lm
		return m, cmd
	}
	return m, nil
}

// click selects the spec row whose bubblezone region holds the press, and
// records the click for the double-click detection later tickets add.
func (m *Model) click(msg tea.MouseMsg) {
	for i, item := range m.list.VisibleItems() {
		si, ok := item.(specItem)
		if !ok {
			continue
		}
		if m.zone.Get(si.zoneID()).InBounds(msg) {
			m.list.Select(i)
			m.lastClick, m.lastZone = time.Now(), si.zoneID()
			return
		}
	}
}

// rebuildItems flattens the snapshot into list rows, keeping the selected
// spec when it still exists. It returns SetItems' command: with a filter
// applied the filtered rows recompute asynchronously, and dropping it
// blanks the list until the next keystroke. The selection restores by
// visible index, the space Select moves in — immediately when unfiltered,
// or when the recompute's FilterMatchesMsg arrives while filtered, because
// the visible rows are empty until then.
func (m *Model) rebuildItems() tea.Cmd {
	var items []list.Item
	for _, r := range m.snap.Repos {
		for i, s := range r.Specs {
			items = append(items, specItem{repo: r.Path, spec: s, showRepo: i == 0})
		}
	}
	m.pendingSelect = ""
	if item, ok := m.list.SelectedItem().(specItem); ok {
		m.pendingSelect = item.zoneID()
	}
	cmd := m.list.SetItems(items)
	m.reselectAfterFilter = m.list.FilterState() != list.Unfiltered && m.pendingSelect != ""
	if !m.reselectAfterFilter {
		m.restoreSelection()
	}
	return cmd
}

// restoreSelection selects the stashed spec when it is visible, then drops
// the stash either way: one restore per reload, never a fight with the
// user's own cursor moves or filter typing.
func (m *Model) restoreSelection() {
	if m.pendingSelect == "" {
		return
	}
	for i, item := range m.list.VisibleItems() {
		if si, ok := item.(specItem); ok && si.zoneID() == m.pendingSelect {
			m.list.Select(i)
			break
		}
	}
	m.pendingSelect = ""
}

// View renders the tab bar, a help line, and the active tab, with every row
// wrapped in its bubblezone click region.
func (m Model) View() string {
	tabs := "Runs  Settings"
	if m.tab == tabRuns {
		tabs = styleTab.Render("Runs") + "  Settings"
	} else {
		tabs = "Runs  " + styleTab.Render("Settings")
	}
	help := styleDim.Render("↑/k ↓/j move • click select • tab tabs • q quit • 1s refresh")
	if m.err != nil {
		help = styleDim.Render("refresh failed: " + m.err.Error() + " • retrying")
	}
	var body string
	if m.tab == tabRuns {
		body = m.list.View()
	} else {
		body = "Settings — the role defaults editor arrives in its own ticket."
	}
	return m.zone.Scan(tabs + "\n" + help + "\n" + body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
