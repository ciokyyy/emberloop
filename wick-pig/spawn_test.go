package wick

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// spawnToolForRole finds the spawn tool a role serves, the way a caller gets it
// over tools/list.
func spawnToolForRole(t *testing.T, role string) (func(string, map[string]any) (string, error), string) {
	t.Helper()
	served, err := RoleTools(role)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range served {
		if tool.Name() != "spawn" {
			continue
		}
		return func(root string, params map[string]any) (string, error) {
			raw, _ := json.Marshal(params)
			return tool.Execute(context.Background(), root, raw)
		}, tool.Description()
	}
	t.Fatalf("%s does not serve spawn", role)
	return nil, ""
}

// spawnTestState points the submit store at a temp dir and silences the launch
// spacing, restoring both after.
func spawnTestState(t *testing.T) {
	t.Helper()
	oldState, oldGap, oldExec, oldPoll := statePath, spawnGap, spawnExec, pollEvery
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	pollEvery = 5 * time.Millisecond
	t.Cleanup(func() { statePath, spawnGap, spawnExec, pollEvery = oldState, oldGap, oldExec, oldPoll })
}

// envIn reads an environment assignment (WICK_AGENT_ID=...) out of a launch
// line.
func envIn(line, key string) string {
	for _, field := range strings.Fields(line) {
		if v, ok := strings.CutPrefix(field, key+"="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// scoutExec launches scouts in a fake grid, and writes each one's submit once
// every scout in the call has launched, so the reports can only come back after
// all three are running.
type scoutExec struct {
	g        *fakeGrid
	mu       sync.Mutex
	ids      []string
	lines    []string
	launched int
	// writeOn is the launch count at which the submits are written.
	writeOn int
}

func (s *scoutExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
		id := envIn(args[3], "WICK_AGENT_ID")
		s.mu.Lock()
		s.ids = append(s.ids, id)
		s.lines = append(s.lines, args[3])
		s.launched++
		ready := s.launched == s.writeOn
		s.mu.Unlock()
		if ready {
			// Each scout reports through the real submit tool, so the spawn
			// waits on exactly what a live agent writes.
			for i, aid := range s.ids {
				tool := submitToolFor(AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}, aid)
				raw, _ := json.Marshal(map[string]any{"status": "answered", "report": fmt.Sprintf("report-%d", i+1)})
				if _, err := tool.Execute(context.Background(), "", raw); err != nil {
					return Result{}, err
				}
			}
		}
		return Result{Stdout: "{}"}, nil
	}
	return s.g.run(cmd, args, opts)
}

// One spawn with three scouts launches all three, waits for every one of them,
// and returns their reports together, in the order they were listed.
func TestSpawnThreeScoutsReturnsTheirReportsInOrder(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	ex := &scoutExec{g: g, writeOn: 3}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	out, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first question"},
		map[string]any{"type": "scout", "prompt": "second question"},
		map[string]any{"type": "scout", "prompt": "third question"},
	}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if ex.launched != 3 {
		t.Fatalf("launched %d scouts, want 3", ex.launched)
	}
	first, second, third := strings.Index(out, "report-1"), strings.Index(out, "report-2"), strings.Index(out, "report-3")
	if first < 0 || second < 0 || third < 0 {
		t.Fatalf("the result must carry every report, got:\n%s", out)
	}
	if !(first < second && second < third) {
		t.Errorf("reports must come back in list order, got:\n%s", out)
	}
	if strings.Contains(out, "end your turn") {
		t.Errorf("a spawn of blocking scouts that all submitted must not say to end the turn, got:\n%s", out)
	}
}

// A scout runs in the repo, with only the read-only tools its type names plus
// submit, and its pane is placed by the grid beside the caller.
func TestScoutRunsReadOnlyInTheCallersGrid(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	caller := g.tabs[0].order[0]
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "plan")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "where is it?"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	var splitSeen bool
	for _, call := range g.calls {
		if strings.Contains(call, "pane split") {
			splitSeen = true
			if !strings.Contains(call, "--pane "+caller) || !strings.Contains(call, "--direction right") {
				t.Errorf("a scout must be placed beside the caller by the grid: %s", call)
			}
		}
	}
	if !splitSeen {
		t.Error("the scout launch issued no pane split")
	}
	// The launch carries the role, agent type and id its MCP server needs, and
	// no read exception: read opens any absolute path on its own, the prompt
	// file included.
	if len(ex.ids) != 1 || ex.ids[0] == "" {
		t.Fatalf("the scout launch carried no agent id: %v", ex.ids)
	}
	if strings.Contains(ex.lines[0], "WICK_READ_ALLOW") {
		t.Errorf("a scout launch must not carry a read exception:\n%s", ex.lines[0])
	}
	if got := envIn(ex.lines[0], "WICK_AGENT_TYPE"); got != "scout" {
		t.Errorf("the scout launch must name its type, got %q", got)
	}
	if got := envIn(ex.lines[0], "WICK_ROLE"); got != "agent" {
		t.Errorf("the scout launch must run the agent role, got %q", got)
	}
}

// Without HERDR_TAB_ID, the caller's own pane names the tab, so a spawn still
// places the scout beside the caller rather than in the focused pane.
func TestSpawnFindsTheCallersTabFromItsPane(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	caller := g.tabs[0].order[0]
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", caller)
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "plan")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "where is it?"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	split := ""
	for _, call := range g.calls {
		if strings.Contains(call, "pane split") {
			split = call
			break
		}
	}
	if !strings.Contains(split, "--pane "+caller) {
		t.Errorf("the split must target the caller's pane named by HERDR_PANE_ID: %s", split)
	}
}

// A spawned agent's wick-mcp serves its type's tools, plus submit, and nothing
// that writes: scout is read-only.
func TestScoutAgentServesReadOnlyToolsAndSubmit(t *testing.T) {
	t.Setenv("WICK_AGENT_TYPE", "scout")
	t.Setenv("WICK_AGENT_ID", "a1")
	served, err := RoleTools("agent")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range served {
		got[tool.Name()] = true
	}
	for _, want := range []string{"read", "find", "submit"} {
		if !got[want] {
			t.Errorf("a scout agent must serve %q, got %v", want, got)
		}
	}
	for _, unwanted := range []string{"edit", "write", "bash", "background", "spawn", "code_review"} {
		if got[unwanted] {
			t.Errorf("a scout agent must not serve %q", unwanted)
		}
	}
}

// spawn's description lists only the types the caller may spawn, the scout
// tool is gone, and so is spawn_worker.
func TestSpawnDescriptionListsTheCallersTypes(t *testing.T) {
	_, desc := spawnToolForRole(t, "implement")
	for _, want := range []string{"scout", "worker"} {
		if !strings.Contains(desc, want) {
			t.Errorf("implement may spawn %s; the description must say so:\n%s", want, desc)
		}
	}
	served, _ := RoleTools("implement")
	for _, tool := range served {
		switch tool.Name() {
		case "scout", "spawn_worker":
			t.Fatalf("the %s tool must be gone", tool.Name())
		}
	}
}

// spawn refuses a type outside the caller's list before starting anything.
func TestSpawnRejectsATypeTheCallerMayNotSpawn(t *testing.T) {
	spawnTestState(t)
	types := map[string]AgentType{
		"scout":  {Name: "scout", Checkout: "none", Blocking: true, Statuses: []string{"answered"}},
		"custom": {Name: "custom", Checkout: "none", Blocking: true, Statuses: []string{"answered"}},
	}
	tool := spawnToolFor(types, []string{"scout"})
	raw, _ := json.Marshal(map[string]any{"agents": []any{
		map[string]any{"type": "custom", "prompt": "do it"},
	}})
	_, err := tool.Execute(context.Background(), "/repo", raw)
	if err == nil || !strings.Contains(err.Error(), "custom") {
		t.Errorf("an unspawnable type must be refused, got %v", err)
	}
}

// A blocking scout submits a full, multi-line report, and the submit is
// recorded as the agent's last submit and as submitted this turn.
func TestSubmitRecordsABlockingReport(t *testing.T) {
	spawnTestState(t)
	types, _ := LoadAgentTypes(t.TempDir())
	scout := types["scout"]
	tool := submitToolFor(scout, "a1")
	report := "line one\nline two\nline three"
	raw, _ := json.Marshal(map[string]any{"status": "answered", "report": report})
	if _, err := tool.Execute(context.Background(), "", raw); err != nil {
		t.Fatalf("submit: %v", err)
	}
	got, ok, err := ReadAgentSubmit("a1")
	if err != nil || !ok {
		t.Fatalf("the submit was not recorded: ok=%v err=%v", ok, err)
	}
	if got.Report != report {
		t.Errorf("a multi-line report must be kept intact, got %q", got.Report)
	}
	if got.Status != "answered" || !got.ThisTurn {
		t.Errorf("recorded submit = %+v", got)
	}
}

// submit rejects a status outside the type's, a missing report, and, for a
// non-blocking type, a missing, over-long or multi-line summary.
func TestSubmitRejectsBadInput(t *testing.T) {
	spawnTestState(t)
	types, _ := LoadAgentTypes(t.TempDir())
	scout := types["scout"]
	blocking := submitToolFor(scout, "a1")
	nonBlocking := submitToolFor(AgentType{
		Name: "reporter", Blocking: false, Statuses: []string{"done", "blocked"},
	}, "a2")

	cases := []struct {
		name string
		tool interface {
			Execute(context.Context, string, json.RawMessage) (string, error)
		}
		params map[string]any
		want   string
	}{
		{"wrong status", blocking, map[string]any{"status": "pass", "report": "x"}, "status"},
		{"missing report", blocking, map[string]any{"status": "answered"}, "report"},
		{"missing summary", nonBlocking, map[string]any{"status": "done"}, "summary"},
		{"multi-line summary", nonBlocking, map[string]any{"status": "done", "summary": "one\ntwo"}, "one line"},
		{"over-long summary", nonBlocking, map[string]any{"status": "done", "summary": strings.Repeat("x", summaryLimit+1)}, "limit"},
		{"wrong status for non-blocking", nonBlocking, map[string]any{"status": "answered", "summary": "x"}, "status"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, _ := json.Marshal(c.params)
			_, err := c.tool.Execute(context.Background(), "", raw)
			if err == nil {
				t.Fatalf("%s must be rejected", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the error must name %q, got %v", c.want, err)
			}
		})
	}
	// A valid one-line summary is accepted and recorded.
	raw, _ := json.Marshal(map[string]any{"status": "done", "summary": "all good"})
	if _, err := nonBlocking.Execute(context.Background(), "", raw); err != nil {
		t.Fatalf("a valid summary was refused: %v", err)
	}
	got, ok, _ := ReadAgentSubmit("a2")
	if !ok || got.Summary != "all good" || got.Status != "done" {
		t.Errorf("recorded summary = %+v ok=%v", got, ok)
	}
}

// A repo override of a built-in reaches the tools a spawned agent of that type
// is given: a tuned scout accepts its own status, not the built-in's.
func TestRepoOverrideReachesTheAgentTools(t *testing.T) {
	spawnTestState(t)
	root := t.TempDir()
	writeAgentFile(t, root, "scout.md", `---
name: scout
description: A tuned scout.
tools: [read]
checkout: none
blocking: true
statuses: [found]
---
Tuned.
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	tool := submitToolFor(types["scout"], "a1")
	answered, _ := json.Marshal(map[string]any{"status": "answered", "report": "x"})
	if _, err := tool.Execute(context.Background(), "", answered); err == nil {
		t.Error("the tuned scout must not accept the built-in status answered")
	}
	found, _ := json.Marshal(map[string]any{"status": "found", "report": "facts"})
	if _, err := tool.Execute(context.Background(), "", found); err != nil {
		t.Fatalf("the tuned scout must accept its own status: %v", err)
	}
}

// A submit is recorded as submitted this turn; the stop gate clears it when a
// new turn starts, so a later turn does not read the previous submit as this
// turn's.
func TestSubmitThisTurnCanBeCleared(t *testing.T) {
	spawnTestState(t)
	s := AgentSubmit{AgentID: "a1", Type: "scout", Status: "answered", Report: "x", ThisTurn: true}
	if err := WriteAgentSubmit(s); err != nil {
		t.Fatal(err)
	}
	if err := ClearAgentSubmitThisTurn("a1"); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := ReadAgentSubmit("a1")
	if !ok || got.ThisTurn {
		t.Errorf("after a new turn the submit must not count as this turn: %+v", got)
	}
	if got.Report != "x" || got.Status != "answered" {
		t.Errorf("clearing this turn must keep the last submit: %+v", got)
	}
}

// mixedExec launches scouts and a worker through one spawn. Both scouts are
// submitted once the second is running; the worker never submits, so only a
// call that starts non-blocking agents and waits on the blocking ones returns.
type mixedExec struct {
	g        *fakeGrid
	mu       sync.Mutex
	scouts   []string
	launched int
}

func (m *mixedExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 3 && args[0] == "pane" && args[1] == "run" {
		line := args[3]
		if envIn(line, "WICK_AGENT_TYPE") == "scout" {
			m.mu.Lock()
			m.scouts = append(m.scouts, envIn(line, "WICK_AGENT_ID"))
			m.launched++
			ready := m.launched == 2
			ids := append([]string{}, m.scouts...)
			m.mu.Unlock()
			if ready {
				for i, aid := range ids {
					tool := submitToolFor(AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}, aid)
					raw, _ := json.Marshal(map[string]any{"status": "answered", "report": fmt.Sprintf("report-%d", i+1)})
					if _, err := tool.Execute(context.Background(), "", raw); err != nil {
						return Result{}, err
					}
				}
			}
		}
		return Result{Stdout: "{}"}, nil
	}
	if cmd == "git" {
		return Result{Stdout: "abc123def\n"}, nil
	}
	return m.g.run(cmd, args, opts)
}

// A mixed spawn returns once both scouts have submitted, with the worker
// reported as started rather than waited on.
func TestMixedSpawnReturnsScoutsAndStartsTheWorker(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	ex := &mixedExec{g: g}
	spawnExec = ex.run

	repo := t.TempDir()
	ticket := filepath.Join(repo, "20-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawn, _ := spawnToolForRole(t, "implement")
	out, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
		map[string]any{"type": "worker", "ticket": ticket},
	}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if ex.launched != 2 {
		t.Fatalf("launched %d scouts, want 2", ex.launched)
	}
	first, second := strings.Index(out, "report-1"), strings.Index(out, "report-2")
	if first < 0 || second < 0 || first > second {
		t.Errorf("the scouts' reports must come back in list order, got:\n%s", out)
	}
	if !strings.Contains(out, "worker 20-x started") {
		t.Errorf("the worker must be reported as started, got:\n%s", out)
	}
	if w, err := recall("20-x"); err != nil || w.Branch != "implement/20-x" {
		t.Errorf("the worker entry must create its worktree and branch: %+v err=%v", w, err)
	}
	if !strings.Contains(out, "end your turn") {
		t.Errorf("a mixed spawn that starts a non-blocking worker must say to end the turn, got:\n%s", out)
	}
}

// firstScoutExec launches scouts and submits only the first, so the second
// outlasts the tool budget and is auto-backgrounded.
type firstScoutExec struct {
	g   *fakeGrid
	mu  sync.Mutex
	ids []string
}

func (s *firstScoutExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 3 && args[0] == "pane" && args[1] == "run" {
		s.mu.Lock()
		s.ids = append(s.ids, envIn(args[3], "WICK_AGENT_ID"))
		n, first := len(s.ids), s.ids[0]
		s.mu.Unlock()
		if n == 1 {
			tool := submitToolFor(AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}, first)
			raw, _ := json.Marshal(map[string]any{"status": "answered", "report": "report-1"})
			if _, err := tool.Execute(context.Background(), "", raw); err != nil {
				return Result{}, err
			}
		}
		return Result{Stdout: "{}"}, nil
	}
	return s.g.run(cmd, args, opts)
}

// With a budget too short for the second scout, the spawn returns the report
// already in and a line saying the rest arrive as notices and the caller
// should end its turn. The late submit is then delivered as a notice, and the
// parent's pending list clears.
func TestBlockingSpawnAutoBackgroundsPastTheBudget(t *testing.T) {
	spawnTestState(t)
	oldBudget, oldDeliver := toolBudget, deliverExec
	toolBudget = 60 * time.Millisecond
	t.Cleanup(func() { toolBudget, deliverExec = oldBudget, oldDeliver })
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("WICK_AGENT_ID", "parent1")
	ex := &firstScoutExec{g: g}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	out, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
	}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.Contains(out, "report-1") {
		t.Errorf("the spawn must return the report already in, got:\n%s", out)
	}
	if !strings.Contains(out, "still running") || !strings.Contains(out, "end your turn") {
		t.Errorf("the spawn must say the rest arrive as notices and to end the turn, got:\n%s", out)
	}
	if strings.Contains(out, "call") {
		t.Errorf("no response may say to call again, got:\n%s", out)
	}

	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("launched %d scouts, want 2", len(ids))
	}
	if got := PendingChildren("parent1"); !contains(got, ids[1]) {
		t.Errorf("the parent's state must list the backgrounded child: %v", got)
	}

	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec.run
	if _, err := submitOnce(t, AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}},
		ids[1], "answered", "report", "late facts"); err != nil {
		t.Fatalf("late submit: %v", err)
	}
	if len(rec.calls) != 1 || !strings.Contains(strings.Join(rec.calls[0], " "), "agent prompt wP ") {
		t.Errorf("a backgrounded submit must be delivered as a notice, got %v", rec.calls)
	}
	if got := PendingChildren("parent1"); len(got) != 0 {
		t.Errorf("the pending list must clear once the notice arrives, got %v", got)
	}
}

// A worker agent serves its type's host tools plus submit, so it can edit, run
// A worker agent serves every file tool, submit, and the spawn it uses to run
// its reviewer types. It never serves the review tool itself.
func TestWorkerAgentServesItsToolsAndSubmit(t *testing.T) {
	t.Setenv("WICK_AGENT_TYPE", "worker")
	t.Setenv("WICK_AGENT_ID", "w1")
	served, err := RoleTools("agent")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range served {
		got[tool.Name()] = true
	}
	for _, want := range []string{"read", "edit", "write", "bash", "find", "background", "submit", "spawn"} {
		if !got[want] {
			t.Errorf("a worker agent must serve %q, got %v", want, got)
		}
	}
	if got["code_review"] {
		t.Errorf("a worker agent must not serve code_review")
	}
}

// noneScoutExec launches scouts and never submits them, so every one is
// backgrounded.
type noneScoutExec struct {
	g   *fakeGrid
	mu  sync.Mutex
	ids []string
}

func (s *noneScoutExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 3 && args[0] == "pane" && args[1] == "run" {
		s.mu.Lock()
		s.ids = append(s.ids, envIn(args[3], "WICK_AGENT_ID"))
		s.mu.Unlock()
		return Result{Stdout: "{}"}, nil
	}
	return s.g.run(cmd, args, opts)
}

// Two backgrounded children clear from the parent's pending list one at a
// time, as each submits: the state is derived from the children, so a notice
// landing in its own wick-mcp process can never leave a delivered child behind.
func TestTwoBackgroundedChildrenClearIndependently(t *testing.T) {
	spawnTestState(t)
	oldBudget, oldDeliver := toolBudget, deliverExec
	toolBudget = 30 * time.Millisecond
	t.Cleanup(func() { toolBudget, deliverExec = oldBudget, oldDeliver })

	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("WICK_AGENT_ID", "parent1")
	ex := &noneScoutExec{g: g}
	spawnExec = ex.run

	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first"},
		map[string]any{"type": "scout", "prompt": "second"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	ex.mu.Lock()
	ids := append([]string{}, ex.ids...)
	ex.mu.Unlock()
	if len(ids) != 2 {
		t.Fatalf("launched %d scouts, want 2", len(ids))
	}
	if got := PendingChildren("parent1"); len(got) != 2 {
		t.Fatalf("both children must be pending, got %v", got)
	}

	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec.run
	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, ids[0], "answered", "report", "one"); err != nil {
		t.Fatal(err)
	}
	if got := PendingChildren("parent1"); len(got) != 1 || got[0] != ids[1] {
		t.Fatalf("one submit must clear only its own child, got %v", got)
	}
	if _, err := submitOnce(t, scout, ids[1], "answered", "report", "two"); err != nil {
		t.Fatal(err)
	}
	if got := PendingChildren("parent1"); len(got) != 0 {
		t.Errorf("all submitted children must clear, got %v", got)
	}
}

// persistentExec is a herdr whose read-only panes answer their prompt and submit
// through the real submit tool, the way a live scout does. A nudge is that
// agent's next turn: it submits again, so a spawn that reuses the agent can
// return the new report. It counts launches and nudges, so a test can tell a
// re-prompt from a relaunch.
type persistentExec struct {
	g        *fakeGrid
	mu       sync.Mutex
	launched int
	nudged   int
	byPane   map[string]string
	turns    int
}

func newPersistentExec(g *fakeGrid) *persistentExec {
	return &persistentExec{g: g, byPane: map[string]string{}}
}

func (p *persistentExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" {
		switch args[1] {
		case "run":
			p.mu.Lock()
			id := envIn(args[3], "WICK_AGENT_ID")
			p.byPane[args[2]] = id
			p.launched++
			p.mu.Unlock()
			return Result{Stdout: "{}"}, p.answer(id)
		case "send-text":
			p.mu.Lock()
			id := p.byPane[args[2]]
			p.nudged++
			p.mu.Unlock()
			return Result{Stdout: "{}"}, p.answer(id)
		}
	}
	if cmd == herdrBin() && len(args) > 1 && args[0] == "agent" && args[1] == "prompt" {
		// herdr does not prompt pig; Nudge falls back to typing.
		return Result{Code: 1}, nil
	}
	return p.g.run(cmd, args, opts)
}

// answer records the agent's next submit, numbered in turn order.
func (p *persistentExec) answer(id string) error {
	if id == "" {
		return nil
	}
	p.mu.Lock()
	p.turns++
	n := p.turns
	p.mu.Unlock()
	tool := submitToolFor(AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}, id)
	raw, _ := json.Marshal(map[string]any{"status": "answered", "report": fmt.Sprintf("report-%d", n)})
	_, err := tool.Execute(context.Background(), "", raw)
	return err
}

// scoutTypeFile is a repo type file named scout.md: read-only, blocking, with
// the persistence the test names.
func scoutTypeFile(t *testing.T, root, persistent string) map[string]AgentType {
	t.Helper()
	writeAgentFile(t, root, "scout.md", `---
name: scout
description: A scout that may stay put.
tools: [read]
checkout: none
blocking: true
persistent: `+persistent+`
statuses: [answered]
---
Answer: {{prompt}}
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	return types
}

// A persistent type keeps one agent per parent: re-spawning it re-prompts the
// live agent with the new prompt instead of launching a second one, and the
// spawn returns that agent's new report. A top-level session has no agent id,
// so its own pane is what names the parent.
func TestPersistentTypeIsRepromptedNotRelaunched(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("WICK_AGENT_ID", "")
	t.Setenv("HERDR_PANE_ID", "parentPane")
	ex := newPersistentExec(g)
	spawnExec = ex.run

	root := t.TempDir()
	tool := spawnToolFor(scoutTypeFile(t, root, "true"), []string{"scout"})
	spawn := func(params map[string]any) (string, error) {
		raw, _ := json.Marshal(params)
		return tool.Execute(context.Background(), root, raw)
	}

	first, err := spawn(map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first question"},
	}})
	if err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	if !strings.Contains(first, "report-1") {
		t.Fatalf("the first spawn must return the scout's report, got:\n%s", first)
	}

	second, err := spawn(map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "second question"},
	}})
	if err != nil {
		t.Fatalf("second spawn: %v", err)
	}
	if ex.launched != 1 {
		t.Errorf("a persistent type must reuse its agent, launched %d", ex.launched)
	}
	if ex.nudged != 1 {
		t.Errorf("a persistent type must be re-prompted, nudged %d", ex.nudged)
	}
	if !strings.Contains(second, "report-2") {
		t.Errorf("the second spawn must return the re-prompted agent's new report, got:\n%s", second)
	}

	// Another session shares the state dir but not the parent: it gets its own
	// agent rather than a message to this session's.
	t.Setenv("HERDR_PANE_ID", "otherPane")
	third, err := spawn(map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "third question"},
	}})
	if err != nil {
		t.Fatalf("third spawn: %v", err)
	}
	if ex.launched != 2 {
		t.Errorf("another parent must launch its own agent, launched %d", ex.launched)
	}
	if !strings.Contains(third, "report-3") {
		t.Errorf("the other session must get the fresh agent's report, got:\n%s", third)
	}
}

// A type that is not persistent starts a fresh agent on every spawn: the live
// agent of the first spawn is left alone and the second spawn launches its own.
func TestNonPersistentTypeIsRelaunched(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("WICK_AGENT_ID", "parent1")
	ex := newPersistentExec(g)
	spawnExec = ex.run

	root := t.TempDir()
	tool := spawnToolFor(scoutTypeFile(t, root, "false"), []string{"scout"})
	spawn := func(params map[string]any) (string, error) {
		raw, _ := json.Marshal(params)
		return tool.Execute(context.Background(), root, raw)
	}

	if _, err := spawn(map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "first question"},
	}}); err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	second, err := spawn(map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "second question"},
	}})
	if err != nil {
		t.Fatalf("second spawn: %v", err)
	}
	if ex.launched != 2 {
		t.Errorf("a non-persistent type must launch a fresh agent, launched %d", ex.launched)
	}
	if ex.nudged != 0 {
		t.Errorf("a non-persistent type must not be re-prompted, nudged %d", ex.nudged)
	}
	if !strings.Contains(second, "report-2") {
		t.Errorf("the fresh agent's report must come back, got:\n%s", second)
	}
}

// worktreeExec runs a worktree spawn against a fake grid and a fake git, so a
// test can re-spawn one ticket without a real repository.
type worktreeExec struct {
	g        *fakeGrid
	mu       sync.Mutex
	launched int
	nudged   []string
	gitCalls []string
}

func (w *worktreeExec) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == "git" {
		w.mu.Lock()
		w.gitCalls = append(w.gitCalls, strings.Join(args, " "))
		w.mu.Unlock()
		return Result{Stdout: "abc123def\n"}, nil
	}
	if cmd == herdrBin() && len(args) > 1 {
		switch args[0] + " " + args[1] {
		case "pane run":
			w.mu.Lock()
			w.launched++
			w.mu.Unlock()
		case "agent prompt":
			w.mu.Lock()
			w.nudged = append(w.nudged, args[2])
			w.mu.Unlock()
			return Result{Stdout: "{}"}, nil
		}
	}
	return w.g.run(cmd, args, opts)
}

// A persistent worktree type is keyed by the ticket's own id: re-spawning the
// same ticket reuses the worker already on it rather than creating a second
// worktree for it.
func TestPersistentWorktreeTypeIsRepromptedNotRelaunched(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("WICK_AGENT_ID", "parent1")
	t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
	ex := &worktreeExec{g: g}
	spawnExec = ex.run

	repo := t.TempDir()
	ticket := filepath.Join(repo, "20-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeAgentFile(t, root, "hand.md", `---
name: hand
description: A worker that stays on its ticket.
tools: [read]
checkout: worktree
blocking: false
persistent: true
statuses: [done, blocked]
---
{{prompt}}
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	tool := spawnToolFor(types, []string{"hand"})
	spawn := func() (string, error) {
		raw, _ := json.Marshal(map[string]any{"agents": []any{
			map[string]any{"type": "hand", "ticket": ticket, "repo": repo},
		}})
		return tool.Execute(context.Background(), repo, raw)
	}

	first, err := spawn()
	if err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	if !strings.Contains(first, "hand 20-x started") {
		t.Fatalf("the first spawn must start the worktree agent, got:\n%s", first)
	}
	if _, err := recall("20-x"); err != nil {
		t.Fatalf("the first spawn must record the worker: %v", err)
	}
	if !containsCall(ex.gitCalls, "worktree add") {
		t.Fatalf("the first spawn must create the ticket's worktree: %v", ex.gitCalls)
	}
	created := len(ex.gitCalls)

	second, err := spawn()
	if err != nil {
		t.Fatalf("a re-spawn of the ticket must reuse its agent, got: %v", err)
	}
	if ex.launched != 1 {
		t.Errorf("a persistent worktree type must reuse its agent, launched %d", ex.launched)
	}
	if len(ex.nudged) != 1 {
		t.Fatalf("the worker must be re-prompted in its own pane, nudged %v", ex.nudged)
	}
	if !strings.Contains(second, "hand 20-x started") {
		t.Errorf("the re-spawn must report the agent as started, got:\n%s", second)
	}
	for _, call := range ex.gitCalls[created:] {
		if strings.Contains(call, "worktree add") {
			t.Errorf("a re-spawn must not create a second worktree: %s", call)
		}
	}
}

// containsCall reports whether any recorded command contains want.
func containsCall(calls []string, want string) bool {
	for _, c := range calls {
		if strings.Contains(c, want) {
			return true
		}
	}
	return false
}

// A re-prompted agent predates the spawn call that reached it: when a later
// entry fails, the rollback must leave the agent the parent already owns
// running, not close its pane.
func TestFailedSpawnLeavesAReusedAgentRunning(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("WICK_AGENT_ID", "parent1")
	ex := newPersistentExec(g)
	spawnExec = ex.run

	root := t.TempDir()
	scoutTypeFile(t, root, "true")
	writeAgentFile(t, root, "hand.md", `---
name: hand
description: A worker for a ticket that is not there.
tools: [read]
checkout: worktree
blocking: false
statuses: [done, blocked]
---
{{prompt}}
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	tool := spawnToolFor(types, []string{"scout", "hand"})
	call := func(agents []any) (string, error) {
		raw, _ := json.Marshal(map[string]any{"agents": agents})
		return tool.Execute(context.Background(), root, raw)
	}

	if _, err := call([]any{map[string]any{"type": "scout", "prompt": "first question"}}); err != nil {
		t.Fatalf("first spawn: %v", err)
	}

	// The scout is re-prompted, then the second entry fails to launch.
	if _, err := call([]any{
		map[string]any{"type": "scout", "prompt": "second question"},
		map[string]any{"type": "hand", "ticket": "/nope/01-x.md", "repo": root},
	}); err == nil {
		t.Fatal("a missing ticket must fail the spawn")
	}
	for _, c := range g.calls {
		if strings.Contains(c, "pane close") {
			t.Errorf("a failed spawn must not close the re-prompted agent's pane: %s", c)
		}
	}
}
