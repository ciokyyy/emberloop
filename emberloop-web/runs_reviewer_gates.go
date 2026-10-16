package web

import (
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Reviewer-flow gates from the Luna Run retro (12 uncommitted submits, 14
// unverified checks, 5 scope edits, 14 needless seat re-runs). The driver
// wiring lives in runs_exec.go (handleWorkerDoneSubmit, ensureReviewers,
// runStartReviewer); this file holds the pure helpers plus the small git
// probes, so the policy reads in one place and unit tests need no Daemon.

// --- 1. Worker submit gate: nothing uncommitted ---

// refuseUncommittedWork reports why a Worker done submit must be refused as
// a tool error (409), or "" when the submit may proceed. A done submit needs
// committed work: the worktree must be clean and the Ticket branch must hold
// at least one commit ahead of base. Either failure tells the Worker to
// commit. Unknowns fail open (a missing worktree or unreadable base never
// blocks), mirroring the patch-id degrade.
func refuseUncommittedWork(repo, branch, base, worktree, tip string) string {
	if worktree != "" {
		if out, err := runGit(worktree, "status", "--porcelain"); err == nil {
			var dirty []string
			for _, line := range strings.Split(out, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				dirty = append(dirty, strings.TrimSpace(line))
			}
			if len(dirty) > 0 {
				list := strings.Join(dirty, ", ")
				if len(dirty) > 3 {
					list = strings.Join(dirty[:3], ", ") + ", and " + strconv.Itoa(len(dirty)-3) + " more"
				}
				return "done refused: the worktree has uncommitted changes (" + list + "); commit them and submit done again"
			}
		}
	}
	if tip == "" {
		return ""
	}
	if base == "" {
		return ""
	}
	out, err := runGit(repo, "rev-list", "--count", base+".."+tip)
	if err != nil {
		return ""
	}
	ahead, aerr := strconv.Atoi(strings.TrimSpace(out))
	if aerr != nil {
		return ""
	}
	if ahead == 0 {
		return "done refused: the Ticket branch " + branch + " has no commits ahead of " + shortSHA(base) + "; commit your work and submit done again"
	}
	return ""
}

// ticketBranchBase resolves the base a Ticket branch is judged against: the
// merge-base with its target branch when one is recorded, else the recorded
// base from worker start. Empty when neither resolves, so callers degrade.
func ticketBranchBase(repo, target, recorded, tip string) string {
	base := recorded
	if target != "" {
		if mb, err := runGit(repo, "merge-base", target, tip); err == nil && strings.TrimSpace(mb) != "" {
			base = strings.TrimSpace(mb)
		} else if base == "" {
			return ""
		}
	}
	return strings.TrimSpace(base)
}

// --- 2. Repo check on the tip before Reviewers ---

// tipCheckEvidenceLine is the green evidence passed to Reviewer prompts so
// they need not rerun the full suite.
func tipCheckEvidenceLine(tip string) string {
	return "Driver check at " + shortSHA(tip) + ": green (the repo check passed on this tip; you need not rerun the full suite)."
}

// isCheckInfraError reports a check result that never ran: the runner's
// "could not run" entries (missing tip, no worktree, unreadable check file).
// A red check names failing tests instead. A flaked check (every run failed
// on different tests) counts too. Infra failures fail open at the tip gate:
// the Worker cannot fix them, so Reviewers still start.
func isCheckInfraError(failing []string) bool {
	for _, f := range failing {
		if strings.HasPrefix(f, "base check could not run") || strings.HasPrefix(f, "tip check could not run") || strings.HasPrefix(f, checkFlakedPrefix) {
			return true
		}
	}
	return false
}

// cachedTipCheck runs the repo check once per Ticket tip and persists its
// result on the Run's TipCheckCache. It reuses the base-red gate's runner
// (runRepoCheckAtTip: .emberloop/check in a detached worktree at the tip);
// d.tipCheck scripts it in tests. Git and check work stay outside d.mu.
func (d *Daemon) cachedTipCheck(runID, repo, ticketBranch, tip string) (bool, []string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return false, []string{"tip check could not run: Run no longer exists"}
	}
	if result, exists := rs.TipCheckCache[tip]; exists {
		d.mu.Unlock()
		return result.OK, append([]string(nil), result.Failing...)
	}
	tipCheck := d.tipCheck
	d.mu.Unlock()

	green, failing := true, []string(nil)
	if tipCheck != nil {
		green, failing = tipCheck(repo, ticketBranch, tip)
	} else {
		green, failing = runRepoCheckAtTip(repo, tip)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok = d.runs[runID]
	if !ok {
		return false, []string{"tip check could not run: Run no longer exists"}
	}
	if rs.TipCheckCache == nil {
		rs.TipCheckCache = map[string]baseCheckResult{}
	}
	if cached, exists := rs.TipCheckCache[tip]; exists {
		return cached.OK, append([]string(nil), cached.Failing...)
	}
	rs.TipCheckCache[tip] = baseCheckResult{OK: green, Failing: append([]string(nil), failing...)}
	d.persistRun(rs)
	return green, append([]string(nil), failing...)
}

// cachedTipEvidence returns the green evidence line for tip when the cached
// tip check passed. It never runs the check: Reviewer first prompts (which
// did not pass through the tip gate) read the cache only, so a cold cache
// means no evidence rather than a blocking check.
func (d *Daemon) cachedTipEvidence(runID, tip string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[runID]
	if !ok || tip == "" {
		return ""
	}
	if res, exists := rs.TipCheckCache[tip]; exists && res.OK {
		return tipCheckEvidenceLine(tip)
	}
	return ""
}

// checkTipBeforeReview runs the cached repo check for tip before Reviewers
// start. A red tip is returned to the Worker with the failing output and no
// Reviewer turn is spent; the tell goes out once per tip. True means the
// caller must not start or prompt Reviewers. Green and unverifiable (infra
// error) both proceed; only a verified red returns. Concurrent passes claim
// the tip before the check runs, so only the claimer ever tells: a pass that
// finds another tip's claim in flight skips Reviewers and retries next pass.
func (d *Daemon) checkTipBeforeReview(runID, ticketID, tip string) bool {
	if tip == "" {
		return false
	}
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return false
	}
	repo, branch := rs.Repo, rs.Branches[ticketID]
	if rs.TipRedTold[ticketID] == tip {
		d.mu.Unlock()
		return true
	}
	if rs.TipRedTold == nil {
		rs.TipRedTold = map[string]string{}
	}
	rs.TipRedTold[ticketID] = tip
	if rs.tipChecking == nil {
		rs.tipChecking = map[string]int{}
	}
	rs.tipChecking[ticketID]++
	d.mu.Unlock()

	green, failing := d.cachedTipCheck(runID, repo, branch, tip)
	d.mu.Lock()
	if rs.tipChecking[ticketID] > 1 {
		rs.tipChecking[ticketID]--
	} else {
		delete(rs.tipChecking, ticketID)
	}
	d.mu.Unlock()
	if green || isCheckInfraError(failing) {
		// Release the claim so the next pass judges the cached result; a
		// newer tip's claim in flight is left alone.
		d.mu.Lock()
		if rs, ok := d.runs[runID]; ok && rs.TipRedTold[ticketID] == tip {
			delete(rs.TipRedTold, ticketID)
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
		}
		d.mu.Unlock()
		return false
	}
	// Red: the claim stands as told. A newer tip's claim in flight is left
	// alone; this tip's tell still goes out, since it judged this tip.
	d.mu.Lock()
	if rs, ok := d.runs[runID]; ok {
		if rs.TipRedTold == nil {
			rs.TipRedTold = map[string]string{}
		}
		if rs.TipRedTold[ticketID] != tip {
			rs.TipRedTold[ticketID] = tip
		}
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	} else {
		d.mu.Unlock()
		return true
	}
	d.mu.Unlock()
	list := strings.Join(failing, ", ")
	if list == "" {
		list = "check failed"
	}
	_ = d.runTellWorker(runID, ticketID, "The repo check is red at tip "+shortSHA(tip)+"; no reviewer turn was spent. Failing: "+list+". Fix the failures, commit, and submit done again.", systemSender)
	return true
}

// --- 3. Selective re-review: carry an old pass when the new diff is safe ---

// selectiveRereviewLineThreshold is the T1..T2 changed-line budget (added
// plus removed) below which an already-passed seat keeps its pass without a
// rerun, provided the diff also avoids the other seat's cited files and stays
// inside the previous diff scope. Small enough that a 40-line fix still gets
// re-judged; regressions from the other seat's fixes in Luna Run 3b52de7a
// all touched cited files or added new ones.
const selectiveRereviewLineThreshold = 40

// gitDiffFiles lists the files changed between two refs, as repo-relative
// paths. ok is false when git fails, so callers rerun rather than carry on
// an unknown diff.
func gitDiffFiles(repo, from, to string) (files []string, ok bool) {
	if repo == "" || from == "" || to == "" {
		return nil, false
	}
	out, err := runGit(repo, "diff", "--name-only", from, to)
	if err != nil {
		return nil, false
	}
	var rel []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			rel = append(rel, line)
		}
	}
	return rel, true
}

// gitDiffChangedLines counts added plus removed lines between two refs. ok
// is false when git fails or numstat does not parse.
func gitDiffChangedLines(repo, from, to string) (int, bool) {
	if repo == "" || from == "" || to == "" {
		return 0, false
	}
	out, err := runGit(repo, "diff", "--numstat", from, to)
	if err != nil {
		return 0, false
	}
	total := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		for _, f := range fields[:2] {
			// Binary diffs report "-" instead of a count: unparseable, so
			// the whole diff is unknown rather than silently small.
			n, cerr := strconv.Atoi(f)
			if cerr != nil {
				return 0, false
			}
			total += n
		}
	}
	return total, true
}

// findingFiles returns the cited files of findings, plus their basenames, so
// a fix that moves a cited file still counts as touching it.
func findingFiles(findings []ReviewFinding) map[string]bool {
	set := map[string]bool{}
	for _, f := range findings {
		if name := strings.TrimSpace(f.File); name != "" {
			set[name] = true
			if base := path.Base(name); base != "" {
				set[base] = true
			}
		}
	}
	return set
}

// selectiveRereviewNeeded reports whether the already-passed seat must judge
// tip T2 after passing at T1 while the other seat said changes: true when the
// T1..T2 diff touches a file the changes verdict cited, changes more than the
// line threshold, reaches files outside the base..T1 diff scope, or cannot be
// read. False carries the pass to T2 with no Reviewer turn.
func selectiveRereviewNeeded(repo, target, recorded, t1, t2 string, cited []ReviewFinding) bool {
	if t1 == "" || t2 == "" || t1 == t2 {
		return true
	}
	changed, ok := gitDiffFiles(repo, t1, t2)
	if !ok {
		return true
	}
	if citedSet := findingFiles(cited); len(citedSet) > 0 {
		for _, f := range changed {
			if citedSet[f] || citedSet[path.Base(f)] {
				return true
			}
		}
	}
	if lines, ok := gitDiffChangedLines(repo, t1, t2); !ok || lines > selectiveRereviewLineThreshold {
		return true
	}
	base := ticketBranchBase(repo, target, recorded, t1)
	if base == "" {
		return true
	}
	scopeFiles, ok := gitDiffFiles(repo, base, t1)
	if !ok {
		return true
	}
	scope := map[string]bool{}
	for _, f := range scopeFiles {
		scope[f] = true
	}
	for _, f := range changed {
		if !scope[f] {
			return true
		}
	}
	return false
}

// selectiveCarryFor returns, per reviewer type, the short T1 whose pass may
// carry to tip without a rerun: the seat passed at T1 while the other seat
// said changes at that same T1, and the T1..T2 diff is safe per
// selectiveRereviewNeeded. Seats already fresh at tip, or reused by the
// same-tip and patch-id short-circuits, are never listed. Git runs unlocked;
// state reads are locked.
func (d *Daemon) selectiveCarryFor(runID, ticketID, tip string) map[string]string {
	if tip == "" {
		return nil
	}
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return nil
	}
	repo, target, recorded := rs.Repo, rs.Targets[ticketID], rs.Bases[ticketID]
	required := requiredReviewerTypesFor(rs, ticketID)
	reviews := map[string]storedReview{}
	for typ, r := range rs.Reviews[ticketID] {
		reviews[typ] = r
	}
	d.mu.Unlock()

	out := map[string]string{}
	for _, typ := range required {
		r, ok := reviews[typ]
		if !ok || r.Verdict != "pass" || r.At == "" || r.At == tip {
			continue
		}
		var changes []ReviewFinding
		for _, other := range required {
			c, ok := reviews[other]
			if ok && c.Verdict == "changes" && c.At == r.At {
				changes = append(changes, c.Findings...)
			}
		}
		if len(changes) == 0 {
			continue
		}
		if selectiveRereviewNeeded(repo, target, recorded, r.At, tip, changes) {
			continue
		}
		out[typ] = shortSHA(r.At)
	}
	return out
}

// --- 4. Scope check and the round-4+ rule ---

// fileInTicketScope reports whether a changed file belongs to the Ticket's
// stated scope: the ticket file mentions the file path, names a parent
// directory of it, or mentions its basename. Conservative on purpose: an
// ambiguous mention reads as in scope, so the warning only lists files the
// ticket never hints at.
func fileInTicketScope(ticketBody, file string) bool {
	file = strings.TrimSpace(file)
	if file == "" {
		return false
	}
	if strings.Contains(ticketBody, file) {
		return true
	}
	dir := path.Dir(file)
	for dir != "." && dir != "/" && dir != "" {
		if mentionsPathToken(ticketBody, dir) {
			return true
		}
		parent := path.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if base := path.Base(file); base != "" && base != file && mentionsPathToken(ticketBody, base) {
		return true
	}
	return false
}

// isPathChar reports the characters that continue a path token: a mention
// of "emberloop" inside "emberloop-web/" names another path, not a scope.
func isPathChar(c byte) bool {
	return c == '/' || c == '-' || c == '.' || c == '_' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// mentionsPathToken reports whether body names token as a path: followed by
// "/" (a parent directory) or standing as its own token. A bare substring
// inside a longer path does not count.
func mentionsPathToken(body, token string) bool {
	if token == "" {
		return false
	}
	if strings.Contains(body, token+"/") {
		return true
	}
	for i := 0; i+len(token) <= len(body); {
		j := strings.Index(body[i:], token)
		if j < 0 {
			return false
		}
		j += i
		var before, after byte
		if j > 0 {
			before = body[j-1]
		}
		if j+len(token) < len(body) {
			after = body[j+len(token)]
		}
		if !isPathChar(before) && !isPathChar(after) {
			return true
		}
		i = j + 1
	}
	return false
}

// scopeWarningFiles returns the sorted changed files outside the ticket's
// stated scope. Empty (no warning) when the diff or the ticket body is
// unknown.
func scopeWarningFiles(ticketBody string, changed []string) []string {
	if strings.TrimSpace(ticketBody) == "" || len(changed) == 0 {
		return nil
	}
	var out []string
	for _, f := range changed {
		if !fileInTicketScope(ticketBody, f) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// scopeWarningPrompt renders the non-blocking scope warning for a Reviewer
// prompt: the listed files never block the ticket by themselves.
func scopeWarningPrompt(files []string) string {
	return "Scope note (non-blocking): these changed files are not mentioned in the ticket file or under a path it names: " + strings.Join(files, ", ") + ". Judge them normally, but report any that look like scope creep or another ticket's work as minor findings; this note alone never blocks the ticket."
}

// workerScopeRule is the scope instruction in every Worker first prompt: keep
// the diff to the Ticket's stated scope.
const workerScopeRule = "Keep the diff to the ticket's stated scope: touch only files the ticket mentions or paths under a path it names, and do not fold in commits from other tickets or unrelated edits."

// reviewerScopeRule is the scope instruction in every Reviewer task prefix.
const reviewerScopeRule = "Also flag files outside the ticket's stated scope (commits from other tickets or unrelated edits) as findings, quoting the ticket line they go beyond."

// lateRoundReviewNote returns the round-cap note for the upcoming review
// prompt when three or more rounds already exist (the next round is the
// fourth or later): a major that only restates a prior-round topic does not
// block; only blocker blocks. Prompt-only by design: matching "same topic"
// mechanically is unreliable, so the gate still counts majors and reviewers
// downgrade repeats to minor instead.
func lateRoundReviewNote(rounds []ticketRound) string {
	if len(rounds) < 3 {
		return ""
	}
	return "This is review round " + strconv.Itoa(len(rounds)+1) + ": a `major` finding that only restates a topic raised in a prior round does not block (only `blocker` blocks). Report repeats as `minor`, or do not re-raise them."
}

// diffBasePrompt tells a Reviewer which commits are the ticket's own. A
// ticket branch starts from a spec branch that already holds its merged
// blockers, so a plain branch-versus-master diff shows their commits as if
// the ticket made them; Reviewers flagged them as scope creep round after
// round. Empty when the base is unknown.
func diffBasePrompt(base, tip string) string {
	if base == "" || tip == "" {
		return ""
	}
	return "Diff base (non-blocking context): the ticket's own work is `git diff " + shortSHA(base) + " " + shortSHA(tip) + "` (commits `git log " + shortSHA(base) + ".." + shortSHA(tip) + "`). Commits at or before " + shortSHA(base) + " are earlier merged tickets: review only the ticket's own diff, and never flag the branch history, stacked blocker commits or files outside that diff."
}

// reviewerPromptWithExtras builds a Reviewer prompt from the task prefix, the
// declined findings, the green driver-check evidence, the scope warning and
// the round-4+ note, ahead of the axis brief. Empty extras are skipped.
func reviewerPromptWithExtras(ticketID, spec, reviewerType, branch, tip string, declined []declinedReviewFinding, evidence, scope, late string) string {
	prefix := reviewerTaskPrefix(ticketID, spec, reviewerType, branch, tip)
	if feedback := declinedFindingsPrompt(declined); feedback != "" {
		prefix += "\n\n" + feedback
	}
	for _, extra := range []string{evidence, scope, late} {
		if strings.TrimSpace(extra) != "" {
			prefix += "\n\n" + strings.TrimSpace(extra)
		}
	}
	brief := standardsBrief
	switch reviewerType {
	case reviewerSpecType:
		brief = specBrief
	case reviewerDesignType:
		brief = designBrief
	}
	return prefix + "\n\n" + brief
}

// reviewerPromptExtras assembles the non-blocking additions for one Reviewer
// prompt at tip: green driver-check evidence (cached only, never a fresh
// run), the scope warning for changed files outside the ticket's scope, and
// the round-4+ note. Git and file reads run unlocked.
func (d *Daemon) reviewerPromptExtras(runID, ticketID, tip string) (evidence, scope, late string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return "", "", ""
	}
	repo, target, recorded := rs.Repo, rs.Targets[ticketID], rs.Bases[ticketID]
	ticketFile := ticketPath(rs, ticketID)
	specBranch := rs.SpecBranch
	rounds := append([]ticketRound(nil), rs.Rounds[ticketID]...)
	if res, exists := rs.TipCheckCache[tip]; exists && res.OK && tip != "" {
		evidence = tipCheckEvidenceLine(tip)
	}
	d.mu.Unlock()

	late = lateRoundReviewNote(rounds)
	if tip == "" {
		return evidence, "", late
	}
	body := ""
	if ticketFile != "" {
		if specBranch != "" {
			if rel, rerr := filepath.Rel(repo, ticketFile); rerr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				if out, gerr := runGit(repo, "show", specBranch+":"+filepath.ToSlash(rel)); gerr == nil {
					body = out
				}
			}
		}
		if body == "" {
			body = readTicketFile(ticketFile)
		}
	}
	base := ticketBranchBase(repo, target, recorded, tip)
	changed := reviewChangedFiles(repo, base, tip)
	if warn := scopeWarningFiles(body, changed); len(warn) > 0 {
		scope = scopeWarningPrompt(warn)
	}
	if note := diffBasePrompt(base, tip); note != "" {
		if scope != "" {
			note += "\n\n" + scope
		}
		scope = note
	}
	return evidence, scope, late
}
