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

// Status glyphs and colours, defined once here in the view layer: the spec
// running/ended glyphs beside every per-ticket glyph.
var (
	glyphRunning                = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	glyphEnded                  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○")
	glyphTicketNotStarted       = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○")
	glyphTicketWorking          = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	glyphTicketRetrying         = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("↻")
	glyphTicketIdle             = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render("◌")
	glyphTicketSubmittedDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("✓")
	glyphTicketSubmittedBlocked = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗")
	glyphTicketInReview         = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("◐")
	glyphTicketMerged           = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("■")
	glyphTicketDiscarded        = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("□")
	glyphTicketFailed           = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✖")
	styleRepo                   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleSel                    = lipgloss.NewStyle().Bold(true)
	styleTab                    = lipgloss.NewStyle().Bold(true).Underline(true)
	styleDim                    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// ticketGlyph is the one place a ticket status becomes its glyph.
func ticketGlyph(s TicketStatus) string {
	switch s {
	case TicketWorking:
		return glyphTicketWorking
	case TicketRetrying:
		return glyphTicketRetrying
	case TicketIdle:
		return glyphTicketIdle
	case TicketSubmittedDone:
		return glyphTicketSubmittedDone
	case TicketSubmittedBlocked:
		return glyphTicketSubmittedBlocked
	case TicketInReview:
		return glyphTicketInReview
	case TicketMerged:
		return glyphTicketMerged
	case TicketDiscarded:
		return glyphTicketDiscarded
	case TicketFailed:
		return glyphTicketFailed
	default:
		return glyphTicketNotStarted
	}
}

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

// ticketItem is one row of the tickets pane: a ticket of the selected spec.
// The repo and spec slug join the zone id because two specs can carry the
// same ticket file name.
type ticketItem struct {
	repo     string
	specSlug string
	ticket   TicketSnap
}

// FilterValue matches the ticket file name. Filtering stays disabled on the
// tickets list; the method only satisfies list.Item.
func (i ticketItem) FilterValue() string { return i.ticket.Name }

// ticketZoneID names the bubblezone click region of a ticket row.
func ticketZoneID(repo, slug, name string) string {
	return "ticket:" + repo + "\x00" + slug + "\x00" + name
}

func (i ticketItem) zoneID() string { return ticketZoneID(i.repo, i.specSlug, i.ticket.Name) }

// ticketDelegate renders ticket rows: the status glyph plus the file name,
// with the code/spec verdicts while in review. Height is one.
type ticketDelegate struct {
	zone *zone.Manager
}

func (d ticketDelegate) Height() int                         { return 1 }
func (d ticketDelegate) Spacing() int                        { return 0 }
func (d ticketDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d ticketDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ti, ok := item.(ticketItem)
	if !ok {
		return
	}
	row := fmt.Sprintf("%s %s", ticketGlyph(ti.ticket.Status), ti.ticket.Name)
	if ti.ticket.Status == TicketInReview {
		code, spec := ti.ticket.CodeVerdict, ti.ticket.SpecVerdict
		if code == "" {
			code = "…"
		}
		if spec == "" {
			spec = "…"
		}
		row += fmt.Sprintf(" [%s/%s]", code, spec)
	}
	if index == m.Index() {
		row = styleSel.Render(row)
	}
	fmt.Fprint(w, d.zone.Mark(ti.zoneID(), row))
}

// progressPercent is the merged/total fraction one row's bar shows.
func progressPercent(merged, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(merged) / float64(total)
}

// Model is the dashboard: a Runs list over the Snapshot read model with the
// selected spec's tickets beside it, plus a Settings placeholder, refreshed
// every second. Mouse clicks land through bubblezone regions; double-click
// detection stays ours — the last click's zone and time are recorded for the
// tickets that open detail views.
type Model struct {
	load                Loader
	snap                Snap
	list                list.Model
	tickets             list.Model
	focusTickets        bool
	pendingTicket       string
	zone                *zone.Manager
	tab                 Tab
	width               int
	height              int
	lastClick           time.Time
	lastZone            string
	pendingSelect       string
	reselectAfterFilter bool
	err                 error
	settings            settingsState
}

// NewModel builds the dashboard over load. Both lists start 80x20 and 40x20;
// the first WindowSizeMsg resizes them side by side.
func NewModel(load Loader) Model {
	zm := zone.New()
	bar := progress.New(progress.WithWidth(12), progress.WithoutPercentage())
	l := list.New(nil, specDelegate{zone: zm, bar: bar}, 80, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	tl := list.New(nil, ticketDelegate{zone: zm}, 40, 20)
	tl.SetShowTitle(false)
	tl.SetShowStatusBar(false)
	tl.SetShowPagination(false)
	tl.SetShowHelp(false)
	tl.SetFilteringEnabled(false)
	return Model{load: load, list: l, tickets: tl, zone: zm, width: 80, height: 24}
}

// SetSettingsBackend wires the Settings tab's seam. The live binary passes
// LiveBackend; tests pass a stub. It lives outside NewModel so unit tests
// stay hermetic: nothing here may touch the real settings file, harness
// CLIs or state dir unless the caller asks for it.
func (m *Model) SetSettingsBackend(be SettingsBackend) { m.settings.backend = be }

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

// Update drives the dashboard: tab switches tabs, q quits, h/l and arrows
// switch panes and move through the focused list, a left press selects the
// clicked spec or ticket row, and every tick reloads the snapshot. A herdr
// failure keeps the last snapshot and retries on the next tick. On the
// Settings tab keys and clicks edit through the settings backend (see
// settings.go); a tick re-reads the file without disturbing an open picker
// or editor.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		half := max(20, msg.Width/2)
		m.list.SetSize(half, max(1, msg.Height-2))
		m.tickets.SetSize(max(1, msg.Width-half), max(1, msg.Height-2))
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.loadCmd(), tickCmd())
	case snapMsg:
		m.snap, m.err = msg.snap, nil
		cmd := m.rebuildItems()
		m.syncTickets()
		m.reloadSettingsValues()
		return m, cmd
	case errMsg:
		m.err = msg.err
		return m, nil
	case settingsRefreshMsg:
		return m.updateSettings(msg)
	case list.FilterMatchesMsg:
		lm, cmd := m.list.Update(msg)
		m.list = lm
		if m.reselectAfterFilter {
			m.reselectAfterFilter = false
			m.restoreSelection()
		}
		m.syncTickets()
		return m, cmd
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if m.tab == tabRuns {
				m.click(msg)
			} else {
				return m.updateSettings(msg)
			}
		}
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyTab && m.list.FilterState() != list.Filtering && !m.settings.capturing() {
			m.tab = (m.tab + 1) % 2
			if m.tab == tabSettings {
				m.loadSettingsTab()
			}
			return m, nil
		}
		if m.tab == tabSettings {
			return m.updateSettings(msg)
		}
		if m.tab == tabRuns && m.list.FilterState() != list.Filtering {
			if msg.Type == tea.KeyRight {
				m.focusTickets = true
				return m, nil
			}
			if msg.Type == tea.KeyLeft {
				m.focusTickets = false
				return m, nil
			}
			if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
				switch msg.Runes[0] {
				case 'l':
					m.focusTickets = true
					return m, nil
				case 'h':
					m.focusTickets = false
					return m, nil
				}
			}
		}
	}
	if m.tab == tabRuns {
		if m.focusTickets {
			tl, cmd := m.tickets.Update(msg)
			m.tickets = tl
			return m, cmd
		}
		lm, cmd := m.list.Update(msg)
		m.list = lm
		m.syncTickets()
		return m, cmd
	}
	return m, nil
}

// click selects the ticket row whose bubblezone region holds the press, else
// the spec row, focusing the clicked pane. It records the click for the
// double-click detection the ticket detail ticket adds.
func (m *Model) click(msg tea.MouseMsg) {
	for i, item := range m.tickets.VisibleItems() {
		ti, ok := item.(ticketItem)
		if !ok {
			continue
		}
		if m.zone.Get(ti.zoneID()).InBounds(msg) {
			m.focusTickets = true
			m.tickets.Select(i)
			m.lastClick, m.lastZone = time.Now(), ti.zoneID()
			return
		}
	}
	for i, item := range m.list.VisibleItems() {
		si, ok := item.(specItem)
		if !ok {
			continue
		}
		if m.zone.Get(si.zoneID()).InBounds(msg) {
			m.focusTickets = false
			m.list.Select(i)
			m.syncTickets()
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

// syncTickets rebuilds the tickets pane from the selected spec, keeping the
// selected ticket when it is still there. A spec change resets to its first
// ticket: the old ticket id is simply not in the new rows.
func (m *Model) syncTickets() {
	m.pendingTicket = ""
	if item, ok := m.tickets.SelectedItem().(ticketItem); ok {
		m.pendingTicket = item.zoneID()
	}
	var items []list.Item
	if si, ok := m.list.SelectedItem().(specItem); ok {
		for _, tk := range si.spec.Tickets {
			items = append(items, ticketItem{repo: si.repo, specSlug: si.spec.Slug, ticket: tk})
		}
	}
	m.tickets.SetItems(items)
	m.restoreTicket()
}

// restoreTicket selects the stashed ticket when it is visible, then drops
// the stash either way: one restore per sync, never a fight with the user's
// own cursor moves.
func (m *Model) restoreTicket() {
	if m.pendingTicket == "" {
		return
	}
	for i, item := range m.tickets.VisibleItems() {
		if ti, ok := item.(ticketItem); ok && ti.zoneID() == m.pendingTicket {
			m.tickets.Select(i)
			break
		}
	}
	m.pendingTicket = ""
}

// View renders the tab bar, a help line, and the active tab, with every row
// wrapped in its bubblezone click region. Runs shows the specs on the left
// and the selected spec's tickets on the right.
func (m Model) View() string {
	tabs := "Runs  Settings"
	if m.tab == tabRuns {
		tabs = styleTab.Render("Runs") + "  Settings"
	} else {
		tabs = "Runs  " + styleTab.Render("Settings")
	}
	help := styleDim.Render("↑/k ↓/j move • ←/→ or h/l pane • click select • tab tabs • q quit • 1s refresh")
	if m.tab == tabSettings {
		help = styleDim.Render(m.settingsHelp())
	}
	if m.err != nil {
		help = styleDim.Render("refresh failed: " + m.err.Error() + " • retrying")
	}
	var body string
	if m.tab == tabRuns {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.list.View(), m.tickets.View())
	} else {
		body = m.viewSettings()
	}
	return m.zone.Scan(tabs + "\n" + help + "\n" + body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
