package tui

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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

// formatCount renders a token count for a row: exact under a thousand,
// one-decimal thousands above.
func formatCount(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// formatCost renders a harness-reported cost exactly as reported, never
// estimated: plain decimals, so a missing cost (shown as "no cost
// reported") never merges with a zero.
func formatCost(c float64) string {
	return "$" + strconv.FormatFloat(c, 'f', -1, 64)
}

// costString is a totals' reported cost, or the "no cost reported" mark
// beside its tokens when no agent reported any.
func costString(cost float64, hasCost bool) string {
	if !hasCost {
		return "no cost reported"
	}
	return formatCost(cost)
}

// totalsBrief is the one-line spend summary spec rows and detail totals
// share: combined input+output tokens, reported cost and current RSS.
func totalsBrief(t Totals) string {
	out := fmt.Sprintf("%s tok · %s", formatCount(t.Input+t.Output), costString(t.Cost, t.HasCost))
	if t.HasRSS {
		out += " · " + formatBytes(t.RSS)
	}
	return out
}

// machineLine is the header's machine-wide memory: the summed current RSS
// of every running agent, or a dash while nothing reports.
func machineLine(rss int64) string {
	if rss <= 0 {
		return "mem —"
	}
	return "mem " + formatBytes(rss)
}

// agentStatus is the status a detail row shows: the herdr agent state
// while the pane is alive ("alive" when herdr names none), "gone" once
// the pane is gone.
func agentStatus(a AgentSnap) string {
	if !a.Alive {
		return "gone"
	}
	if a.Status == "" {
		return "alive"
	}
	return a.Status
}

// specDelegate renders Runs rows: a repo header line plus the spec line with
// its running/ended glyph, merged/total progress, progress bar, and live
// spend (tokens, reported cost, RSS). Height is two, always — the header
// line is blank past the first spec of a repo.
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
	row := fmt.Sprintf("%s %s  %d/%d %s  %s",
		glyph, si.spec.Slug, si.spec.Merged, si.spec.Total,
		d.bar.ViewAs(progressPercent(si.spec.Merged, si.spec.Total)),
		totalsBrief(si.spec.Totals))
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

// detailSel names the open ticket detail: the ticket of a spec that Enter
// or a double-click opened. Nil while the dashboard shows the lists.
type detailSel struct {
	repo string
	slug string
	name string
}

// doubleClickWithin is the press gap that turns two single clicks on one
// ticket row into an open.
const doubleClickWithin = 500 * time.Millisecond

// Model is the dashboard: a Runs list over the Snapshot read model with the
// selected spec's tickets beside it, a ticket detail with the ticket's
// Markdown and its agents, plus a Settings placeholder, refreshed every
// second. Mouse clicks land through bubblezone regions; double-click
// detection stays ours — the last click's zone and time are recorded for
// the tickets that open detail views.
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
	detail              *detailSel
	md                  viewport.Model
	agentRows           table.Model
	err                 error
	settings            settingsState
}

// agentTableWidth is the table's full content width: every column plus
// the default cell padding, so rows never truncate inside the table.
// Narrow terminals cut the line visually instead of hiding spend.
func agentTableWidth() int {
	w := 0
	for _, c := range agentColumns() {
		w += c.Width + 2
	}
	return w
}

// agentColumns is the ticket detail's agent table: who each agent is, how
// it runs, and what it spent. Cost shows the reported total or the "no
// cost reported" mark, never an estimate. Tokens merge input, output and
// both cache directions into one in/out/cache cell, and roles shorten to
// fit, so the full table fits a ~100-column pane with every required
// field visible.
func agentColumns() []table.Column {
	return []table.Column{
		{Title: "agent", Width: 10},
		{Title: "role", Width: 6},
		{Title: "harness", Width: 6},
		{Title: "model", Width: 12},
		{Title: "status", Width: 7},
		{Title: "tokens", Width: 14},
		{Title: "cost", Width: 16},
		{Title: "find", Width: 5},
		{Title: "rss", Width: 7},
	}
}

// roleShort is the role label the agent table shows: the full kind does
// not fit the table, so reviewers shorten to their three-letter slot.
func roleShort(k AgentKind) string {
	switch k {
	case AgentWorker:
		return "worker"
	case AgentReviewerStandards:
		return "std"
	case AgentReviewerSpec:
		return "spec"
	case AgentScout:
		return "scout"
	case AgentOrchestrator:
		return "orch"
	}
	return string(k)
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
	vp := viewport.New(76, 10)
	tb := table.New(table.WithColumns(agentColumns()))
	tb.SetWidth(agentTableWidth())
	tb.SetHeight(2)
	return Model{load: load, list: l, tickets: tl, zone: zm, width: 80, height: 24, md: vp, agentRows: tb}
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
// switch panes and move through the focused list, enter on a ticket opens
// its detail and esc leaves it, a left press selects the clicked spec or
// ticket row, and every tick reloads the snapshot. A herdr failure keeps
// the last snapshot and retries on the next tick. On the Settings tab keys
// and clicks edit through the settings backend (see settings.go); a tick
// re-reads the file without disturbing an open picker or editor.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		half := max(20, msg.Width/2)
		m.list.SetSize(half, max(1, msg.Height-2))
		m.tickets.SetSize(max(1, msg.Width-half), max(1, msg.Height-2))
		m.sizeDetail()
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.loadCmd(), tickCmd())
	case snapMsg:
		m.snap, m.err = msg.snap, nil
		cmd := m.rebuildItems()
		m.syncTickets()
		m.reloadSettingsValues()
		m.refreshDetail()
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
		m.refreshDetail()
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
		if m.detail != nil {
			return m.updateDetail(msg)
		}
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
			if msg.Type == tea.KeyEnter && m.focusTickets {
				m.openDetail()
				return m, nil
			}
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

// updateDetail drives the open ticket detail: esc goes back to the lists,
// tab leaves the detail for the other tab, q quits, and every other key
// scrolls the ticket Markdown.
func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.detail = nil
		return m, nil
	case tea.KeyTab:
		m.detail = nil
		if m.list.FilterState() != list.Filtering {
			m.tab = (m.tab + 1) % 2
		}
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyRunes:
		if len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
			return m, tea.Quit
		}
	}
	vp, cmd := m.md.Update(msg)
	m.md = vp
	return m, cmd
}

// click selects the ticket row whose bubblezone region holds the press, else
// the spec row, focusing the clicked pane. A second press on the same
// ticket row within the double-click window opens its detail instead.
func (m *Model) click(msg tea.MouseMsg) {
	now := time.Now()
	for i, item := range m.tickets.VisibleItems() {
		ti, ok := item.(ticketItem)
		if !ok {
			continue
		}
		if m.zone.Get(ti.zoneID()).InBounds(msg) {
			m.focusTickets = true
			m.tickets.Select(i)
			if m.lastZone == ti.zoneID() && now.Sub(m.lastClick) < doubleClickWithin {
				m.openDetail()
			}
			m.lastClick, m.lastZone = now, ti.zoneID()
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
			m.lastClick, m.lastZone = now, si.zoneID()
			return
		}
	}
}

// findDetail looks the open detail's ticket up in the current snapshot.
// Polls replace the tree every second, so the detail re-resolves against
// the fresh rows rather than holding a stale copy.
func (m Model) findDetail() (TicketSnap, bool) {
	if m.detail == nil {
		return TicketSnap{}, false
	}
	for _, r := range m.snap.Repos {
		if r.Path != m.detail.repo {
			continue
		}
		for _, s := range r.Specs {
			if s.Slug != m.detail.slug {
				continue
			}
			for _, tk := range s.Tickets {
				if tk.Name == m.detail.name {
					return tk, true
				}
			}
		}
	}
	return TicketSnap{}, false
}

// openDetail opens the selected ticket's detail, rendering its Markdown
// and sizing the agent table to its agents.
func (m *Model) openDetail() {
	item, ok := m.tickets.SelectedItem().(ticketItem)
	if !ok {
		return
	}
	m.detail = &detailSel{repo: item.repo, slug: item.specSlug, name: item.ticket.Name}
	m.renderDetail()
}

// refreshDetail re-renders the open detail against the fresh snapshot the
// poll just delivered: numbers stay live while it is open. A ticket that
// vanished (its spec reloaded without the file) closes the detail.
func (m *Model) refreshDetail() {
	if m.detail == nil {
		return
	}
	m.renderDetail()
}

// renderDetail fills the Markdown viewport and the agent table from the
// detail's ticket, preserving the scroll position across live refreshes.
// A ticket the snapshot no longer names closes the detail.
func (m *Model) renderDetail() {
	tk, ok := m.findDetail()
	if !ok {
		m.detail = nil
		return
	}
	off := m.md.YOffset
	m.md.SetContent(renderMarkdown(m.width, tk.Body))
	rows := make([]table.Row, 0, len(tk.Agents))
	for _, a := range tk.Agents {
		rows = append(rows, agentRow(a))
	}
	m.agentRows.SetRows(rows)
	m.agentRows.SetHeight(max(2, len(rows)+1))
	m.sizeDetail()
	m.md.SetYOffset(min(off, m.maxYOffset()))
}

// maxYOffset is the deepest the Markdown viewport may scroll.
func (m Model) maxYOffset() int {
	return max(0, m.md.TotalLineCount()-m.md.Height)
}

// sizeDetail fits the detail panes to the terminal: the agent table takes
// its rows, the Markdown viewport the rest.
func (m *Model) sizeDetail() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.agentRows.SetWidth(agentTableWidth())
	rows := len(m.agentRows.Rows())
	detailHeight := max(1, m.height-6-rows-3)
	m.md.Width = max(20, m.width-2)
	m.md.Height = detailHeight
}

// renderMarkdown renders a ticket body with glamour for the detail's
// scrollable viewport, falling back to the raw Markdown when the
// renderer cannot be built.
func renderMarkdown(width int, body string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(max(20, width-4)),
	)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return out
}

// agentRow is one detail table row: identity, how the agent runs, and what
// the usage logs and memory sampler report. Tokens read in/out/cache;
// cost is reported or marked missing, never estimated.
func agentRow(a AgentSnap) table.Row {
	rss := "—"
	if a.HasRSS {
		rss = formatBytes(a.RSS)
	}
	tokens := formatCount(a.Input) + "/" + formatCount(a.Output) + "/" +
		formatCount(a.CacheRead+a.CacheWrite)
	return table.Row{
		a.ID, roleShort(a.Kind), a.Harness, a.ModelEffort(), agentStatus(a),
		tokens, costString(a.Cost, a.HasCost), strconv.Itoa(a.FindCalls), rss,
	}
}

// orchestratorLine is the spend line over the tickets pane: the selected
// spec's own agent, so its cost stays visible beside its tickets. Empty
// without a run.
func orchestratorLine(s SpecSnap) string {
	if s.Orchestrator == nil {
		return ""
	}
	o := s.Orchestrator
	return styleDim.Render(fmt.Sprintf("orchestrator · %s %s · %s",
		o.Harness, o.ModelEffort(), totalsBrief(agentTotals(*o))))
}

// agentTotals folds one agent into Totals for the shared brief line.
func agentTotals(a AgentSnap) Totals {
	t := Totals{}
	t.add(a)
	return t
}

// detailTotalsLine is the detail's total row: the ticket's summed tokens,
// reported cost and RSS, with its find calls.
func detailTotalsLine(t Totals) string {
	out := fmt.Sprintf("total %s in · %s out · %s cache · %s",
		formatCount(t.Input), formatCount(t.Output),
		formatCount(t.CacheRead+t.CacheWrite), costString(t.Cost, t.HasCost))
	if t.FindCalls > 0 {
		out += fmt.Sprintf(" · find %d", t.FindCalls)
		if t.HasFindCost {
			out += " " + formatCost(t.FindCost)
		}
	}
	if t.HasRSS {
		out += " · " + formatBytes(t.RSS)
	}
	return out
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
// and the selected spec's tickets on the right under its orchestrator line;
// an open ticket shows its Markdown, its agents and its totals instead.
func (m Model) View() string {
	tabs := "Runs  Settings"
	if m.tab == tabRuns {
		tabs = styleTab.Render("Runs") + "  Settings"
	} else {
		tabs = "Runs  " + styleTab.Render("Settings")
	}
	mem := machineLine(m.snap.MachineRSS)
	help := styleDim.Render("↑/k ↓/j move • ←/→ or h/l pane • enter detail • click select, double-click open • tab tabs • q quit • 1s refresh • " + mem)
	if m.tab == tabSettings {
		help = styleDim.Render(m.settingsHelp() + " • " + mem)
	}
	if m.detail != nil {
		help = styleDim.Render("↑↓/j k scroll • esc back • q quit • 1s refresh • " + mem)
	}
	if m.err != nil {
		help = styleDim.Render("refresh failed: " + m.err.Error() + " • retrying")
	}
	var body string
	switch {
	case m.tab == tabSettings:
		body = m.viewSettings()
	case m.detail != nil:
		body = m.detailView()
	default:
		right := m.tickets.View()
		if si, ok := m.list.SelectedItem().(specItem); ok {
			if line := orchestratorLine(si.spec); line != "" {
				right = lipgloss.JoinVertical(lipgloss.Left, line, right)
			}
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.list.View(), right)
	}
	return m.zone.Scan(tabs + "\n" + help + "\n" + body)
}

// detailView is the open ticket: its title, the scrollable Markdown, the
// agent table and the total row. A ticket the snapshot lost reads closed.
func (m Model) detailView() string {
	tk, ok := m.findDetail()
	if !ok {
		return "the ticket is gone • esc back"
	}
	title := tk.Title
	if title == "" {
		title = tk.Name
	}
	parts := []string{styleSel.Render(title), m.md.View(), m.agentRows.View(), detailTotalsLine(tk.Totals)}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
