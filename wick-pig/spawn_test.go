package wick

import (
	"context"
	"encoding/json"
	"fmt"
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

// spawn's description lists only the types the caller may spawn, and the scout
// tool is gone.
func TestSpawnDescriptionListsTheCallersTypes(t *testing.T) {
	_, desc := spawnToolForRole(t, "implement")
	if !strings.Contains(desc, "scout") {
		t.Errorf("implement may spawn scout; the description must say so:\n%s", desc)
	}
	if strings.Contains(desc, "worker") {
		t.Errorf("implement may not spawn worker; the description must not offer it:\n%s", desc)
	}
	served, _ := RoleTools("implement")
	for _, tool := range served {
		if tool.Name() == "scout" {
			t.Fatal("the scout tool must be gone")
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
