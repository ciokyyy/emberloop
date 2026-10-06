package wick

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/wick/tools"
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

// coverage compares what the review diff holds against what the working tree
// changed or added since base. The diff's files are parsed from the diff text
// itself; the tree's are the same population from git: base against the
// working-state tree the diff was taken over (or the worktree itself when no
// snapshot exists), plus untracked files. It returns both file sets, or an
// error when git cannot say what the tree holds, in which case the prompt
// claims no counts.
func coverage(run Exec, tree, base, treeID, diff string) (map[string]bool, map[string]bool, error) {
	changed, err := treeChanged(run, tree, base, treeID)
	if err != nil {
		return nil, nil, err
	}
	return diffPaths(diff), changed, nil
}

// diffPaths is the distinct paths the diff text covers: the new name of each
// changed file, the old one for a deletion.
func diffPaths(text string) map[string]bool {
	out := map[string]bool{}
	var cur *diffBlock
	flush := func() {
		if cur == nil {
			return
		}
		if p := cur.path(); p != "" {
			out[p] = true
		}
		cur = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
			flush()
			cur = &diffBlock{header: rest}
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case cur.plus == "" && strings.HasPrefix(line, "+++ "):
			cur.plus = strings.TrimPrefix(line, "+++ ")
		case cur.minus == "" && strings.HasPrefix(line, "--- "):
			cur.minus = strings.TrimPrefix(line, "--- ")
		case cur.binary == "" && strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ"):
			cur.binary = line
		}
	}
	flush()
	return out
}

// diffBlock is one file's section of a unified diff.
type diffBlock struct {
	header      string // the two paths of the `diff --git` line
	plus, minus string // the +++ and --- lines' sides
	binary      string // a `Binary files … differ` line, when present
}

// path resolves the block to the path it covers: the +++ side, the --- side
// for a deletion, the Binary line's sides, and only then the header. The +++,
// --- and Binary lines each carry a single path, so names with spaces resolve
// exactly; the header's two paths stay ambiguous for such names and are only
// a fallback for a change with no content lines, like a mode-only change.
func (b *diffBlock) path() string {
	if b.plus != "" && b.plus != "/dev/null" {
		return cleanDiffSide(b.plus)
	}
	if b.minus != "" && b.minus != "/dev/null" {
		return cleanDiffSide(b.minus)
	}
	if a, c, ok := binarySides(b.binary); ok {
		if c != "" && c != "/dev/null" {
			return c
		}
		return a
	}
	if a, c, ok := splitDiffSides(b.header); ok {
		if c != "" && c != "/dev/null" {
			return c
		}
		return a
	}
	return ""
}

// binarySides splits a `Binary files A and B differ` line into its cleaned
// sides. A side is /dev/null or carries git's a/ or b/ prefix, quoted or not;
// the first ` and ` that splits two such sides wins, else the last one is
// tried, which holds for an old name holding " and ". Every candidate failing,
// the block is skipped: a wrong split silently produces a wrong count.
func binarySides(line string) (a, b string, ok bool) {
	rest, ok := strings.CutPrefix(line, "Binary files ")
	if !ok {
		return "", "", false
	}
	rest, ok = strings.CutSuffix(rest, " differ")
	if !ok {
		return "", "", false
	}
	if first, second, ok := strings.Cut(rest, " and "); ok {
		if a, b, ok := validBinarySides(first, second); ok {
			return a, b, true
		}
	}
	if i := strings.LastIndex(rest, " and "); i >= 0 {
		if a, b, ok := validBinarySides(rest[:i], rest[i+len(" and "):]); ok {
			return a, b, true
		}
	}
	return "", "", false
}

// validBinarySides cleans two raw Binary sides and reports whether they look
// like git's pair: /dev/null or an a/ side first, /dev/null or a b/ side
// second. A wrong split is refused rather than counted.
func validBinarySides(rawA, rawB string) (a, b string, ok bool) {
	if rawA != "/dev/null" && !isDiffSide(rawA, "a/") {
		return "", "", false
	}
	if rawB != "/dev/null" && !isDiffSide(rawB, "b/") {
		return "", "", false
	}
	return cleanDiffSide(rawA), cleanDiffSide(rawB), true
}

// splitDiffSides splits the two paths of a `diff --git` header. Git does not
// quote spaces in these headers, so the first space separates two plain names
// only; when it does not split an a/ b/ pair, the last ` b/` is tried, which
// holds for a spaced name. Both failing, the block is skipped: a wrong split
// silently produces a wrong count.
func splitDiffSides(rest string) (a, b string, ok bool) {
	if i := firstSpaceOutsideQuotes(rest); i >= 0 {
		if a, b, ok := validDiffSides(rest[:i], rest[i+1:]); ok {
			return a, b, true
		}
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		if a, b, ok := validDiffSides(rest[:i], rest[i+1:]); ok {
			return a, b, true
		}
	}
	return "", "", false
}

// validDiffSides cleans two raw header sides and reports whether they look
// like git's a/ and b/ pair: a wrong split is refused rather than counted.
func validDiffSides(rawA, rawB string) (a, b string, ok bool) {
	if !isDiffSide(rawA, "a/") {
		return "", "", false
	}
	if rawB != "/dev/null" && !isDiffSide(rawB, "b/") {
		return "", "", false
	}
	return cleanDiffSide(rawA), cleanDiffSide(rawB), true
}

// isDiffSide says the raw side carries git's prefix, quoted or not.
func isDiffSide(s, prefix string) bool {
	return strings.HasPrefix(s, prefix) || strings.HasPrefix(s, `"`+prefix)
}

// cleanDiffSide dequotes a diff side and strips its a/ or b/ prefix. Git pads
// these lines with a trailing tab, which is not part of the name.
func cleanDiffSide(s string) string {
	s = strings.TrimRight(s, " \t")
	if len(s) >= 2 && s[0] == '"' {
		if un, err := strconv.Unquote(s); err == nil {
			s = un
		}
	}
	if p, ok := strings.CutPrefix(s, "a/"); ok {
		return p
	}
	return strings.TrimPrefix(s, "b/")
}

// firstSpaceOutsideQuotes is the index of the first space outside C quotes,
// or -1. Quoting still matters for names with quotes or non-ASCII bytes,
// which git C-quotes even though it leaves spaces bare.
func firstSpaceOutsideQuotes(s string) int {
	inQuotes, escaped := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case escaped:
			escaped = false
		case c == '\\' && inQuotes:
			escaped = true
		case c == '"':
			inQuotes = !inQuotes
		case c == ' ' && !inQuotes:
			return i
		}
	}
	return -1
}

// treeChanged is the distinct paths changed or added between base and the
// working state: the same population the review diff is taken over. That is
// base against the snapshot tree when one exists, else against the worktree,
// plus untracked files, which a worktree diff leaves out. Measuring the same
// population on both sides keeps renames and reverts consistent: a rename
// counts once under its new name on each side, and a file reverted to base
// counts nowhere.
func treeChanged(run Exec, tree, base, treeID string) (map[string]bool, error) {
	out := map[string]bool{}
	args := []string{"diff", "--name-only", "-z", base}
	if treeID != "" {
		args = append(args, treeID)
	}
	named, err := gitIn(run, tree, args...)
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(named, "\x00") {
		if p != "" {
			out[p] = true
		}
	}
	status, err := gitIn(run, tree, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	// Only untracked entries count here: every tracked change is already in
	// the name list above, under git's rename detection. Entries stay aligned
	// by skipping a rename or copy's second NUL-terminated path.
	for rest := status; len(rest) >= 4; {
		x, y := rest[0], rest[1]
		path, after, _ := strings.Cut(rest[3:], "\x00")
		if x == '?' && y == '?' && path != "" {
			out[path] = true
		}
		rest = after
		if x == 'R' || y == 'R' || x == 'C' || y == 'C' {
			_, rest, _ = strings.Cut(rest, "\x00")
		}
	}
	return out, nil
}

// coverageNotice is the prompt's file-count line: how many files the diff
// covers against how many the working tree changed or added. When the diff
// leaves out files the tree holds, the prompt flags the gap and names what
// is missing, so it is visible to the reviewer instead of discovered by
// accident. The flag keys on the missing files themselves, not on the counts:
// two sets can hold the same number of files while covering different ones.
func coverageNotice(diffFiles, treeFiles int, missing []string) string {
	if len(missing) > 0 {
		return fmt.Sprintf("Coverage WARNING: the diff covers %s but the working tree changed or added %s. The diff does not cover: %s. Read the missing files in the worktree.",
			pluralFiles(diffFiles), pluralFiles(treeFiles), strings.Join(missing, ", "))
	}
	return fmt.Sprintf("Coverage: the diff covers %s; the working tree changed or added %s.",
		pluralFiles(diffFiles), pluralFiles(treeFiles))
}

// pluralFiles is "1 file" or "N files".
func pluralFiles(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

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
	// VerdictPatch is the same pass keyed by change instead of place: the
	// hash of the worker's own diff from its base, which a clean rebase leaves
	// identical while a conflicting or content-changing rebase alters.
	Verdict      string `json:"verdict"`
	VerdictAt    string `json:"verdict_at"`
	VerdictTree  string `json:"verdict_tree,omitempty"`
	VerdictPatch string `json:"verdict_patch,omitempty"`
	Findings     string `json:"findings"`
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
			r.Verdict, r.VerdictAt, r.VerdictTree, r.VerdictPatch, r.Findings = prev.Verdict, prev.VerdictAt, prev.VerdictTree, prev.VerdictPatch, prev.Findings
		}
		all[key][typ] = r
		return nil
	})
}

// mergeReviewerVerdict folds a reviewer's submit into the record for its
// worktree: its status is the verdict, its report the findings kept for
// follow-ups and for briefing a replacement, and reviewed the commit it judged.
// treeID is the content snapshot it judged and patch its change identity: the
// gate accepts a pass when either matches the tip.
func mergeReviewerVerdict(tree, typ, verdict, at, reviewed, treeID, patch, findings string) error {
	return updateReviewers(func(all map[string]map[string]reviewer) error {
		key := filepath.Clean(tree)
		if all[key] == nil {
			all[key] = map[string]reviewer{}
		}
		r := all[key][typ]
		r.Verdict, r.VerdictAt, r.VerdictTree, r.VerdictPatch, r.Reviewed, r.Findings = verdict, at, treeID, patch, reviewed, findings
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
// pass holds (holds[type]) needs a pass at some commit, not at head. Otherwise
// a pass counts when its content (VerdictTree against headTree) or its change
// (VerdictPatch against headPatch) matches the tip, so a clean rebase keeps
// the pass while a conflicting or content-changing one does not.
func reviewGate(tree, head, headTree, headPatch string, required []string, holds map[string]bool) string {
	have := reviewersOf(tree)
	var missing []string
	for _, typ := range required {
		r, ok := have[typ]
		switch {
		case !ok || r.Verdict == "":
			missing = append(missing, typ+": no review yet")
		case r.Verdict != verdictPass:
			missing = append(missing, fmt.Sprintf("%s: %s at %s", typ, r.Verdict, short(r.VerdictAt)))
		case r.VerdictAt != head && !holds[typ] && !passCoversTip(r, headTree, headPatch):
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

// samePassState says the pass on record covers the exact content in p: the same
// tree when both know it, so changed code is reviewed again and unchanged code
// is not; the commit string otherwise. It stays exact on purpose: a clean
// rebase moves the tree, so only the patch identity (passCovers) keeps a pass
// there, while a changes verdict still gets its follow-up.
func samePassState(prev reviewer, p *reviewPacket) bool {
	if prev.VerdictTree != "" && p.treeID != "" {
		return prev.VerdictTree == p.treeID
	}
	return prev.VerdictAt == p.verdictAt
}

// patchIDOf is the identity of the worker's own change: the SHA-256 of its
// diff from its base. The diff names no commit, only the change itself, so the
// hash does not depend on where the branch sits: a clean rebase leaves it
// identical, a conflict resolution or an edit alters it. Empty when there is
// no change, so an empty diff never matches a judged one.
func patchIDOf(diff string) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(diff))
	return hex.EncodeToString(sum[:])
}

// passCoversTip says a pass on record covers the tip: its content matches, or
// its change identity does. Either opens the gate.
func passCoversTip(r reviewer, headTree, headPatch string) bool {
	if r.VerdictTree != "" && headTree != "" && r.VerdictTree == headTree {
		return true
	}
	return r.VerdictPatch != "" && headPatch != "" && r.VerdictPatch == headPatch
}

// passCovers says a pass on record covers the packet: the exact content, or
// the same change on a new base after a clean rebase.
func passCovers(prev reviewer, p *reviewPacket) bool {
	if samePassState(prev, p) {
		return true
	}
	return prev.VerdictPatch != "" && p.patchID != "" && prev.VerdictPatch == p.patchID
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

// runChecks runs the repo's check at the root of tree, so the reviewers
// judge real results rather than the worker's word. A plain .wick/check runs
// its one command; a named map runs "default", or the first name in order
// when no default exists, and the packet says which. A repo with no such
// file is reviewed without checks, and the packet says so.
func runChecks(run Exec, tree string) string {
	cmd, display, err := tools.SpawnCheck(tree)
	if err != nil {
		if errors.Is(err, tools.ErrNoChecks) {
			return err.Error()
		}
		return fmt.Sprintf("the repo's .wick/check is invalid: %v", err)
	}
	start := time.Now()
	res, err := run("sh", []string{"-c", cmd}, ExecOpts{Cwd: tree, Timeout: tools.CheckTimeout})
	out := tools.CheckTail(strings.TrimSpace(res.Stdout + "\n" + res.Stderr))
	switch {
	case err != nil:
		return fmt.Sprintf("`%s` could not run: %v", display, err)
	case res.Code != 0 && time.Since(start) >= tools.CheckTimeout:
		return fmt.Sprintf("`%s` TIMED OUT after %s, which is not a pass. Output so far:\n%s", display, tools.CheckTimeout, out)
	case res.Code != 0:
		return fmt.Sprintf("`%s` FAILED (exit %d):\n%s", display, res.Code, out)
	default:
		return fmt.Sprintf("`%s` passed (exit 0):\n%s", display, out)
	}
}

// reviewPacket is what one call hands its reviewers.
type reviewPacket struct {
	tree, head, base string
	verdictAt        string // head, or head+"+uncommitted" when the tree is dirty
	treeID           string // git tree of the working state, new files included
	patchID          string // hash of the worker's own diff from its base
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
	// coverOK says the file counts below were computed. diffFiles is how many
	// files the full diff covers, treeFiles how many the working tree changed
	// or added since the base, and missing the changed files with no hunk in
	// the diff. The prompt states the counts and flags a diff that covers
	// fewer files than the tree holds.
	coverOK              bool
	diffFiles, treeFiles int
	missing              []string
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
		patchID:  patchIDOf(full),
		fullDiff: filepath.Join(dir, name+".diff"),
	}
	p.noChanges = strings.TrimSpace(full) == ""
	// The coverage counts compare what the reviewer was given against what the
	// tree holds: the diff's files are parsed from the diff text itself, the
	// tree's are the same population from git, so a gap between them is a
	// file the reviewer was not given. The gap is flagged in the prompt
	// rather than left for the reviewer to notice. When git cannot say what
	// the tree holds, the prompt says nothing instead of claiming counts it
	// did not measure.
	if diff, tree, err := coverage(run, tree, base, treeID, full); err == nil {
		p.coverOK = true
		p.diffFiles = len(diff)
		p.treeFiles = len(tree)
		for f := range tree {
			if !diff[f] {
				p.missing = append(p.missing, f)
			}
		}
		sort.Strings(p.missing)
	}
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
// Every reviewer it starts or re-prompts joins groupID, so its submit joins
// the group's single notice.
func launchReviewer(run Exec, p *reviewPacket, t AgentType, note, parentPane, parentID, groupID string) (*spawnedAgent, error) {
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
				agent, err := freshReviewer(run, p, t, prev, false, parentPane, parentID, groupID)
				if err != nil {
					return err
				}
				out = agent
				return nil
			}
			if prev.Verdict == verdictPass && passCovers(prev, p) {
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
			agent, err := followUpReviewer(run, p, t, prev, parentPane, parentID, groupID)
			if err != nil {
				return err
			}
			out = agent
			return nil
		}
		agent, err := freshReviewer(run, p, t, prev, had, parentPane, parentID, groupID)
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
// Its submit for the new turn joins groupID's single notice, so the previous
// one is dropped first.
func followUpReviewer(run Exec, p *reviewPacket, t AgentType, prev reviewer, parentPane, parentID, groupID string) (*spawnedAgent, error) {
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
	// The previous turn's submit must not satisfy the group for this one.
	// A follow-up is a new turn, so a previous failed-turn mark goes too.
	clearAgentSubmit(prev.SessionID)
	clearChildError(prev.SessionID)
	if err := writeReviewState(prev.SessionID, t.Name, p, parentPane, parentID, groupID, agent.Pane, agent.Tab); err != nil {
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
// them. The reviewer joins groupID, so its submit joins the group's single
// notice.
func freshReviewer(run Exec, p *reviewPacket, t AgentType, prev reviewer, replaced bool, parentPane, parentID, groupID string) (*spawnedAgent, error) {
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
			return writeReviewState(id, t.Name, p, parentPane, parentID, groupID, a.Pane, a.Tab)
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
// and id and the spawn group it joins, so the submit joins the group's single
// notice. The pane lives here too, so the stop gate's pending check sees the
// same liveness a scout's state carries. The reviewer process reads it back
// when it submits, so the verdict lands on the right record even when the
// spawn never waits.
func writeReviewState(id, typ string, p *reviewPacket, parentPane, parentID, groupID, pane, tab string) error {
	s, _, err := ReadAgentState(id)
	if err != nil {
		return err
	}
	s.ID, s.Type = id, typ
	s.Pane, s.Tab = pane, tab
	s.Parent, s.ParentID, s.GroupID, s.Deliver = parentPane, parentID, groupID, true
	s.Review = &ReviewTarget{Tree: p.tree, Type: typ, Head: p.head, At: p.verdictAt, TreeID: p.treeID, PatchID: p.patchID}
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
	if p.coverOK {
		fmt.Fprintf(&b, "\n%s\n", coverageNotice(p.diffFiles, p.treeFiles, p.missing))
	}
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

// workerPatchID is the patch identity of the worker's tip: its diff from its
// base, hashed. Empty when git cannot say, so the gate falls back to content.
func workerPatchID(run Exec, w *Worker, tip, tipTree string) string {
	if w == nil {
		return ""
	}
	base := diffBase(run, w.Worktree, w)
	if base == "" {
		return ""
	}
	endpoint := tip
	if tipTree != "" {
		endpoint = tipTree
	}
	d, err := gitIn(run, w.Worktree, "diff", base, endpoint)
	if err != nil {
		return ""
	}
	return patchIDOf(d)
}
