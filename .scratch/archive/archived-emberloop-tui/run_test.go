package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	el "example.com/emberloop"
	tea "github.com/charmbracelet/bubbletea"
)

// cannedRunRows is a fixed run pane: a running ticket with verdicts, a
// needs-you ticket with a refusal, and a merged ticket.
func cannedRunRows() []RunTicket {
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	return []RunTicket{
		{Row: el.DriverRow{
			ID: "01-first", Path: "/spec/issues/01-first.md", Num: 1,
			State: el.DriverRowRunning, Worker: "01-first", Pane: "pane-w1",
			Tip: "tip1", Required: []string{"reviewer-standards", "reviewer-spec"},
			Verdicts: map[string]el.DriverReview{
				"reviewer-standards": {Verdict: "pass", At: "tip1"},
			},
			Since: at.Add(-3 * time.Minute),
		}, Agents: []AgentSnap{
			{ID: "w-one", Kind: AgentWorker, Harness: "pig", Pane: "pane-w1"},
		}},
		{Row: el.DriverRow{
			ID: "02-second", Path: "/spec/issues/02-second.md", Num: 2,
			State: el.DriverRowNeedsYou, Worker: "02-second", Pane: "pane-w2",
			Tip: "tip2", Required: []string{"reviewer-standards", "reviewer-spec"},
			Verdicts: map[string]el.DriverReview{
				"reviewer-standards": {Verdict: "pass", At: "tip2"},
				"reviewer-spec":      {Verdict: "changes", At: "tip2"},
			},
			Since:       at.Add(-65 * time.Minute),
			LastRefusal: "conflict in work.go",
		}, Agents: []AgentSnap{
			{ID: "w-two", Kind: AgentWorker, Harness: "pig", Pane: "pane-w2"},
		}},
		{Row: el.DriverRow{
			ID: "03-third", Path: "/spec/issues/03-third.md", Num: 3,
			State: el.DriverRowMerged,
			Since: at,
		}},
	}
}

// runLoader serves rows and counts calls, so reload tests see polling.
func runLoader(rows *[]RunTicket, calls *int) RunLoader {
	return func() ([]RunTicket, error) {
		*calls++
		return append([]RunTicket(nil), *rows...), nil
	}
}

// loadRunModel builds a run model over canned rows and runs its initial
// load the way the runtime would.
func loadRunModel(t *testing.T, rows *[]RunTicket, calls *int) RunModel {
	t.Helper()
	m := NewRunModel(runLoader(rows, calls))
	loaded, err := m.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	updated, _ := m.Update(runSnapMsg{rows: loaded})
	m = updated.(RunModel)
	if len(m.rows) != len(*rows) {
		t.Fatalf("rows = %d, want %d", len(m.rows), len(*rows))
	}
	m.now = func() time.Time { return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC) }
	return m
}

// runCmds drives one command's data messages back through the run model the
// way the runtime does. Ticks stay with the runtime's rescheduling.
func driveRunCmds(m RunModel, cmd tea.Cmd) RunModel {
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
		case runSnapMsg, runErrMsg:
			updated, next := m.Update(msg)
			m = updated.(RunModel)
			pending = append(pending, next)
		}
	}
	return m
}

func pressRunKey(m RunModel, s string) (RunModel, tea.Cmd) {
	updated, cmd := m.Update(keyMsg(s))
	return updated.(RunModel), cmd
}

func selectedRunID(m RunModel) string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rows[m.cursor].Row.ID
}

// Every row shows its state, worker, verdicts at the tip and time in
// state; the needs-you row shows the last refusal text.
func TestRunPaneRendersEveryRow(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	view := m.View()
	for _, want := range []string{
		"01-first", "running", "01-first", "std:pass", "spec:…", "3m",
		"02-second", "needs you", "conflict in work.go",
		"03-third", "merged",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view must contain %q, got:\n%s", want, view)
		}
	}
}

// One row per ticket in every state: queued, running, in review, merging,
// needs you, parked, merged.
func TestRunPaneRendersEveryState(t *testing.T) {
	states := []el.DriverRowState{
		el.DriverRowQueued,
		el.DriverRowRunning,
		el.DriverRowInReview,
		el.DriverRowMerging,
		el.DriverRowNeedsYou,
		el.DriverRowParked,
		el.DriverRowMerged,
	}
	var rows []RunTicket
	for i, s := range states {
		rows = append(rows, RunTicket{Row: el.DriverRow{
			ID:    "0" + string(rune('1'+i)) + "-ticket",
			State: s,
			Since: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
		}})
	}
	var calls int
	m := loadRunModel(t, &rows, &calls)
	view := m.View()
	for _, s := range states {
		if !strings.Contains(view, runStateLabel(s)) {
			t.Errorf("view must contain state %q, got:\n%s", runStateLabel(s), view)
		}
	}
	for i := range states {
		id := "0" + string(rune('1'+i)) + "-ticket"
		if strings.Count(view, id+" ") != 1 {
			t.Errorf("view must hold one row for %q, got:\n%s", id, view)
		}
	}
}

// Enter jumps to the selected ticket worker's herdr pane through the jump
// seam, leaving the pane running.
func TestRunPaneEnterJumpsToWorkerPane(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, map[string]string{"pane-w1": "tab-a"}))

	m, _ = pressRunKey(m, "enter")
	want := []string{"herdr pane list", "herdr tab focus tab-a", "herdr agent focus pane-w1"}
	if !reflect.DeepEqual(jcalls, want) {
		t.Fatalf("jump calls = %q, want %q", jcalls, want)
	}
	if m.notice != "" {
		t.Fatalf("notice = %q, want none after a clean jump", m.notice)
	}
}

// A worker whose pane herdr no longer lists shows the gone message instead
// of focusing anything.
func TestRunPaneEnterOnGonePaneShowsGoneMessage(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	var jcalls []string
	m.SetJumpExec(jumpExecWith(&jcalls, map[string]string{}))

	m, _ = pressRunKey(m, "enter")
	if !reflect.DeepEqual(jcalls, []string{"herdr pane list"}) {
		t.Fatalf("jump calls = %q, want only the lookup", jcalls)
	}
	if m.notice != "worker 01-first's pane is gone" {
		t.Fatalf("notice = %q, want the gone message", m.notice)
	}
}

// j and k move through the tickets; the agents view below follows the
// selected ticket.
func TestRunPaneKeysMoveAndAgentsFollow(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	if got := selectedRunID(m); got != "01-first" {
		t.Fatalf("initial selection = %q, want 01-first", got)
	}
	m, _ = pressRunKey(m, "j")
	if got := selectedRunID(m); got != "02-second" {
		t.Fatalf("after j selection = %q, want 02-second", got)
	}
	if view := m.View(); !strings.Contains(view, "w-two") {
		t.Fatalf("agents view must follow the selected ticket, got:\n%s", view)
	}
	m, _ = pressRunKey(m, "k")
	if got := selectedRunID(m); got != "01-first" {
		t.Fatalf("after k selection = %q, want 01-first", got)
	}
}

// p parks the selected ticket through the park command and reloads.
func TestRunPaneParkParksTheSelectedTicket(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	var parked []string
	m.SetPark(func(path string) error {
		parked = append(parked, path)
		return nil
	})

	m, cmd := pressRunKey(m, "p")
	if !reflect.DeepEqual(parked, []string{"/spec/issues/01-first.md"}) {
		t.Fatalf("parked = %q, want the selected ticket path", parked)
	}
	if m.notice != "parked 01-first" {
		t.Fatalf("notice = %q, want the park confirmation", m.notice)
	}
	m = driveRunCmds(m, cmd)
	if calls < 2 {
		t.Fatalf("loader calls = %d, want a reload after parking", calls)
	}
}

// A failing park shows the error and parks nothing else.
func TestRunPaneParkFailureShowsTheError(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	m.SetPark(func(path string) error { return errors.New("ticket is gone") })

	m, _ = pressRunKey(m, "p")
	if m.notice != "park failed: ticket is gone" {
		t.Fatalf("notice = %q, want the park error", m.notice)
	}
}

// A failing load keeps the last rows and retries on the next tick.
func TestRunPaneLoadFailureKeepsLastRows(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	updated, _ := m.Update(runErrMsg{err: errors.New("herdr down")})
	m = updated.(RunModel)
	if len(m.rows) != 3 {
		t.Fatalf("rows = %d, want the last rows kept", len(m.rows))
	}
	if view := m.View(); !strings.Contains(view, "01-first") {
		t.Fatalf("view must keep the last rows, got:\n%s", view)
	}
}

// RunLiveRows joins the shared driver rows with the snapshot's agents by
// ticket path: states and order come from the run, agents from the state
// dir. The loop runs through the driver seam with a fake world and clock.
func TestRunLiveRowsJoinsAgentsByTicketPath(t *testing.T) {
	dir := t.TempDir()
	first := dir + "/01-first.md"
	second := dir + "/02-second.md"
	files := map[string]string{
		first:  "# 01\n\n**Blocked by:** None\n",
		second: "# 02\n\n**Blocked by:** None\n",
	}
	spec := el.SpecRun{Spec: dir + "/spec.md", Tickets: dir, Branch: "spec/run-join"}
	world := el.DriverWorld{
		Files: files,
		Now:   func() time.Time { return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC) },
		StartWorker: func(tk el.DriverTicket) error {
			return nil
		},
		Tip: func(id string) string { return "tip-" + id },
		Reviews: func(id string) map[string]el.DriverReview {
			if id == "02-second" {
				return nil
			}
			return map[string]el.DriverReview{
				"reviewer-standards": {Verdict: "pass", At: "tip-" + id},
				"reviewer-spec":      {Verdict: "pass", At: "tip-" + id},
			}
		},
		Finish: func(id string) (el.DriverFinish, error) {
			return el.DriverFinish{Merged: true, SHA: "sha-" + id}, nil
		},
	}
	run := el.NewDriverRun(world.Now)
	world.Run = run
	if _, err := el.RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	snap := Snap{Repos: []RepoSnap{
		{Path: dir, Specs: []SpecSnap{
			{Slug: "run-join", Tickets: []TicketSnap{
				{Path: first, Name: "01-first.md", Agents: []AgentSnap{
					{ID: "w-one", Kind: AgentWorker, Harness: "pig"},
				}},
				{Path: second, Name: "02-second.md"},
			}},
		}},
	}}
	joined := RunLiveRows(run.Snapshot(), snap)
	if len(joined) != 2 {
		t.Fatalf("joined = %d rows, want one per ticket", len(joined))
	}
	if joined[0].Row.State != el.DriverRowMerged {
		t.Errorf("first state = %q, want merged", joined[0].Row.State)
	}
	if len(joined[0].Agents) != 1 || joined[0].Agents[0].ID != "w-one" {
		t.Errorf("first agents = %+v, want the snapshot worker", joined[0].Agents)
	}
	if joined[1].Row.State != el.DriverRowRunning {
		t.Errorf("second state = %q, want running", joined[1].Row.State)
	}
	if len(joined[1].Agents) != 0 {
		t.Errorf("second agents = %+v, want none", joined[1].Agents)
	}
}

// The finished loop quits the pane with its report, so the command prints
// where the run ended.
func TestRunPaneQuitsWithReportOnDone(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	updated, cmd := m.Update(RunDoneMsg{Report: "merged 01-first as sha\n"})
	if cmd == nil {
		t.Fatal("done must quit the pane")
	}
	if got := updated.(RunModel).report; got != "merged 01-first as sha\n" {
		t.Fatalf("report = %q, want the final report", got)
	}
}

// p on a merged row refuses with an already notice and parks nothing: the
// merge record stays untouched.
func TestRunPaneParkOnMergedShowsAlready(t *testing.T) {
	rows := cannedRunRows()
	var calls int
	m := loadRunModel(t, &rows, &calls)
	var parked []string
	m.SetPark(func(path string) error {
		parked = append(parked, path)
		return nil
	})
	m, _ = pressRunKey(m, "j")
	m, _ = pressRunKey(m, "j")
	if got := selectedRunID(m); got != "03-third" {
		t.Fatalf("selection = %q, want the merged row", got)
	}
	m, _ = pressRunKey(m, "p")
	if m.notice != "ticket already merged" {
		t.Fatalf("notice = %q, want the already refusal", m.notice)
	}
	if len(parked) != 0 {
		t.Fatalf("parked = %q, want no park call", parked)
	}
}
