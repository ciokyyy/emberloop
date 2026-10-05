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
)

// Reviewer tabs are named here so a test can tell the worker's ticket tab from
// the orchestrator's tab. `reviewTab` is the ticket tab, labelled
// `reviewTicket`; `reviewCaller` is where the caller (orchestrator) runs.
const (
	reviewTab    = "w1:impl"
	reviewTicket = "07-x"
	reviewCaller = "w1:caller"
)

// fakeReviewers is a herdr whose panes run reviewer agents, laid out over tabs
// the way herdr reports them. Each pane is a persistent agent: launched by pane
// run, it answers its prompt and submits through the real submit tool, so the
// spawn waits on exactly what a live reviewer writes. git and sh are real, in a
// temp repo.
type fakeReviewers struct {
	mu      sync.Mutex
	next    int
	panes   map[string]*fakePane
	paneIDs []string
	tabs    map[string]string // tab id -> label
	tabIDs  []string
	calls   [][]string
	// checkRuns counts the repo check commands the spawn ran.
	checkRuns int
	// answer reports the status and findings a reviewer of a type submits on
	// its turn-th message.
	answer func(typ string, turn int, prompt string) (status, report string)
}

type fakePane struct {
	typ, transcript string
	turns           int
	closed          bool
	launched        bool // a reviewer agent was run in it
	tab             string
	id              string
	harness         string
	prompts         []string
	// line is the launch line pane run typed; the tests read the harness,
	// model and effort it carries from it.
	line string
}

func newFakeReviewers(answer func(string, int, string) (string, string)) *fakeReviewers {
	resetGrid()
	f := &fakeReviewers{panes: map[string]*fakePane{}, tabs: map[string]string{}, answer: answer}
	f.addTab(reviewTab, reviewTicket)
	f.addPane("root", reviewTab)
	f.addTab(reviewCaller, "orchestrator")
	f.addPane("caller", reviewCaller)
	return f
}

func (f *fakeReviewers) addTab(id, label string) {
	f.tabs[id] = label
	f.tabIDs = append(f.tabIDs, id)
}

func (f *fakeReviewers) addPane(id, tab string) {
	f.panes[id] = &fakePane{tab: tab, id: id}
	f.paneIDs = append(f.paneIDs, id)
}

func (f *fakeReviewers) paneID() string {
	f.next++
	return fmt.Sprintf("rv%d", f.next)
}

// panesIn is the panes of one tab, in creation order.
func (f *fakeReviewers) panesIn(tab string) []string {
	var out []string
	for _, id := range f.paneIDs {
		if p := f.panes[id]; p != nil && p.tab == tab {
			out = append(out, id)
		}
	}
	return out
}

// fill opens n extra panes in tab, to model a tab that is already full.
func (f *fakeReviewers) fill(tab string, n int) {
	for i := 0; i < n; i++ {
		f.addPane(f.paneID(), tab)
	}
}

// tabWithLabel is the id of the tab labelled label, or "".
func (f *fakeReviewers) tabWithLabel(label string) string {
	for _, id := range f.tabIDs {
		if f.tabs[id] == label {
			return id
		}
	}
	return ""
}

func (f *fakeReviewers) closeTab(id string) {
	if _, ok := f.tabs[id]; !ok {
		return
	}
	delete(f.tabs, id)
	for i, t := range f.tabIDs {
		if t == id {
			f.tabIDs = append(f.tabIDs[:i], f.tabIDs[i+1:]...)
			break
		}
	}
	for _, pid := range f.panesIn(id) {
		f.panes[pid].closed = true
	}
}

func (f *fakeReviewers) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == "git" {
		return osExec(cmd, args, opts)
	}
	if cmd == "sh" {
		f.mu.Lock()
		f.checkRuns++
		f.mu.Unlock()
		return osExec(cmd, args, opts)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{cmd}, args...))
	if len(args) < 2 {
		return Result{Stdout: "{}"}, nil
	}
	switch args[0] + " " + args[1] {
	case "pane list":
		// herdr drops a closed pane from the list; the fake does too, so a
		// dead reviewer really looks dead to the grid.
		panes := []any{}
		for _, tab := range f.tabIDs {
			for _, id := range f.panesIn(tab) {
				if f.panes[id].closed {
					continue
				}
				panes = append(panes, map[string]any{"pane_id": id, "tab_id": tab})
			}
		}
		return jsonReply(map[string]any{"panes": panes}), nil
	case "pane split":
		tab := reviewTab
		if p := f.panes[argValue(args, "--pane")]; p != nil {
			tab = p.tab
		}
		id := f.paneID()
		f.addPane(id, tab)
		return Result{Stdout: `{"result":{"pane":{"pane_id":"` + id + `","tab_id":"` + tab + `"}}}`}, nil
	case "pane run":
		p := f.panes[args[2]]
		p.launched = true
		line := args[3]
		p.line = line
		cmd := launchCmd(line)
		p.harness = cmd.Bin
		for _, e := range cmd.Env {
			if v, ok := strings.CutPrefix(e, "WICK_AGENT_ID="); ok {
				p.id = v
			}
			if v, ok := strings.CutPrefix(e, "WICK_AGENT_TYPE="); ok {
				p.typ = v
			}
		}
		var dir, id, prompt string
		for i, a := range cmd.Args {
			switch {
			case a == "--session-dir" && i+1 < len(cmd.Args):
				dir = cmd.Args[i+1]
			case a == "--session-id" && i+1 < len(cmd.Args):
				id = cmd.Args[i+1]
			case a == "--" && i+1 < len(cmd.Args):
				prompt = cmd.Args[i+1]
			}
		}
		// claude writes its transcript under ~/.claude/projects and takes no
		// --session-dir; the fake reads HOME to land it where the harness
		// looks, so a claude reviewer completes like a pig one.
		if dir == "" && p.harness == "claude" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".claude", "projects", "wt")
		}
		_ = os.MkdirAll(filepath.Join(dir, "--x--"), 0o755)
		p.transcript = filepath.Join(dir, "--x--", "t_"+id+".jsonl")
		// claude takes its first message after startup, so its turn is the
		// late prompt, not the command line: a pig turn answers here, with
		// the full prompt the launch carries as a real argument.
		if p.harness != "claude" {
			f.turn(p, prompt)
		}
	case "pane get":
		if p := f.panes[args[2]]; p != nil && !p.closed {
			return Result{Stdout: "{}"}, nil
		}
		return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
	case "pane close":
		if p := f.panes[args[2]]; p != nil {
			p.closed = true
		}
	case "pane send-text":
		p := f.panes[args[2]]
		prompt := args[3]
		// A follow-up names a file to read; the agent reads it.
		for _, w := range strings.Fields(prompt) {
			if raw, err := os.ReadFile(strings.TrimSuffix(w, ":")); err == nil {
				prompt = string(raw)
			}
		}
		f.turn(p, prompt)
	case "tab list":
		tabs := []any{}
		for _, id := range f.tabIDs {
			tabs = append(tabs, map[string]any{"tab_id": id, "label": f.tabs[id], "pane_count": len(f.panesIn(id))})
		}
		return jsonReply(map[string]any{"tabs": tabs}), nil
	case "tab create":
		id := fmt.Sprintf("w1:t%d", len(f.tabIDs)+1)
		f.addTab(id, argValue(args, "--label"))
		root := f.paneID()
		f.addPane(root, id)
		return jsonReply(map[string]any{
			"tab":       map[string]any{"tab_id": id},
			"root_pane": map[string]any{"pane_id": root, "tab_id": id},
		}), nil
	case "tab close":
		f.closeTab(args[2])
	case "agent prompt":
		// herdr prompts a named claude agent (its late first message);
		// pig is only reported, so Nudge falls back to typing.
		if p := f.panes[args[2]]; p != nil && p.harness == "claude" {
			f.turn(p, args[3])
			return Result{Stdout: "{}"}, nil
		}
		return Result{Code: 1}, nil
	}
	return Result{Stdout: "{}"}, nil
}

// turn records the prompt and the agent's answer, then has the reviewer report
// through the real submit tool so its verdict is folded in exactly as a live
// reviewer's would be.
func (f *fakeReviewers) turn(p *fakePane, prompt string) {
	p.turns++
	p.prompts = append(p.prompts, prompt)
	status, report := f.answer(p.typ, p.turns, prompt)
	var user, asst []byte
	if p.harness == "claude" {
		user, _ = json.Marshal(map[string]any{"type": "user", "message": map[string]any{
			"role": "user", "content": []any{map[string]any{"type": "text", "text": prompt}}}})
		asst, _ = json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
			"stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": report}}}})
	} else {
		user, _ = json.Marshal(map[string]any{"type": "message", "message": map[string]any{
			"role": "user", "content": []any{map[string]any{"type": "text", "text": prompt}}}})
		asst, _ = json.Marshal(map[string]any{"type": "message", "message": map[string]any{
			"role": "assistant", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": report}}}})
	}
	fh, _ := os.OpenFile(p.transcript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintf(fh, "%s\n%s\n", user, asst)
	fh.Close()

	tool := submitToolFor(AgentType{Name: p.typ, Blocking: true, Statuses: []string{verdictPass, verdictChanges}}, p.id)
	raw, _ := json.Marshal(map[string]any{"status": status, "report": report})
	if _, err := tool.Execute(context.Background(), "", raw); err != nil {
		panic(fmt.Sprintf("reviewer %s submit: %v", p.typ, err))
	}
}

func (f *fakeReviewers) splits() int {
	n := 0
	for _, c := range f.calls {
		if len(c) > 2 && c[1] == "pane" && c[2] == "split" {
			n++
		}
	}
	return n
}

// paneOf is the live pane running the reviewer of type typ.
func (f *fakeReviewers) paneOf(t *testing.T, typ string) *fakePane {
	t.Helper()
	for _, p := range f.panes {
		if p.typ == typ && !p.closed {
			return p
		}
	}
	t.Fatalf("no live %s reviewer", typ)
	return nil
}

// reviewSpawn drives the reviewer spawn a worker gets: the spawn tool with both
// reviewer types allowed, its herdr the fake, its cwd the worker's worktree, and
// the built-in types.
func reviewSpawn(t *testing.T, f *fakeReviewers, tree string, entries ...map[string]any) (string, error) {
	t.Helper()
	return reviewSpawnWith(t, f, t.TempDir(), tree, entries...)
}

// reviewSpawnWith is reviewSpawn with the types loaded from typesRoot, so a test
// can spawn a repo's own reviewer type.
func reviewSpawnWith(t *testing.T, f *fakeReviewers, typesRoot, tree string, entries ...map[string]any) (string, error) {
	t.Helper()
	oldExec := spawnExec
	spawnExec = f.run
	t.Cleanup(func() { spawnExec = oldExec })
	types, err := LoadAgentTypes(typesRoot)
	if err != nil {
		t.Fatal(err)
	}
	tool := spawnToolFor(types, []string{reviewerStandards, reviewerSpec})
	agents := make([]any, len(entries))
	for i, e := range entries {
		agents[i] = e
	}
	raw, _ := json.Marshal(map[string]any{"agents": agents})
	return tool.Execute(context.Background(), tree, raw)
}

// bothReviewers is the spawn a worker makes to review its worktree.
func bothReviewers() []map[string]any {
	return []map[string]any{{"type": reviewerStandards}, {"type": reviewerSpec}}
}

// withNote carries a worker's answer to earlier findings to both reviewers.
func withNote(note string) []map[string]any {
	return []map[string]any{{"type": reviewerStandards, "prompt": note}, {"type": reviewerSpec, "prompt": note}}
}

// reviewRepo is a repo with a worker worktree holding one committed change,
// recorded as a worker with a ticket and a spec. Its state dir holds the
// reviewers, submits and agent states the spawn writes.
func reviewRepo(t *testing.T) (repo, tree string) {
	t.Helper()
	old := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	oldGap, oldPoll := spawnGap, pollEvery
	spawnGap, pollEvery = 0, 5_000_000 // 5ms
	t.Cleanup(func() {
		statePath, spawnGap, pollEvery = old, oldGap, oldPoll
	})
	// Hermetic settings: a missing temp file and no read-only override, so
	// the reviewers resolve the built-in pig default unless a test says more.
	resolverSettings(t, "")
	repo = t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	writeFile(t, filepath.Join(repo, "a.go"), "package a\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	tree = filepath.Join(repo, ".wick-worktrees", "07-x")
	gitOut(t, repo, "worktree", "add", "-q", "-b", "implement/07-x", tree, "main")
	writeFile(t, filepath.Join(tree, "a.go"), "package a\n\nfunc A() {}\n")
	gitOut(t, tree, "commit", "-qam", "work")
	if err := save(map[string]Worker{"07-x": {
		ID: "07-x", Ticket: "/t/07-x.md", Spec: "/t/spec.md", Repo: repo, Base: base, Target: "main",
		Branch: "implement/07-x", Worktree: tree, Pane: "root", Tab: reviewTab, Workspace: "w1",
		AgentType: "worker", Started: "2026-10-05T00:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	return repo, tree
}

// A worker's reviewers resolve their own slots: the standards reviewer runs
// code-review, the spec reviewer spec-review, each with its slot's harness,
// model and effort rather than the worker's model. The reviewer records store
// what was resolved.
func TestReviewersResolveTheirOwnSlots(t *testing.T) {
	_, tree := reviewRepo(t)
	resolverSettings(t, `{"roles":{
		"code-review":{"harness":"pig","model":"standards-model","effort":"high"},
		"spec-review":{"harness":"pig","model":"spec-model","effort":"low"}}}`)
	f := newFakeReviewers(answers(verdictPass, verdictPass))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	standards := f.paneOf(t, reviewerStandards)
	if got := flagIn(standards.line, "--model"); got != "standards-model" {
		t.Errorf("code reviewer model = %q, want standards-model", got)
	}
	if got := flagIn(standards.line, "--thinking"); got != "high" {
		t.Errorf("code reviewer effort = %q, want high", got)
	}
	spec := f.paneOf(t, reviewerSpec)
	if got := flagIn(spec.line, "--model"); got != "spec-model" {
		t.Errorf("spec reviewer model = %q, want spec-model", got)
	}
	if got := flagIn(spec.line, "--thinking"); got != "low" {
		t.Errorf("spec reviewer effort = %q, want low", got)
	}
	rec := reviewersOf(tree)
	if got := rec[reviewerStandards]; got.Harness != "pig" || got.Model != "standards-model" || got.Effort != "high" {
		t.Errorf("standards reviewer record = harness %q model %q effort %q, want pig/standards-model/high",
			got.Harness, got.Model, got.Effort)
	}
	if got := rec[reviewerSpec]; got.Model != "spec-model" || got.Effort != "low" {
		t.Errorf("spec reviewer record = model %q effort %q, want spec-model/low", got.Model, got.Effort)
	}
}

// A reviewer does not copy the worker's model: with the worker's model set and
// the slot empty, the reviewer launches with none.
func TestReviewersDoNotCopyTheWorkerModel(t *testing.T) {
	_, tree := reviewRepo(t)
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	w := store["07-x"]
	w.Model = "worker-model"
	store["07-x"] = w
	if err := save(store); err != nil {
		t.Fatal(err)
	}
	f := newFakeReviewers(answers(verdictPass, verdictPass))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{reviewerStandards, reviewerSpec} {
		if got := flagIn(f.paneOf(t, typ).line, "--model"); got != "" {
			t.Errorf("%s reviewer model = %q, want none: the worker's model must not leak", typ, got)
		}
		if rec := reviewersOf(tree)[typ]; rec.Model != "" {
			t.Errorf("%s reviewer record model = %q, want none", typ, rec.Model)
		}
	}
}

// WICK_READONLY_HARNESS is a resolver flag layer: it overrides the slot's
// harness, so a reviewer on a machine with the variable set still launches
// with it.
func TestReadonlyHarnessOverridesTheReviewerSlot(t *testing.T) {
	_, tree := reviewRepo(t)
	resolverSettings(t, `{"roles":{"code-review":{"harness":"claude","model":"slot-model"}}}`)
	t.Setenv("WICK_READONLY_HARNESS", "pig")
	f := newFakeReviewers(answers(verdictPass, verdictPass))
	if _, err := reviewSpawn(t, f, tree, map[string]any{"type": reviewerStandards}); err != nil {
		t.Fatal(err)
	}
	if got := binIn(f.paneOf(t, reviewerStandards).line); got != "pig" {
		t.Errorf("reviewer harness = %q, want pig from WICK_READONLY_HARNESS", got)
	}
}

// A claude reviewer receives the model and effort configured for its slot: the
// rule that dropped the model for a non-pig harness is gone.
func TestClaudeReviewerGetsTheConfiguredModel(t *testing.T) {
	_, tree := reviewRepo(t)
	t.Setenv("HOME", t.TempDir())
	resolverSettings(t, `{"roles":{"code-review":{"harness":"claude","model":"claude-model","effort":"minimal"}}}`)
	f := newFakeReviewers(answers(verdictPass, verdictPass))
	if _, err := reviewSpawn(t, f, tree, map[string]any{"type": reviewerStandards}); err != nil {
		t.Fatal(err)
	}
	line := f.paneOf(t, reviewerStandards).line
	if got := binIn(line); got != "claude" {
		t.Errorf("reviewer harness = %q, want claude", got)
	}
	if got := flagIn(line, "--model"); got != "claude-model" {
		t.Errorf("claude reviewer model = %q, want claude-model", got)
	}
	if got := flagIn(line, "--effort"); got != "minimal" {
		t.Errorf("claude reviewer effort = %q, want minimal", got)
	}
	if rec := reviewersOf(tree)[reviewerStandards]; rec.Harness != "claude" || rec.Model != "claude-model" || rec.Effort != "minimal" {
		t.Errorf("reviewer record = harness %q model %q effort %q, want claude/claude-model/minimal",
			rec.Harness, rec.Model, rec.Effort)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// answers gives each reviewer type a fixed status for every turn, with a
// findings line for the spec reviewer so a replacement has something to carry.
func answers(standards, spec string) func(string, int, string) (string, string) {
	return func(typ string, _ int, _ string) (string, string) {
		if typ == reviewerSpec {
			return spec, "a.go:3 major: ticket asks for B"
		}
		return standards, "no findings"
	}
}

// A worker's spawn of both reviewer types runs the repo checks once, starts both
// reviewers in the worker's worktree, and returns both reports together. A
// multi-line findings report comes back intact.
func TestSpawnReviewersRunsChecksOnceAndReturnsBoth(t *testing.T) {
	_, tree := reviewRepo(t)
	writeFile(t, filepath.Join(tree, ".wick", "check"), "echo checking; echo 3 passed\n")
	f := newFakeReviewers(func(typ string, _ int, _ string) (string, string) {
		if typ == reviewerSpec {
			return verdictChanges, "a.go:3 major: ticket asks for B\nline two of the finding"
		}
		return verdictPass, "a.go:1 minor: naming"
	})
	out, err := reviewSpawn(t, f, tree, bothReviewers()...)
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 2 {
		t.Errorf("want two reviewers launched, got %d splits", f.splits())
	}
	if f.checkRuns != 1 {
		t.Errorf("the repo check ran %d times, want once", f.checkRuns)
	}
	for _, want := range []string{
		reviewerStandards, reviewerSpec, "submitted (pass)", "submitted (changes)",
		"a.go:3 major: ticket asks for B\nline two of the finding",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("reply missing %q:\n%s", want, out)
		}
	}
	// Both reviewers run in the worker's worktree.
	for _, typ := range defaultReviewers {
		if p := f.paneOf(t, typ); p == nil || !strings.Contains(p.prompts[0], tree) {
			t.Errorf("the %s reviewer is not pointed at the worktree %s", typ, tree)
		}
	}
	// Reviewers stay alive for follow-ups.
	for id, p := range f.panes {
		if p.launched && p.closed {
			t.Errorf("reviewer pane %s was closed after its first reply", id)
		}
	}
	have := reviewersOf(tree)
	if have[reviewerStandards].Verdict != verdictPass || have[reviewerSpec].Verdict != verdictChanges {
		t.Errorf("stored reviewers = %+v", have)
	}
}

// A spawn on a worktree whose diff against its base is empty reports that and
// starts no reviewer, the way the review machinery always did.
func TestReviewSpawnWithNoChangesStartsNothing(t *testing.T) {
	_, tree := reviewRepo(t)
	// Drop the worker's change, so the branch and its base are the same tip.
	gitOut(t, tree, "reset", "--hard", "HEAD~1")
	f := newFakeReviewers(answers("pass", "pass"))
	out, err := reviewSpawn(t, f, tree, bothReviewers()...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no changes") {
		t.Errorf("an unchanged tree must report nothing to review, got %q", out)
	}
	if f.splits() != 0 {
		t.Errorf("no reviewer may launch for an empty diff, got %d splits", f.splits())
	}
}

// Spawning only one reviewer type starts only that reviewer.
func TestReviewSpawnOnlyTheNamedType(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, map[string]any{"type": reviewerStandards}); err != nil {
		t.Fatal(err)
	}
	if f.splits() != 1 {
		t.Errorf("want one reviewer launched, got %d splits", f.splits())
	}
	if _, ok := reviewersOf(tree)[reviewerSpec]; ok {
		t.Error("spawning standards must not start the spec reviewer")
	}
}

// A second spawn goes, by default, only to reviewers with open findings: the
// one that passed at the tip is left alone, the one asking for changes is
// re-prompted in its own pane, carrying the worker's note.
func TestReviewSpawnFollowUpGoesToOpenFindingsOnly(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	f.answer = answers("pass", "pass")
	out, err := reviewSpawn(t, f, tree, withNote("B is out of scope: the ticket says A only")...)
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 2 {
		t.Errorf("a follow-up must reuse the live reviewers, got %d splits", f.splits())
	}
	spec, standards := f.paneOf(t, reviewerSpec), f.paneOf(t, reviewerStandards)
	if spec.turns != 2 || standards.turns != 1 {
		t.Errorf("turns: spec %d (want 2), standards %d (want 1)", spec.turns, standards.turns)
	}
	if !strings.Contains(spec.prompts[1], "B is out of scope") || !strings.Contains(spec.prompts[1], "Follow-up") {
		t.Errorf("the follow-up must carry the note:\n%s", spec.prompts[1])
	}
	if !strings.Contains(out, reviewerSpec) || strings.Contains(out, reviewerStandards) {
		t.Errorf("the reply must carry only the reviewer that was re-prompted: %s", out)
	}
	if got := reviewersOf(tree)[reviewerSpec].Verdict; got != verdictPass {
		t.Errorf("spec verdict = %q", got)
	}
}

// A reviewer type that is not persistent is not kept between spawns: the live
// reviewer is closed and a fresh one takes its place, while a persistent type
// in the same spawn is re-prompted as usual.
func TestNonPersistentReviewerIsReplacedNotReprompted(t *testing.T) {
	repo, tree := reviewRepo(t)
	writeAgentFile(t, repo, "reviewer-standards.md", `---
name: reviewer-standards
description: "Review a worker's worktree, fresh each time."
tools: [read, find]
checkout: parent
blocking: true
persistent: false
statuses: [pass, changes]
---
Short-lived reviewer.
`)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawnWith(t, f, repo, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	old := f.paneOf(t, reviewerStandards)

	// The standards reviewer passed; a persistent one would be left alone, but
	// this type is not kept.
	f.answer = answers("pass", "pass")
	out, err := reviewSpawnWith(t, f, repo, tree, bothReviewers()...)
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 3 {
		t.Errorf("a non-persistent reviewer must be relaunched, got %d splits", f.splits())
	}
	if !old.closed {
		t.Error("the live non-persistent reviewer must be closed, not left behind")
	}
	fresh := f.paneOf(t, reviewerStandards)
	if fresh == old || fresh.turns != 1 {
		t.Errorf("want a fresh standards reviewer on its first turn, got %+v", fresh)
	}
	if strings.Contains(fresh.prompts[0], "You replace") {
		t.Errorf("a fresh non-persistent reviewer must not inherit its predecessor's findings:\n%s", fresh.prompts[0])
	}
	if spec := f.paneOf(t, reviewerSpec); spec.turns != 2 {
		t.Errorf("the persistent spec reviewer must be re-prompted, turns = %d", spec.turns)
	}
	if !strings.Contains(out, reviewerStandards) {
		t.Errorf("the fresh reviewer's report must come back, got:\n%s", out)
	}
}

// A reviewer whose pane is gone is replaced by a fresh one briefed with its
// predecessor's findings, in the dead reviewer's slot.
func TestReviewSpawnReplacesADeadReviewer(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	before := reviewersOf(tree)[reviewerSpec]
	f.paneOf(t, reviewerSpec).closed = true
	out, err := reviewSpawn(t, f, tree, bothReviewers()...)
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 3 {
		t.Errorf("want one replacement launch, got %d splits", f.splits())
	}
	fresh := f.paneOf(t, reviewerSpec)
	if !strings.Contains(fresh.prompts[0], "ticket asks for B") || !strings.Contains(fresh.prompts[0], "You replace") {
		t.Errorf("the replacement must inherit the findings:\n%s", fresh.prompts[0])
	}
	if !strings.Contains(out, "a new reviewer took over") {
		t.Errorf("the worker must be told: %s", out)
	}
	after := reviewersOf(tree)[reviewerSpec]
	if after.Tab != before.Tab || after.Slot != before.Slot {
		t.Errorf("the replacement went to %s slot %d, want the dead reviewer's %s slot %d",
			after.Tab, after.Slot, before.Tab, before.Slot)
	}
}

// A pass is tied to a commit: once HEAD moves, the reviewer that passed sees
// the diff since its pass and is asked whether the change broke what it
// approved.
func TestReviewSpawnRechecksAStalePass(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	passedAt := gitOut(t, tree, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(tree, "a.go"), "package a\n\nfunc A() {}\n\nfunc B() {}\n")
	gitOut(t, tree, "commit", "-qam", "add B for the spec reviewer")
	f.answer = answers("pass", "pass")
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	standards := f.paneOf(t, reviewerStandards)
	if standards.turns != 2 {
		t.Fatalf("a stale pass must be rechecked, standards turns = %d", standards.turns)
	}
	if !strings.Contains(standards.prompts[1], "You passed this work at "+short(passedAt)) {
		t.Errorf("recheck prompt:\n%s", standards.prompts[1])
	}
	since, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "reviews", "07-x."+reviewerStandards+".since.diff"))
	if err != nil || !strings.Contains(string(since), "func B") || strings.Contains(string(since), "+func A") {
		t.Errorf("the recheck diff must be only what changed since the pass:\n%s", since)
	}
	if out, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(out, "nothing to review") {
		t.Errorf("both passed at HEAD: want nothing to review, got %s", out)
	}
}

// A review spawn opens its reviewers in the worker's ticket tab, beside the
// implementor, and never splits the caller's pane.
func TestReviewSpawnSplitsTheTicketTabNotTheCallers(t *testing.T) {
	_, tree := reviewRepo(t)
	t.Setenv("HERDR_TAB_ID", reviewCaller)
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	var targets []string
	for _, c := range f.calls {
		if len(c) > 2 && c[1] == "pane" && c[2] == "split" {
			targets = append(targets, argValue(c, "--pane"))
			if argValue(c, "--pane") == "caller" {
				t.Errorf("a reviewer split the caller's pane: %v", c)
			}
			if argValue(c, "--direction") != "right" {
				t.Errorf("the first row grows to the right: %v", c)
			}
		}
	}
	if len(targets) != 2 || !contains(targets, "root") {
		t.Errorf("splits target %v, want two with the implementor pane root", targets)
	}
	have := reviewersOf(tree)
	slots := map[int]bool{}
	for _, typ := range defaultReviewers {
		r, ok := have[typ]
		if !ok || r.Tab != reviewTab {
			t.Errorf("%s reviewer = %+v, want it in the ticket tab %s", typ, r, reviewTab)
			continue
		}
		slots[r.Slot] = true
	}
	if !slots[2] || !slots[3] {
		t.Errorf("reviewer slots = %v, want 2 and 3", slots)
	}
}

// A full ticket tab sends the next reviewers to the ticket's overflow tab,
// `<ticket-id> (2)`, still in the project workspace.
func TestReviewSpawnOverflowsIntoTheTicketSecondTab(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	f.fill(reviewTab, gridMax-len(f.panesIn(reviewTab)))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	over := f.tabWithLabel(reviewTicket + " (2)")
	if over == "" {
		t.Fatalf("no overflow tab %q; tabs = %v", reviewTicket+" (2)", f.tabs)
	}
	for _, typ := range defaultReviewers {
		if r := reviewersOf(tree)[typ]; r.Tab != over {
			t.Errorf("%s reviewer is in %s, want the overflow tab %s", typ, r.Tab, over)
		}
	}
	if got := len(f.panesIn(reviewTab)); got != gridMax {
		t.Errorf("the ticket tab holds %d panes, want it left full at %d", got, gridMax)
	}
}

// Discard closes the ticket tab and its overflow tabs, and forgets the
// reviewers.
func TestDiscardClosesTheTicketAndItsOverflowTabs(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	f.fill(reviewTab, gridMax-len(f.panesIn(reviewTab)))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	if f.tabWithLabel(reviewTicket+" (2)") == "" {
		t.Fatal("no overflow tab to close")
	}
	if _, err := Finish(f.run, "07-x", false); err != nil {
		t.Fatal(err)
	}
	if f.tabWithLabel(reviewTicket) != "" || f.tabWithLabel(reviewTicket+" (2)") != "" {
		t.Errorf("the ticket tabs are still open: %v", f.tabs)
	}
	if len(reviewersOf(tree)) != 0 {
		t.Error("the reviewer records were not forgotten")
	}
}

// The repo's check command runs before review and its result reaches both
// reviewers.
func TestReviewSpawnRunsTheRepoCheck(t *testing.T) {
	_, tree := reviewRepo(t)
	writeFile(t, filepath.Join(tree, ".wick", "check"), "echo checking; echo 3 passed; exit 1\n")
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	for _, typ := range defaultReviewers {
		prompt := f.paneOf(t, typ).prompts[0]
		if !strings.Contains(prompt, "FAILED (exit 1)") || !strings.Contains(prompt, "3 passed") {
			t.Errorf("%s reviewer did not get the check result:\n%s", typ, prompt)
		}
	}
}

func TestRunChecksPassAndTimeout(t *testing.T) {
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, ".wick", "check"), "echo all good\n")
	if got := runChecks(osExec, tree); !strings.Contains(got, "passed (exit 0)") || !strings.Contains(got, "all good") {
		t.Errorf("got %q", got)
	}
	old := checkTimeout
	checkTimeout = 200e6 // 200ms
	t.Cleanup(func() { checkTimeout = old })
	writeFile(t, filepath.Join(tree, ".wick", "check"), "sleep 5\n")
	if got := runChecks(osExec, tree); !strings.Contains(got, "TIMED OUT") {
		t.Errorf("a timed-out check must say so, got %q", got)
	}
}

// finish merges only once every reviewer type the worker's type requires passed
// at the worker's tip, names what is missing otherwise, and closes the
// reviewers when the ticket ends either way.
func TestFinishNeedsBothReviewerTypesToPassAtTheTip(t *testing.T) {
	repo, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))

	res, err := Finish(f.run, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged || !res.Kept || !strings.Contains(res.Detail, "no review yet") {
		t.Fatalf("an unreviewed worker must not merge: %+v", res)
	}

	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	res, _ = Finish(f.run, "07-x", true)
	if res.Merged || !strings.Contains(res.Detail, reviewerSpec+": changes at") {
		t.Fatalf("a spec reviewer asking for changes must block the merge: %+v", res)
	}

	f.answer = answers("pass", "pass")
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	gitOut(t, tree, "commit", "-q", "--allow-empty", "-m", "after the review")
	res, _ = Finish(f.run, "07-x", true)
	if res.Merged || !strings.Contains(res.Detail, "not at the tip") {
		t.Fatalf("a pass older than the tip must block the merge: %+v", res)
	}

	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	res, err = Finish(f.run, "07-x", true)
	if err != nil || !res.Merged {
		t.Fatalf("both passed at the tip: want merged, got %+v %v", res, err)
	}
	if gitOut(t, repo, "rev-parse", "main") != gitOut(t, repo, "rev-parse", "implement/07-x") {
		t.Error("main was not fast-forwarded to the worker")
	}
	for id, p := range f.panes {
		if p.launched && !p.closed {
			t.Errorf("reviewer pane %s still open after the merge", id)
		}
	}
	if len(reviewersOf(tree)) != 0 {
		t.Error("the merged worker's reviewers are still on record")
	}
}

// A repo worker type that names a different requires_review changes what the
// gate needs: one that requires only the spec reviewer merges on its pass.
func TestRepoWorkerRequiresReviewChangesTheGate(t *testing.T) {
	_, tree := reviewRepo(t)
	// The override lives in the worker's repo, which is where finish reads it.
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	w := store["07-x"]
	writeFile(t, filepath.Join(w.Repo, ".wick", "agents", "worker.md"), `---
name: worker
description: "only the spec reviewer gates this one"
tools: [read, edit, write, bash, find, background]
checkout: worktree
blocking: false
statuses: [done, blocked]
spawns: [reviewer-standards, reviewer-spec]
requires_review: [reviewer-spec]
---
{{prompt}}
`)
	tip := gitOut(t, tree, "rev-parse", "HEAD")
	if err := mergeReviewerVerdict(tree, reviewerStandards, verdictPass, tip, tip, ""); err != nil {
		t.Fatal(err)
	}
	res, err := Finish(osExec, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged {
		t.Fatalf("the standards reviewer passed but the worker type requires only the spec reviewer, so it must wait: %+v", res)
	}
	if err := mergeReviewerVerdict(tree, reviewerSpec, verdictPass, tip, tip, ""); err != nil {
		t.Fatal(err)
	}
	res, err = Finish(osExec, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Merged {
		t.Fatalf("with the required spec pass, want merged: %+v", res)
	}
}

func TestDiscardNeedsNoReviewAndClosesReviewers(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	res, err := Finish(f.run, "07-x", false)
	if err != nil || res.Merged || res.Kept {
		t.Fatalf("discard: %+v %v", res, err)
	}
	for id, p := range f.panes {
		if p.launched && !p.closed {
			t.Errorf("reviewer pane %s still open after the discard", id)
		}
	}
}

// passReviews records the reviewers a worker's type requires as passed at the
// branch tip, for tests of finish's git side that are not about the gate.
func passReviews(t *testing.T, tree, tip string) {
	t.Helper()
	w := workerAt(tree)
	for _, typ := range requiredReviewers(w) {
		if err := mergeReviewerVerdict(tree, typ, verdictPass, tip, tip, ""); err != nil {
			t.Fatal(err)
		}
	}
}

// A pass on uncommitted work is tied to no commit: it never opens the merge
// gate, and finish refuses a dirty worktree whose work it would drop.
func TestAPassOnUncommittedWorkDoesNotMerge(t *testing.T) {
	_, tree := reviewRepo(t)
	writeFile(t, filepath.Join(tree, "b.go"), "package a\n")
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	res, err := Finish(f.run, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged || !strings.Contains(res.Detail, "uncommitted") {
		t.Fatalf("a dirty worktree must not merge: %+v", res)
	}
	gitOut(t, tree, "add", ".")
	gitOut(t, tree, "commit", "-qm", "commit b")
	if res, _ := Finish(f.run, "07-x", true); res.Merged || !strings.Contains(res.Detail, "not at the tip") {
		t.Fatalf("the pass on the dirty tree must not count for the commit: %+v", res)
	}
}

// After the worker rebases onto a target that moved, its diff starts at the
// new tip, not at the spawn commit: other tickets' work stays out of it, and
// the reviewer is told to read the full diff again.
func TestReviewSpawnAfterARebaseShowsOnlyTheWorkersChange(t *testing.T) {
	repo, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "other.go"), "package other\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "another ticket merged")
	gitOut(t, tree, "rebase", "-q", "main")
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	full, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "reviews", "07-x.diff"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(full), "other.go") || !strings.Contains(string(full), "func A") {
		t.Errorf("the diff must hold the worker's change only:\n%s", full)
	}
	if p := f.paneOf(t, reviewerStandards); !strings.Contains(p.prompts[1], "was rebased") {
		t.Errorf("a rebased branch's follow-up must say so:\n%s", p.prompts[1])
	}
}

// Both reviewers dead at once: each replacement finds a free slot, and the
// review completes rather than failing on a source pane that is gone too. The
// spec reviewer is only replaced when its verdict is not a pass, so its first
// verdict is changes here.
func TestReviewSpawnReplacesBothDeadReviewers(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	f.answer = answers("pass", "pass")
	// A new commit makes both reviewers due for review again, so both are
	// replaced rather than left alone as already-passed.
	gitOut(t, tree, "commit", "-q", "--allow-empty", "-m", "move the tip")
	for _, typ := range defaultReviewers {
		f.paneOf(t, typ).closed = true
	}
	out, err := reviewSpawn(t, f, tree, bothReviewers()...)
	if err != nil {
		t.Fatalf("both reviewers replaced: %v\n%s", err, out)
	}
	for _, typ := range defaultReviewers {
		r := reviewersOf(tree)[typ]
		if r.Verdict != verdictPass || r.Tab != reviewTab || !r.agent().alive(f.run) {
			t.Errorf("%s reviewer after replacement = %+v", typ, r)
		}
	}
}

// A spec reviewer's pass holds across later commits: after it passed, a code
// change that only answers the standards reviewer does not send the spec
// reviewer back in. Only the standards reviewer is prompted again, and the
// merge gate accepts the older spec pass.
func TestSpecPassHoldsWhileOnlyStandardsIsReprompted(t *testing.T) {
	repo, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "pass"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	gitOut(t, tree, "commit", "-q", "--allow-empty", "-m", "fix the standards findings")
	f.answer = answers("pass", "pass")
	out, err := reviewSpawn(t, f, tree, bothReviewers()...)
	if err != nil {
		t.Fatal(err)
	}
	spec, standards := f.paneOf(t, reviewerSpec), f.paneOf(t, reviewerStandards)
	if spec.turns != 1 || standards.turns != 2 {
		t.Errorf("turns: spec %d (want 1, its pass holds), standards %d (want 2)", spec.turns, standards.turns)
	}
	if strings.Contains(out, reviewerSpec) {
		t.Errorf("the reply must not carry the spec reviewer: %s", out)
	}
	tip := gitOut(t, tree, "rev-parse", "HEAD")
	if why := reviewGate(tree, tip, defaultReviewers, passHolds(repo)); why != "" {
		t.Errorf("a held spec pass and a tip standards pass must open the gate, got %q", why)
	}
	if why := reviewGate(tree, tip, defaultReviewers, nil); !strings.Contains(why, reviewerSpec) {
		t.Errorf("without a holding type the older spec pass is stale, got %q", why)
	}
}

// A spec reviewer that asked for changes is re-prompted like any other.
func TestSpecWithChangesIsStillReprompted(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	gitOut(t, tree, "commit", "-q", "--allow-empty", "-m", "address the spec findings")
	f.answer = answers("pass", "pass")
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	if turns := f.paneOf(t, reviewerSpec).turns; turns != 2 {
		t.Errorf("spec turns = %d, want 2", turns)
	}
}
