package wick

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// A worktree is reviewed on two axes, by two reviewers that stay alive for the
// whole ticket: code (standards and correctness) and spec (does the diff do
// what the ticket asks, and nothing outside it).
const (
	axisCode = "code"
	axisSpec = "spec"
)

var allAxes = []string{axisCode, axisSpec}

const (
	verdictPass    = "pass"
	verdictChanges = "changes"
)

// reviewer is what wick remembers of one persistent reviewer between calls.
type reviewer struct {
	Pane      string `json:"pane"`
	Harness   string `json:"harness"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model,omitempty"`
	// Reviewed is the commit the reviewer last saw the worktree at.
	Reviewed string `json:"reviewed"`
	// Verdict holds at VerdictAt only: a pass on an older commit is stale.
	Verdict   string `json:"verdict"`
	VerdictAt string `json:"verdict_at"`
	// Findings is its last reply, kept to brief a replacement.
	Findings string `json:"findings"`
}

func (r reviewer) agent() readOnlyAgent {
	return readOnlyAgent{Pane: r.Pane, Harness: r.Harness, Session: readOnlySession(r.SessionID, r.Cwd, r.Model)}
}

// reviewersPath keeps the reviewers of every worktree, keyed by its path.
func reviewersPath() string { return filepath.Join(filepath.Dir(statePath), "reviewers.json") }

var reviewersMu sync.Mutex

func loadReviewers() (map[string]map[string]reviewer, error) {
	raw, err := os.ReadFile(reviewersPath())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]map[string]reviewer{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]reviewer{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveReviewers(all map[string]map[string]reviewer) error {
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(reviewersPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(reviewersPath(), raw, 0o644)
}

// reviewersOf is the reviewers of the worktree at tree.
func reviewersOf(tree string) map[string]reviewer {
	reviewersMu.Lock()
	defer reviewersMu.Unlock()
	all, err := loadReviewers()
	if err != nil {
		return map[string]reviewer{}
	}
	if r := all[filepath.Clean(tree)]; r != nil {
		return r
	}
	return map[string]reviewer{}
}

func storeReviewer(tree, axis string, r reviewer) error {
	reviewersMu.Lock()
	defer reviewersMu.Unlock()
	all, err := loadReviewers()
	if err != nil {
		return err
	}
	key := filepath.Clean(tree)
	if all[key] == nil {
		all[key] = map[string]reviewer{}
	}
	all[key][axis] = r
	return saveReviewers(all)
}

// closeReviewers closes the reviewer panes of tree and forgets them. The
// ticket is over, merged or discarded, and no reviewer outlives it.
func closeReviewers(run Exec, tree string) {
	reviewersMu.Lock()
	defer reviewersMu.Unlock()
	all, err := loadReviewers()
	if err != nil {
		return
	}
	key := filepath.Clean(tree)
	for _, r := range all[key] {
		if r.Pane != "" {
			_, _ = run(herdrBin(), []string{"pane", "close", r.Pane}, ExecOpts{Timeout: 15 * time.Second})
		}
	}
	delete(all, key)
	_ = saveReviewers(all)
}

// reviewGate is why the worktree at tree, whose tip is head, may not merge
// yet, or "" when every reviewer it needs passed at head.
func reviewGate(tree, head string, axes []string) string {
	have := reviewersOf(tree)
	var missing []string
	for _, axis := range axes {
		r, ok := have[axis]
		switch {
		case !ok || r.Verdict == "":
			missing = append(missing, axis+" reviewer: no review yet")
		case r.Verdict != verdictPass:
			missing = append(missing, fmt.Sprintf("%s reviewer: %s at %s", axis, r.Verdict, short(r.VerdictAt)))
		case r.VerdictAt != head:
			missing = append(missing, fmt.Sprintf("%s reviewer: passed at %s, not at the tip %s", axis, short(r.VerdictAt), short(head)))
		}
	}
	return strings.Join(missing, "; ")
}

// axesFor is the reviewers a worktree needs: spec review needs a ticket to
// judge against, so a tree wick did not spawn gets the code reviewer only.
func axesFor(w *Worker) []string {
	if w == nil || w.Ticket == "" {
		return []string{axisCode}
	}
	return allAxes
}

func workerAt(tree string) *Worker {
	store, err := load()
	if err != nil {
		return nil
	}
	for _, w := range store {
		if filepath.Clean(w.Worktree) == filepath.Clean(tree) {
			return &w
		}
	}
	return nil
}

// checkTimeout bounds the repo's check command, inside the tool budget.
var checkTimeout = 3 * time.Minute

// runChecks runs the one command in .wick/check at the root of tree, so the
// reviewers judge real results rather than the worker's word. A repo with no
// such file is reviewed without checks, and the packet says so.
func runChecks(run Exec, tree string) string {
	raw, err := os.ReadFile(filepath.Join(tree, ".wick", "check"))
	cmd := strings.TrimSpace(string(raw))
	if err != nil || cmd == "" {
		return "no checks configured (the repo has no .wick/check)"
	}
	start := time.Now()
	res, err := run("sh", []string{"-c", cmd}, ExecOpts{Cwd: tree, Timeout: checkTimeout})
	out := tail(strings.TrimSpace(res.Stdout+"\n"+res.Stderr), 60)
	switch {
	case err != nil:
		return fmt.Sprintf("`%s` could not run: %v", cmd, err)
	case res.Code != 0 && time.Since(start) >= checkTimeout:
		return fmt.Sprintf("`%s` TIMED OUT after %s, which is not a pass. Output so far:\n%s", cmd, checkTimeout, out)
	case res.Code != 0:
		return fmt.Sprintf("`%s` FAILED (exit %d):\n%s", cmd, res.Code, out)
	default:
		return fmt.Sprintf("`%s` passed (exit 0):\n%s", cmd, out)
	}
}

// tail keeps the last n lines of text: a check's verdict is at its end.
func tail(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return "…\n" + strings.Join(lines[len(lines)-n:], "\n")
}

// reviewPacket is what one call hands its reviewers.
type reviewPacket struct {
	tree, head, base string
	verdictAt        string // head, or head+"+uncommitted" when the tree is dirty
	fullDiff         string // path of the full diff, base..working tree
	checks           string
	note             string
	ticket, spec     string
	model            string
}

// CodeReview has the worktree at tree reviewed. The first call starts a code
// and a spec reviewer; they stay alive, and a later call sends them a
// follow-up with what changed since they last looked, rather than a fresh
// reviewer re-reading everything. Without only, a call goes to every reviewer
// still asking for changes, plus any whose pass is older than HEAD; only
// names one reviewer instead. note is the worker's answer to the findings.
func CodeReview(run Exec, tree, only, note string) (string, error) {
	w := workerAt(tree)
	axes := axesFor(w)
	if only != "" && !contains(axes, only) {
		return "", fmt.Errorf("reviewer must be one of %s", strings.Join(axes, ", "))
	}
	git := func(args ...string) (string, error) {
		res, err := run("git", args, ExecOpts{Cwd: tree, Timeout: time.Minute})
		if err != nil {
			return "", err
		}
		if res.Code != 0 {
			return "", fmt.Errorf("git %s failed: %s", args[0], firstLine(res.Stderr, "unknown error"))
		}
		return res.Stdout, nil
	}
	headOut, err := git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(headOut)
	// A pass is tied to a commit, and uncommitted work is not in one: a pass
	// on a dirty tree is recorded against no commit, so it never opens the
	// merge gate.
	verdictAt := head
	if status, err := git("status", "--porcelain"); err == nil && strings.TrimSpace(status) != "" {
		verdictAt = head + "+uncommitted"
	}
	base := diffBase(run, tree, w)
	// The base against the *working tree*: a worker reviews mid-branch, and
	// only this form covers commits and uncommitted work in one diff.
	full, err := git("diff", base)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(full) == "" {
		return "no changes: nothing differs from the base. Nothing to review.", nil
	}

	have := reviewersOf(tree)
	var targets []string
	for _, axis := range axes {
		r, ok := have[axis]
		switch {
		case only != "":
			if axis == only {
				targets = append(targets, axis)
			}
		case !ok || r.Verdict != verdictPass || r.VerdictAt != verdictAt:
			targets = append(targets, axis)
		}
	}
	if len(targets) == 0 {
		return fmt.Sprintf("every reviewer passed at %s; nothing to review.", short(head)), nil
	}

	dir := filepath.Join(filepath.Dir(statePath), "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(tree)
	p := reviewPacket{tree: tree, head: head, verdictAt: verdictAt, base: base, note: note, fullDiff: filepath.Join(dir, name+".diff")}
	if err := os.WriteFile(p.fullDiff, []byte(full), 0o644); err != nil {
		return "", err
	}
	if w != nil {
		p.ticket, p.spec, p.model = w.Ticket, w.Spec, w.Model
	}
	p.checks = runChecks(run, tree)

	type outcome struct {
		axis, reply, verdict, notice string
		err                          error
	}
	results := make([]outcome, len(targets))
	var wg sync.WaitGroup
	for i, axis := range targets {
		wg.Add(1)
		go func(i int, axis string) {
			defer wg.Done()
			prev, had := have[axis]
			r, reply, notice, err := reviewOn(run, git, dir, name, axis, p, prev, had)
			results[i] = outcome{axis: axis, reply: reply, verdict: r.Verdict, notice: notice, err: err}
			if err == nil {
				err = storeReviewer(tree, axis, r)
				results[i].err = err
			}
		}(i, axis)
	}
	wg.Wait()

	var b strings.Builder
	fmt.Fprintf(&b, "Review at %s. Checks: %s\n", short(head), firstLine(p.checks, p.checks))
	failed := 0
	for _, o := range results {
		if o.err != nil {
			failed++
			fmt.Fprintf(&b, "\n## %s reviewer: failed\n%v\n", o.axis, o.err)
			continue
		}
		fmt.Fprintf(&b, "\n## %s reviewer: %s at %s\n", o.axis, o.verdict, short(head))
		if o.notice != "" {
			b.WriteString(o.notice + "\n")
		}
		b.WriteString(o.reply + "\n")
	}
	if failed == len(results) {
		return "", errors.New(strings.TrimSpace(b.String()))
	}
	if verdictAt != head {
		b.WriteString("\nThis review covered uncommitted work, so no pass counts toward merging: commit, then call code_review again.\n")
	}
	b.WriteString("\nFix what you agree with and call code_review again; decline the rest with a reason in note.")
	return strings.TrimSpace(b.String()), nil
}

// reviewOn runs one reviewer for one call: a follow-up to the live reviewer it
// had, or a fresh one when it had none or its pane is gone.
func reviewOn(run Exec, git func(...string) (string, error), dir, name, axis string, p reviewPacket,
	prev reviewer, had bool) (reviewer, string, string, error) {
	var agent readOnlyAgent
	var reply, notice string
	var err error

	if had && prev.agent().alive(run) {
		agent = prev.agent()
		// A branch rebased since the reviewer last looked no longer holds
		// the commit it saw, and a diff from it would show every other
		// ticket merged meanwhile: the reviewer gets the full diff again.
		incPath := p.fullDiff
		if _, err := git("merge-base", "--is-ancestor", prev.Reviewed, "HEAD"); err == nil {
			inc, err := git("diff", prev.Reviewed)
			if err != nil {
				return reviewer{}, "", "", err
			}
			incPath = filepath.Join(dir, name+"."+axis+".since.diff")
			if err := os.WriteFile(incPath, []byte(inc), 0o644); err != nil {
				return reviewer{}, "", "", err
			}
		}
		promptPath := filepath.Join(dir, name+"."+axis+".followup")
		if err := os.WriteFile(promptPath, []byte(followUpPrompt(axis, p, prev, incPath)), 0o644); err != nil {
			return reviewer{}, "", "", err
		}
		reply, err = agent.tell(run, fmt.Sprintf("Follow-up review: read %s and follow it.", promptPath))
		if err != nil {
			return reviewer{}, "", "", err
		}
	} else {
		prompt := firstReviewPrompt(axis, p)
		if had {
			// The reviewer this one replaces is gone; it inherits the
			// findings, so a fix is checked against them and not lost.
			prompt += fmt.Sprintf("\n\nYou replace a %s reviewer whose pane closed. Its last reply, at %s, was:\n\n%s\n\nCheck whether each of its findings still holds.",
				axis, short(prev.Reviewed), prev.Findings)
			notice = fmt.Sprintf("(the previous %s reviewer's pane was gone: a new reviewer took over, briefed with its findings)", axis)
		}
		agent, err = startReadOnly(run, p.tree, p.model, prompt)
		if err != nil {
			return reviewer{}, "", "", err
		}
		reply, err = agent.reply(run, 0)
		if err != nil {
			// Nothing remembers a reviewer that never answered, so its pane
			// is closed here or it is never closed at all.
			_, _ = run(herdrBin(), []string{"pane", "close", agent.Pane}, ExecOpts{Timeout: 15 * time.Second})
			return reviewer{}, "", "", err
		}
	}

	verdict := parseVerdict(reply)
	if verdict == "" {
		more, err := agent.tell(run, "End your reply with exactly one line: VERDICT: pass or VERDICT: changes.")
		if err == nil {
			reply = strings.TrimSpace(reply + "\n" + more)
			verdict = parseVerdict(more)
		}
		if verdict == "" {
			verdict = verdictChanges
			reply += "\n(no VERDICT line after a nudge: counted as changes)"
		}
	}
	r := reviewer{
		Pane: agent.Pane, Harness: agent.Harness, SessionID: agent.Session.ID, Cwd: agent.Session.Cwd, Model: agent.Session.Model,
		Reviewed: p.head, Verdict: verdict, VerdictAt: p.verdictAt, Findings: reply,
	}
	return r, reply, notice, nil
}

var verdictLine = regexp.MustCompile(`(?im)^\s*\**\s*VERDICT:\s*\**\s*(pass|changes)\b`)

// parseVerdict is the verdict on the reply's last VERDICT line, or "".
func parseVerdict(reply string) string {
	m := verdictLine.FindAllStringSubmatch(reply, -1)
	if len(m) == 0 {
		return ""
	}
	return strings.ToLower(m[len(m)-1][1])
}

const findingRules = `One line per finding: file:line severity: problem. Severity is blocker, major or minor. Skip style and praise.
The ticket's hard limits bind the worker: never ask for an edit they forbid. The worker may decline a finding with a reason; accept a reasoned decline of anything outside the ticket's scope or against the spec, and say so.
You can only read and find. You stay open after this reply: later messages bring follow-ups with what changed, and you re-judge your open findings against them.
End with exactly one line: VERDICT: pass (no blocker or major left) or VERDICT: changes.`

func firstReviewPrompt(axis string, p reviewPacket) string {
	var b strings.Builder
	switch axis {
	case axisSpec:
		fmt.Fprintf(&b, "You are the spec reviewer for the worktree %s.\n\n", p.tree)
		fmt.Fprintf(&b, "The ticket is %s.", p.ticket)
		if p.spec != "" {
			fmt.Fprintf(&b, " It belongs to the spec at %s.", p.spec)
		}
		b.WriteString(" Judge whether the diff does what the ticket asks, every acceptance criterion, and stays inside the ticket and consistent with the spec's decisions. Work the ticket does not ask for is a finding. Code quality is the code reviewer's, not yours.\n\n")
	default:
		fmt.Fprintf(&b, "You are the code reviewer for the worktree %s.\n\n", p.tree)
		b.WriteString("Judge the diff for correctness and this repo's standards (read its AGENTS.md, CLAUDE.md or CONTRIBUTING if present): bugs, missed edge cases, callers the change broke, new logic with no test. Whether it matches the ticket is the spec reviewer's, not yours.\n\n")
	}
	fmt.Fprintf(&b, "The change is the git diff at %s, from %s to the working tree; read files in the worktree for context.\n\n", p.fullDiff, short(p.base))
	fmt.Fprintf(&b, "Checks run by wick on this tree: %s\n\n", p.checks)
	if p.note != "" {
		fmt.Fprintf(&b, "The worker says: %s\n\n", p.note)
	}
	b.WriteString(findingRules)
	return b.String()
}

func followUpPrompt(axis string, p reviewPacket, prev reviewer, incPath string) string {
	var b strings.Builder
	if incPath == p.fullDiff {
		fmt.Fprintf(&b, "Follow-up for the %s review. The branch was rebased since you last looked (%s), so read the full diff again, at %s.\n\n",
			axis, short(prev.Reviewed), p.fullDiff)
	} else {
		fmt.Fprintf(&b, "Follow-up for the %s review. What changed since you last looked (%s) is the diff at %s; the full diff is still at %s.\n\n",
			axis, short(prev.Reviewed), incPath, p.fullDiff)
	}
	if prev.Verdict == verdictPass {
		fmt.Fprintf(&b, "You passed this work at %s. It changed since, likely for the other reviewer: check that the change did not break what you approved.\n\n", short(prev.VerdictAt))
	} else {
		b.WriteString("Re-judge each of your open findings: say which are resolved, which still hold, and anything new the change introduced.\n\n")
	}
	fmt.Fprintf(&b, "Checks run by wick on this tree: %s\n\n", p.checks)
	if p.note != "" {
		fmt.Fprintf(&b, "The worker says: %s\n\n", p.note)
	}
	b.WriteString(findingRules)
	return b.String()
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// diffBase is where the worker's own change starts. For a worker with a
// target it is the merge-base with that target: after the worker rebases onto
// the integration tip, that is the tip, so other tickets merged meanwhile stay
// out of its diff. Otherwise it is the commit recorded at spawn. Outside a
// worktree wick created there is no record, and the default branch is the
// only fallback, main or else master.
func diffBase(run Exec, tree string, w *Worker) string {
	git := func(args ...string) (string, bool) {
		res, err := run("git", args, ExecOpts{Cwd: tree, Timeout: 30 * time.Second})
		return strings.TrimSpace(res.Stdout), err == nil && res.Code == 0
	}
	if w != nil && w.Target != "" {
		if mb, ok := git("merge-base", "HEAD", w.Target); ok && mb != "" {
			return mb
		}
	}
	if w != nil && w.Base != "" {
		return w.Base
	}
	for _, b := range []string{"main", "master"} {
		if _, ok := git("rev-parse", "--verify", "--quiet", b); ok {
			return b
		}
	}
	return "main"
}
