package wick

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The reviewer types wick ships. A worker's reviewers are the types its
// requires_review names; these are the defaults when a record names none.
const (
	reviewerStandards = "reviewer-standards"
	reviewerSpec      = "reviewer-spec"
)

var defaultReviewers = []string{reviewerStandards, reviewerSpec}

// verdictPass and verdictChanges are the built-in reviewers' statuses, and the
// values the merge gate reads.
const (
	verdictPass    = "pass"
	verdictChanges = "changes"
)

// reviewer is what wick remembers of one persistent reviewer between calls. Tab
// and Slot are where it lives: the ticket's tab and its slot in the grid, so a
// dead reviewer is replaced in the same place.
type reviewer struct {
	Pane      string `json:"pane"`
	Tab       string `json:"tab,omitempty"`
	Slot      int    `json:"slot,omitempty"`
	Harness   string `json:"harness"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
	// Reviewed is the commit the reviewer last saw the worktree at.
	Reviewed string `json:"reviewed"`
	// Verdict holds at VerdictAt only: a pass on an older commit is stale,
	// unless VerdictTree, the git tree of the content judged, is the content
	// the commit holds: committing reviewed work changes nothing it judged.
	Verdict     string `json:"verdict"`
	VerdictAt   string `json:"verdict_at"`
	VerdictTree string `json:"verdict_tree,omitempty"`
	// Findings is its last reply, kept to brief a replacement.
	Findings string `json:"findings"`
}

func (r reviewer) agent() readOnlyAgent {
	return readOnlyAgent{
		Pane: r.Pane, Tab: r.Tab, Slot: r.Slot, Harness: r.Harness,
		Session: readOnlySession(r.SessionID, r.Cwd, r.Model, r.Effort),
	}
}

// reviewersPath keeps the reviewers of every worktree, keyed by its path.
func reviewersPath() string { return filepath.Join(filepath.Dir(statePath), "reviewers.json") }

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

// saveReviewers writes the store through a temp file and a rename: the
// reviewer's own process now writes it too, so a reader must never see a
// half-written file.
func saveReviewers(all map[string]map[string]reviewer) error {
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(reviewersPath()), 0o755); err != nil {
		return err
	}
	tmp := reviewersPath() + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, reviewersPath())
}

// updateReviewers runs fn on the store under a cross-process lock. A worker's
// spawn and a reviewer's own submit are separate processes, so the in-process
// mutex the store used to hold could not stop them dropping each other's
// update; the file lock does.
func updateReviewers(fn func(map[string]map[string]reviewer) error) error {
	return withFileLock("reviewers", func() error {
		all, err := loadReviewers()
		if err != nil {
			return err
		}
		if err := fn(all); err != nil {
			return err
		}
		return saveReviewers(all)
	})
}

// reviewersOf is the reviewers of the worktree at tree.
func reviewersOf(tree string) map[string]reviewer {
	var out map[string]reviewer
	_ = withFileLock("reviewers", func() error {
		all, err := loadReviewers()
		if err != nil {
			return err
		}
		out = all[filepath.Clean(tree)]
		return nil
	})
	if out == nil {
		return map[string]reviewer{}
	}
	return out
}

// storeReviewerPane records where a reviewer runs, keeping any verdict already
// on file: a replacement is set up before its pane starts, so its submit can
// fold the new verdict in without losing the slot it took.
func storeReviewerPane(tree, typ string, r reviewer) error {
	return updateReviewers(func(all map[string]map[string]reviewer) error {
		key := filepath.Clean(tree)
		if all[key] == nil {
			all[key] = map[string]reviewer{}
		}
		prev := all[key][typ]
		if prev.Verdict != "" {
			r.Verdict, r.VerdictAt, r.VerdictTree, r.Findings = prev.Verdict, prev.VerdictAt, prev.VerdictTree, prev.Findings
		}
		all[key][typ] = r
		return nil
	})
}

// mergeReviewerVerdict folds a reviewer's submit into the record for its
// worktree: its status is the verdict, its report the findings kept for
// follow-ups and for briefing a replacement, and reviewed the commit it judged.
func mergeReviewerVerdict(tree, typ, verdict, at, reviewed, treeID, findings string) error {
	return updateReviewers(func(all map[string]map[string]reviewer) error {
		key := filepath.Clean(tree)
		if all[key] == nil {
			all[key] = map[string]reviewer{}
		}
		r := all[key][typ]
		r.Verdict, r.VerdictAt, r.VerdictTree, r.Reviewed, r.Findings = verdict, at, treeID, reviewed, findings
		all[key][typ] = r
		return nil
	})
}

// closeReviewers closes the reviewer panes of tree and forgets them. The
// ticket is over, merged or discarded, and no reviewer outlives it.
func closeReviewers(run Exec, tree string) {
	var panes []string
	_ = updateReviewers(func(all map[string]map[string]reviewer) error {
		key := filepath.Clean(tree)
		for _, r := range all[key] {
			if r.Pane != "" {
				panes = append(panes, r.Pane)
			}
		}
		delete(all, key)
		return nil
	})
	for _, pane := range panes {
		_, _ = run(herdrBin(), []string{"pane", "close", pane}, ExecOpts{Timeout: 15 * time.Second})
	}
}

// reviewGate is why the worktree at tree, whose tip is head, may not merge
// yet, or "" when every required reviewer type passed at head. A type whose
// pass holds (holds[type]) needs a pass at some commit, not at head.
func reviewGate(tree, head, headTree string, required []string, holds map[string]bool) string {
	have := reviewersOf(tree)
	var missing []string
	for _, typ := range required {
		r, ok := have[typ]
		switch {
		case !ok || r.Verdict == "":
			missing = append(missing, typ+": no review yet")
		case r.Verdict != verdictPass:
			missing = append(missing, fmt.Sprintf("%s: %s at %s", typ, r.Verdict, short(r.VerdictAt)))
		case r.VerdictAt != head && !holds[typ] && (r.VerdictTree == "" || r.VerdictTree != headTree):
			missing = append(missing, fmt.Sprintf("%s: passed at %s, not at the tip %s", typ, short(r.VerdictAt), short(head)))
		}
	}
	return strings.Join(missing, "; ")
}

// requiredReviewers is the reviewer types a worker's finish needs: its type's
// requires_review. A record without a type falls back to the two built-in
// reviewers, so a worker spawned before types recorded their name still gates
// on review.
func requiredReviewers(w *Worker) []string {
	if w == nil || w.AgentType == "" {
		return defaultReviewers
	}
	types, err := LoadAgentTypes(w.Repo)
	if err != nil {
		return defaultReviewers
	}
	t, ok := types[w.AgentType]
	if !ok {
		return defaultReviewers
	}
	return t.RequiresReview
}

// passHolds is the reviewer types of repo whose pass holds across commits.
func passHolds(repo string) map[string]bool {
	out := map[string]bool{}
	types, err := LoadAgentTypes(repo)
	if err != nil {
		return out
	}
	for name, t := range types {
		out[name] = t.PassHolds
	}
	return out
}

// snapshotTree is the git tree of tree's working state: tracked, modified and
// new files, taken through a throwaway index so the worker's own is untouched.
// Empty when git cannot say; callers then fall back to the commit.
func snapshotTree(run Exec, tree string) string {
	f, err := os.CreateTemp("", "wick-index-*")
	if err != nil {
		return ""
	}
	idx := f.Name()
	f.Close()
	_ = os.Remove(idx)
	defer os.Remove(idx)
	env := []string{"GIT_INDEX_FILE=" + idx}
	for _, args := range [][]string{{"read-tree", "HEAD"}, {"add", "-A"}} {
		if res, err := run("git", args, ExecOpts{Cwd: tree, Env: env, Timeout: time.Minute}); err != nil || res.Code != 0 {
			return ""
		}
	}
	res, err := run("git", []string{"write-tree"}, ExecOpts{Cwd: tree, Env: env, Timeout: time.Minute})
	if err != nil || res.Code != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// samePassState says the pass on record covers the content in p: the same
// tree when both know it, so changed code is reviewed again and unchanged code
// is not; the commit string otherwise.
func samePassState(prev reviewer, p *reviewPacket) bool {
	if prev.VerdictTree != "" && p.treeID != "" {
		return prev.VerdictTree == p.treeID
	}
	return prev.VerdictAt == p.verdictAt
}

func workerAt(tree string) *Worker {
	store, err := loadWorkersLocked()
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

// workerPlace is where a worker's reviewers open: in the ticket's tab, beside
// the implementor, with the ticket's first overflow tab labelled <ticket> (2).
// A worker with no tab predates the ticket tab, or was not spawned by wick;
// its reviewers fall back to the caller's tab, the way a scout does.
func workerPlace(w *Worker) readOnlyPlace {
	if w == nil || w.Tab == "" {
		return readOnlyPlace{Overflow: "scouts"}
	}
	return readOnlyPlace{Tab: w.Tab, Overflow: w.ID + " (2)"}
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
	treeID           string // git tree of the working state, new files included
	fullDiff         string // path of the full diff, base..working tree
	checks           string
	note             string
	ticket, spec     string
	// noChanges is set when the diff against the base is empty: there is
	// nothing for a reviewer to judge, so the spawn reports that instead of
	// starting one.
	noChanges bool
	// place is where the reviewers open: the worker's ticket tab, or the
	// caller's tab when there is no ticket (a tree wick did not spawn).
	place readOnlyPlace
}

// prepareReview builds the packet every reviewer of a worktree judges: the
// head, the diff from the worker's base, and the repo checks. It runs once per
// reviewer spawn, so two reviewers launched together share one check run and
// one diff. note is the worker's answer to earlier findings.
func prepareReview(run Exec, tree, note string) (*reviewPacket, error) {
	w := workerAt(tree)
	head, err := gitIn(run, tree, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	head = strings.TrimSpace(head)
	// A pass is tied to a commit, and uncommitted work is not in one: a review
	// of a dirty tree is recorded against no commit, so it never opens the
	// merge gate.
	verdictAt := head
	if status, err := gitIn(run, tree, "status", "--porcelain"); err == nil && strings.TrimSpace(status) != "" {
		verdictAt = head + "+uncommitted"
	}
	base := diffBase(run, tree, w)
	treeID := snapshotTree(run, tree)
	// The diff is base against a snapshot of the working state, so files git
	// does not track yet are in it; a plain `git diff base` leaves them out.
	diffArgs := []string{"diff", base}
	if treeID != "" {
		diffArgs = append(diffArgs, treeID)
	}
	full, err := gitIn(run, tree, diffArgs...)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(statePath), "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := filepath.Base(tree)
	p := &reviewPacket{
		tree: tree, head: head, verdictAt: verdictAt, treeID: treeID, base: base, note: note,
		fullDiff: filepath.Join(dir, name+".diff"),
	}
	p.noChanges = strings.TrimSpace(full) == ""
	if err := os.WriteFile(p.fullDiff, []byte(full), 0o644); err != nil {
		return nil, err
	}
	if w != nil {
		p.ticket, p.spec = w.Ticket, w.Spec
	}
	p.place = workerPlace(w)
	// An empty diff starts no reviewer, so the repo check is not worth the
	// wait: the spawn reports nothing to review before any check runs.
	if !p.noChanges {
		p.checks = runChecks(run, tree)
	}
	return p, nil
}

// gitIn runs one git command in tree and returns its stdout.
func gitIn(run Exec, tree string, args ...string) (string, error) {
	res, err := run("git", args, ExecOpts{Cwd: tree, Timeout: time.Minute})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("git %s failed: %s", args[0], firstLine(res.Stderr, "unknown error"))
	}
	return res.Stdout, nil
}

// launchReviewer starts or re-prompts one reviewer type for the packet. A
// persistent type keeps its live reviewer: one that already passed at this
// packet's commit is left alone and nil is returned, so a re-spawn only nudges
// reviewers that still have something to look at. A type that is not persistent
// is not kept between spawns: its live reviewer is closed and a fresh one takes
// its place. A dead reviewer is replaced in its slot, briefed with its findings.
func launchReviewer(run Exec, p *reviewPacket, t AgentType, note, parentPane, parentID string) (*spawnedAgent, error) {
	p.note = note
	// Two spawns of the same reviewer type for one worktree race: both find
	// no live reviewer, both start one, and the second record overwrites the
	// first, leaving its pane running and unowned. The check-then-launch runs
	// under a cross-process lock per worktree and type, so the second waits
	// for the first's record and re-prompts the reviewer already starting
	// instead of starting another. A different type or worktree takes a
	// different lock and is unaffected.
	var out *spawnedAgent
	err := withFileLock(reviewerLockKey(p.tree, t.Name), func() error {
		have := reviewersOf(p.tree)
		prev, had := have[t.Name]

		// A pass that holds is never asked again, live reviewer or not.
		if had && t.Persistent && t.PassHolds && prev.Verdict == verdictPass {
			return nil
		}
		if had && prev.agent().alive(run) {
			if !t.Persistent {
				closeAgent(run, prev.agent())
				agent, err := freshReviewer(run, p, t, prev, false, parentPane, parentID)
				if err != nil {
					return err
				}
				out = agent
				return nil
			}
			if prev.Verdict == verdictPass && samePassState(prev, p) {
				return nil
			}
			// The reviewer already judged exactly this state and asked for
			// changes: a spawn that lost the first answer (a cancelled call, a
			// restarted harness) gets that answer back, not a second review.
			if prev.Verdict != "" && p.note == "" && samePassState(prev, p) {
				if s, ok, _ := ReadAgentSubmit(prev.SessionID); ok && s.Status == prev.Verdict {
					agent := prev.agent()
					out = &spawnedAgent{ID: prev.SessionID, Type: t, Pane: agent.Pane, Tab: agent.Tab, Reused: true,
						Notice: "(this reviewer already judged exactly this state; its answer is repeated, not re-reviewed)"}
					return nil
				}
			}
			agent, err := followUpReviewer(run, p, t, prev, parentPane, parentID)
			if err != nil {
				return err
			}
			out = agent
			return nil
		}
		agent, err := freshReviewer(run, p, t, prev, had, parentPane, parentID)
		if err != nil {
			return err
		}
		out = agent
		return nil
	})
	return out, err
}

// reviewerLockKey is the cross-process lock for one reviewer's check-then-launch:
// the worktree it reviews and the type it is, so simultaneous spawns of the
// same key serialize while any other key runs free. The tree rides as a short
// hash: the lock helper turns its key into a file name, and a full absolute
// path could exceed the file name limit. A hash collision only costs
// concurrency, never correctness: the record is re-checked inside the lock.
func reviewerLockKey(tree, typ string) string {
	sum := fnv.New64a()
	_, _ = sum.Write([]byte(filepath.Clean(tree)))
	return "reviewer-" + strconv.FormatUint(sum.Sum64(), 16) + "-" + lockName(typ)
}

// followUpReviewer re-prompts the live reviewer with only what changed since it
// last looked: the diff since its commit, the new checks and the worker's note.
// Its submit for the new turn is what the caller waits on, so the previous one
// is dropped first.
func followUpReviewer(run Exec, p *reviewPacket, t AgentType, prev reviewer, parentPane, parentID string) (*spawnedAgent, error) {
	agent := prev.agent()
	dir := filepath.Join(filepath.Dir(statePath), "reviews")
	name := filepath.Base(p.tree)
	// A branch rebased since the reviewer last looked no longer holds the
	// commit it saw, and a diff from it would show every other ticket merged
	// meanwhile: the reviewer gets the full diff again.
	incPath := p.fullDiff
	// When the branch was not rebased, the increment is exact if both states
	// are known by tree (uncommitted and new files included), else the diff
	// since the commit the reviewer saw.
	if _, err := gitIn(run, p.tree, "merge-base", "--is-ancestor", prev.Reviewed, "HEAD"); err == nil {
		args := []string{"diff", prev.Reviewed}
		if prev.VerdictTree != "" && p.treeID != "" {
			if _, err := gitIn(run, p.tree, "cat-file", "-e", prev.VerdictTree+"^{tree}"); err == nil {
				args = []string{"diff", prev.VerdictTree, p.treeID}
			}
		}
		inc, err := gitIn(run, p.tree, args...)
		if err != nil {
			return nil, err
		}
		incPath = filepath.Join(dir, name+"."+t.Name+".since.diff")
		if err := os.WriteFile(incPath, []byte(inc), 0o644); err != nil {
			return nil, err
		}
	}
	promptPath := filepath.Join(dir, name+"."+t.Name+".followup")
	if err := os.WriteFile(promptPath, []byte(followUpPrompt(p, prev, incPath)), 0o644); err != nil {
		return nil, err
	}
	// The previous turn's submit must not satisfy the wait for this one.
	clearAgentSubmit(prev.SessionID)
	if err := writeReviewState(prev.SessionID, t.Name, p, parentPane, parentID); err != nil {
		return nil, err
	}
	if err := Nudge(run, agent.Pane, fmt.Sprintf("Follow-up review: read %s and follow it.", promptPath)); err != nil {
		return nil, err
	}
	// Reused: the reviewer was already running, so a failed spawn call has
	// nothing to roll back here.
	return &spawnedAgent{ID: prev.SessionID, Type: t, Pane: agent.Pane, Tab: agent.Tab, Reused: true}, nil
}

// freshReviewer launches a reviewer agent of type t in the worker's tab, or in
// the dead reviewer's slot when one is being replaced. It records the reviewer
// and its review target before the pane starts, so the agent's submit can find
// them.
func freshReviewer(run Exec, p *reviewPacket, t AgentType, prev reviewer, replaced bool, parentPane, parentID string) (*spawnedAgent, error) {
	// The reviewer resolves its own slot, not the worker's model:
	// WICK_READONLY_HARNESS is a flag layer, the type's frontmatter the
	// typeOrSpawn layer, then the slot's settings, then pig.
	choice := Resolve(reviewSlot(t.Name), readonlyHarnessChoice(),
		Choice{Harness: t.Harness, Model: t.Model, Effort: t.Effort})
	id := newSessionID()
	sess := Session{
		Role: "agent", Cwd: p.tree, Model: choice.Model, Effort: choice.Effort, ID: id,
		AgentType: t.Name, AgentID: id,
		Dir: filepath.Join(readOnlyDir(), "sessions"),
		// A reviewer inside a run archives under it: the spawning worker
		// carries its spec in the environment, the way workers record it.
		Spec: os.Getenv("WICK_SPEC"),
	}
	place := p.place
	notice := ""
	if replaced {
		if prev.Tab != "" {
			place.Tab, place.Slot = prev.Tab, prev.Slot
		}
		notice = fmt.Sprintf("(the previous %s reviewer's pane was gone: a new reviewer took over, briefed with its findings)",
			t.Name)
	}
	spaceSpawns()
	agent, err := launchPlacedAgent(run, place, choice.Harness, sess, renderPrompt(t, packetPrompt(p, t.Name, prev, replaced)),
		func(a readOnlyAgent) error {
			if err := storeReviewerPane(p.tree, t.Name, reviewer{
				Pane: a.Pane, Tab: a.Tab, Slot: a.Slot, Harness: choice.Harness,
				SessionID: id, Cwd: p.tree, Model: choice.Model, Effort: choice.Effort, Reviewed: p.head,
			}); err != nil {
				return err
			}
			return writeReviewState(id, t.Name, p, parentPane, parentID)
		})
	if err != nil {
		return nil, err
	}
	return &spawnedAgent{ID: id, Type: t, Pane: agent.Pane, Tab: agent.Tab, Sess: agent.Session, Notice: notice}, nil
}

// reviewSlot names the settings slot a reviewer type resolves through: the spec
// reviewer has its own, every other reviewer type shares code-review.
func reviewSlot(typ string) string {
	if typ == reviewerSpec {
		return slotSpecReview
	}
	return slotCodeReview
}

// writeReviewState records where a reviewer's submit belongs: the worktree, the
// reviewer type, and the commit the review is tied to, plus the worker's pane
// and id so an auto-backgrounded reviewer's submit still reaches it. The
// reviewer process reads it back when it submits, so the verdict lands on the
// right record even when the spawn auto-backgrounds it. Deliver is cleared so a
// re-prompt's result returns to the waiting spawn, not as a stale notice;
// backgroundAgent sets it again only if this spawn runs out of budget.
func writeReviewState(id, typ string, p *reviewPacket, parentPane, parentID string) error {
	s, _, err := ReadAgentState(id)
	if err != nil {
		return err
	}
	s.ID, s.Type = id, typ
	s.Parent, s.ParentID, s.Deliver = parentPane, parentID, false
	s.Review = &ReviewTarget{Tree: p.tree, Type: typ, Head: p.head, At: p.verdictAt, TreeID: p.treeID}
	return WriteAgentState(s)
}

// packetPrompt is the reviewer's brief: where it runs, the ticket and spec for
// the spec reviewer, the diff to read, the checks wick ran, and the worker's
// note. It is appended to the type's own standing instructions.
func packetPrompt(p *reviewPacket, typ string, prev reviewer, replaced bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The worktree is %s.\n", p.tree)
	if typ == reviewerSpec && p.ticket != "" {
		fmt.Fprintf(&b, "\nThe ticket is %s.", p.ticket)
		if p.spec != "" {
			fmt.Fprintf(&b, " It belongs to the spec at %s.", p.spec)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\nThe change is the git diff at %s, from %s to the working tree; read files in the worktree for context.\n", p.fullDiff, short(p.base))
	fmt.Fprintf(&b, "\nChecks run by wick on this tree: %s\n", p.checks)
	if p.note != "" {
		fmt.Fprintf(&b, "\nThe worker says: %s\n", p.note)
	}
	if replaced {
		fmt.Fprintf(&b, "\nYou replace a reviewer whose pane closed. Its last findings, at %s, were:\n\n%s\n\nCheck whether each of them still holds.\n",
			short(prev.Reviewed), prev.Findings)
	}
	return b.String()
}

func followUpPrompt(p *reviewPacket, prev reviewer, incPath string) string {
	var b strings.Builder
	if incPath == p.fullDiff {
		fmt.Fprintf(&b, "Follow-up review. The branch was rebased since you last looked (%s), so read the full diff again, at %s.\n\n",
			short(prev.Reviewed), p.fullDiff)
	} else {
		fmt.Fprintf(&b, "Follow-up review. What changed since you last looked (%s) is the diff at %s; the full diff is still at %s.\n\n",
			short(prev.Reviewed), incPath, p.fullDiff)
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
	b.WriteString("When the review is done, call the submit tool with status \"pass\" or \"changes\" and put your findings in its report.")
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
