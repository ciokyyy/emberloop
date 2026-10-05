package wick

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeReviewers is a herdr whose panes run reviewer agents. Each pane is a
// persistent agent: launched by pane run, it answers its prompt; a message
// typed into it later is a follow-up it answers in the same transcript. git
// and sh are real, in a temp repo.
type fakeReviewers struct {
	mu     sync.Mutex
	next   int
	panes  map[string]*fakePane
	calls  [][]string
	answer func(axis string, turn int, prompt string) string
}

type fakePane struct {
	axis, transcript string
	turns            int
	closed           bool
	prompts          []string
}

func newFakeReviewers(answer func(axis string, turn int, prompt string) string) *fakeReviewers {
	return &fakeReviewers{panes: map[string]*fakePane{}, answer: answer}
}

func (f *fakeReviewers) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == "git" || cmd == "sh" {
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
		ids := []string{"root"}
		for id := range f.panes {
			ids = append(ids, id)
		}
		sort.Strings(ids[1:])
		tab := os.Getenv("HERDR_TAB_ID")
		panes := []any{}
		for _, id := range ids {
			panes = append(panes, map[string]any{"pane_id": id, "tab_id": tab})
		}
		return jsonReply(map[string]any{"panes": panes}), nil
	case "pane split":
		f.next++
		id := fmt.Sprintf("rv%d", f.next)
		f.panes[id] = &fakePane{}
		return Result{Stdout: `{"result":{"pane":{"pane_id":"` + id + `"}}}`}, nil
	case "pane run":
		p := f.panes[args[2]]
		var dir, id, promptPath string
		for i, w := range strings.Fields(args[3]) {
			w = strings.Trim(w, `"`)
			switch {
			case w == "--session-dir":
				dir = strings.Trim(strings.Fields(args[3])[i+1], `"`)
			case w == "--session-id":
				id = strings.Trim(strings.Fields(args[3])[i+1], `"`)
			case strings.HasSuffix(w, ".prompt"):
				promptPath = w
			}
		}
		raw, _ := os.ReadFile(promptPath)
		prompt := string(raw)
		p.axis = axisCode
		if strings.Contains(prompt, "You are the spec reviewer") {
			p.axis = axisSpec
		}
		_ = os.MkdirAll(filepath.Join(dir, "--x--"), 0o755)
		p.transcript = filepath.Join(dir, "--x--", "t_"+id+".jsonl")
		f.turn(p, prompt)
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
	case "agent prompt":
		// herdr does not prompt pig; Nudge falls back to typing.
		return Result{Code: 1}, nil
	}
	return Result{Stdout: "{}"}, nil
}

// turn appends the prompt and the agent's answer to the pane's transcript.
func (f *fakeReviewers) turn(p *fakePane, prompt string) {
	p.turns++
	p.prompts = append(p.prompts, prompt)
	reply := f.answer(p.axis, p.turns, prompt)
	user, _ := json.Marshal(map[string]any{"type": "message", "message": map[string]any{
		"role": "user", "content": []any{map[string]any{"type": "text", "text": prompt}}}})
	asst, _ := json.Marshal(map[string]any{"type": "message", "message": map[string]any{
		"role": "assistant", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": reply}}}})
	fh, _ := os.OpenFile(p.transcript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintf(fh, "%s\n%s\n", user, asst)
	fh.Close()
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

// paneOf is the live pane running the reviewer of axis.
func (f *fakeReviewers) paneOf(t *testing.T, axis string) *fakePane {
	t.Helper()
	for _, p := range f.panes {
		if p.axis == axis && !p.closed {
			return p
		}
	}
	t.Fatalf("no live %s reviewer", axis)
	return nil
}

// reviewRepo is a repo with a worker worktree holding one committed change,
// recorded as a worker with a ticket and a spec.
func reviewRepo(t *testing.T) (repo, tree string) {
	t.Helper()
	old := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = old })
	t.Setenv("WICK_READONLY_HARNESS", "pig")
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
		Branch: "implement/07-x", Worktree: tree, Pane: "%1", Workspace: "%2",
	}}); err != nil {
		t.Fatal(err)
	}
	return repo, tree
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

// answers gives each axis a fixed verdict for every turn.
func answers(code, spec string) func(string, int, string) string {
	return func(axis string, _ int, _ string) string {
		if axis == axisSpec {
			return "a.go:3 major: ticket asks for B\nVERDICT: " + spec
		}
		return "VERDICT: " + code
	}
}

func TestCodeReviewStartsTwoReviewers(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	out, err := CodeReview(f.run, tree, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 2 {
		t.Errorf("want two reviewers launched, got %d splits", f.splits())
	}
	for _, want := range []string{"## code reviewer: pass", "## spec reviewer: changes", "ticket asks for B", "no checks configured"} {
		if !strings.Contains(out, want) {
			t.Errorf("reply missing %q:\n%s", want, out)
		}
	}
	spec := f.paneOf(t, axisSpec).prompts[0]
	if !strings.Contains(spec, "/t/07-x.md") || !strings.Contains(spec, "/t/spec.md") {
		t.Errorf("the spec reviewer must be pointed at ticket and spec:\n%s", spec)
	}
	have := reviewersOf(tree)
	head := gitOut(t, tree, "rev-parse", "HEAD")
	if have[axisCode].Verdict != verdictPass || have[axisCode].VerdictAt != head || have[axisSpec].Verdict != verdictChanges {
		t.Errorf("stored reviewers = %+v", have)
	}
	// Reviewers stay alive for follow-ups.
	for _, p := range f.panes {
		if p.closed {
			t.Error("a reviewer pane was closed after its first reply")
		}
	}
}

// A follow-up goes, by default, only to reviewers with open findings, in the
// pane it already has, carrying the worker's note; a satisfied reviewer at
// HEAD is left alone.
func TestCodeReviewFollowUpGoesToOpenFindingsOnly(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	f.answer = answers("pass", "pass")
	out, err := CodeReview(f.run, tree, "", "B is out of scope: the ticket says A only")
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 2 {
		t.Errorf("a follow-up must reuse the live reviewers, got %d splits", f.splits())
	}
	spec, code := f.paneOf(t, axisSpec), f.paneOf(t, axisCode)
	if spec.turns != 2 || code.turns != 1 {
		t.Errorf("turns: spec %d (want 2), code %d (want 1)", spec.turns, code.turns)
	}
	if !strings.Contains(spec.prompts[1], "B is out of scope") || !strings.Contains(spec.prompts[1], "Follow-up") {
		t.Errorf("the follow-up must carry the note:\n%s", spec.prompts[1])
	}
	if !strings.Contains(out, "## spec reviewer: pass") || strings.Contains(out, "## code reviewer") {
		t.Errorf("reply = %s", out)
	}
	if got := reviewersOf(tree)[axisSpec].Verdict; got != verdictPass {
		t.Errorf("spec verdict = %q", got)
	}
}

func TestCodeReviewNamedReviewerOnly(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := CodeReview(f.run, tree, axisCode, "look again"); err != nil {
		t.Fatal(err)
	}
	if spec, code := f.paneOf(t, axisSpec), f.paneOf(t, axisCode); spec.turns != 1 || code.turns != 2 {
		t.Errorf("reviewer=code must reach code only: spec %d, code %d", spec.turns, code.turns)
	}
	if _, err := CodeReview(f.run, tree, "style", ""); err == nil {
		t.Error("an unknown reviewer must be refused")
	}
}

// A pass is tied to a commit: once HEAD moves, the reviewer that passed sees
// the diff since its pass and is asked whether the change broke what it
// approved.
func TestCodeReviewRechecksAStalePass(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	passedAt := gitOut(t, tree, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(tree, "a.go"), "package a\n\nfunc A() {}\n\nfunc B() {}\n")
	gitOut(t, tree, "commit", "-qam", "add B for the spec reviewer")
	f.answer = answers("pass", "pass")
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	code := f.paneOf(t, axisCode)
	if code.turns != 2 {
		t.Fatalf("a stale pass must be rechecked, code turns = %d", code.turns)
	}
	if !strings.Contains(code.prompts[1], "You passed this work at "+short(passedAt)) {
		t.Errorf("recheck prompt:\n%s", code.prompts[1])
	}
	since, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "reviews", "07-x.code.since.diff"))
	if err != nil || !strings.Contains(string(since), "func B") || strings.Contains(string(since), "+func A") {
		t.Errorf("the recheck diff must be only what changed since the pass:\n%s", since)
	}
	if out, _ := CodeReview(f.run, tree, "", ""); !strings.Contains(out, "every reviewer passed") {
		t.Errorf("both passed at HEAD: nothing to review, got %s", out)
	}
}

func TestCodeReviewMissingVerdictIsNudgedThenChanges(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(func(axis string, turn int, _ string) string {
		if axis == axisSpec && turn == 2 {
			return "ok, VERDICT: pass"
		}
		return "looks fine"
	})
	out, err := CodeReview(f.run, tree, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Both were nudged once; the spec reviewer's answer to the nudge still
	// has no verdict *line*, so both count as changes.
	if !strings.Contains(out, "## code reviewer: changes") || !strings.Contains(out, "## spec reviewer: changes") {
		t.Errorf("reply = %s", out)
	}
	if p := f.paneOf(t, axisCode); p.turns != 2 || !strings.Contains(p.prompts[1], "VERDICT") {
		t.Errorf("the code reviewer must be nudged for a verdict: %+v", p.prompts)
	}
}

func TestParseVerdict(t *testing.T) {
	for in, want := range map[string]string{
		"x\nVERDICT: pass":                      "pass",
		"VERDICT: changes\nmore\nVERDICT: pass": "pass",
		"**VERDICT:** changes":                  "changes",
		"no verdict":                            "",
		"the VERDICT: pass is inline":           "",
	} {
		if got := parseVerdict(in); got != want {
			t.Errorf("parseVerdict(%q) = %q, want %q", in, got, want)
		}
	}
}

// A reviewer whose pane is gone is replaced by a fresh one briefed with its
// predecessor's findings, and the worker is told.
func TestCodeReviewReplacesADeadReviewer(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	f.paneOf(t, axisSpec).closed = true
	out, err := CodeReview(f.run, tree, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 3 {
		t.Errorf("want one replacement launch, got %d splits", f.splits())
	}
	fresh := f.paneOf(t, axisSpec)
	if !strings.Contains(fresh.prompts[0], "ticket asks for B") || !strings.Contains(fresh.prompts[0], "You replace") {
		t.Errorf("the replacement must inherit the findings:\n%s", fresh.prompts[0])
	}
	if !strings.Contains(out, "a new reviewer took over") {
		t.Errorf("the worker must be told: %s", out)
	}
}

// The repo's check command runs before review and its result reaches both
// reviewers; a failing check is reported as such.
func TestCodeReviewRunsTheRepoCheck(t *testing.T) {
	_, tree := reviewRepo(t)
	writeFile(t, filepath.Join(tree, ".wick", "check"), "echo checking; echo 3 passed; exit 1\n")
	f := newFakeReviewers(answers("changes", "changes"))
	out, err := CodeReview(f.run, tree, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, axis := range allAxes {
		prompt := f.paneOf(t, axis).prompts[0]
		if !strings.Contains(prompt, "FAILED (exit 1)") || !strings.Contains(prompt, "3 passed") {
			t.Errorf("%s reviewer did not get the check result:\n%s", axis, prompt)
		}
	}
	if !strings.Contains(out, "FAILED") {
		t.Errorf("the reply must say the check failed: %s", out)
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

// finish_worker merges only once both reviewers passed at the worker's tip,
// names what is missing otherwise, and closes the reviewers when the ticket
// ends either way.
func TestFinishNeedsBothReviewersToPassAtTheTip(t *testing.T) {
	repo, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))

	res, err := Finish(f.run, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged || !res.Kept || !strings.Contains(res.Detail, "no review yet") {
		t.Fatalf("an unreviewed worker must not merge: %+v", res)
	}

	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	res, _ = Finish(f.run, "07-x", true)
	if res.Merged || !strings.Contains(res.Detail, "spec reviewer: changes at") {
		t.Fatalf("a spec reviewer asking for changes must block the merge: %+v", res)
	}

	f.answer = answers("pass", "pass")
	if _, err := CodeReview(f.run, tree, "", "done"); err != nil {
		t.Fatal(err)
	}
	gitOut(t, tree, "commit", "-q", "--allow-empty", "-m", "after the review")
	res, _ = Finish(f.run, "07-x", true)
	if res.Merged || !strings.Contains(res.Detail, "not at the tip") {
		t.Fatalf("a pass older than the tip must block the merge: %+v", res)
	}

	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
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
		if !p.closed {
			t.Errorf("reviewer pane %s still open after the merge", id)
		}
	}
	if len(reviewersOf(tree)) != 0 {
		t.Error("the merged worker's reviewers are still on record")
	}
}

func TestDiscardNeedsNoReviewAndClosesReviewers(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	res, err := Finish(f.run, "07-x", false)
	if err != nil || res.Merged || res.Kept {
		t.Fatalf("discard: %+v %v", res, err)
	}
	for id, p := range f.panes {
		if !p.closed {
			t.Errorf("reviewer pane %s still open after the discard", id)
		}
	}
}

func TestCodeReviewToolTakesReviewerAndNote(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	old := reviewExec
	reviewExec = f.run
	t.Cleanup(func() { reviewExec = old })
	args, _ := json.Marshal(map[string]any{"reviewer": "code", "note": "first look"})
	out, err := codeReviewTool.Execute(context.Background(), tree, args)
	if err != nil {
		t.Fatal(err)
	}
	if f.splits() != 1 || !strings.Contains(out, "## code reviewer: pass") || !strings.Contains(f.paneOf(t, axisCode).prompts[0], "first look") {
		t.Errorf("out = %s", out)
	}
}

// passReviews records both reviewers as passed at the branch tip, for tests
// of finish_worker's git side that are not about the review gate.
func passReviews(t *testing.T, tree, tip string) {
	t.Helper()
	for _, axis := range allAxes {
		if err := storeReviewer(tree, axis, reviewer{Verdict: verdictPass, VerdictAt: tip}); err != nil {
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
	out, err := CodeReview(f.run, tree, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "uncommitted work") {
		t.Errorf("the reply must say the pass does not count: %s", out)
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
func TestReviewAfterARebaseShowsOnlyTheWorkersChange(t *testing.T) {
	repo, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "other.go"), "package other\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "another ticket merged")
	gitOut(t, tree, "rebase", "-q", "main")
	if _, err := CodeReview(f.run, tree, "", ""); err != nil {
		t.Fatal(err)
	}
	full, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "reviews", "07-x.diff"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(full), "other.go") || !strings.Contains(string(full), "func A") {
		t.Errorf("the diff must hold the worker's change only:\n%s", full)
	}
	if p := f.paneOf(t, axisCode); !strings.Contains(p.prompts[1], "was rebased") {
		t.Errorf("a rebased branch's follow-up must say so:\n%s", p.prompts[1])
	}
}
