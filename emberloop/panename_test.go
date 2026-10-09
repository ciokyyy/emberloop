package el

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Pane names come from one place: the agent type maps to the herdr pane
// name, and the header strip ticket reads the same value for its chip.
func TestPaneNameForTypeMapsAgentTypes(t *testing.T) {
	cases := map[string]string{
		"worker":             "implementer",
		"reviewer-spec":      "spec-reviewer",
		"reviewer-standards": "code-reviewer",
		"scout":              "scout",
		"orchestrator":       "orchestrator",
	}
	for typ, want := range cases {
		if got := PaneNameForType(typ); got != want {
			t.Errorf("PaneNameForType(%q) = %q, want %q", typ, got, want)
		}
	}
	if got := PaneNameForType("my-custom"); got != "my-custom" {
		t.Errorf("custom type must keep its name, got %q", got)
	}
}

// A one-shot read-only launch carries no agent type in its env: the pane
// name is cosmetic and must not change runtime behaviour (stop gate,
// execute and readhint branch on an empty EL_AGENT_TYPE).
func TestReadOnlyLaunchEnvCarriesNoAgentType(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
			return Result{Stdout: "{}"}
		}
		res, _ := g.run(cmd, args, ExecOpts{})
		return res
	}}
	sess := readOnlySession(newSessionID(), t.TempDir(), "", "")
	if _, err := launchPlacedAgent(rec.run, readOnlyPlace{}, "pig", sess, "look"); err != nil {
		t.Fatal(err)
	}
	cmd, err := readLaunch(launchPath(sess))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "EL_AGENT_TYPE=") {
			t.Errorf("one-shot read-only launch must carry no EL_AGENT_TYPE, got %q", e)
		}
	}
	var renames [][]string
	for _, c := range rec.calls {
		if len(c) > 2 && c[1] == "pane" && c[2] == "rename" {
			renames = append(renames, c)
		}
	}
	if len(renames) != 1 || renames[0][4] != "scout" {
		t.Errorf("typeless pane must still be named scout, got %v", renames)
	}
}

func renameCalls(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.Contains(c, "pane rename") {
			out = append(out, c)
		}
	}
	return out
}

// A spawned scout's pane carries the scout name at placement.
func TestScoutSpawnNamesItsPaneScout(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("EL_AGENT_ID", "parent1")
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "where is it?"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	var renames []string
	for _, c := range g.calls {
		if strings.Contains(c, "pane rename") {
			renames = append(renames, c)
		}
	}
	if len(renames) != 1 {
		t.Fatalf("want one pane rename, got %d: %v", len(renames), g.calls)
	}
	if !strings.Contains(renames[0], "scout") {
		t.Errorf("scout pane must be named scout, got %q", renames[0])
	}
}

// A spawned worker's pane carries the implementer name at placement.
func TestWorkerSpawnNamesItsPaneImplementer(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("EL_AGENT_ID", "")
	var calls []string
	base := spawnToolFake()
	spawnExec = func(cmd string, args []string, o ExecOpts) (Result, error) {
		calls = append(calls, strings.Join(append([]string{cmd}, args...), " "))
		return base(cmd, args, o)
	}
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
	repo := t.TempDir()
	ticket := filepath.Join(repo, "20-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": ticket},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	got := renameCalls(calls)
	if len(got) != 1 {
		t.Fatalf("want one pane rename, got %d: %v", len(got), calls)
	}
	if !strings.Contains(got[0], "w1:p1") || !strings.Contains(got[0], "implementer") {
		t.Errorf("worker pane must be renamed implementer, got %q", got[0])
	}
}

// A placed reviewer pane carries its reviewer name.
func TestLaunchPlacedAgentNamesReviewerPane(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
			return Result{Stdout: "{}"}
		}
		res, _ := g.run(cmd, args, ExecOpts{})
		return res
	}}
	sess := readOnlySession(newSessionID(), t.TempDir(), "", "")
	sess.AgentType = "reviewer-spec"
	if _, err := launchPlacedAgent(rec.run, readOnlyPlace{}, "pig", sess, "review it"); err != nil {
		t.Fatal(err)
	}
	var renames [][]string
	for _, c := range rec.calls {
		if len(c) > 2 && c[1] == "pane" && c[2] == "rename" {
			renames = append(renames, c)
		}
	}
	if len(renames) != 1 {
		t.Fatalf("want one pane rename, got %d: %v", len(renames), rec.calls)
	}
	if renames[0][4] != "spec-reviewer" {
		t.Errorf("reviewer-spec pane must be named spec-reviewer, got %q", renames[0])
	}
}

// A re-prompted persistent agent keeps the right name on its reused pane.
func TestRepromptedAgentKeepsItsName(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scout := types["scout"]
	sess := AgentState{ID: "reprompt-1", Type: "scout"}
	sess.Pane = g.tabs[0].order[0]
	sess.Tab = g.tabs[0].tab
	sess.ParentID = "parent1"
	if err := WriteAgentState(sess); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_AGENT_ID", "parent1")
	t.Setenv("HERDR_PANE_ID", "wP")
	var calls [][]string
	run := func(cmd string, args []string, o ExecOpts) (Result, error) {
		calls = append(calls, append([]string{cmd}, args...))
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
			return livePaneProcs(paneArg(args)), nil
		}
		if cmd == herdrBin() && len(args) > 1 && args[0] == "agent" {
			return Result{Stdout: "{}"}, nil
		}
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && (args[1] == "send-text" || args[1] == "send-keys") {
			return Result{Stdout: "{}"}, nil
		}
		return Result{Stdout: "{}"}, nil
	}
	prev, ok := liveAgentFor(run, scout, "parent1", "wP", spawnRequest{Type: "scout", Prompt: "q"})
	if !ok {
		t.Fatal("want the live scout")
	}
	if _, err := repromptAgent(run, scout, prev, spawnRequest{Type: "scout", Prompt: "again"}, "group1"); err != nil {
		t.Fatal(err)
	}
	var renames [][]string
	for _, c := range calls {
		if len(c) > 2 && c[1] == "pane" && c[2] == "rename" {
			renames = append(renames, c)
		}
	}
	if len(renames) != 1 || renames[0][4] != "scout" {
		t.Errorf("reused pane must be renamed scout, got %v", renames)
	}
}

// The top-level session names its own pane orchestrator inside herdr,
// and does nothing outside herdr.
func TestOrchestratorPaneNamedAtStartup(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p7")
	rec := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	NameOrchestratorPane(rec.run)
	found := false
	for _, c := range rec.calls {
		if len(c) == 5 && c[1] == "pane" && c[2] == "rename" && c[3] == "w1:p7" && c[4] == "orchestrator" {
			found = true
		}
	}
	if !found {
		t.Errorf("want pane rename w1:p7 orchestrator, got %v", rec.calls)
	}
	t.Setenv("HERDR_PANE_ID", "")
	rec = &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	NameOrchestratorPane(rec.run)
	if len(rec.calls) != 0 {
		t.Errorf("outside herdr there must be no rename, got %v", rec.calls)
	}
}

// A failed rename is silent and never fails the spawn.
func TestFailedRenameNeverFailsSpawn(t *testing.T) {
	oldState, oldGap, oldExec := statePath, spawnGap, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	spawnGap = 0
	t.Cleanup(func() { statePath, spawnGap, spawnExec = oldState, oldGap, oldExec })
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("EL_AGENT_ID", "")
	base := spawnToolFake()
	spawnExec = func(cmd string, args []string, o ExecOpts) (Result, error) {
		if len(args) > 1 && args[0] == "pane" && args[1] == "rename" {
			return Result{Code: 1, Stderr: "no such pane"}, nil
		}
		return base(cmd, args, o)
	}
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("EL_HERDR_REPORT", "/fake/herdr-report")
	repo := t.TempDir()
	ticket := filepath.Join(repo, "21-x.md")
	if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn(repo, map[string]any{"agents": []any{
		map[string]any{"type": "worker", "ticket": ticket},
	}}); err != nil {
		t.Fatalf("a failed rename must not fail the spawn: %v", err)
	}
}

// Fresh reviewer panes carry their reviewer names, and a follow-up keeps
// the right name on the reused pane.
func TestReviewerPanesCarryReviewerNames(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	spec, standards := f.paneOf(t, reviewerSpec), f.paneOf(t, reviewerStandards)
	paneID := func(want *fakePane) string {
		for id, p := range f.panes {
			if p == want {
				return id
			}
		}
		return ""
	}
	renames := map[string]string{}
	for _, c := range f.calls {
		if len(c) == 5 && c[1] == "pane" && c[2] == "rename" {
			renames[c[3]] = c[4]
		}
	}
	if renames[paneID(spec)] != "spec-reviewer" {
		t.Errorf("spec reviewer pane = %q, want spec-reviewer (renames %v)", renames[paneID(spec)], renames)
	}
	if renames[paneID(standards)] != "code-reviewer" {
		t.Errorf("standards reviewer pane = %q, want code-reviewer (renames %v)", renames[paneID(standards)], renames)
	}
	before := len(f.calls)
	changeTree(t, tree, "answer the findings")
	f.answer = answers("pass", "pass")
	if _, err := reviewSpawn(t, f, tree, withNote("addressed")...); err != nil {
		t.Fatal(err)
	}
	reused := 0
	for _, c := range f.calls[before:] {
		if len(c) == 5 && c[1] == "pane" && c[2] == "rename" {
			reused++
		}
	}
	if reused == 0 {
		t.Error("a follow-up must rename the reused reviewer panes again")
	}
}
