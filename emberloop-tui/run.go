package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	emberloop "example.com/emberloop"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// RunTicket is one run pane row: the shared driver row (state, worker and
// pane, verdicts at the tip, state entry time, last refusal) plus the
// ticket's agents for the agents view below the table.
type RunTicket struct {
	Row    emberloop.DriverRow
	Agents []AgentSnap
}

// RunLoader reloads the run pane rows. The live run mode passes a closure
// over the shared driver run state; tests pass canned rows.
type RunLoader func() ([]RunTicket, error)

// RunParker parks the ticket file at path through the park command. The
// live run mode passes the driver's park; tests pass a fake.
type RunParker func(path string) error

// RunModel is the run pane: the ticket table on top (one row per ticket
// with state, worker, reviewer verdicts at the current tip, time in state,
// and the last refusal text while needs-you) and the existing agents view
// below. Enter jumps to the selected worker's herdr pane through the jump
// seam; p parks the selected ticket through the park command.
type RunModel struct {
	load   RunLoader
	rows   []RunTicket
	cursor int
	jump   CommandRunner
	park   RunParker
	now    func() time.Time
	notice string
	err    error
	report string
	width  int
	height int
	agents table.Model
}

// NewRunModel builds the run pane over load. The agents table starts empty;
// the first load fills it from the selected ticket.
func NewRunModel(load RunLoader) RunModel {
	tb := table.New(table.WithColumns(agentColumns()))
	tb.SetWidth(agentTableWidth())
	tb.SetHeight(1)
	return RunModel{load: load, now: time.Now, width: 80, height: 24, agents: tb}
}

// SetJumpExec wires the pane-jump seam: Enter focuses the selected worker's
// herdr tab and pane through this runner. Tests pass a fake; nil jumps
// nowhere.
func (m *RunModel) SetJumpExec(run CommandRunner) { m.jump = run }

// SetPark wires the park command: p parks the selected ticket file through
// it. Tests pass a fake; nil parks nothing.
func (m *RunModel) SetPark(park RunParker) { m.park = park }

type runTickMsg struct{}
type runSnapMsg struct{ rows []RunTicket }
type runErrMsg struct{ err error }

// RunDoneMsg ends the run pane: the driver loop finished with the final
// report, or failed. The pane quits so the command prints the report.
type RunDoneMsg struct {
	Report string
	Err    error
}

// RunLiveRows joins the shared driver rows with the dashboard snapshot's
// agents: each row keeps the run's order and state and carries the agents
// the snapshot attaches to the same ticket file. Tickets the snapshot no
// longer names carry no agents.
func RunLiveRows(rows []emberloop.DriverRow, snap Snap) []RunTicket {
	byPath := map[string]TicketSnap{}
	for _, repo := range snap.Repos {
		for _, spec := range repo.Specs {
			for _, tk := range spec.Tickets {
				byPath[tk.Path] = tk
			}
		}
	}
	out := make([]RunTicket, 0, len(rows))
	for _, r := range rows {
		t := RunTicket{Row: r}
		if tk, ok := byPath[r.Path]; ok {
			t.Agents = tk.Agents
		}
		out = append(out, t)
	}
	return out
}

func runTickCmd() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return runTickMsg{} })
}

func (m RunModel) runLoadCmd() tea.Cmd {
	return func() tea.Msg {
		rows, err := m.load()
		if err != nil {
			return runErrMsg{err}
		}
		return runSnapMsg{rows: rows}
	}
}

// Init loads the first rows and starts the one-second poll.
func (m RunModel) Init() tea.Cmd {
	return tea.Batch(m.runLoadCmd(), runTickCmd())
}

// Update drives the run pane: j/k and arrows move, enter jumps to the
// worker's pane, p parks the ticket, q quits, and every tick reloads the
// rows. A failing load keeps the last rows and retries on the next tick.
func (m RunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case runTickMsg:
		return m, tea.Batch(m.runLoadCmd(), runTickCmd())
	case runSnapMsg:
		m.rows, m.err = msg.rows, nil
		m.clampCursor()
		m.syncAgents()
		return m, nil
	case runErrMsg:
		m.err = msg.err
		return m, nil
	case RunDoneMsg:
		m.report = msg.Report
		if msg.Err != nil {
			m.notice = msg.Err.Error()
		}
		return m, tea.Quit
	case tea.KeyMsg:
		m.notice = ""
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyUp:
			m.move(-1)
			return m, nil
		case tea.KeyDown:
			m.move(1)
			return m, nil
		case tea.KeyEnter:
			m.jumpSelected()
			return m, nil
		case tea.KeyRunes:
			if len(msg.Runes) == 1 {
				switch msg.Runes[0] {
				case 'q':
					return m, tea.Quit
				case 'k':
					m.move(-1)
					return m, nil
				case 'j':
					m.move(1)
					return m, nil
				case 'p':
					return m, m.parkSelected()
				}
			}
		}
	}
	return m, nil
}

// move steps the ticket selection, keeping it inside the rows.
func (m *RunModel) move(d int) {
	m.cursor += d
	m.clampCursor()
	m.syncAgents()
}

// clampCursor keeps the selection on a row, holding the ticket across
// reloads by id when it is still there.
func (m *RunModel) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

// syncAgents points the agents view at the selected ticket's agents.
func (m *RunModel) syncAgents() {
	var rows []table.Row
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		for _, a := range m.rows[m.cursor].Agents {
			rows = append(rows, agentRow(a))
		}
	}
	m.agents.SetRows(rows)
	m.agents.SetHeight(max(2, len(rows)+1))
	m.agents.GotoTop()
}

// jumpSelected focuses the selected worker's herdr tab and pane through the
// jump seam. A pane herdr no longer lists sets the gone notice instead of
// focusing anything.
func (m *RunModel) jumpSelected() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	row := m.rows[m.cursor].Row
	if m.jump == nil {
		return
	}
	if err := focusAgentPane(m.jump, row.Pane); err != nil {
		if errors.Is(err, errPaneGone) {
			m.notice = "worker " + row.ID + "'s pane is gone"
		} else {
			m.notice = "jump failed: " + err.Error()
		}
		return
	}
	m.notice = ""
}

// parkSelected parks the selected ticket through the park command and
// reloads the rows, so the parked state shows on the next tick. A merged or
// discarded row refuses with an already notice and parks nothing, so the
// merge record stays untouched.
func (m *RunModel) parkSelected() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	row := m.rows[m.cursor].Row
	if row.State == emberloop.DriverRowMerged || row.State == emberloop.DriverRowDiscarded {
		m.notice = "ticket already " + runStateLabel(row.State)
		return nil
	}
	if m.park == nil {
		m.notice = "parking is not wired"
		return nil
	}
	if err := m.park(row.Path); err != nil {
		m.notice = "park failed: " + err.Error()
		return nil
	}
	m.notice = "parked " + row.ID
	return m.runLoadCmd()

}

// runStateLabel is the one place a driver row state becomes its pane text.
func runStateLabel(s emberloop.DriverRowState) string {
	return strings.ReplaceAll(string(s), "-", " ")
}

// runReviewerShort shortens a reviewer type the way the agent table
// shortens roles.
func runReviewerShort(typ string) string {
	switch typ {
	case "reviewer-standards":
		return "std"
	case "reviewer-spec":
		return "spec"
	}
	return typ
}

// runVerdicts formats the reviewer verdicts at the current tip in required
// order: pass or changes at the tip, … while pending or stale.
func runVerdicts(row emberloop.DriverRow) string {
	required := row.Required
	if len(required) == 0 {
		for typ := range row.Verdicts {
			required = append(required, typ)
		}
		sort.Strings(required)
	}
	parts := make([]string, 0, len(required))
	for _, typ := range required {
		v := row.Verdicts[typ]
		cell := "…"
		if v.Verdict != "" && v.At == row.Tip {
			cell = v.Verdict
		}
		parts = append(parts, runReviewerShort(typ)+":"+cell)
	}
	return strings.Join(parts, " ")
}

// formatRunAge renders how long a ticket has been in its state.
func formatRunAge(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}

// runWorker names the ticket's worker, or a dash while queued.
func runWorker(row emberloop.DriverRow) string {
	if row.Worker == "" {
		return "—"
	}
	return row.Worker
}

// runTicketLine is one ticket table row: state, ticket, worker, verdicts
// and time in state. A needs-you ticket carries its last refusal on the
// line after, so the pane shows why it needs you.
func (m RunModel) runTicketLine(row emberloop.DriverRow) string {
	age := "—"
	if !row.Since.IsZero() {
		age = formatRunAge(m.now().Sub(row.Since))
	}
	return fmt.Sprintf("%s %s %s %s %s",
		runStateLabel(row.State), row.ID, runWorker(row), runVerdicts(row), age)
}

// View renders the ticket table on top and the selected ticket's agents
// below it, with the notice or load error under.
func (m RunModel) View() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("run · %d tickets\n", len(m.rows)))
	for i, t := range m.rows {
		mark := "  "
		if i == m.cursor {
			mark = "> "
		}
		b.WriteString(mark + m.runTicketLine(t.Row) + "\n")
		if t.Row.State == emberloop.DriverRowNeedsYou && t.Row.LastRefusal != "" {
			b.WriteString("    ! " + t.Row.LastRefusal + "\n")
		}
	}
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		b.WriteString("agents of " + m.rows[m.cursor].Row.ID + "\n")
		b.WriteString(m.agents.View() + "\n")
	}
	if m.err != nil {
		b.WriteString("refresh failed: " + m.err.Error() + " • retrying\n")
	}
	if m.notice != "" {
		b.WriteString(m.notice + "\n")
	}
	b.WriteString("↑/k ↓/j move • enter jump • p park • q quit\n")
	return b.String()
}
