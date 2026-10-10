package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7")).Bold(true)
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
	good   = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
	warn   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af"))
	bad    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
)

type option struct {
	label, desc string
	on          bool
}

type phase int

const (
	phaseSelect phase = iota
	phaseRun
	phaseDone
)

type stepState int

const (
	statePending stepState = iota
	stateRunning
	stateOK
	stateWarn
	stateFail
)

type stepResult struct {
	state  stepState
	detail string
}

type stepDoneMsg struct {
	idx int
	res stepResult
}

type model struct {
	env     *Env
	opts    []option
	cursor  int
	phase   phase
	steps   []Step
	results []stepResult
	current int
	quit    bool
	failed  bool
}

func newModel(env *Env, c Choices) *model {
	return &model{env: env, opts: []option{
		{"pig", "Pig harness: MCP entry and Antigravity extension", c.Pig},
		{"claude", "Claude Code harness: installed if missing, wired per run", c.Claude},
		{"memory", "Code graph MCP (codebase-memory-mcp) for reviewers and scouts", c.Graph},
	}}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) choices() Choices {
	return Choices{Pig: m.opts[0].on, Claude: m.opts[1].on, Graph: m.opts[2].on}
}

func (m *model) runStep(i int) tea.Cmd {
	step, env := m.steps[i], m.env
	return func() tea.Msg {
		detail, err := step.Run(env)
		var w warning
		switch {
		case err == nil:
			return stepDoneMsg{i, stepResult{stateOK, detail}}
		case asWarning(err, &w):
			return stepDoneMsg{i, stepResult{stateWarn, w.msg}}
		default:
			return stepDoneMsg{i, stepResult{stateFail, err.Error()}}
		}
	}
}

func asWarning(err error, w *warning) bool {
	ww, ok := err.(warning)
	if ok {
		*w = ww
	}
	return ok
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			if m.phase != phaseRun {
				m.quit = true
				return m, tea.Quit
			}
		}
		if m.phase == phaseSelect {
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.opts)-1 {
					m.cursor++
				}
			case " ", "x":
				m.opts[m.cursor].on = !m.opts[m.cursor].on
			case "enter":
				m.env.Choices = m.choices()
				m.steps = Steps(m.env.Choices)
				m.results = make([]stepResult, len(m.steps))
				m.phase = phaseRun
				m.results[0].state = stateRunning
				return m, m.runStep(0)
			}
		} else if m.phase == phaseDone && msg.String() == "enter" {
			return m, tea.Quit
		}
	case stepDoneMsg:
		m.results[msg.idx] = msg.res
		if msg.res.state == stateFail {
			m.failed = true
			m.phase = phaseDone
			return m, nil
		}
		if msg.idx+1 >= len(m.steps) {
			m.phase = phaseDone
			return m, nil
		}
		m.current = msg.idx + 1
		m.results[m.current].state = stateRunning
		return m, m.runStep(m.current)
	}
	return m, nil
}

func (m *model) View() string {
	var b strings.Builder
	b.WriteString(accent.Render("el installer") + "\n")
	b.WriteString(dim.Render("the binaries and configs always install.") + "\n\n")
	switch m.phase {
	case phaseSelect:
		b.WriteString("Choose what to install:\n\n")
		for i, o := range m.opts {
			cur, box := "  ", "[ ]"
			if i == m.cursor {
				cur = accent.Render("> ")
			}
			if o.on {
				box = good.Render("[x]")
			}
			fmt.Fprintf(&b, "%s%s %s  %s\n", cur, box, o.label, dim.Render(o.desc))
		}
		b.WriteString("\n" + dim.Render("up/down move · space toggle · enter install · q quit") + "\n")
	default:
		for i, s := range m.steps {
			mark := dim.Render("·")
			switch m.results[i].state {
			case stateRunning:
				mark = accent.Render("…")
			case stateOK:
				mark = good.Render("✓")
			case stateWarn:
				mark = warn.Render("!")
			case stateFail:
				mark = bad.Render("✗")
			}
			fmt.Fprintf(&b, "%s %s", mark, s.Name)
			if d := m.results[i].detail; d != "" {
				fmt.Fprintf(&b, "  %s", dim.Render(d))
			}
			b.WriteString("\n")
		}
		if m.phase == phaseDone {
			if m.failed {
				b.WriteString("\n" + bad.Render("install stopped: fix the error above and rerun") + "\n")
			} else {
				b.WriteString("\n" + dim.Render("press enter to finish dependency checks") + "\n")
			}
		}
	}
	return b.String()
}
