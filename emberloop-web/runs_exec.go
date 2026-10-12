package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	el "example.com/emberloop"
)

// Run statuses: a Run stays needs-you when a checkpoint rebase or final
// merge needs human intervention; paused and abandoned Runs do not advance.
const (
	runStatusRunning   = "running"
	runStatusDone      = "done"
	runStatusFailed    = "failed"
	runStatusPaused    = "paused"
	runStatusNeedsYou  = "needs-you"
	runStatusAbandoned = "abandoned"
)

// staticTicket is one Ticket as previewed at Start: its identity for the
// table. Live state comes from the shared driver rows.
type staticTicket struct {
	ID       string   `json:"id"`
	Num      int      `json:"num"`
	Title    string   `json:"title"`
	Blockers []string `json:"blockers"`
	Path     string   `json:"path"`
}

// submitRec is one Session's submits: how many arrived, the last
// status, its completed checkbox line texts and submit summary (workers;
// follow-ups come from the summary after Follow-ups:) and its report
// (kept for backward compatibility with worker reports).
type submitRec struct {
	Status    string   `json:"status"`
	Completed []string `json:"completed"`
	Report    string   `json:"report,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Count     int      `json:"count"`
}

// blockedTrack is one Ticket's blocked-question state for the submit gate:
// the normalized question hash, the question text for the needs-you reason,
// and how many times the Worker submitted blocked with that hash. A second
// blocked submit with the same hash becomes needs-you, so a Worker does not
// loop asking the same question.
type blockedTrack struct {
	Hash     string `json:"hash"`
	Question string `json:"question,omitempty"`
	Count    int    `json:"count"`
}

// The reviewer types every Ticket gets, one persistent Reviewer Session
// each. The strings match core's built-in reviewers, which the web world
// names literally so the gate agrees with the driver.
const (
	reviewerStandardsType = "reviewer-standards"
	reviewerSpecType      = "reviewer-spec"
)

// requiredReviewerTypes is the gate every finish needs: both reviewers
// passed at the worker's current tip.
var requiredReviewerTypes = []string{reviewerStandardsType, reviewerSpecType}

// storedReview is one reviewer type's verdict on a Ticket: the status it
// reported and the branch tip it was given at, plus the findings and the
// commands it ran. Report keeps the rendered `file:line severity: problem`
// lines for the refusal text; Findings keeps them structured for the rounds;
// Ran keeps the `ran` commands with exit codes from the submit tool, so a
// pass without evidence stays visible. PatchID is the `git patch-id` of the
// diff against the base at At, so a rebase with an identical diff reuses the
// verdict without a new Reviewer turn. A pass at an older tip is stale and
// does not count.
type storedReview struct {
	Verdict  string          `json:"verdict"`
	At       string          `json:"at"`
	Report   string          `json:"report,omitempty"`
	Findings []ReviewFinding `json:"findings,omitempty"`
	Ran      []ReviewRan     `json:"ran,omitempty"`
	PatchID  string          `json:"patchId,omitempty"`
}

// runWarning is one overlap warning Event on a Run: at each Ticket merge
// the driver diffs the Run's spec branch against every other live Run's
// spec branch in the same repo, and when the changed-file sets intersect
// it records the shared files here and on the other Run. The Event lives
// on the Run row and streams with the Run table over SSE (the Run's event
// channel); there is no Session Event log entry and no push. Old Run
// files without it read as no warnings.
type runWarning struct {
	At          time.Time `json:"at"`
	OtherRun    string    `json:"otherRun"`
	OtherBranch string    `json:"otherBranch"`
	Files       []string  `json:"files"`
	Message     string    `json:"message"`
}

// savedRow is one Ticket's persisted driver row: state, tip, refusal, state
// entry time and per-role Chain positions. It carries the live table across a
// Daemon restart.
type savedRow struct {
	State          string         `json:"state"`
	Tip            string         `json:"tip,omitempty"`
	LastRefusal    string         `json:"lastRefusal,omitempty"`
	Since          time.Time      `json:"since"`
	ChainPositions map[string]int `json:"chainPositions,omitempty"`
}

// runState is one deterministic Run: the spec, the repo, one Worker Session
// per Ticket in its own worktree and branch, two persistent Reviewer
// Sessions per Ticket once its Worker submits done, and the shared driver
// rows the loop publishes every pass. Sessions maps ticket ids to Worker
// Session ids, Reviewers ticket ids to reviewer type to Session id,
// ReviewTips ticket ids to reviewer type to the latest tip the reviewer was
// prompted on, ReviewPending ticket ids to reviewer type to the owed prompt
// queue (oldest unanswered tip first), and Reviews ticket ids to reviewer
// type to verdict; Submits lives on the Daemon so the submit route finds it
// without a run. Rows keeps the last published driver row per Ticket in
// the Run file, so the table survives a Daemon restart. Owner names the User
// who started the Run; its Worker and Reviewer Sessions inherit it.
type runState struct {
	ID              string                             `json:"id"`
	Spec            string                             `json:"spec"`
	Repo            string                             `json:"repo"`
	Status          string                             `json:"status"`
	Harness         string                             `json:"harness"`
	Model           string                             `json:"model"`
	Effort          string                             `json:"effort,omitempty"`
	CreatedAt       time.Time                          `json:"createdAt"`
	Error           string                             `json:"error,omitempty"`
	FinalMergeFiles []string                           `json:"finalMergeFiles,omitempty"`
	Owner           string                             `json:"owner,omitempty"`
	Tickets         []staticTicket                     `json:"tickets"`
	Sessions        map[string]string                  `json:"sessions"`
	Branches        map[string]string                  `json:"branches"`
	Worktrees       map[string]string                  `json:"worktrees"`
	Targets         map[string]string                  `json:"targets"`
	Bases           map[string]string                  `json:"bases,omitempty"`
	Merged          map[string]string                  `json:"merged,omitempty"`
	Reviewers       map[string]map[string]string       `json:"reviewers,omitempty"`
	Reviews         map[string]map[string]storedReview `json:"reviews,omitempty"`
	ReviewTips      map[string]map[string]string       `json:"reviewTips,omitempty"`
	ReviewPending   map[string]map[string][]string     `json:"reviewPending,omitempty"`
	Reprompted      map[string]int                     `json:"reprompted,omitempty"`
	// SpecBranch is the Run's integration branch, created from master (master
	// ref when present, else main) when absent and reused as it stands otherwise.
	// Ticket branches finish into it; only the final repo-check gate moves master.
	// SpecBase is the master tip at Start, even on reuse, for final rebases and
	// overlap warnings. Both survive a restart in the Run file.
	SpecBranch string `json:"specBranch,omitempty"`
	SpecBase   string `json:"specBase,omitempty"`
	// BaseCheckCache stores each spec-branch tip's repo-check result. A tip is
	// checked once per Run, and the cache survives a Daemon restart.
	BaseCheckCache map[string]baseCheckResult `json:"baseCheckCache,omitempty"`
	// Warnings holds the overlap warnings published at Ticket merges: one
	// entry per live spec branch this Run's changed files intersect with.
	// It survives a restart in the Run file and streams with the Run table.
	Warnings []runWarning `json:"warnings,omitempty"`
	// SubmitRecs keeps each Worker's last submit by ticket so a Daemon
	// restart does not reset the submit count below Reprompted.
	SubmitRecs map[string]submitRec `json:"submitRecs,omitempty"`
	// Blocked tracks each Ticket's blocked-question submits for the gate:
	// the second blocked submit with the same question hash becomes
	// needs-you. Old run files without it read as no blocked history.
	Blocked map[string]blockedTrack `json:"blocked,omitempty"`
	// ReviewStarted holds when the current tip was prompted per ticket per
	// reviewer type, so a verdict's durationMs measures prompt to verdict.
	ReviewStarted map[string]map[string]time.Time `json:"reviewStarted,omitempty"`
	// ReviewSeen holds the last prompt seq consumed per ticket per reviewer
	// type, so a turn drained inside finishTurn and the runTurn edge never
	// record one reply twice.
	ReviewSeen map[string]map[string]int64 `json:"reviewSeen,omitempty"`
	// FormatReprompt marks reviewers already re-prompted once for the
	// VERDICT format: a second unparseable reply becomes needs-you.
	FormatReprompt map[string]map[string]bool `json:"formatReprompt,omitempty"`
	// Rounds is the append-only review history per ticket for the ticket
	// page. Old run files without it still load (nil reads as no rounds).
	Rounds map[string][]ticketRound `json:"rounds,omitempty"`
	// NeedsYouReason holds the needs-you alert text per ticket: the same
	// refusal x3, idle 10m or stall-nudge message from the driver Notify,
	// or a reviewer-format error needing the user.
	NeedsYouReason map[string]string `json:"needsYouReason,omitempty"`
	// pushedNeedsYou is the needs-you reason last pushed per ticket, so the
	// driver pushing every pass still notifies once per reason. Memory only:
	// a Daemon restart may repeat one push.
	pushedNeedsYou map[string]string
	// Followups holds the worker's follow-ups per ticket: the text its last
	// submit named after Follow-ups:.
	Followups map[string]string `json:"followups,omitempty"`
	// Parked holds the user's park action: the driver never starts or finishes
	// a parked Ticket, but its Worker Session remains available for retry.
	Parked map[string]bool `json:"parked,omitempty"`
	// TicketPaused blocks a Ticket while it is explicitly paused or its
	// Run-level roles are stopped. The pause is cleared only by its Resume.
	TicketPaused map[string]bool `json:"ticketPaused,omitempty"`
	// TicketPauseHolds keeps an explicit Ticket Pause active across Run Resume.
	TicketPauseHolds map[string]bool `json:"ticketPauseHolds,omitempty"`
	// RunPausedRoles is the subset of PausedRoles that Run Resume may replace;
	// roles paused only by a Ticket action stay paused.
	RunPausedRoles map[string]map[string]bool `json:"runPausedRoles,omitempty"`
	// PausedRoles records Run-owned Sessions awaiting manual Ticket or Run
	// Resume, keyed by Ticket ID and Run role.
	PausedRoles map[string]map[string]bool `json:"pausedRoles,omitempty"`
	// ReviewerOverrides replaces active Reviewer Chain entries after a Ticket
	// or Run Switch; later fallback entries stay intact.
	ReviewerOverrides map[string]map[string]chainEntry `json:"reviewerOverrides,omitempty"`
	// WorkerOverrides replaces the active Worker Chain entry for one Ticket
	// after a Ticket or Run Switch, without changing fallback entries.
	WorkerOverrides map[string]chainEntry `json:"workerOverrides,omitempty"`
	// Discarded holds the user's discard per ticket from POST
	// /api/runs/{id}/tickets/{tid}/discard: the ticket's worktree and
	// branch are removed and the driver never touches it again.
	Discarded map[string]bool `json:"discarded,omitempty"`
	// RetryGen is the user's retry generation per ticket from POST
	// /api/runs/{id}/tickets/{tid}/retry: each retry bumps it, and the
	// driver drops that ticket's needs-you mark, backoff, refusal streak
	// and stall-nudge count on its next pass.
	RetryGen map[string]int `json:"retryGen,omitempty"`
	// RecallOffN is the Run A/B setting (ticket 30): every Nth ticket in
	// ticket order starts with Rule recall off (SessionMeta.NoRecall), so
	// reviewer verdicts give a paired comparison against recall-on Workers.
	// 0 disables the A/B arm; 1 is refused at Start (no control arm left);
	// large N is allowed and selects nothing in a small run. It is a Run
	// setting and is recorded in the Run file.
	RecallOffN int `json:"recallOffN,omitempty"`
	// RecallOff marks the tickets whose Worker runs with recall off, by
	// ticket ID. Assigned at worker start from RecallOffN in ticket order,
	// stable across retries and resumes; old Run files without it read as
	// no A/B Workers.
	RecallOff map[string]bool `json:"recallOff,omitempty"`
	// ResumeOnRestart marks a Run that a graceful restart paused: the next
	// Daemon resumes it on its own (ResumeAfterRestart). A Pause by hand
	// never sets it.
	ResumeOnRestart bool                    `json:"resumeOnRestart,omitempty"`
	Rows            map[string]savedRow     `json:"rows,omitempty"`
	Chains          map[string][]chainEntry `json:"chains,omitempty"`
	ChainExhausted  map[string]bool         `json:"chainExhausted,omitempty"`
	// ChainPendingSubmit invalidates an old Worker done submit on a provider
	// Chain failure until the replacement Worker submits done.
	ChainPendingSubmit map[string]bool `json:"chainPendingSubmit,omitempty"`
	chainHandling      map[string]bool `json:"-"`
	SpecRun            el.SpecRun      `json:"-"`
	Driver             *el.DriverRun   `json:"-"`
	notify             chan struct{}   `json:"-"`
	unpause            chan struct{}   `json:"-"`
	driving            bool            `json:"-"`
}

// runPollInterval caps the driver sleep: the loop polls for Worker submits
// on this cadence instead of the CLI's slower poll.
const runPollInterval = 500 * time.Millisecond

// roleWorkerConfig reads the Run worker harness, model and effort from the
// settings roles: the implementer slot of the settings page first, then the
// older worker slot. Empty when neither is set, so the caller falls back to
// the Session default.
func roleWorkerConfig() (harness, model, effort string) {
	return lookupWorkerRole(readRoles())
}

// daemonRoleWorkerConfig reads this Daemon's Run worker harness, model and
// effort from its settings roles.
func (d *Daemon) daemonRoleWorkerConfig() (harness, model, effort string) {
	return lookupWorkerRole(d.settingsRoles())
}

// lookupWorkerRole resolves the worker slot from a decoded roles map: shared
// by the host reader and the Daemon-scoped test seam.
func lookupWorkerRole(roles map[string]roleSetting) (harness, model, effort string) {
	for _, name := range []string{"implementer", "worker"} {
		r, ok := roles[name]
		if !ok || r.Harness == "" || r.Model == "" {
			continue
		}
		if r.Harness == "claude" && !validClaudeModel(r.Model) {
			continue
		}
		return r.Harness, r.Model, r.Effort
	}
	return "", "", ""
}

// roleReviewerConfig reads one reviewer type's harness and model from the
// settings roles: its own type name first, then the settings-page alias
// (spec-review, code-review). Empty when neither role is set, so the caller
// falls back the same way workers do.
func roleReviewerConfig(reviewerType string) (harness, model string) {
	roles := readRoles()
	if r, ok := roles[reviewerType]; ok && r.Harness != "" && r.Model != "" {
		if r.Harness != "claude" || validClaudeModel(r.Model) {
			return r.Harness, r.Model
		}
	}
	alias := map[string]string{
		reviewerSpecType:      "spec-review",
		reviewerStandardsType: "code-review",
	}[reviewerType]
	if alias != "" {
		if r, ok := roles[alias]; ok && r.Harness != "" && r.Model != "" {
			if r.Harness != "claude" || validClaudeModel(r.Model) {
				return r.Harness, r.Model
			}
		}
	}
	return "", ""
}

// gitRoot walks up from path to the repo holding it: the first directory
// with a .git entry. Empty when path sits in no repo.
func gitRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(abs) {
		return ""
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// runGit runs git in dir and returns trimmed stdout, failing on git errors
// and nonzero exits.
func runGit(dir string, args ...string) (string, error) {
	var out string
	var err error
	// Another git process (a parallel Worker finish, the user) can hold
	// index.lock for a moment; retry that failure instead of failing the Run.
	for attempt := 0; attempt < 8; attempt++ {
		out, err = runGitOnce(dir, args...)
		if err == nil || !strings.Contains(err.Error(), "index.lock") {
			return out, err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return out, err
}

func runGitOnce(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// specBranchExists reports whether branch exists in repo, without failing
// on its absence: show-ref exits 1 when the ref is missing, which is the
// reuse signal, not an error.
func specBranchExists(repo, branch string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repo
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	} else {
		return false, err
	}
}

// masterTip resolves the master tip of repo explicitly, never HEAD: the
// master ref when present, else main. A checkout on another branch or
// detached must not move where a new spec branch forks from.
func masterTip(repo string) (string, error) {
	for _, ref := range []string{"refs/heads/master", "refs/heads/main"} {
		if out, err := runGit(repo, "rev-parse", ref); err == nil && out != "" {
			return out, nil
		}
	}
	return "", fmt.Errorf("could not read the master tip in %s: neither master nor main exists", repo)
}

// ensureSpecBranch creates the Run's spec/<slug> branch from the master tip
// when absent and reuses it as it stands otherwise. It returns the master
// tip at Start as the base the Run records: on reuse the branch keeps its
// own tip while the recorded base still names master for the later
// final-merge rebase. The main checkout never moves, so two specs run at
// once.
func ensureSpecBranch(repo, branch string) (base string, created bool, err error) {
	base, err = masterTip(repo)
	if err != nil {
		return "", false, err
	}
	exists, err := specBranchExists(repo, branch)
	if err != nil {
		return "", false, err
	}
	if exists {
		return base, false, nil
	}
	if _, err := runGit(repo, "branch", branch, base); err != nil {
		return "", false, err
	}
	return base, true, nil
}

// runStagedDiff reports whether rel has staged changes: git diff --cached
// --quiet exits 0 with none, 1 with some. Other errors fail the finish.
func runStagedDiff(dir, rel string) (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--", rel)
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		return false, nil
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return true, nil
	} else if ok {
		// Exit 128 and the like are git failures, not "has changes".
		return false, fmt.Errorf("git diff --cached %s failed: %w", rel, err)
	} else {
		return false, err
	}
}

// runGitCommitDone commits the ticket done mark, returning combined output
// with the error: git prints "nothing to commit" on stdout, which runGit
// drops, so the caller checks both.
func runGitCommitDone(dir, rel, workerID string) (string, error) {
	var out string
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		cmd := exec.Command("git", "commit", "-m", "mark "+workerID+" done", "--", rel)
		cmd.Dir = dir
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		out = stdout.String() + stderr.String()
		if err == nil || !strings.Contains(out, "index.lock") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		// Keep git's own words: a bare "exit status 128" hides the cause.
		err = fmt.Errorf("git commit %s failed: %w: %s", rel, err, strings.TrimSpace(out))
	}
	return out, err
}

// reviewPatchIDFor returns the `git patch-id` of the diff of tip against its
// base: the merge-base with the ticket's target branch, or the recorded base
// when detached. Empty when it cannot be computed (missing tip, no base, an
// empty diff or any git failure); callers then degrade to tip equality, so a
// missing patch id never blocks or reuses on its own. It locks only to read
// the run state; the git calls run unlocked.
func (d *Daemon) reviewPatchIDFor(runID, ticketID, tip string) string {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	var repo, target, recorded string
	if ok {
		repo = rs.Repo
		target = rs.Targets[ticketID]
		recorded = rs.Bases[ticketID]
	}
	d.mu.Unlock()
	if !ok || repo == "" || tip == "" {
		return ""
	}
	base := recorded
	if target != "" {
		if mb, err := runGit(repo, "merge-base", target, tip); err == nil && mb != "" {
			base = mb
		} else if base == "" {
			return ""
		}
	}
	if base == "" || base == tip {
		return ""
	}
	return gitPatchID(repo, base, tip)
}

// gitPatchID pipes `git diff base tip` into `git patch-id` and returns the
// patch id. Empty on any failure or empty diff.
func gitPatchID(repo, base, tip string) string {
	diff := exec.Command("git", "diff", base, tip)
	diff.Dir = repo
	raw, err := diff.Output()
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	for _, args := range [][]string{{"patch-id", "--stable"}, {"patch-id"}} {
		patch := exec.Command("git", args...)
		patch.Dir = repo
		patch.Stdin = bytes.NewReader(raw)
		out, err := patch.Output()
		if err != nil {
			continue
		}
		if fields := strings.Fields(strings.TrimSpace(string(out))); len(fields) > 0 {
			return fields[0]
		}
		return ""
	}
	return ""
}

// handleRuns serves the Run collection: list Runs, or start one.
func (d *Daemon) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.mu.Lock()
		states := make([]*runState, 0, len(d.runs))
		for _, rs := range d.runs {
			if rs.Owner != "" {
				states = append(states, rs)
			}
		}
		d.mu.Unlock()
		runs := make([]map[string]any, 0, len(states))
		for _, rs := range states {
			if out := d.runJSON(rs); out != nil {
				runs = append(runs, out)
			}
		}
		sort.Slice(runs, func(i, j int) bool {
			return runs[i]["id"].(string) < runs[j]["id"].(string)
		})
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	case "POST":
		d.createRun(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunSub serves one Run and its live Ticket table over SSE.
func (d *Daemon) handleRunSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	// A Run with no Owner is hidden: every route answers 404 for it.
	if d.hiddenRun(id) {
		http.NotFound(w, r)
		return
	}
	switch {
	case sub == "" && r.Method == "GET":
		d.mu.Lock()
		rs, ok := d.runs[id]
		d.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		out := d.runJSON(rs)
		writeJSON(w, http.StatusOK, map[string]any{"run": out})
	case sub == "events" && r.Method == "GET":
		d.serveRunEvents(w, r, id)
	case sub == "pause" && r.Method == "POST":
		d.pauseRun(w, r, id)
	case sub == "resume" && r.Method == "POST":
		d.resumeRun(w, r, id)
	case sub == "switch" && r.Method == "POST":
		d.switchRunBackend(w, r, id)
	case sub == "rebase" && r.Method == "POST":
		d.rebaseRun(w, r, id)
	case sub == "abandon" && r.Method == "POST":
		d.abandonRun(w, r, id)
	case strings.HasPrefix(sub, "tickets/") && r.Method == "POST":
		rest := strings.TrimPrefix(sub, "tickets/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		switch parts[1] {
		case "pause":
			d.pauseTicket(w, r, id, parts[0])
		case "resume":
			d.resumeTicket(w, r, id, parts[0])
		case "switch":
			d.switchTicketBackend(w, r, id, parts[0])
		case "steer":
			d.steerTicket(w, r, id, parts[0])
		case "retry":
			d.retryTicket(w, r, id, parts[0])
		case "park":
			d.parkTicket(w, r, id, parts[0])
		case "discard":
			d.discardTicket(w, r, id, parts[0])
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

// createRun resolves the spec, refuses a second active Run on it, records
// the Run and starts its driver loop in the background.
func (d *Daemon) createRun(w http.ResponseWriter, r *http.Request) {
	creator, ok := d.creatorOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Spec       string `json:"spec"`
		Repo       string `json:"repo"`
		Harness    string `json:"harness"`
		Model      string `json:"model"`
		Effort     string `json:"effort"`
		RecallOffN int    `json:"recallOffN"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Spec == "" {
		http.Error(w, "spec is required", http.StatusBadRequest)
		return
	}
	// The A/B arm needs a control arm: 0 disables it, 1 would switch every
	// Worker off and leave nothing to compare against. Large N is allowed;
	// the arm then selects nothing in a small run.
	if req.RecallOffN < 0 || req.RecallOffN == 1 {
		http.Error(w, "recallOffN must be 0 or at least 2", http.StatusBadRequest)
		return
	}
	specRun, err := el.ResolveSpec(req.Spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	files, err := el.ReadDriverFiles(specRun)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	previews := el.PreviewTickets(specRun, files)
	for _, p := range previews {
		if p.Error != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": p.Error})
			return
		}
	}
	repo := req.Repo
	if repo == "" {
		repo = gitRoot(specRun.Spec)
	}
	if repo == "" {
		http.Error(w, "repo is required", http.StatusBadRequest)
		return
	}
	if info, err := os.Stat(repo); err != nil || !info.IsDir() {
		http.Error(w, "repo is not a directory", http.StatusBadRequest)
		return
	}
	if rel, err := filepath.Rel(repo, specRun.Spec); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.Error(w, "spec is not inside the repo", http.StatusBadRequest)
		return
	}
	harness, model, effort := d.daemonRoleWorkerConfig()
	if harness == "" {
		harness = req.Harness
	}
	if model == "" {
		model = req.Model
	}
	if effort == "" {
		effort = req.Effort
	}
	if harness == "" || model == "" {
		http.Error(w, "harness and model are required", http.StatusBadRequest)
		return
	}
	max := d.MaxRunning()
	d.mu.Lock()
	for _, rs := range d.runs {
		if rs.Spec == specRun.Spec && (rs.Status == runStatusRunning || rs.Status == runStatusNeedsYou) {
			d.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]any{"error": "a run is already active for this spec"})
			return
		}
	}
	// The Run works on its own spec/<slug> branch, created from the master
	// tip (master ref when present, else main) when absent and reused as it
	// stands otherwise; the recorded base is always the master tip at
	// Start, even on reuse. It runs after the duplicate-Run check, so a
	// refused Start leaves no branch behind. Ticket branches fork from it
	// and finish back into it; the main checkout never moves, so two specs
	// run at once. The branch and the base SHA persist on the Run row and
	// survive a restart.
	specBase, _, err := ensureSpecBranch(repo, specRun.Branch)
	if err != nil {
		d.mu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pathByID := map[string]string{}
	for path := range files {
		if filepath.Dir(filepath.Clean(path)) == filepath.Clean(specRun.Tickets) && strings.HasSuffix(path, ".md") {
			pathByID[strings.TrimSuffix(filepath.Base(path), ".md")] = path
		}
	}
	rs := &runState{
		ID:                 newID(),
		Spec:               specRun.Spec,
		Repo:               repo,
		Status:             runStatusRunning,
		Harness:            harness,
		Model:              model,
		Effort:             effort,
		RecallOffN:         req.RecallOffN,
		CreatedAt:          time.Now().UTC(),
		Owner:              creator.ID,
		SpecBranch:         specRun.Branch,
		SpecBase:           specBase,
		Sessions:           map[string]string{},
		Branches:           map[string]string{},
		Worktrees:          map[string]string{},
		Targets:            map[string]string{},
		Bases:              map[string]string{},
		Merged:             map[string]string{},
		Reviewers:          map[string]map[string]string{},
		Reviews:            map[string]map[string]storedReview{},
		ReviewTips:         map[string]map[string]string{},
		ReviewPending:      map[string]map[string][]string{},
		Reprompted:         map[string]int{},
		ReviewStarted:      map[string]map[string]time.Time{},
		FormatReprompt:     map[string]map[string]bool{},
		Rounds:             map[string][]ticketRound{},
		NeedsYouReason:     map[string]string{},
		Followups:          map[string]string{},
		Rows:               map[string]savedRow{},
		Chains:             d.daemonConfiguredRunChains(harness, model, effort),
		ChainExhausted:     map[string]bool{},
		ChainPendingSubmit: map[string]bool{},
		SpecRun:            specRun,
		Driver:             el.NewDriverRun(nil),
		Parked:             map[string]bool{},
		TicketPaused:       map[string]bool{},
		TicketPauseHolds:   map[string]bool{},
		PausedRoles:        map[string]map[string]bool{},
		RunPausedRoles:     map[string]map[string]bool{},
		WorkerOverrides:    map[string]chainEntry{},
		ReviewerOverrides:  map[string]map[string]chainEntry{},
		Discarded:          map[string]bool{},
		RetryGen:           map[string]int{},
		RecallOff:          map[string]bool{},

		notify:  make(chan struct{}),
		unpause: make(chan struct{}),
		driving: true,
	}
	for _, p := range previews {
		blockers := p.Blockers
		if blockers == nil {
			blockers = []string{}
		}
		rs.Tickets = append(rs.Tickets, staticTicket{
			ID: p.ID, Num: p.Num, Title: p.Title, Blockers: blockers, Path: pathByID[p.ID],
		})
	}
	world := d.webDriverWorld(rs, files, max)
	d.runs[rs.ID] = rs
	d.persistRun(rs)
	d.mu.Unlock()
	out := d.runJSON(rs)
	go d.driveRun(rs.ID, world)
	writeJSON(w, http.StatusCreated, map[string]any{"run": out})

}

// driveRun runs the core deterministic driver against the web-backed world,
// then gates the spec branch's final merge into master.
func (d *Daemon) driveRun(id string, world el.DriverWorld) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	d.mu.Unlock()
	if !ok {
		return
	}
	res, err := el.RunDriver(rs.SpecRun, world)
	var final finalMergeOutcome
	if err == nil && rs.SpecBranch != "" {
		final = d.finalMergeReadiness(id, res)
		if !final.NeedsYou {
			final, err = d.finalizeSpecRun(id)
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok = d.runs[id]
	if !ok {
		return
	}
	rs.driving = false
	d.snapshotRowsLocked(rs)
	if rs.Status != runStatusAbandoned && rs.Status != runStatusNeedsYou {
		for _, m := range res.Merged {
			rs.Merged[m.ID] = m.SHA
		}
	}
	if rs.Status != runStatusAbandoned && rs.Status != runStatusNeedsYou && rs.Status != runStatusPaused {
		if err != nil {
			rs.Status = runStatusFailed
			rs.Error = err.Error()
			rs.FinalMergeFiles = nil
		} else if final.NeedsYou {
			rs.Status = runStatusNeedsYou
			rs.Error = final.Reason
			rs.FinalMergeFiles = append([]string(nil), final.Files...)
		} else {
			rs.Status = runStatusDone
			rs.Error = ""
			rs.FinalMergeFiles = nil
		}
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
}

func (d *Daemon) webDriverWorld(rs *runState, files map[string]string, max int) el.DriverWorld {
	if max <= 0 {
		max = defaultMaxRunning
	}
	return el.DriverWorld{
		Files:      files,
		MaxWorkers: max,
		// The Daemon spawns the Reviewers after a done submit, so a stalled
		// Worker is told to finish, never to spawn its own reviewers.
		StallNudge: webStallNudge,
		// clock is the test seam for idle and stall alerts; nil uses time.Now.
		Now: d.clock,
		// Parked and Discarded read the user's actions each pass. For an
		// unstarted spec-branch Ticket, Parked also runs the base-red gate;
		// a red or unverifiable base blocks the start and publishes Needs-you.
		Parked: func(ticketID string) bool {
			d.mu.Lock()
			userParked := rs.Parked[ticketID] || rs.TicketPaused[ticketID]
			started := rs.Sessions[ticketID] != ""
			discarded := rs.Discarded[ticketID]
			repo, specBranch, runID := rs.Repo, rs.SpecBranch, rs.ID
			d.mu.Unlock()
			if discarded {
				return true
			}
			if userParked || started || specBranch == "" {
				return userParked
			}
			tip, err := runGit(repo, "rev-parse", specBranch)
			if err != nil || strings.TrimSpace(tip) == "" {
				cause := "empty spec-branch tip"
				if err != nil {
					cause = strings.TrimSpace(err.Error())
				}
				d.recordBaseRedForTicket(runID, ticketID, specBranch, "unknown", []string{"base check could not read spec branch: " + cause})
				return true
			}
			tip = strings.TrimSpace(tip)
			green, failing := d.cachedBaseCheck(runID, repo, specBranch, tip)
			if !green {
				d.recordBaseRedForTicket(runID, ticketID, specBranch, tip, failing)
				return true
			}
			d.clearBaseRedForTicket(runID, ticketID)
			return false
		},
		Discarded: func(ticketID string) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.Discarded[ticketID]
		},
		RetryGen: func(ticketID string) int {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.RetryGen[ticketID]
		},
		// HasWorker names started Tickets from their recorded Worker Sessions.
		// A discarded queued Ticket also seeds as started so the core driver
		// observes it as gone instead of starting it after the user's discard.
		HasWorker: func(ticketID string) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.Sessions[ticketID] != "" || rs.ChainExhausted[ticketID] || rs.Discarded[ticketID]
		},
		MergedSHA: func(ticketID string) string {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.Merged[ticketID]
		},
		StartWorker: func(tk el.DriverTicket) error {
			return d.runStartWorker(rs.ID, tk)
		},
		Tip: func(ticketID string) string {
			d.mu.Lock()
			branch := rs.Branches[ticketID]
			repo := rs.Repo
			d.mu.Unlock()
			if branch == "" {
				return ""
			}
			sha, err := runGit(repo, "rev-parse", branch)
			if err != nil {
				return ""
			}
			return sha
		},
		Reviews: func(ticketID string) map[string]el.DriverReview {
			d.ensureReviewers(rs.ID, ticketID)
			return d.runReviews(rs.ID, ticketID)
		},
		Tell: func(ticketID, text string) error {
			return d.runTellWorker(rs.ID, ticketID, text, systemSender)
		},
		StallNudgeAllowed: func(ticketID string) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return !sessionLimitCapped(d.sessions[rs.Sessions[ticketID]])
		},
		Working: func(ticketID string) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			s, ok := d.sessions[rs.Sessions[ticketID]]
			return ok && s.meta.Status == "working"
		},
		Submits: func(ticketID string) int {
			return d.runSubmitCount(rs.ID, ticketID)
		},
		Completed: func(ticketID string) []string {
			d.mu.Lock()
			defer d.mu.Unlock()
			rec, ok := d.submits[rs.Sessions[ticketID]]
			if !ok {
				return nil
			}
			return append([]string(nil), rec.Completed...)
		},
		TickCheckboxes: func(ticketID string, boxes []string) ([]string, error) {
			d.mu.Lock()
			path := ticketPath(rs, ticketID)
			repo, specBranch := rs.Repo, rs.SpecBranch
			d.mu.Unlock()
			if path == "" {
				return boxes, nil
			}
			if specBranch != "" {
				// Ticket progress is report-only until the reviewed branch is merged
				// into the spec branch. The done-mark commit ticks the boxes there.
				body := readTicketFile(path)
				if rel, err := filepath.Rel(repo, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					if out, err := runGit(repo, "show", specBranch+":"+filepath.ToSlash(rel)); err == nil {
						body = out
					}
				}
				return ticketUnknownBoxes(body, boxes), nil
			}
			miss, err := el.TickDriverTicketFile(path, boxes)
			if err == nil {
				// Legacy Runs without an integration branch still publish progress
				// ticks directly; spec-branch Runs defer them until merge.
				d.snapshotAndPersist(rs.ID)
			}
			return miss, err
		},

		Followups: func(ticketID string) string {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.Followups[ticketID]
		},
		Notify: func(ticketID, message string) error {
			d.mu.Lock()
			if rs.NeedsYouReason == nil {
				rs.NeedsYouReason = map[string]string{}
			}
			rs.NeedsYouReason[ticketID] = message
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
			push := d.noteNeedsYouPushLocked(rs, ticketID, message)
			num, title := ticketNumTitleLocked(rs, ticketID)
			d.mu.Unlock()
			if push {
				d.pushRunNeedsYou(rs.ID, ticketID, num, title, message)
			}
			return nil
		},
		Finish: func(ticketID string) (el.DriverFinish, error) {
			return d.runFinishWorker(rs.ID, ticketID)
		},
		Required: append([]string(nil), requiredReviewerTypes...),
		Run:      rs.Driver,
		// Sleep caps the driver's poll, writes the pass to the Run file
		// first (state, tip, refusal and times per Ticket), then waits out
		// a pause with no lock held, so Resume never blocks on the driver.
		Sleep: func(wait time.Duration) {
			d.snapshotAndPersist(rs.ID)
			d.pauseGate(rs.ID)
			if cap := d.pollIntervalOrDefault(); wait > cap {
				wait = cap
			}
			time.Sleep(wait)
		},
	}
}

// noteNeedsYouPushLocked records a needs-you reason as pushed and reports
// whether it is new or changed since the last push, so repeated driver
// passes still notify once per reason. Callers hold d.mu.
func (d *Daemon) noteNeedsYouPushLocked(rs *runState, ticketID, reason string) bool {
	if rs.pushedNeedsYou == nil {
		rs.pushedNeedsYou = map[string]string{}
	}
	if rs.pushedNeedsYou[ticketID] == reason {
		return false
	}
	rs.pushedNeedsYou[ticketID] = reason
	return true
}

// ticketNumTitleLocked names one ticket for a push. Callers hold d.mu.
func ticketNumTitleLocked(rs *runState, ticketID string) (int, string) {
	for _, t := range rs.Tickets {
		if t.ID == ticketID {
			return t.Num, t.Title
		}
	}
	return 0, ticketID
}

// ticketPath returns the ticket file for one Ticket of the Run; callers hold
// d.mu.
func ticketPath(rs *runState, ticketID string) string {
	for _, t := range rs.Tickets {
		if t.ID == ticketID {
			return t.Path
		}
	}
	return ""
}

// runTicketBody returns the ticket file content the live table shows: from
// the Run's spec branch when it has one (the merge target, so ticks and the
// done mark show the way they did when tickets merged to master), else from
// the checkout. Missing files read empty; callers fall back.
func runTicketBody(rs *runState, ticketID string) string {
	path := ticketPath(rs, ticketID)
	if path == "" {
		return ""
	}
	if rs.SpecBranch != "" {
		rel, err := filepath.Rel(rs.Repo, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if out, err := runGit(rs.Repo, "show", rs.SpecBranch+":"+filepath.ToSlash(rel)); err == nil {
				return out
			}
		}
	}
	return readTicketFile(path)
}

// baseCheckResult is one spec-branch tip's repo-check outcome, cached on
// the Run so every Ticket starting from that tip shares the same result.
type baseCheckResult struct {
	OK      bool     `json:"ok"`
	Failing []string `json:"failing,omitempty"`
}

// ticketUnknownBoxes reports submitted names absent from the ticket file
// without mutating it. Spec-branch Runs defer every tick until merge.
func ticketUnknownBoxes(content string, boxes []string) []string {
	have := map[string]bool{}
	for _, box := range parseTicketBoxes(content) {
		have[strings.TrimSpace(box.Text)] = true
	}
	var unknown []string
	for _, box := range boxes {
		if !have[strings.TrimSpace(box)] {
			unknown = append(unknown, box)
		}
	}
	return unknown
}

func isBaseRedReason(reason string) bool {
	return strings.Contains(reason, ": base ") && strings.Contains(reason, " is red")
}

func baseRedReason(ticketID, specBranch, tip string, failing []string) string {
	list := strings.Join(failing, ", ")
	if list == "" {
		list = "check failed"
	}
	return fmt.Sprintf("ticket %s needs you: base %s at %s is red: failing: %s", ticketID, specBranch, shortSHA(tip), list)
}

// cachedBaseCheck runs the repo check once per spec-branch tip and persists
// its result. Git and check work stay outside d.mu; cache accesses are locked.
func (d *Daemon) cachedBaseCheck(runID, repo, specBranch, tip string) (bool, []string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return false, []string{"base check could not run: Run no longer exists"}
	}
	if result, exists := rs.BaseCheckCache[tip]; exists {
		d.mu.Unlock()
		return result.OK, append([]string(nil), result.Failing...)
	}
	d.mu.Unlock()

	green, failing := true, []string(nil)
	if d.baseCheck != nil {
		green, failing = d.baseCheck(repo, specBranch, tip)
	} else {
		green, failing = runRepoCheckAtTip(repo, tip)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok = d.runs[runID]
	if !ok {
		return false, []string{"base check could not run: Run no longer exists"}
	}
	if rs.BaseCheckCache == nil {
		rs.BaseCheckCache = map[string]baseCheckResult{}
	}
	if cached, exists := rs.BaseCheckCache[tip]; exists {
		return cached.OK, append([]string(nil), cached.Failing...)
	}
	rs.BaseCheckCache[tip] = baseCheckResult{OK: green, Failing: append([]string(nil), failing...)}
	d.persistRun(rs)
	return green, append([]string(nil), failing...)
}

// recordBaseRedForTicket persists and publishes a red-base Needs-you banner.
// The Parked callback calls it only for tickets that have not started.
func (d *Daemon) recordBaseRedForTicket(runID, ticketID, specBranch, tip string, failing []string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok || rs.Sessions[ticketID] != "" || rs.Merged[ticketID] != "" || rs.Parked[ticketID] || rs.Discarded[ticketID] {
		d.mu.Unlock()
		return
	}
	reason := baseRedReason(ticketID, specBranch, tip, failing)
	if rs.NeedsYouReason[ticketID] == reason {
		d.mu.Unlock()
		return
	}
	if rs.NeedsYouReason == nil {
		rs.NeedsYouReason = map[string]string{}
	}
	rs.NeedsYouReason[ticketID] = reason
	push := d.noteNeedsYouPushLocked(rs, ticketID, reason)
	num, title := ticketNumTitleLocked(rs, ticketID)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	if push {
		d.pushRunNeedsYou(runID, ticketID, num, title, reason)
	}
}

// clearBaseRedForTicket clears only this gate's banner after a later tip
// checks green; unrelated Needs-you reasons are left untouched.
func (d *Daemon) clearBaseRedForTicket(runID, ticketID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[runID]
	if !ok {
		return
	}
	reason := rs.NeedsYouReason[ticketID]
	if !isBaseRedReason(reason) {
		return
	}
	delete(rs.NeedsYouReason, ticketID)
	if rs.pushedNeedsYou[ticketID] == reason {
		delete(rs.pushedNeedsYou, ticketID)
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
}

// runRepoCheckAtTip runs .emberloop/check in a detached worktree at tip.
// A repository with no check file is green; a check that cannot be verified
// or run fails closed so no Worker starts on an unknown base.
func runRepoCheckAtTip(repo, tip string) (bool, []string) {
	if _, err := runGit(repo, "cat-file", "-e", tip); err != nil {
		return false, []string{"base check could not run: base tip: " + strings.TrimSpace(err.Error())}
	}
	if _, err := runGit(repo, "show", tip+":.emberloop/check"); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return true, nil
		}
		return false, []string{"base check could not run: check file: " + strings.TrimSpace(err.Error())}
	}
	dir, err := os.MkdirTemp("", "el-base-check-*")
	if err != nil {
		return false, []string{"base check could not run: temp dir: " + strings.TrimSpace(err.Error())}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if _, err := runGit(repo, "worktree", "add", "--detach", dir, tip); err != nil {
		return false, []string{"base check could not run: worktree: " + strings.TrimSpace(err.Error())}
	}
	defer func() { _, _ = runGit(repo, "worktree", "remove", "--force", dir) }()
	cmd := exec.Command("sh", ".emberloop/check")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return false, []string{"base check could not run: " + strings.TrimSpace(err.Error())}
	}
	return false, parseCheckFailing(string(out))
}

// parseCheckFailing extracts failing Go tests and packages for Needs-you.
// A check failure without parseable test output still gets a useful banner.
func parseCheckFailing(output string) []string {
	var failing []string
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] && len(failing) < 20 {
			seen[name] = true
			failing = append(failing, name)
		}
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "--- FAIL:"); i >= 0 {
			rest := strings.TrimSpace(line[i+len("--- FAIL:"):])
			if fields := strings.Fields(rest); len(fields) > 0 {
				add(fields[0])
			}
		} else if strings.HasPrefix(line, "FAIL") {
			add(line)
		}
	}
	if len(failing) == 0 {
		failing = []string{"check failed"}
	}
	return failing
}

// runReviews returns one Ticket's reviewer verdicts as the driver sees them:
// each type's submitted status at the tip it was given at.
func (d *Daemon) runReviews(runID, ticketID string) map[string]el.DriverReview {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[runID]
	if !ok {
		return map[string]el.DriverReview{}
	}
	out := map[string]el.DriverReview{}
	for typ, r := range rs.Reviews[ticketID] {
		out[typ] = el.DriverReview{Verdict: r.Verdict, At: r.At}
	}
	return out
}

// runSubmitCount counts one Ticket's submits for the driver's retry: the
// Worker submits plus every Reviewer submit. A reviewer verdict advances the
// count, so the finish retries without another Worker submit once both
// reviewers passed at the tip.
func (d *Daemon) runSubmitCount(runID, ticketID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[runID]
	if !ok {
		return 0
	}
	n := 0
	if rec := d.submits[rs.Sessions[ticketID]]; rec != nil {
		n += rec.Count
	}
	for _, sess := range rs.Reviewers[ticketID] {
		if rec := d.submits[sess]; rec != nil {
			n += rec.Count
		}
	}
	return n
}

// workerReviewNote tells a Worker who reviews its work. The Daemon starts the
// spec and standards Reviewers itself after the first done submit, so a Worker
// never spawns reviewers of its own.
const workerReviewNote = "Do not spawn reviewers: after you submit done, the Daemon starts the spec and standards reviewers itself and sends you every recorded finding, including minors, marked blocking or non-blocking. When you receive findings, answer every blocker/major in your next done submit using dispositions with the exact findingId and disposition; declined also needs reason, deferred also needs ticket (the Ticket number). Minor dispositions are optional. Spawn scouts only for read-only research."

// webStallNudge is the fixed nudge for a Worker that went idle with no submit.
const webStallNudge = "Nudge: you have not submitted. Finish the ticket and submit done with the completed checkboxes; the reviewers start after your submit."

// reviewerTaskPrefix is the Reviewer task without its skill brief: the gate
// prompt the re-review path reuses. First prompts use it plus the injected
// Ticket, spec, AGENTS.md chain and the role's skills (runs_inject.go), so
// the brief travels once as the first skill entry.
func reviewerTaskPrefix(ticketID, spec, reviewerType, branch, tip string) string {
	axis := "correctness and this repo's standards"
	if reviewerType == reviewerSpecType {
		axis = "fit to the ticket and the spec"
	}
	return fmt.Sprintf("Review ticket %s for spec %s as %s: judge %s on branch %s at tip %s. Do not edit files; judge only. Report through the submit tool: verdict `pass` or `changes`, tip the full SHA you judged (this tip unless you re-read it), ran the commands you ran with their exit codes (for example `go vet ./...` with its exit code; a pass without ran is refused), and findings as `{file, line, severity, problem}` objects (severity blocker, major or minor). A `changes` needs at least one blocker or major; minors never block a `pass` but still reach the worker. Do not re-raise any declined finding unless its cited lines have changed since the finding was declined.", ticketID, spec, reviewerType, axis, branch, tip)
}

// declinedFindingsPrompt adds prior Worker declines to each later review. The
// citation and reason stay structured so the Reviewer can distinguish the
// declined report from new findings.
func declinedFindingsPrompt(findings []declinedReviewFinding) string {
	if len(findings) == 0 {
		return ""
	}
	raw, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return ""
	}
	return "Previously declined findings and the Worker's reasons:\n" + string(raw)
}

// reviewerPrompt asks one reviewer type to judge the Ticket's worker branch
// at the given tip and report through the submit tool. Reviewers judge only:
// they never edit the worktree. The verdict is a submit tool call, not free
// text: verdict `pass` or `changes`, tip the full SHA judged, ran the
// commands run with exit codes, and findings as `{file, line, severity,
// problem}` objects (severity blocker, major or minor). A `pass` may carry
// minors and needs `ran`; a `changes` needs at least one blocker or major.
// The Daemon records the verdict at its tip; a tip behind the branch tip
// records stale and does not count. Only the submit to this prompt counts.
// The legacy `VERDICT: pass|changes` free-text reply stays only as a fallback
// for a Backend without the tool. It carries no memory/DOX text of its own:
// the Reviewer's system prompt already carries the reviewer extra
// (rolePromptFor), so repeating it here would inject it twice. It names no
// remember instruction.
func reviewerPrompt(ticketID, spec, reviewerType, branch, tip string) string {
	return reviewerPromptWithDeclined(ticketID, spec, reviewerType, branch, tip, nil)
}

func reviewerPromptWithDeclined(ticketID, spec, reviewerType, branch, tip string, declined []declinedReviewFinding) string {
	brief := standardsBrief
	if reviewerType == reviewerSpecType {
		brief = specBrief
	}
	prefix := reviewerTaskPrefix(ticketID, spec, reviewerType, branch, tip)
	if feedback := declinedFindingsPrompt(declined); feedback != "" {
		prefix += "\n\n" + feedback
	}
	return prefix + "\n\n" + brief
}

// standardsBrief and specBrief carry the two axes of the code-review skill
// into the Reviewer prompt: Reviewer Sessions load no skills, so the brief
// travels with the prompt. The axes stay separate on purpose: code that
// follows every standard can still miss the ticket, and the reverse.
const standardsBrief = `Standards axis. Read the repo's documented standards first (AGENTS.md files on the route to each changed path, CLAUDE.md, CONTRIBUTING.md, CODING_STANDARDS.md). Report (a) each place the diff breaks a documented standard, citing the file and rule, and (b) bugs, missed edge cases, callers the change broke and new logic with no test. DOX check: report a diff that changes a documented contract or layout without updating the owning AGENTS.md, or leaves a Child DOX Index stale. Smell baseline, always a judgement call and always overridden by a documented repo standard: Mysterious Name, Duplicated Code, Feature Envy, Data Clumps, Primitive Obsession, Repeated Switches, Shotgun Surgery, Divergent Change, Speculative Generality, Message Chains, Middle Man, Refused Bequest. Name a smell as "possible <smell>" and quote the hunk. Skip anything tooling already enforces. Whether the change matches the ticket is the spec reviewer's job, not yours.`

const specBrief = `Spec axis. Read the ticket file and the spec. Report (a) requirements the ticket or spec asked for that are missing or only partly done, (b) behaviour in the diff nobody asked for (scope creep), and (c) requirements that look done but are implemented wrong. Quote the ticket or spec line for each finding. Check every acceptance checkbox against the diff, not against the worker's claim. The spec's hard limits bind the worker: never ask for an edit they forbid. Code style and repo standards are the standards reviewer's job, not yours.`

// reviewerFormatReprompt is the single retry when a reviewer's reply does
// not parse: it names the exact format once more. A second failure becomes
// needs-you for the ticket.
func reviewerFormatReprompt() string {
	return "RESPOND IN FORMAT: one own line `VERDICT: pass` or `VERDICT: changes`, then one finding per line as `file:line severity: problem` (severity blocker, major or minor)."
}

// reviewerOf names the Run, Ticket and reviewer type of a Reviewer Session.
// ok is false when the Session is no reviewer's.
func (d *Daemon) reviewerOf(id string) (runID, ticketID, reviewerType string, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	runID, ticketID, reviewerType = d.findReviewerLocked(id)
	return runID, ticketID, reviewerType, reviewerType != ""
}

// reviewerToolSubmitFromEvents finds one reviewer `submit` tool payload in the
// turn's Events: the latest submit Event after the last prompt whose text
// parses as a ReviewerSubmit with a verdict. The fake Backend scripts tool
// submits this way (a submit Event carrying the payload JSON); real tool
// calls arrive through the submit route and already recorded the verdict, so
// this only fires for the fake seam. False when no tool payload is present.
func reviewerToolSubmitFromEvents(events []Event) (ReviewerSubmit, bool) {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "prompt" {
			start = i + 1
			break
		}
	}
	for i := len(events) - 1; i >= start; i-- {
		if events[i].Kind != "submit" {
			continue
		}
		var sub ReviewerSubmit
		if err := json.Unmarshal([]byte(events[i].Text), &sub); err != nil {
			continue
		}
		if strings.TrimSpace(sub.Verdict) == "" {
			continue
		}
		return sub, true
	}
	return ReviewerSubmit{}, false
}

// removeReviewPendingTip drops one tip from the Ticket's owed prompt queue
// for the reviewer type: a tool verdict answers its own tip explicitly, so
// the matching owed prompt is satisfied. Callers hold d.mu.
func removeReviewPendingTip(rs *runState, ticketID, reviewerType, tip string) {
	q := rs.ReviewPending[ticketID][reviewerType]
	if len(q) == 0 || tip == "" {
		return
	}
	kept := make([]string, 0, len(q))
	removed := false
	for _, t := range q {
		if !removed && t == tip {
			removed = true
			continue
		}
		kept = append(kept, t)
	}
	if removed {
		if len(kept) == 0 {
			delete(rs.ReviewPending[ticketID], reviewerType)
		} else {
			rs.ReviewPending[ticketID][reviewerType] = kept
		}
	}
}

// storeReviewerVerdictLocked records one reviewer verdict at its tip: the
// Reviews entry, the round that prompted the tip, and the owed-prompt queue
// cleanup. patchID is the verdict tip's diff patch id against the base,
// computed by the caller without the lock. It returns the re-prompt text when
// the verdict is already stale at birth and the reviewer must judge the
// current tip, else "". Callers hold d.mu. The verdict is always recorded,
// even stale: staleness only decides whether it counts toward the gate.
func storeReviewerVerdictLocked(rs *runState, ticketID, reviewerType string, sub ReviewerSubmit, cur, patchID string, now time.Time) string {
	verdict := strings.ToLower(strings.TrimSpace(sub.Verdict))
	tip := strings.TrimSpace(sub.Tip)
	if rs.Reviews == nil {
		rs.Reviews = map[string]map[string]storedReview{}
	}
	if rs.Reviews[ticketID] == nil {
		rs.Reviews[ticketID] = map[string]storedReview{}
	}
	report := reviewReport(sub.Findings)
	rs.Reviews[ticketID][reviewerType] = storedReview{
		Verdict:  verdict,
		At:       tip,
		Report:   report,
		Findings: append([]ReviewFinding(nil), sub.Findings...),
		Ran:      append([]ReviewRan(nil), sub.Ran...),
		PatchID:  patchID,
	}
	if rounds := rs.Rounds[ticketID]; len(rounds) > 0 {
		idx := -1
		for i := len(rounds) - 1; i >= 0; i-- {
			if rounds[i].Tip == tip {
				idx = i
				break
			}
		}
		if idx >= 0 {
			if rs.Rounds[ticketID][idx].Reviews == nil {
				rs.Rounds[ticketID][idx].Reviews = map[string]roundReview{}
			}
			var dur int64
			if st := rs.ReviewStarted[ticketID][reviewerType]; !st.IsZero() {
				dur = now.Sub(st).Milliseconds()
				if dur < 0 {
					dur = 0
				}
			}
			rs.Rounds[ticketID][idx].Reviews[reviewerType] = roundReview{Verdict: verdict, Findings: append([]ReviewFinding(nil), sub.Findings...), At: tip, DurationMs: dur}
			closeRoundIfCompleteLocked(rs, ticketID, now)
		}
	}
	removeReviewPendingTip(rs, ticketID, reviewerType, tip)
	if tip != "" && cur != "" && tip != cur && rs.ReviewTips[ticketID][reviewerType] != cur {
		if rs.ReviewTips == nil {
			rs.ReviewTips = map[string]map[string]string{}
		}
		if rs.ReviewTips[ticketID] == nil {
			rs.ReviewTips[ticketID] = map[string]string{}
		}
		rs.ReviewTips[ticketID][reviewerType] = cur
		if rs.ReviewStarted == nil {
			rs.ReviewStarted = map[string]map[string]time.Time{}
		}
		if rs.ReviewStarted[ticketID] == nil {
			rs.ReviewStarted[ticketID] = map[string]time.Time{}
		}
		rs.ReviewStarted[ticketID][reviewerType] = now
		appendReviewPending(rs, ticketID, reviewerType, cur)
		return reviewerPromptWithDeclined(ticketID, rs.Spec, reviewerType, rs.Branches[ticketID], cur, declinedFindingsForRounds(rs.Rounds[ticketID]))
	}
	return ""
}

// markReviewSeenLocked marks the turn's last prompt consumed for the
// Ticket's reviewer type, so the turn never records twice. Callers hold
// d.mu. A zero lastPrompt (no prompt Event yet) marks nothing.
func markReviewSeenLocked(rs *runState, ticketID, reviewerType string, lastPrompt int64) {
	if rs.ReviewSeen == nil {
		rs.ReviewSeen = map[string]map[string]int64{}
	}
	if rs.ReviewSeen[ticketID] == nil {
		rs.ReviewSeen[ticketID] = map[string]int64{}
	}
	if lastPrompt != 0 {
		rs.ReviewSeen[ticketID][reviewerType] = lastPrompt
	}
}

// recordReviewerToolVerdictLocked stores one reviewer submit-tool verdict and
// marks the turn seen: the Reviews entry via storeReviewerVerdictLocked, the
// ReviewSeen mark, and the submits bookkeeping (count, verdict, report).
// Both the fake-seam turn end and the submit route record through it, so the
// two paths cannot drift: a new submit Event is appended only by the route,
// whose tool call arrived over HTTP rather than the Event log. Callers hold
// d.mu. It returns the stale-at-birth retell, "" when fresh.
func recordReviewerToolVerdictLocked(d *Daemon, rs *runState, id, ticketID, reviewerType string, sub ReviewerSubmit, cur, patchID string, now time.Time, lastPrompt int64) string {
	retell := storeReviewerVerdictLocked(rs, ticketID, reviewerType, sub, cur, patchID, now)
	markReviewSeenLocked(rs, ticketID, reviewerType, lastPrompt)
	rec := d.submits[id]
	if rec == nil {
		rec = &submitRec{}
		d.submits[id] = rec
	}
	rec.Count++
	rec.Status = strings.ToLower(strings.TrimSpace(sub.Verdict))
	rec.Report = reviewReport(sub.Findings)
	return retell
}

// handleReviewerTurnEnd records one reviewer turn's verdict. The submit tool
// is primary: a submit Event carrying a ReviewerSubmit payload (the fake
// Backend's scripted tool call) records at its own tip. The legacy
// `VERDICT: pass|changes` free-text reply stays only as a fallback for a
// Backend without the tool: only the reply to the gate prompt counts, the
// tail after the last prompt. A parseable fallback reply lands at the
// prompted tip (the oldest owed prompt, so a mid-review worker commit reads
// stale, never fresh). An unparseable reply re-prompts the same reviewer
// once for the format; a second failure marks the ticket needs-you.
func (d *Daemon) handleReviewerTurnEnd(id string) {
	d.mu.Lock()
	runID, ticketID, typ := d.findReviewerLocked(id)
	if typ == "" {
		d.mu.Unlock()
		return
	}
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return
	}
	if rs.Status == runStatusPaused || rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		return
	}
	sess, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return
	}
	events := append([]Event(nil), sess.events...)
	repo, branch, spec := rs.Repo, rs.Branches[ticketID], rs.Spec
	lastPrompt := int64(0)
	for _, e := range events {
		if e.Kind == "prompt" && e.Seq > lastPrompt {
			lastPrompt = e.Seq
		}
	}
	if seen := rs.ReviewSeen[ticketID][typ]; seen == lastPrompt && lastPrompt != 0 {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	// The submit tool path first: a scripted tool payload in this turn's
	// submit Events records at its own tip with the tool verdict rules. An
	// invalid tool payload never falls through to the free-text fallback in
	// the same turn (the turn already submitted): the first failure re-prompts
	// with the tool error, the second marks the ticket needs-you, mirroring
	// the fallback format path below.
	if sub, ok := reviewerToolSubmitFromEvents(events); ok {
		if err := ValidateReviewerSubmit(sub); err != nil {
			d.mu.Lock()
			rs, ok := d.runs[runID]
			if !ok {
				d.mu.Unlock()
				return
			}
			if rs.FormatReprompt == nil {
				rs.FormatReprompt = map[string]map[string]bool{}
			}
			if rs.FormatReprompt[ticketID] == nil {
				rs.FormatReprompt[ticketID] = map[string]bool{}
			}
			if !rs.FormatReprompt[ticketID][typ] {
				rs.FormatReprompt[ticketID][typ] = true
				markReviewSeenLocked(rs, ticketID, typ, lastPrompt)
				d.persistRun(rs)
				d.broadcastRunLocked(rs)
				d.mu.Unlock()
				_ = d.runTellSession(id, "submit refused: "+err.Error()+". Call submit again with verdict pass|changes, tip the full SHA you judged, ran the commands you ran with their exit codes, and findings as file/line/severity/problem objects (a pass needs ran, a changes needs a blocker or major).")
				return
			}
			if rs.NeedsYouReason == nil {
				rs.NeedsYouReason = map[string]string{}
			}
			reason := "reviewer " + typ + " needs you: invalid submit tool payload after a re-prompt (" + err.Error() + "); steer the worker or retry the ticket"
			rs.NeedsYouReason[ticketID] = reason
			markReviewSeenLocked(rs, ticketID, typ, lastPrompt)
			push := d.noteNeedsYouPushLocked(rs, ticketID, reason)
			num, title := ticketNumTitleLocked(rs, ticketID)
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
			d.mu.Unlock()
			if push {
				d.pushRunNeedsYou(runID, ticketID, num, title, reason)
			}
			return
		}
		cur, _ := runGit(repo, "rev-parse", branch)
		patchID := d.reviewPatchIDFor(runID, ticketID, strings.TrimSpace(sub.Tip))
		now := time.Now().UTC()
		d.mu.Lock()
		rs, ok := d.runs[runID]
		if !ok {
			d.mu.Unlock()
			return
		}
		retell := recordReviewerToolVerdictLocked(d, rs, id, ticketID, typ, sub, cur, patchID, now, lastPrompt)
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
		d.mu.Unlock()
		if retell != "" {
			_ = d.runTellSession(id, retell)
		}
		return
	}

	reply := reviewerReplyText(events)
	parsed := ParseReview(reply)
	now := time.Now().UTC()
	if !parsed.OK {
		d.mu.Lock()
		rs, ok := d.runs[runID]
		if !ok {
			d.mu.Unlock()
			return
		}
		if rs.FormatReprompt == nil {
			rs.FormatReprompt = map[string]map[string]bool{}
		}
		if rs.FormatReprompt[ticketID] == nil {
			rs.FormatReprompt[ticketID] = map[string]bool{}
		}
		if !rs.FormatReprompt[ticketID][typ] {
			rs.FormatReprompt[ticketID][typ] = true
			if rs.ReviewSeen == nil {
				rs.ReviewSeen = map[string]map[string]int64{}
			}
			if rs.ReviewSeen[ticketID] == nil {
				rs.ReviewSeen[ticketID] = map[string]int64{}
			}
			rs.ReviewSeen[ticketID][typ] = lastPrompt
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
			d.mu.Unlock()
			_ = d.runTellSession(id, reviewerFormatReprompt())
			return
		}
		if rs.NeedsYouReason == nil {
			rs.NeedsYouReason = map[string]string{}
		}
		reason := "reviewer " + typ + " needs you: could not parse its verdict after a re-prompt; steer the worker or retry the ticket"
		rs.NeedsYouReason[ticketID] = reason
		if rs.ReviewSeen == nil {
			rs.ReviewSeen = map[string]map[string]int64{}
		}
		if rs.ReviewSeen[ticketID] == nil {
			rs.ReviewSeen[ticketID] = map[string]int64{}
		}
		rs.ReviewSeen[ticketID][typ] = lastPrompt
		push := d.noteNeedsYouPushLocked(rs, ticketID, reason)
		num, title := ticketNumTitleLocked(rs, ticketID)
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
		d.mu.Unlock()
		if push {
			d.pushRunNeedsYou(runID, ticketID, num, title, reason)
		}
		return
	}

	cur, _ := runGit(repo, "rev-parse", branch)
	d.mu.Lock()
	rs, ok = d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return
	}
	if rs.Reviews == nil {
		rs.Reviews = map[string]map[string]storedReview{}
	}
	if rs.Reviews[ticketID] == nil {
		rs.Reviews[ticketID] = map[string]storedReview{}
	}
	// A verdict answers the oldest unanswered prompt, not the latest
	// prompted tip: a Worker commit between prompts never lets a T1
	// judgement read fresh at T2. Runs predating the queue fall back to
	// the latest prompted tip, then the branch tip.
	tip := popReviewPending(rs, ticketID, typ)
	if tip == "" {
		tip = rs.ReviewTips[ticketID][typ]
	}
	if tip == "" {
		tip = cur
	}
	report := reviewReport(parsed.Findings)
	// The patch id anchors the short-circuit: a rebase with an identical
	// diff reuses this verdict. The base reads from the locked run state;
	// the git calls never touch d.mu, so holding it here only stalls.
	patchID := ""
	base := rs.Bases[ticketID]
	if target := rs.Targets[ticketID]; target != "" {
		if mb, merr := runGit(repo, "merge-base", target, tip); merr == nil && mb != "" {
			base = mb
		}
	}
	if base != "" && base != tip {
		patchID = gitPatchID(repo, base, tip)
	}
	rs.Reviews[ticketID][typ] = storedReview{Verdict: parsed.Verdict, At: tip, Report: report, Findings: append([]ReviewFinding(nil), parsed.Findings...), PatchID: patchID}
	// Fill the round that prompted this tip.
	if rounds := rs.Rounds[ticketID]; len(rounds) > 0 {
		idx := -1
		for i := len(rounds) - 1; i >= 0; i-- {
			if rounds[i].Tip == tip {
				idx = i
				break
			}
		}
		if idx >= 0 {
			if rs.Rounds[ticketID][idx].Reviews == nil {
				rs.Rounds[ticketID][idx].Reviews = map[string]roundReview{}
			}
			var dur int64
			if st := rs.ReviewStarted[ticketID][typ]; !st.IsZero() {
				dur = now.Sub(st).Milliseconds()
				if dur < 0 {
					dur = 0
				}
			}
			rs.Rounds[ticketID][idx].Reviews[typ] = roundReview{Verdict: parsed.Verdict, Findings: append([]ReviewFinding(nil), parsed.Findings...), At: tip, DurationMs: dur}
			closeRoundIfCompleteLocked(rs, ticketID, now)
		}
	}
	// A verdict already stale at birth means the tip moved under review:
	// re-prompt this reviewer once at the current tip, so the stale verdict
	// never reads fresh later. When the re-prompt for the current tip is
	// already out (queued by a later done), the verdict stays stale with no
	// extra Tell.
	retell := ""
	if tip != "" && cur != "" && tip != cur && rs.ReviewTips[ticketID][typ] != cur {
		if rs.ReviewTips == nil {
			rs.ReviewTips = map[string]map[string]string{}
		}
		if rs.ReviewTips[ticketID] == nil {
			rs.ReviewTips[ticketID] = map[string]string{}
		}
		rs.ReviewTips[ticketID][typ] = cur
		if rs.ReviewStarted == nil {
			rs.ReviewStarted = map[string]map[string]time.Time{}
		}
		if rs.ReviewStarted[ticketID] == nil {
			rs.ReviewStarted[ticketID] = map[string]time.Time{}
		}
		rs.ReviewStarted[ticketID][typ] = now
		appendReviewPending(rs, ticketID, typ, cur)
		retell = reviewerPromptWithDeclined(ticketID, spec, typ, branch, cur, declinedFindingsForRounds(rs.Rounds[ticketID]))
	}
	if rs.ReviewSeen == nil {
		rs.ReviewSeen = map[string]map[string]int64{}
	}
	if rs.ReviewSeen[ticketID] == nil {
		rs.ReviewSeen[ticketID] = map[string]int64{}
	}
	rs.ReviewSeen[ticketID][typ] = lastPrompt
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	if retell != "" {
		_ = d.runTellSession(id, retell)
	}
}

// appendReviewPending queues one owed judgement at tip for the Ticket's
// reviewer type. Consecutive duplicates collapse: re-prompting the same tip
// twice still owes one judgement. Callers hold d.mu.
func appendReviewPending(rs *runState, ticketID, reviewerType, tip string) {
	if tip == "" {
		return
	}
	if rs.ReviewPending == nil {
		rs.ReviewPending = map[string]map[string][]string{}
	}
	if rs.ReviewPending[ticketID] == nil {
		rs.ReviewPending[ticketID] = map[string][]string{}
	}
	q := rs.ReviewPending[ticketID][reviewerType]
	if len(q) > 0 && q[len(q)-1] == tip {
		return
	}
	rs.ReviewPending[ticketID][reviewerType] = append(q, tip)
}

// popReviewPending takes the oldest unanswered prompt for the Ticket's
// reviewer type: the tip of the prompt the next verdict answers. Empty when
// the reviewer owes nothing (runs predating the queue fall back to the
// latest prompted tip). Callers hold d.mu.
func popReviewPending(rs *runState, ticketID, reviewerType string) string {
	q := rs.ReviewPending[ticketID][reviewerType]
	if len(q) == 0 {
		return ""
	}
	tip := q[0]
	rs.ReviewPending[ticketID][reviewerType] = append([]string(nil), q[1:]...)
	return tip
}

// appendRoundLocked starts the ticket's next review round at tip, closing
// the previous open round. Passes at the same tip carry as held entries
// (heldFromRound) instead of a rerun; their types need no new prompt. reuse
// names the types whose verdict the driver reuses without a Reviewer turn
// (same tip, or a patch-identical rebase): they carry as held entries from
// the previous round whatever their verdict, and need no new prompt even
// when the previous round holds no entry for them. Callers hold d.mu. It
// returns the new round number and the types still needing a prompt.
func appendRoundLocked(rs *runState, ticketID, tip string, now time.Time, reuse map[string]bool) (int, map[string]bool) {
	if rs.Rounds == nil {
		rs.Rounds = map[string][]ticketRound{}
	}
	rounds := rs.Rounds[ticketID]
	if len(rounds) > 0 && rounds[len(rounds)-1].EndedAt == nil {
		end := now
		rounds[len(rounds)-1].EndedAt = &end
	}
	n := len(rounds) + 1
	rd := ticketRound{N: n, Tip: tip, StartedAt: now, Reviews: map[string]roundReview{}}
	need := map[string]bool{}
	if len(rounds) > 0 {
		prev := rounds[len(rounds)-1]
		if prev.Tip == tip {
			for typ, rv := range prev.Reviews {
				if rv.Verdict == "pass" {
					held := prev.N
					rd.Reviews[typ] = roundReview{Verdict: "pass", Findings: append([]ReviewFinding(nil), rv.Findings...), HeldFromRound: &held, At: tip}
				}
			}
		}
		for typ := range reuse {
			if _, ok := rd.Reviews[typ]; ok {
				continue
			}
			if prevRv, ok := prev.Reviews[typ]; ok && prevRv.Verdict != "" {
				held := prev.N
				rd.Reviews[typ] = roundReview{Verdict: prevRv.Verdict, Findings: append([]ReviewFinding(nil), prevRv.Findings...), HeldFromRound: &held, At: tip}
			}
		}
	}
	for _, typ := range requiredReviewerTypes {
		if reuse[typ] {
			continue
		}
		if _, ok := rd.Reviews[typ]; !ok {
			need[typ] = true
		}
	}
	// A round no type needs a prompt for is complete at birth: every
	// verdict is reused, so no judgement is in flight and the finish may
	// tell a reused `changes` instead of waiting under review.
	if len(need) == 0 {
		rd.EndedAt = &now
	}
	// A new round is a new attempt: types needing a prompt get their one
	// format re-prompt back.
	for typ := range need {
		if rs.FormatReprompt != nil && rs.FormatReprompt[ticketID] != nil {
			delete(rs.FormatReprompt[ticketID], typ)
		}
	}
	rs.Rounds[ticketID] = append(rounds, rd)
	return n, need
}

// closeRoundIfCompleteLocked ends the ticket's last round when every
// required type holds a verdict in it. Callers hold d.mu.
func closeRoundIfCompleteLocked(rs *runState, ticketID string, now time.Time) {
	rounds := rs.Rounds[ticketID]
	if len(rounds) == 0 {
		return
	}
	last := &rounds[len(rounds)-1]
	if last.EndedAt != nil {
		rs.Rounds[ticketID] = rounds
		return
	}
	for _, typ := range requiredReviewerTypes {
		if _, ok := last.Reviews[typ]; !ok {
			rs.Rounds[ticketID] = rounds
			return
		}
	}
	last.EndedAt = &now
	rs.Rounds[ticketID] = rounds
}

// ensureReviewers spawns the Ticket's two persistent Reviewer Sessions once
// its Worker submitted done, and re-prompts them when the Worker submits
// done again, so they keep their context across Passes: a re-review is a new
// message in the same Session, never a new Session. Each prompt queues its
// tip as owed; a verdict answers the oldest queued tip, so a Worker commit
// between prompts never lets a T1 judgement read fresh at T2. A done at the
// already-judged tip, or at a new tip whose diff patch-id against the base
// is unchanged (a rebase), reuses the verdicts with no Reviewer turn: a
// patch-identical verdict is carried to the new tip so the gate reads fresh.
// A Reviewer turn that ended without a recorded verdict at the current tip
// counts as no verdict: the Ticket stays under review and never merges on
// it. It spawns nothing before the first done submit. A refused
// live-Session cap leaves the missing reviewers for the next pass, which
// retries, as does a branch tip it cannot read. Reviewer starts never fail
// the Run: the driver keeps polling and the next pass tries again. The
// submit route runs this sync (bounded to two starts plus two steers) so
// prompts queue in submit order and the driver's next finish sees the
// re-prompt already out.
func (d *Daemon) ensureReviewers(runID, ticketID string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return
	}
	// A paused Run spawns no Reviewers: the driver is frozen and the
	// Resume's driver re-runs this for every done submit it finds.
	if rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		return
	}
	if rs.Status == runStatusPaused {
		d.mu.Unlock()
		return
	}
	if rs.ChainExhausted[ticketID] {
		d.mu.Unlock()
		return
	}
	rec := d.submits[rs.Sessions[ticketID]]
	if rec == nil || rec.Status != "done" {
		d.mu.Unlock()
		return
	}
	repo, branch := rs.Repo, rs.Branches[ticketID]
	worktree := rs.Worktrees[ticketID]
	spec := rs.Spec
	if branch == "" || worktree == "" {
		d.mu.Unlock()
		return
	}
	missing := []string{}
	existing := map[string]string{}
	for _, typ := range requiredReviewerTypes {
		if sess := rs.Reviewers[ticketID][typ]; sess == "" {
			missing = append(missing, typ)
		} else {
			existing[typ] = sess
		}
	}
	count := rec.Count
	if len(missing) == 0 && rs.Reprompted[ticketID] >= count {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	tip, err := runGit(repo, "rev-parse", branch)
	if err != nil || tip == "" {
		return
	}
	// The short-circuit patch id: a rebase that kept the diff identical
	// reuses both verdicts with no Reviewer turn.
	curPatch := d.reviewPatchIDFor(runID, ticketID, tip)
	for _, typ := range missing {
		if err := d.runStartReviewer(runID, ticketID, typ, tip); err != nil {
			return
		}
	}
	d.mu.Lock()
	rs, ok = d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return
	}
	if rs.Reprompted[ticketID] >= count {
		d.mu.Unlock()
		return
	}
	rs.Reprompted[ticketID] = count
	// Reuse the verdicts that already judged this code: the same tip, or a
	// new tip with an unchanged diff patch-id. A patch-identical verdict is
	// carried to the new tip so the gate reads fresh; the round still
	// records the carry as held, and no prompt goes out for these types.
	reuse := map[string]bool{}
	for _, typ := range requiredReviewerTypes {
		r, ok := rs.Reviews[ticketID][typ]
		if !ok || r.Verdict == "" {
			continue
		}
		switch {
		case r.At == tip:
			reuse[typ] = true
		case r.PatchID != "" && curPatch != "" && r.PatchID == curPatch:
			reuse[typ] = true
		}
	}
	for typ := range reuse {
		if r := rs.Reviews[ticketID][typ]; r.At != tip {
			r.At = tip
			rs.Reviews[ticketID][typ] = r
		}
	}
	now := time.Now().UTC()
	if rs.ReviewTips == nil {
		rs.ReviewTips = map[string]map[string]string{}
	}
	if rs.ReviewTips[ticketID] == nil {
		rs.ReviewTips[ticketID] = map[string]string{}
	}
	if rs.ReviewStarted == nil {
		rs.ReviewStarted = map[string]map[string]time.Time{}
	}
	if rs.ReviewStarted[ticketID] == nil {
		rs.ReviewStarted[ticketID] = map[string]time.Time{}
	}
	_, need := appendRoundLocked(rs, ticketID, tip, now, reuse)
	for _, typ := range requiredReviewerTypes {
		if !need[typ] {
			continue
		}
		rs.ReviewTips[ticketID][typ] = tip
		rs.ReviewStarted[ticketID][typ] = now
		appendReviewPending(rs, ticketID, typ, tip)
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	declined := declinedFindingsForRounds(rs.Rounds[ticketID])
	d.mu.Unlock()
	for _, typ := range requiredReviewerTypes {
		if !need[typ] {
			continue
		}
		if sess := existing[typ]; sess != "" {
			_ = d.runTellSession(sess, reviewerPromptWithDeclined(ticketID, spec, typ, branch, tip, declined))
		}
	}
}

// runTellSession steers one Session by id, resuming it first when it was
// released. Best effort: a capped or ended Session keeps the caller moving.
func (d *Daemon) runTellSession(sessionID, text string) error {
	if code, msg := d.resumeReleased(sessionID); code != 0 {
		return fmt.Errorf("could not reach session: %s", msg)
	}
	if _, code := d.routeLive(sessionID, "steer", text); code != http.StatusAccepted {
		return fmt.Errorf("could not reach session")
	}
	return nil
}

// runStartReviewer creates one Ticket's Reviewer Session of the given type:
// a persistent Subagent of the Run in the Worker's worktree that judges the
// branch at the given tip. Reviewer roles choose its harness and model with
// the same fallback as Workers; the Session goes through the shared start,
// so it counts against the live-Session cap like every other Session.
func (d *Daemon) runStartReviewer(runID, ticketID, reviewerType, tip string) error {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("unknown run %q", runID)
	}
	if rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		return fmt.Errorf("ticket %s is paused", ticketID)
	}
	// A paused Run spawns no Reviewers: fail fast so the driver's next
	// pass retries after the Resume.
	if rs.Status == runStatusPaused {
		d.mu.Unlock()
		return fmt.Errorf("run is paused")
	}
	branch, worktree := rs.Branches[ticketID], rs.Worktrees[ticketID]
	spec := rs.Spec
	repo := rs.Repo
	ticketFile := ticketPath(rs, ticketID)
	base := ""
	if rs.Bases != nil {
		base = rs.Bases[ticketID]
	}
	owner := rs.Owner
	chain := append([]chainEntry(nil), runRoleChain(rs, ticketID, reviewerType)...)
	if len(chain) == 0 {
		chain = []chainEntry{{Backend: rs.Harness, Model: rs.Model, Effort: rs.Effort}}
	}
	chainPos := clampChainPosition(chainPosition(rs, ticketID, reviewerType), len(chain))
	d.mu.Unlock()
	if branch == "" || worktree == "" {
		return fmt.Errorf("ticket %s has no worker", ticketID)
	}
	var meta SessionMeta
	var be Backend
	for {
		entry := chain[chainPos]
		meta = SessionMeta{
			ID: newID(), Harness: entry.Backend, Model: entry.Model, Effort: entry.Effort,
			WorkDir: worktree, Status: "idle", CreatedAt: time.Now().UTC(),
			ParentID: runID, Owner: owner, Reviewer: reviewerType,
		}
		var serr *startError
		be, serr = d.startSession(meta)
		if serr == nil {
			break
		}
		if !chainAdvances(errors.New(serr.msg)) {
			return fmt.Errorf("could not start %s for %s: %s", reviewerType, ticketID, serr.msg)
		}
		if chainPos+1 >= len(chain) {
			d.mu.Lock()
			if rs := d.runs[runID]; rs != nil {
				setChainPosition(rs, ticketID, reviewerType, chainPos)
				d.persistRun(rs)
			}
			d.mu.Unlock()
			d.markRunChainNeedsYou(runID, ticketID, reviewerType, serr.msg, true)
			return fmt.Errorf("%s Chain exhausted", reviewerType)
		}
		chainPos++
	}
	d.mu.Lock()
	rs, ok = d.runs[runID]
	if !ok {
		d.mu.Unlock()
		if be != nil {
			be.CloseSession(meta.ID)
		}
		d.removeSession(meta.ID)
		return fmt.Errorf("unknown run %q", runID)
	}

	// A Pause landing after the entry check drops this attempt instead of
	// recording a Reviewer for a paused Run; the Resume's driver spawns it.
	if rs.Status == runStatusPaused {
		d.mu.Unlock()
		if be != nil {
			be.CloseSession(meta.ID)
		}
		d.removeSession(meta.ID)
		return fmt.Errorf("run is paused")
	}
	if rs.Reviewers == nil {
		rs.Reviewers = map[string]map[string]string{}
	}
	if rs.Reviewers[ticketID] == nil {
		rs.Reviewers[ticketID] = map[string]string{}
	}
	if rs.Reviewers[ticketID][reviewerType] != "" {
		// Another pass won the race and recorded its Session first: drop
		// this one so no orphaned Session eats the live-Session cap.
		d.mu.Unlock()
		if be != nil {
			be.CloseSession(meta.ID)
		}
		d.removeSession(meta.ID)
		return nil
	}
	setChainPosition(rs, ticketID, reviewerType, chainPos)
	if rs.ChainExhausted != nil {
		delete(rs.ChainExhausted, ticketID)
	}
	rs.Reviewers[ticketID][reviewerType] = meta.ID
	if rs.ReviewTips == nil {
		rs.ReviewTips = map[string]map[string]string{}
	}
	if rs.ReviewTips[ticketID] == nil {
		rs.ReviewTips[ticketID] = map[string]string{}
	}
	rs.ReviewTips[ticketID][reviewerType] = tip
	appendReviewPending(rs, ticketID, reviewerType, tip)
	now := time.Now().UTC()
	if rs.ReviewStarted == nil {
		rs.ReviewStarted = map[string]map[string]time.Time{}
	}
	if rs.ReviewStarted[ticketID] == nil {
		rs.ReviewStarted[ticketID] = map[string]time.Time{}
	}
	rs.ReviewStarted[ticketID][reviewerType] = now
	// Rounds open in ensureReviewers per worker done submit, not here: a
	// start alone never begins a round, so the spawn and the prompt share
	// the one round the done submit opened.
	declined := declinedFindingsForRounds(rs.Rounds[ticketID])
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.persistResumeToken(be, meta.ID)
	d.appendEvent(meta.ID, "status", "", "idle")
	prompt := buildReviewerFirstPromptWithDeclined(ticketID, spec, ticketFile, reviewerType, branch, tip, worktree, repo, base, declined)
	d.mu.Lock()
	if s, ok := d.sessions[meta.ID]; ok {
		s.busy = true
		s.active++
	}
	d.mu.Unlock()
	d.appendPromptEvent(meta.ID, prompt, "", 0, nil, systemSender)
	go d.runTurn(meta.ID, be, prompt, nil)
	return nil
}

// worktreeGitIdentity is the per-worktree git identity every Ticket worktree
// carries, so Worker commits and the finish rebase never abort on a
// missing user.name/user.email (ticket 03). It is scoped with
// `git config --worktree`, never the shared repo config, so the User's own
// identity in the main checkout is untouched.
const (
	worktreeGitUserName  = "emberloop"
	worktreeGitUserEmail = "emberloop@localhost"
)

// ensureWorktreeConfigExtension enables extensions.worktreeConfig once per
// repo, so per-worktree config (`git config --worktree`: the identity below
// and the spec excludes file) takes effect. Repeating it is a no-op, and it
// changes nothing else about how git reads the shared config.
func ensureWorktreeConfigExtension(repo string) error {
	if _, err := runGit(repo, "config", "extensions.worktreeConfig", "true"); err != nil {
		return err
	}
	return nil
}

// ensureWorktreeGitIdentity sets the worktree's own user.name and user.email
// via `git config --worktree`, which lands in the worktree's private
// config.worktree and leaves the shared repo config alone. Callers enable
// the worktreeConfig extension first: without it git refuses --worktree.
func ensureWorktreeGitIdentity(worktree string) error {
	if _, err := runGit(worktree, "config", "--worktree", "user.name", worktreeGitUserName); err != nil {
		return err
	}
	if _, err := runGit(worktree, "config", "--worktree", "user.email", worktreeGitUserEmail); err != nil {
		return err
	}
	return nil
}

// ensureWorktreeSpecDir mirrors the spec directory (the spec file parent,
// holding spec.md and issues/) from the main checkout into the worktree at
// the same repo-relative path, then strips write bits from the mirrored files
// so the Worker reads them read-only. Only files missing in the worktree are
// copied, so a committed spec stays clean for the finish dirty-worktree
// check; copied gap files (uncommitted new files a fresh worktree add missed)
// are listed in a per-worktree excludes file, so they read as ignored instead
// of untracked `??`, which would otherwise refuse every finish. Only the
// driver ticks boxes, and it ticks the main checkout ticket files
// (TickCheckboxes), never the mirror: the read-only bit keeps a Worker from
// ticking its own copy and diverging.
func ensureWorktreeSpecDir(repo, spec, worktree string) error {
	specDir := filepath.Dir(spec)
	rel, err := filepath.Rel(repo, specDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	if rel == "." {
		return mirrorSpecRootFiles(repo, spec, worktree)
	}
	dst := filepath.Join(worktree, rel)
	gaps, err := mirrorDirReadOnly(specDir, dst)
	if err != nil {
		return err
	}
	return excludeWorktreeGaps(repo, worktree, gaps)
}

// mirrorSpecRootFiles mirrors a repo-root spec without touching the rest of
// the worktree: the spec file plus the sibling issues/ directory when one
// exists. Copied gap files are excluded from git status like above.
func mirrorSpecRootFiles(repo, spec, worktree string) error {
	var gaps []string
	copied, err := mirrorFileReadOnly(spec, filepath.Join(worktree, filepath.Base(spec)))
	if err != nil {
		return err
	}
	if copied {
		gaps = append(gaps, filepath.Join(worktree, filepath.Base(spec)))
	}
	tickets := filepath.Join(repo, "issues")
	if st, err := os.Stat(tickets); err != nil || !st.IsDir() {
		return excludeWorktreeGaps(repo, worktree, gaps)
	}
	more, err := mirrorDirReadOnly(tickets, filepath.Join(worktree, "issues"))
	if err != nil {
		return err
	}
	return excludeWorktreeGaps(repo, worktree, append(gaps, more...))
}

// mirrorDirReadOnly fills files missing in dst from src, then strips write
// bits from the mirrored files. It returns the absolute paths it copied, so
// the caller can keep them out of git status. Existing worktree files are
// never overwritten, so a committed spec stays clean for the finish
// dirty-worktree check; only gaps (uncommitted new files a fresh worktree add
// missed) become readable instead of missing. The source tree is never
// chmodded.
func mirrorDirReadOnly(src, dst string) ([]string, error) {
	var gaps []string
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	walkErr := filepath.WalkDir(src, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !e.Type().IsRegular() {
			return nil
		}
		copied, err := copyFileIfMissing(path, target)
		if err != nil {
			return err
		}
		if copied {
			gaps = append(gaps, target)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if err := stripWriteBits(dst); err != nil {
		return nil, err
	}
	return gaps, nil
}

// copyFileIfMissing copies src to dst when dst does not exist yet, leaving
// an existing worktree file untouched so the mirror never dirties a
// checkout. It reports whether it copied. Parent directories are created as
// needed.
func copyFileIfMissing(src, dst string) (bool, error) {
	if _, err := os.Stat(dst); err == nil {
		return false, nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// mirrorFileReadOnly copies one file into place read-only, leaving an
// existing worktree file untouched so the mirror never dirties a checkout.
// It reports whether it copied.
func mirrorFileReadOnly(src, dst string) (bool, error) {
	copied, err := copyFileIfMissing(src, dst)
	if err != nil {
		return false, err
	}
	if err := stripWriteBits(dst); err != nil {
		return false, err
	}
	return copied, nil
}

// excludeWorktreeGaps lists mirrored gap files in a per-worktree excludes
// file, so `git status --porcelain` reads them as ignored instead of
// untracked `??` and the finish dirty-worktree check sees the Ticket work,
// not the spec input. A linked worktree ignores its own gitdir info/exclude,
// so this enables extensions.worktreeConfig once per repo and points the
// worktree's core.excludesFile at a list inside its own gitdir: the main
// checkout's ignore rules stay untouched, and removing the worktree removes
// the list with it. Failures return an error so the start fails loudly
// instead of leaving a worktree that can never finish.
func excludeWorktreeGaps(repo, worktree string, absPaths []string) error {
	if len(absPaths) == 0 {
		return nil
	}
	rels := make([]string, 0, len(absPaths))
	for _, p := range absPaths {
		rel, err := filepath.Rel(worktree, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	if len(rels) == 0 {
		return nil
	}
	gitdir, err := runGit(worktree, "rev-parse", "--git-dir")
	if err != nil || gitdir == "" {
		return fmt.Errorf("could not read git dir in %s: %v", worktree, err)
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktree, gitdir)
	}
	if _, err := runGit(repo, "config", "extensions.worktreeConfig", "true"); err != nil {
		return fmt.Errorf("could not enable worktreeConfig in %s: %v", repo, err)
	}
	exclude := filepath.Join(gitdir, "info", "worktree-exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	raw, _ := os.ReadFile(exclude)
	have := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, rel := range rels {
		if !have[rel] {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		out := strings.TrimRight(string(raw), "\n")
		if out != "" {
			out += "\n"
		}
		out += strings.Join(missing, "\n") + "\n"
		if err := os.WriteFile(exclude, []byte(out), 0o644); err != nil {
			return err
		}
	}
	if _, err := runGit(worktree, "config", "--worktree", "core.excludesFile", exclude); err != nil {
		return fmt.Errorf("could not set excludesFile in %s: %v", worktree, err)
	}
	return nil
}

// stripWriteBits removes every write bit from files under root; directories
// stay writable so `git worktree remove --force` can still delete the mirror.
// Files keep their read bits, so the mirror stays readable but a Worker
// cannot overwrite (tick) its own copy. Git tracks only the executable bit,
// so a 644->444 demotion stays clean for the finish status --porcelain check.
func stripWriteBits(root string) error {
	return filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			// Traversable and deletable; the read-only guarantee lives on
			// the files, not their parents.
			if err := os.Chmod(path, 0o755); err != nil {
				return err
			}
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := os.Chmod(path, info.Mode().Perm()&^0o222); err != nil {
			return err
		}
		return nil
	})
}

// ensureWorktreeDOX seeds the DOX chain helper for runStartWorker. Tests
// replace it to simulate a seeding failure without touching the filesystem.
var ensureWorktreeDOX = ensureAgentsMD

// runStartWorker creates the Ticket's worktree and branch from the Run's
// spec branch and starts its Worker Session, a Subagent of the Run, running
// the Ticket prompt on it. A refused live-Session cap retries briefly
// before failing the Run.
func (d *Daemon) runStartWorker(runID string, tk el.DriverTicket) error {
	// A paused Run starts nothing new: wait for the Resume while holding
	// no lock, so the Resume itself never blocks on this worker. Running
	// Workers stay live and untouched meanwhile.
	d.pauseGate(runID)
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("unknown run %q", runID)
	}
	if rs.Status == runStatusAbandoned {
		d.mu.Unlock()
		return fmt.Errorf("run %s was abandoned", runID)
	}
	chain := append([]chainEntry(nil), workerRunChain(rs, tk.ID)...)
	if len(chain) == 0 {
		chain = []chainEntry{{Backend: rs.Harness, Model: rs.Model, Effort: rs.Effort}}
	}
	chainPos := clampChainPosition(chainPosition(rs, tk.ID, "worker"), len(chain))
	firstEntry := chain[chainPos]
	harness, model, effort := firstEntry.Backend, firstEntry.Model, firstEntry.Effort
	repo, specPath := rs.Repo, rs.Spec
	owner := rs.Owner
	specBranch := rs.SpecBranch
	// Run A/B arm (ticket 30): the ticket's index in ticket order decides,
	// so a retry, a pause/resume or a different start order never moves the
	// arm. Tickets and RecallOffN are fixed at Start, so one read holds for
	// every attempt below.
	noRecall := RecallOffForTicket(rs.Tickets, tk.ID, rs.RecallOffN)
	workerID := el.WorkerID(tk.Path)
	branch := "implement/" + workerID
	worktree := filepath.Join(repo, ".el-worktrees", workerID)
	d.mu.Unlock()

	cleanup := func() {
		_, _ = runGit(repo, "worktree", "remove", "--force", worktree)
		_, _ = runGit(repo, "branch", "-D", branch)
	}
	// A Pause or checkpoint conflict landing below never records a Worker: the
	// record-time check drops the attempt and waits for Resume, then starts from
	// a clean worktree.
	for {
		// Ticket branches fork from the Run's spec branch, not master: every
		// ticket in one spec sees the earlier merges. Runs predating the
		// spec branch (SpecBranch empty) keep the old HEAD fork.
		target, base := "", "HEAD"
		if specBranch != "" {
			target = specBranch
			sha, err := runGit(repo, "rev-parse", specBranch)
			if err != nil || sha == "" {
				return fmt.Errorf("could not read spec branch %s in %s: %v", specBranch, repo, err)
			}
			base = sha
		} else {
			if t, err := runGit(repo, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
				target = t
			}
			if sha, err := runGit(repo, "rev-parse", "HEAD"); err == nil {
				base = sha
			} else {
				return fmt.Errorf("could not read HEAD in %s: %w", repo, err)
			}
		}
		if _, err := runGit(repo, "worktree", "add", "-b", branch, worktree, base); err != nil {
			return err
		}
		// Ticket 03: every worktree carries its own git identity so Worker
		// commits and the finish rebase never abort on a missing
		// user.name/user.email, and a read-only mirror of the spec directory
		// so the Worker never hits a stat error on spec or ticket files. The
		// worktreeConfig extension scopes both to the worktree, leaving the
		// User's shared repo config untouched. A failure here stops this
		// worker like any other start error.
		if err := ensureWorktreeConfigExtension(repo); err != nil {
			cleanup()
			return fmt.Errorf("could not enable worktreeConfig in %s: %w", repo, err)
		}
		if err := ensureWorktreeGitIdentity(worktree); err != nil {
			cleanup()
			return fmt.Errorf("could not set git identity in %s: %w", worktree, err)
		}
		if err := ensureWorktreeSpecDir(repo, specPath, worktree); err != nil {
			cleanup()
			return fmt.Errorf("could not mirror spec directory into %s: %w", worktree, err)
		}
		// A Run-launched Worker reads its DOX chain from its own worktree
		// root. Seed a root AGENTS.md there when the checkout has none, so
		// the chain exists even when the repo root lacks one; the file rides
		// the worktree (never the main checkout, which stays clean). The task
		// tells a seeded Worker to commit it with its work: left untracked it
		// would trip the merge's dirty-worktree refusal on every finish. An
		// existing file is untouched and a failure stops this worker like any
		// other start error.
		seeded, err := ensureWorktreeDOX(worktree)
		if err != nil {
			cleanup()
			return fmt.Errorf("could not ensure AGENTS.md in %s: %w", worktree, err)
		}
		meta := SessionMeta{
			ID:        newID(),
			Harness:   harness,
			Model:     model,
			Effort:    effort,
			WorkDir:   worktree,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
			ParentID:  runID,
			Owner:     owner,
			RunWorker: true,
			NoRecall:  noRecall,
		}
		var be Backend
		var startExhausted bool
		for {
			entry := chain[chainPos]
			meta.ID = newID()
			meta.Harness, meta.Model, meta.Effort = entry.Backend, entry.Model, entry.Effort
			advanced := false
			for i := 0; ; i++ {
				var serr *startError
				be, serr = d.startSession(meta)
				if serr == nil {
					break
				}
				if chainAdvances(errors.New(serr.msg)) {
					if chainPos+1 < len(chain) {
						chainPos++
						advanced = true
						break
					}
					startExhausted = true
					break
				}
				if serr.code != http.StatusTooManyRequests || i >= 60 {
					cleanup()
					return fmt.Errorf("could not start worker for %s: %s", tk.ID, serr.msg)
				}
				// Freeze the cap wait while paused instead of burning retries.
				d.pauseGate(runID)
				time.Sleep(d.pollIntervalOrDefault())
			}
			if startExhausted {
				cleanup()
				d.mu.Lock()
				if rs := d.runs[runID]; rs != nil {
					setChainPosition(rs, tk.ID, "worker", chainPos)
					d.persistRun(rs)
				}
				d.mu.Unlock()
				d.markRunChainNeedsYou(runID, tk.ID, "worker", "provider Chain exhausted while starting the Worker", true)
				return nil
			}
			if advanced {
				continue
			}
			break
		}
		d.mu.Lock()
		rs, ok = d.runs[runID]
		if !ok {
			d.mu.Unlock()
			if be != nil {
				be.CloseSession(meta.ID)
			}
			d.removeSession(meta.ID)
			cleanup()
			return fmt.Errorf("unknown run %q", runID)
		}
		if rs.Status == runStatusAbandoned {
			d.mu.Unlock()
			if be != nil {
				be.CloseSession(meta.ID)
			}
			d.removeSession(meta.ID)
			cleanup()
			return fmt.Errorf("run %s was abandoned", runID)
		}
		if rs.TicketPaused[tk.ID] {
			d.mu.Unlock()
			if be != nil {
				be.CloseSession(meta.ID)
			}
			d.removeSession(meta.ID)
			cleanup()
			return nil
		}
		if rs.Status == runStatusPaused || rs.Status == runStatusNeedsYou {
			// Pause won the race after the gate: drop this attempt so no
			// Worker exists for a paused Run, then wait for the Resume
			// and start over.
			d.mu.Unlock()
			if be != nil {
				be.CloseSession(meta.ID)
			}
			d.removeSession(meta.ID)
			cleanup()
			d.pauseGate(runID)
			continue
		}
		if noRecall {
			if rs.RecallOff == nil {
				rs.RecallOff = map[string]bool{}
			}
			rs.RecallOff[tk.ID] = true
		}
		setChainPosition(rs, tk.ID, "worker", chainPos)
		if rs.ChainExhausted != nil {
			delete(rs.ChainExhausted, tk.ID)
		}
		rs.Sessions[tk.ID] = meta.ID
		rs.Branches[tk.ID] = branch
		rs.Worktrees[tk.ID] = worktree
		rs.Targets[tk.ID] = target
		rs.Bases[tk.ID] = base
		spec := rs.Spec
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
		d.mu.Unlock()

		d.persistResumeToken(be, meta.ID)
		d.appendEvent(meta.ID, "status", "", "idle")
		prompt := buildWorkerFirstPrompt(tk.ID, spec, tk.Path, branch, seeded, worktree)
		d.mu.Lock()
		if s, ok := d.sessions[meta.ID]; ok {
			s.busy = true
			s.active++
		}
		d.mu.Unlock()
		d.appendPromptEvent(meta.ID, prompt, "", 0, nil, systemSender)
		go d.runTurn(meta.ID, be, prompt, nil)
		return nil
	}
}

// runTellWorker steers the Ticket's Worker Session with the driver's text:
// a refused merge goes back to the Worker unchanged. from types the prompt:
// the driver sends system text; a User's steer from the Runs view sends user text.
func (d *Daemon) runTellWorker(runID, ticketID, text string, from promptSender) error {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("unknown run %q", runID)
	}
	if rs.Status == runStatusPaused || rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		return nil
	}
	sessionID := rs.Sessions[ticketID]
	d.mu.Unlock()
	if sessionID == "" {
		return fmt.Errorf("ticket %s has no worker", ticketID)
	}
	if code, msg := d.resumeReleased(sessionID); code != 0 {
		return fmt.Errorf("could not reach worker for %s: %s", ticketID, msg)
	}
	if _, code := d.routeLiveOpt(sessionID, "steer", text, nil, nil, true, from); code != http.StatusAccepted {
		return fmt.Errorf("could not reach worker for %s", ticketID)
	}
	// The tell carries the refusal the row shows: write the pass now, so
	// the file holds it even when the next pass never sleeps.
	d.snapshotAndPersist(runID)
	return nil
}

// runFinishWorker merges a submitted done through the reviewer gate into the
// Run's spec branch: without both Passes at the current tip, or with a
// dirty worktree or a rebase conflict, the Worker stays alive with the
// refusal. A Ticket with no verdict on the current tip is still under
// review, not refused:
// the finish reports nothing and the driver polls again. The gate judges
// the pre-rebase tip: the rebase replays the reviewed commits onto the spec
// branch without changing their content. On a merge the
// ticket is marked done and committed on the spec branch, and the worktree
// is removed; the Worker Session stays for its transcript. Master never
// moves here: the final merge into master is a later ticket. Runs predating
// the spec branch (no SpecBranch recorded) keep the old master merge.
func (d *Daemon) runFinishWorker(runID, ticketID string) (el.DriverFinish, error) {
	d.needsYouGate(runID)
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return el.DriverFinish{}, fmt.Errorf("unknown run %q", runID)
	}
	if rs.Status == runStatusPaused || rs.TicketPaused[ticketID] {
		d.mu.Unlock()
		return el.DriverFinish{}, nil
	}
	if rs.Status == runStatusAbandoned {
		d.mu.Unlock()
		return el.DriverFinish{}, fmt.Errorf("run %s was abandoned", runID)
	}
	if rs.Status == runStatusNeedsYou {
		d.mu.Unlock()
		return el.DriverFinish{}, nil
	}
	repo, branch, worktree, target, _, path := rs.Repo, rs.Branches[ticketID], rs.Worktrees[ticketID], rs.Targets[ticketID], rs.Bases[ticketID], ticketPath(rs, ticketID)
	specBranch := rs.SpecBranch
	submitStatus := ""
	if rec, ok := d.submits[rs.Sessions[ticketID]]; ok && rec != nil {
		submitStatus = rec.Status
	}
	workerID := el.WorkerID(ticketID)
	pendingChainSubmit := rs.ChainPendingSubmit[ticketID]
	d.mu.Unlock()
	if branch == "" || worktree == "" {
		return el.DriverFinish{}, fmt.Errorf("ticket %s has no worker", ticketID)
	}
	if pendingChainSubmit {
		// A failed Worker invalidates its old done submit until the replacement
		// Session submits done; preserve same-tip Reviewer verdicts meanwhile.
		return el.DriverFinish{}, nil
	}
	if submitStatus != "" && submitStatus != "done" {
		return el.DriverFinish{Merged: false, Detail: fmt.Sprintf("worker submitted %q, waiting for done; submit done with the completed checkboxes", submitStatus)}, nil
	}
	d.ensureReviewers(runID, ticketID)
	d.mu.Lock()
	rs, ok = d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return el.DriverFinish{}, fmt.Errorf("unknown run %q", runID)
	}
	if rs.Status == runStatusAbandoned {
		d.mu.Unlock()
		return el.DriverFinish{}, fmt.Errorf("run %s was abandoned", runID)
	}
	if rs.Status == runStatusNeedsYou {
		d.mu.Unlock()
		return el.DriverFinish{}, nil
	}
	verdicts := map[string]storedReview{}
	for typ, r := range rs.Reviews[ticketID] {
		verdicts[typ] = r
	}
	sessions := map[string]string{}
	for typ, sess := range rs.Reviewers[ticketID] {
		sessions[typ] = sess
	}
	d.mu.Unlock()
	tip, gerr := runGit(repo, "rev-parse", branch)
	if gerr != nil || tip == "" {
		return el.DriverFinish{}, fmt.Errorf("ticket %s has no tip; start its worker again", ticketID)
	}
	if !reviewGateReady(sessions, verdicts) {
		return el.DriverFinish{}, nil
	}
	if why := reviewGateDetail(verdicts, tip); why != "" {
		// No verdict judged the current tip: the Ticket is under review,
		// not refused, so there is no new refusal to tell. A fresh
		// changes verdict is still told unchanged below.
		if noneFreshAtTip(verdicts, tip) {
			return el.DriverFinish{}, nil
		}
		// A round still open at this tip means a reviewer has not judged it
		// yet. Telling a refusal now would park the driver until the
		// worker's next submit while the worker waits for the verdict:
		// nothing moves. Report under review; the verdict closes the round
		// and the next pass tells it.
		d.mu.Lock()
		open := false
		if rs, ok := d.runs[runID]; ok {
			open = roundOpenAtTip(rs.Rounds[ticketID], tip)
		}
		d.mu.Unlock()
		if open {
			return el.DriverFinish{}, nil
		}
		detail := why
		d.mu.Lock()
		if rs, ok := d.runs[runID]; ok {
			if rounds := rs.Rounds[ticketID]; len(rounds) > 0 {
				idx := -1
				for i := len(rounds) - 1; i >= 0; i-- {
					if rounds[i].Tip == tip {
						idx = i
						break
					}
				}
				if idx >= 0 {
					var declined []string
					for _, typ := range requiredReviewerTypes {
						if r, ok := verdicts[typ]; ok && r.Verdict != "pass" && r.At == tip {
							for _, f := range r.Findings {
								declined = append(declined, f.File+":"+fmt.Sprintf("%d", f.Line)+" "+f.Severity+": "+strings.ReplaceAll(f.Problem, "\n", " / "))
							}
						}
					}
					if len(declined) == 0 {
						declined = []string{why}
					}
					rounds[idx].Declined = declined
					rs.Rounds[ticketID] = rounds
				}
			}
			feedback, _ := recordWorkerFindingsSentLocked(rs, ticketID, tip)
			if feedback == "" {
				// A legacy Run can hold only the aggregate refusal, with no
				// structured round findings. Preserve its sent marker so the
				// ticket page and old Worker behavior remain consistent.
				for i := len(rs.Rounds[ticketID]) - 1; i >= 0; i-- {
					if rs.Rounds[ticketID][i].Tip == tip && rs.Rounds[ticketID][i].SentToWorkerAt == nil {
						now := time.Now().UTC()
						rs.Rounds[ticketID][i].SentToWorkerAt = &now
						break
					}
				}
			}
			if feedback != "" {
				detail += "\n\n" + feedback
			}
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
		}
		d.mu.Unlock()
		return el.DriverFinish{Merged: false, Detail: detail}, nil
	}
	// Passing verdicts can still carry minor findings. They do not hold the
	// merge, but are sent to the Worker and persisted on the review round.
	var workerSessionID, feedback string
	d.mu.Lock()
	if rs, ok := d.runs[runID]; ok {
		workerSessionID = rs.Sessions[ticketID]
		var newlySent bool
		feedback, newlySent = recordWorkerFindingsSentLocked(rs, ticketID, tip)
		if newlySent {
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
		}
		if !newlySent {
			feedback = ""
		}
	}
	d.mu.Unlock()
	if feedback != "" && workerSessionID != "" {
		// Minor findings are advisory: a failed notice cannot turn them into
		// a gate refusal or hold an otherwise passing Ticket.
		_ = d.runTellSession(workerSessionID, feedback)
	}
	// Ticket branches merge into the spec branch, never master: the rebase
	// lands on the spec branch as it stands at finish, since another Ticket
	// may have merged after this worker started. The main checkout never
	// moves, so two specs run at once. Runs predating the spec branch keep
	// the old master merge below.
	d.mu.Lock()
	current := d.runs[runID]
	paused := current == nil || current.Status == runStatusPaused || current.TicketPaused[ticketID]
	d.mu.Unlock()
	if paused {
		return el.DriverFinish{}, nil
	}
	if specBranch != "" {
		sha, kept, err := d.mergeTicketIntoSpec(repo, worktree, branch, specBranch, path, ticketID, workerID)
		if err != nil {
			return el.DriverFinish{}, err
		}
		if kept != "" {
			return el.DriverFinish{Merged: false, Detail: kept}, nil
		}
		if _, err := runGit(repo, "worktree", "remove", "--force", worktree); err != nil {
			return el.DriverFinish{}, err
		}
		d.mu.Lock()
		if rs, ok := d.runs[runID]; ok {
			rs.Merged[ticketID] = sha
			if rs.NeedsYouReason != nil {
				delete(rs.NeedsYouReason, ticketID)
			}
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
		}
		d.mu.Unlock()
		// Each Ticket merge re-checks the spec branch against the other live
		// spec branches in the same repo: an intersecting changed-file set
		// warns on both Runs that a final-merge conflict is coming.
		d.checkSpecOverlap(runID)
		return el.DriverFinish{Merged: true, SHA: sha}, nil
	}
	// A detached repo has no target branch: rebase onto the repo HEAD as it
	// stands at finish, since another Ticket may have moved it after this
	// worker started. The recorded base is too old then; it only proves the
	// worker started. A branch name target moves on its own.
	onto := target
	if onto == "" {
		sha, gerr := runGit(repo, "rev-parse", "HEAD")
		if gerr != nil || sha == "" {
			return el.DriverFinish{}, fmt.Errorf("ticket %s has no merge base; start its worker again", ticketID)
		}
		onto = sha
	}
	sha, kept, err := el.MergeCheckedBranch(el.OSExec, repo, worktree, branch, onto, target, "have it commit them and submit done again")
	if err != nil {
		return el.DriverFinish{}, err
	}
	if kept != "" {
		return el.DriverFinish{Merged: false, Detail: kept}, nil
	}
	if path != "" {
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return el.DriverFinish{}, err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return el.DriverFinish{}, fmt.Errorf("ticket %s is outside the repo", ticketID)
		}
		if _, err := stageAndCommitTicketDone(repo, path, rel, workerID); err != nil {
			return el.DriverFinish{}, err
		}
	}
	if _, err := runGit(repo, "worktree", "remove", "--force", worktree); err != nil {
		return el.DriverFinish{}, err
	}
	d.mu.Lock()
	if rs, ok := d.runs[runID]; ok {
		rs.Merged[ticketID] = sha
		if rs.NeedsYouReason != nil {
			delete(rs.NeedsYouReason, ticketID)
		}
		if rs.Blocked != nil {
			delete(rs.Blocked, ticketID)
		}
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	}
	d.mu.Unlock()
	return el.DriverFinish{Merged: true, SHA: sha}, nil
}

// isAncestorRef reports whether ancestor contains ref in repo: git
// merge-base --is-ancestor exits 0 when it holds and 1 when it does not,
// and runGit fails either nonzero exit, so this reads the exit code.
func isAncestorRef(repo, ancestor, ref string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, ref)
	cmd.Dir = repo
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	} else {
		return false, err
	}
}

// ffUpdateSpecBranch fast-forwards the spec branch to the ticket branch tip
// without touching any checkout: update-ref moves the ref only when the tip
// contains the spec tip, so a spec branch a User has checked out in another
// worktree still merges (that worktree simply falls behind). sha is the new
// spec tip; kept holds the refusal when the tip is not ahead (the next
// finish rebases onto the moved spec and retries); err is a hard git
// failure.
func ffUpdateSpecBranch(repo, branch, specBranch, ticketID string) (sha, kept string, err error) {
	tip, err := runGit(repo, "rev-parse", branch)
	if err != nil || tip == "" {
		return "", "", fmt.Errorf("ticket %s has no tip; start its worker again", ticketID)
	}
	old, err := runGit(repo, "rev-parse", specBranch)
	if err != nil || old == "" {
		return "", "", fmt.Errorf("could not read spec branch %s in %s: %v", specBranch, repo, err)
	}
	if old == tip {
		return tip, "", nil
	}
	ok, err := isAncestorRef(repo, old, tip)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", fmt.Sprintf("could not fast-forward %s: it moved under the ticket; finish again", specBranch), nil
	}
	if out, err := runGit(repo, "update-ref", "refs/heads/"+specBranch, tip, old); err != nil {
		return "", fmt.Sprintf("could not fast-forward %s: %s; finish again", specBranch, strings.TrimSpace(out+err.Error())), nil
	}
	return tip, "", nil
}

// stageAndCommitTicketDone marks the ticket file done (ticking every box)
// and commits it in gitDir: the spec-branch finish commits in the ticket
// worktree, the legacy finish in the main checkout. For a file without a
// Status line it still ticks and commits its boxes after merge, when present.
func stageAndCommitTicketDone(gitDir, filePath, rel, workerID string) (changed bool, err error) {
	done, merr := el.MarkDriverTicketDone(filePath)
	if merr != nil {
		if !strings.Contains(merr.Error(), "has no status line") {
			return false, merr
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return false, err
		}
		var boxes []string
		for _, box := range parseTicketBoxes(string(raw)) {
			boxes = append(boxes, box.Text)
		}
		if len(boxes) == 0 {
			return false, nil
		}
		if _, err := el.TickDriverTicketFile(filePath, boxes); err != nil {
			return false, err
		}
		done = true
	}
	if !done {
		return false, nil
	}
	if _, err := runGit(gitDir, "add", "--", rel); err != nil {
		return false, err
	}
	staged, err := runStagedDiff(gitDir, rel)
	if err != nil {
		return false, err
	}
	if !staged {
		return false, nil
	}
	if out, err := runGitCommitDone(gitDir, rel, workerID); err != nil {
		if !strings.Contains(out+err.Error(), "nothing to commit") {
			return false, err
		}
	}
	return true, nil
}

// mergeTicketIntoSpec runs the git half of a spec-branch finish: the
// dirty-worktree check, a rebase of the ticket branch onto the spec branch
// as it stands at finish (aborted on conflict), a fast-forward of the spec
// branch to the rebased tip, and the ticket done mark committed on the spec
// branch. sha is the spec tip after the merge; kept holds the refusal text
// when the branch is kept and the worker stays alive; err is a hard git
// failure. The main checkout never moves.
func (d *Daemon) mergeTicketIntoSpec(repo, worktree, branch, specBranch, path, ticketID, workerID string) (sha, kept string, err error) {
	if out, err := runGit(worktree, "status", "--porcelain"); err != nil {
		return "", "", err
	} else if strings.TrimSpace(out) != "" {
		return "", "the worktree has uncommitted changes, which would be lost; have it commit them and submit done again", nil
	}
	if _, err := runGit(worktree, "rebase", specBranch); err != nil {
		files := ""
		if u, uerr := runGit(worktree, "diff", "--name-only", "--diff-filter=U"); uerr == nil {
			files = strings.Join(strings.Fields(u), ", ")
		}
		_, _ = runGit(worktree, "rebase", "--abort")
		detail := "rebase onto " + specBranch + " conflicted"
		if files != "" {
			detail += " in " + files
		} else {
			detail += ": " + strings.TrimSpace(strings.Replace(err.Error(), "\n", " ", -1))
		}
		return "", detail + "; the rebase was aborted, the branch is unchanged", nil
	}
	sha, kept, err = ffUpdateSpecBranch(repo, branch, specBranch, ticketID)
	if err != nil || kept != "" {
		return "", kept, err
	}
	if path != "" {
		rel, rerr := filepath.Rel(repo, path)
		if rerr != nil {
			return "", "", rerr
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("ticket %s is outside the repo", ticketID)
		}
		// The worktree spec mirror is read-only so the Worker cannot tick
		// its own copy; the driver restores write on the ticket file to
		// record the done mark. The worktree is the ticket's own and is
		// removed after the finish, so worker behavior is unchanged. A
		// file the mirror never laid down stays missing and fails below
		// the way it did before the mirror.
		if err := os.Chmod(filepath.Join(worktree, rel), 0o644); err != nil && !os.IsNotExist(err) {
			return "", "", err
		}
		changed, err := stageAndCommitTicketDone(worktree, filepath.Join(worktree, rel), rel, workerID)
		if err != nil {
			return "", "", err
		}
		if changed {
			sha, kept, err = ffUpdateSpecBranch(repo, branch, specBranch, ticketID)
			if err != nil || kept != "" {
				return "", kept, err
			}
		}
	}
	return sha, "", nil
}

// specChangedFiles lists the files the spec branch changed since its base:
// `git diff --name-only base branch`, sorted. Empty when either ref is
// missing or git fails; callers then warn on nothing, so a missing base
// never blocks a merge.
func specChangedFiles(repo, base, branch string) []string {
	if repo == "" || base == "" || branch == "" {
		return nil
	}
	out, err := runGit(repo, "diff", "--name-only", base, branch)
	if err != nil {
		return nil
	}
	set := map[string]bool{}
	for _, f := range strings.Fields(out) {
		f = strings.TrimSpace(f)
		if f != "" {
			set[f] = true
		}
	}
	files := make([]string, 0, len(set))
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// intersectFiles returns the sorted intersection of two sorted file lists.
func intersectFiles(a, b []string) []string {
	set := map[string]bool{}
	for _, f := range a {
		set[f] = true
	}
	var out []string
	for _, f := range b {
		if set[f] {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// warningCovers reports whether warnings already warn about otherRun with
// at least these files: a later merge that adds nothing new stays silent
// instead of repeating the same Event.
func warningCovers(warnings []runWarning, otherRun string, files []string) bool {
	have := map[string]bool{}
	for _, w := range warnings {
		if w.OtherRun != otherRun {
			continue
		}
		for _, f := range w.Files {
			have[f] = true
		}
	}
	for _, f := range files {
		if !have[f] {
			return false
		}
	}
	return len(files) > 0
}

// overlapWarningText renders the warning Event text naming both spec
// branches and the shared files, so the User knows a final-merge conflict
// is coming.
func overlapWarningText(branch, otherBranch string, files []string) string {
	return fmt.Sprintf("%s and %s both touch %s; a conflict is coming at final merge", branch, otherBranch, strings.Join(files, ", "))
}

// checkSpecOverlap compares the just-merged Run's spec branch changed files
// with every other live Run's spec branch in the same repo and publishes a
// warning Event on both Runs when the sets intersect. It runs after a
// Ticket merge with no lock held (git first, state second) and never fails
// the merge: git errors simply warn on nothing.
func (d *Daemon) checkSpecOverlap(runID string) {
	d.mu.Lock()
	cur, ok := d.runs[runID]
	if !ok || cur.SpecBranch == "" || cur.SpecBase == "" {
		d.mu.Unlock()
		return
	}
	repo, branch, base := cur.Repo, cur.SpecBranch, cur.SpecBase
	type other struct {
		id, specBranch, specBase string
	}
	var others []other
	for id, rs := range d.runs {
		if id == runID || rs.Repo != repo || rs.SpecBranch == "" || rs.SpecBranch == branch || rs.SpecBase == "" {
			continue
		}
		// Live means running or paused: a paused Run resumes on the same
		// branch, while done and failed Runs are over and never merge
		// again.
		if rs.Status != runStatusRunning && rs.Status != runStatusPaused && rs.Status != runStatusNeedsYou {
			continue
		}
		others = append(others, other{id: id, specBranch: rs.SpecBranch, specBase: rs.SpecBase})
	}
	d.mu.Unlock()
	if len(others) == 0 {
		return
	}
	mine := specChangedFiles(repo, base, branch)
	if len(mine) == 0 {
		return
	}
	type hit struct {
		id, otherBranch string
		files           []string
	}
	var hits []hit
	for _, o := range others {
		theirs := specChangedFiles(repo, o.specBase, o.specBranch)
		if shared := intersectFiles(mine, theirs); len(shared) > 0 {
			hits = append(hits, hit{id: o.id, otherBranch: o.specBranch, files: shared})
		}
	}
	if len(hits) == 0 {
		return
	}
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok = d.runs[runID]
	if !ok {
		return
	}
	for _, h := range hits {
		other, ok := d.runs[h.id]
		if !ok {
			continue
		}
		if warningCovers(cur.Warnings, h.id, h.files) && warningCovers(other.Warnings, runID, h.files) {
			continue
		}
		if !warningCovers(cur.Warnings, h.id, h.files) {
			cur.Warnings = append(cur.Warnings, runWarning{At: now, OtherRun: h.id, OtherBranch: h.otherBranch, Files: append([]string(nil), h.files...), Message: overlapWarningText(branch, h.otherBranch, h.files)})
			d.persistRun(cur)
			d.broadcastRunLocked(cur)
		}
		if !warningCovers(other.Warnings, runID, h.files) {
			other.Warnings = append(other.Warnings, runWarning{At: now, OtherRun: runID, OtherBranch: branch, Files: append([]string(nil), h.files...), Message: overlapWarningText(h.otherBranch, branch, h.files)})
			d.persistRun(other)
			d.broadcastRunLocked(other)
		}
	}
}

// reviewGateReady says the finish may judge the gate: every required
// reviewer type has a Session and a submitted verdict.
func reviewGateReady(sessions map[string]string, verdicts map[string]storedReview) bool {
	for _, typ := range requiredReviewerTypes {
		if sessions[typ] == "" {
			return false
		}
		if _, ok := verdicts[typ]; !ok {
			return false
		}
	}
	return true
}

// reviewGateDetail refuses the merge unless every required reviewer type
// passed at the tip: a missing pass, a changes verdict or a pass at an
// older commit. A new commit makes older Passes stale. Empty means the gate
// passed. Fresh findings ride along verbatim, so the Tell reaches the Worker
// unchanged; stale findings stay behind (at only, never the old report),
// since the re-prompt asked for a new judgement.
func reviewGateDetail(verdicts map[string]storedReview, tip string) string {
	var missing []string
	for _, typ := range requiredReviewerTypes {
		r, ok := verdicts[typ]
		switch {
		case !ok || r.Verdict == "":
			missing = append(missing, typ+": no review yet")
		case r.Verdict != "pass" && r.At != tip:
			missing = append(missing, fmt.Sprintf("%s: %s at %s, not at the tip %s", typ, r.Verdict, shortSHA(r.At), shortSHA(tip)))
		case r.Verdict != "pass":
			at := shortSHA(r.At)
			if strings.TrimSpace(r.Report) != "" {
				missing = append(missing, fmt.Sprintf("%s: %s at %s: %s", typ, r.Verdict, at, strings.TrimSpace(r.Report)))
			} else {
				missing = append(missing, fmt.Sprintf("%s: %s at %s", typ, r.Verdict, at))
			}
		case r.At != tip:
			missing = append(missing, fmt.Sprintf("%s: passed at %s, not at the tip %s", typ, shortSHA(r.At), shortSHA(tip)))
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "not reviewed to a pass: " + strings.Join(missing, "; ")
}

// noneFreshAtTip says no reviewer judged the current tip yet: every verdict
// predates it, so the re-prompt for this tip is still in flight.
func noneFreshAtTip(verdicts map[string]storedReview, tip string) bool {
	for _, typ := range requiredReviewerTypes {
		if r, ok := verdicts[typ]; ok && r.At == tip {
			return false
		}
	}
	return true
}

// roundOpenAtTip says the ticket's last review round is still open at tip:
// at least one required reviewer has not delivered its verdict for it yet.
func roundOpenAtTip(rounds []ticketRound, tip string) bool {
	if len(rounds) == 0 {
		return false
	}
	last := rounds[len(rounds)-1]
	return last.Tip == tip && last.EndedAt == nil
}

// shortSHA names a commit the way refusals do: the first eight characters.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// runSpecBehind reads the current master tip and counts commits absent from
// the spec branch. Call it only without d.mu held: git may block on disk.
func runSpecBehind(repo, branch string) any {
	if branch == "" {
		return nil
	}
	master, err := masterTip(repo)
	if err != nil {
		return nil
	}
	count, err := runGit(repo, "rev-list", "--count", branch+".."+master)
	if err != nil {
		return nil
	}
	behind, err := strconv.Atoi(count)
	if err != nil {
		return nil
	}
	return behind
}

// runJSON calculates the master distance without holding d.mu, then
// re-reads and renders the current Run state under the lock.
func (d *Daemon) runJSON(rs *runState) map[string]any {
	if rs == nil {
		return nil
	}
	id := rs.ID
	d.mu.Lock()
	current, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		return nil
	}
	repo, branch := current.Repo, current.SpecBranch
	d.mu.Unlock()

	specBehind := runSpecBehind(repo, branch)
	d.mu.Lock()
	defer d.mu.Unlock()
	current, ok = d.runs[id]
	if !ok {
		return nil
	}
	return d.runJSONLocked(current, specBehind)
}

// runJSONLocked renders one Run with its live Ticket table. Callers hold
// d.mu; specBehind is precomputed by runJSON so the master and rev-list
// queries never block Run updates.
// The snapshot also includes the Worker Session link, reviewer verdicts,
// refusal, ticket body/boxes, branch, tip, state time, needs-you reason,
// follow-ups, agents, gate and append-only rounds.
func (d *Daemon) runJSONLocked(rs *runState, specBehind any) map[string]any {
	rows := map[string]el.DriverRow{}
	if rs.Driver != nil {
		for _, row := range rs.Driver.Snapshot() {
			rows[row.ID] = row
		}
	}
	tickets := make([]map[string]any, 0, len(rs.Tickets))
	for _, t := range rs.Tickets {
		state := "queued"
		lastRefusal := ""
		tip := ""
		var stateSince time.Time
		if row, ok := rows[t.ID]; ok {
			state = string(row.State)
			lastRefusal = row.LastRefusal
			tip = row.Tip
			stateSince = row.Since
		} else if saved, ok := rs.Rows[t.ID]; ok {
			// No live Driver (reloaded or finished on another boot): show
			// the row the last pass wrote.
			state = saved.State
			lastRefusal = saved.LastRefusal
			tip = saved.Tip
			stateSince = saved.Since
		} else if rs.Status == runStatusPaused {
			state = runStatusPaused
		}
		needsYou := rs.NeedsYouReason[t.ID]
		if needsYou != "" && state != "merged" && state != "discarded" && state != "needs-you" && (state != "parked" || !rs.Parked[t.ID]) {
			// A base-red gate uses the driver's parked state to block a start;
			// expose its Needs-you reason unless the User explicitly parked it.
			state = "needs-you"
		}
		// User actions override the last driver row immediately.
		if state != "merged" {
			if rs.Discarded[t.ID] {
				state = "discarded"
			} else if rs.TicketPaused[t.ID] {
				state = runStatusPaused
			} else if rs.Parked[t.ID] {
				state = "parked"
			}
		}
		blockers := t.Blockers
		if blockers == nil {
			blockers = []string{}
		}
		verdicts := map[string]any{}
		for _, typ := range requiredReviewerTypes {
			if r, ok := rs.Reviews[t.ID][typ]; ok {
				verdicts[typ] = map[string]any{
					"verdict": r.Verdict, "at": r.At,
					"fresh": tip != "" && r.At == tip,
				}
			}
		}
		reviewers := map[string]string{}
		for typ, sess := range rs.Reviewers[t.ID] {
			reviewers[typ] = sess
		}
		gate := map[string]any{
			"required": append([]string(nil), requiredReviewerTypes...),
			"verdicts": verdicts,
		}
		body := runTicketBody(rs, t.ID)
		title := parseTicketTitle(body)
		if title == "" {
			title = t.Title
		}
		boxes := parseTicketBoxes(body)
		rounds := rs.Rounds[t.ID]
		if rounds == nil {
			rounds = []ticketRound{}
		}
		tickets = append(tickets, map[string]any{
			"id": t.ID, "num": t.Num, "title": title, "blockers": blockers,
			"state": state, "stateSince": stateSince, "workerSession": rs.Sessions[t.ID],
			"reviewerSessions": reviewers, "verdicts": verdicts, "gate": gate, "tip": tip,
			"branch": rs.Branches[t.ID], "body": body, "boxes": boxes,
			"effort": rs.Effort, "lastRefusal": lastRefusal, "needsYou": needsYou,
			"followups": rs.Followups[t.ID], "agents": d.ticketAgentsLocked(rs, t.ID),
			"rounds": rounds, "recallOff": rs.RecallOff[t.ID], "chain": ticketChainEntries(rs, t.ID),
		})
	}
	warnings := rs.Warnings
	if warnings == nil {
		warnings = []runWarning{}
	}
	return map[string]any{
		"id": rs.ID, "spec": rs.Spec, "title": el.SpecTitle(rs.Spec), "repo": rs.Repo, "status": rs.Status,
		"owner":   rs.Owner,
		"harness": rs.Harness, "model": rs.Model, "effort": rs.Effort,
		"recallOffN": rs.RecallOffN,
		"specBranch": rs.SpecBranch, "specBase": rs.SpecBase, "specBehind": specBehind,
		"warnings":  warnings,
		"createdAt": rs.CreatedAt, "error": rs.Error, "finalMergeFiles": rs.FinalMergeFiles, "tickets": tickets,
	}
}

// ticketAgentsLocked lists one ticket's sessions: its worker, both
// reviewers and the scouts the worker spawned (ParentID the worker).
// Callers hold d.mu.
func (d *Daemon) ticketAgentsLocked(rs *runState, ticketID string) []ticketAgent {
	out := []ticketAgent{}
	worker := rs.Sessions[ticketID]
	agentName := func(meta SessionMeta) string {
		if meta.Name != "" {
			return meta.Name
		}
		return meta.Title
	}
	if worker != "" {
		if s, ok := d.sessions[worker]; ok {
			out = append(out, ticketAgent{SessionID: worker, Role: "worker", Harness: s.meta.Harness, Status: s.meta.Status, Name: agentName(s.meta)})
		}
	}
	for _, typ := range requiredReviewerTypes {
		sess := rs.Reviewers[ticketID][typ]
		if sess == "" {
			continue
		}
		if s, ok := d.sessions[sess]; ok {
			out = append(out, ticketAgent{SessionID: sess, Role: typ, Harness: s.meta.Harness, Status: s.meta.Status, Name: agentName(s.meta)})
		}
	}
	if worker != "" {
		for _, s := range d.sessions {
			if s.meta.ParentID == worker && s.meta.Scout {
				out = append(out, ticketAgent{SessionID: s.meta.ID, Role: "scout", Harness: s.meta.Harness, Status: s.meta.Status, Name: agentName(s.meta)})
			}
		}
	}
	return out
}

// broadcastRunLocked wakes the Run's SSE streams; callers hold d.mu.
func (d *Daemon) broadcastRunLocked(rs *runState) {
	d.live.publish("runs", "run:"+rs.ID)
	notify := rs.notify
	rs.notify = make(chan struct{})
	close(notify)
}

// runEventSnapshot pairs one serialized Run with the notification channel
// under the lock, while calculating the git-based distance outside it.
func (d *Daemon) runEventSnapshot(id string) (string, chan struct{}, bool) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		return "", nil, false
	}
	repo, branch := rs.Repo, rs.SpecBranch
	d.mu.Unlock()

	specBehind := runSpecBehind(repo, branch)
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok = d.runs[id]
	if !ok {
		return "", nil, false
	}
	return mustJSON(d.runJSONLocked(rs, specBehind)), rs.notify, true
}

// serveRunEvents streams the live Ticket table over SSE, resending the Run
// whenever the driver publishes new rows.
func (d *Daemon) serveRunEvents(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	_, ok := d.runs[id]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	last := ""
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		body, ch, ok := d.runEventSnapshot(id)
		if !ok {
			return
		}
		if body != last {
			last = body
			fmt.Fprintf(w, "data: %s\n\n", body)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if r.URL.Query().Get("wait") == "0" {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		case <-ch:
		}
	}
}

// handleSessionSubmit records a Worker's submit (its status and completed
// checkbox line texts) or a Reviewer's submit tool call (verdict, tip, ran
// and findings). The driver loop reads both: it ticks the boxes and merges
// a done submit once the gate passes. A Worker's done submit also spawns its
// Reviewers, once, and re-prompts them on later dones. A Reviewer's verdict
// records at its tip: a `pass` needs `ran`, a `changes` needs a blocker or
// major, and a tip behind the branch tip records stale and does not count.
// The legacy `VERDICT: pass|changes` free-text reply stays only as a fallback
// for a Backend without the tool. The Worker submit gate refuses a done with
// no new commit since the last changes verdict, refuses a done whose report
// or pre-Follow-ups summary says the work is not done, and turns a second
// blocked submit with the same question hash into needs-you.
// notDonePhrases is the conservative "work is not done" pattern for the
// Worker submit gate: a done submit whose report or pre-Follow-ups summary
// names one of these is refused, so a ticked Ticket is complete. The list
// stays narrow on purpose: only phrases that clearly say the work itself is
// unfinished. The Follow-ups section of the summary is excluded from the
// scan, so legitimate notes like "Follow-ups: still needs docs" or reports
// about handling "incomplete input" do not refuse.
var notDonePhrases = []string{
	"not done",
	"not finished",
	"not complete",
}

// stripFollowups returns the summary text before the Follow-ups: marker, so
// the not-done scan never reads the worker's follow-up notes.
func stripFollowups(summary string) string {
	if i := strings.Index(summary, "Follow-ups:"); i >= 0 {
		return summary[:i]
	}
	return summary
}

// refuseNotDoneText reports why a done submit's text says the work is not
// done, or empty when the text reads finished. It scans the report and the
// pre-Follow-ups summary together, case-insensitively.
func refuseNotDoneText(report, summary string) string {
	text := strings.ToLower(strings.Join(strings.Fields(report+" "+stripFollowups(summary)), " "))
	if text == "" {
		return ""
	}
	for _, phrase := range notDonePhrases {
		if strings.Contains(text, phrase) {
			return "done refused: the submit text says the work is not done (" + phrase + "); finish the work and submit done again"
		}
	}
	return ""
}

// refuseEmptyResubmit reports why a done submit at tip is an empty resubmit
// after a changes verdict, or empty when the Worker committed since. Any
// changes verdict at the current tip means no commit followed it, so the
// submit is refused with the tip named.
func refuseEmptyResubmit(reviews map[string]storedReview, tip string) string {
	if tip == "" {
		return ""
	}
	for _, typ := range requiredReviewerTypes {
		if r, ok := reviews[typ]; ok && r.Verdict == "changes" && r.At == tip {
			return "done refused: no new commit since the last changes verdict at " + shortSHA(tip) + "; commit your fixes and submit done again"
		}
	}
	return ""
}

// blockedQuestionText is the Worker's blocked question for the gate: the
// report and summary together, trimmed. Empty reads as an empty question.
func blockedQuestionText(report, summary string) string {
	return strings.TrimSpace(strings.TrimSpace(report) + "\n" + strings.TrimSpace(summary))
}

// blockedQuestionHash normalizes a blocked question (collapsing whitespace)
// and hashes it, so the same question asked twice has the same hash. The
// normalization keeps case: the same words with different answers still
// match, while trivial whitespace edits do not hide a repeat.
func blockedQuestionHash(question string) string {
	normalized := strings.Join(strings.Fields(question), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// truncateQuestion shortens a blocked question for the needs-you reason.
func truncateQuestion(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if s == "" {
		return "no question given"
	}
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "..."
}

// storeSubmitLocked records one submit in d.submits and, for Run workers,
// in the Run's SubmitRecs, bumping past Reprompted after a restart.
// Callers hold d.mu. A nil rs means a non-Run session: only d.submits.
func (d *Daemon) storeSubmitLocked(sessionID, ticketID string, rs *runState, status string, completed []string, report, summary string) int {
	rec := d.submits[sessionID]
	if rec == nil {
		rec = &submitRec{}
		d.submits[sessionID] = rec
	}
	rec.Count++
	rec.Status = status
	rec.Completed = append([]string(nil), completed...)
	rec.Report = report
	rec.Summary = summary
	count := rec.Count
	if rs != nil && ticketID != "" {
		if status == "done" && rs.ChainPendingSubmit != nil {
			delete(rs.ChainPendingSubmit, ticketID)
		}
		if n := rs.Reprompted[ticketID]; rec.Count <= n {
			rec.Count = n + 1
			count = rec.Count
		}
		if rs.SubmitRecs == nil {
			rs.SubmitRecs = map[string]submitRec{}
		}
		rs.SubmitRecs[ticketID] = *rec
	}
	return count
}

// recordWorkerDispositionsLocked validates and stores the Worker's decisions
// on the most recently delivered finding set. Callers hold d.mu.
func recordWorkerDispositionsLocked(rs *runState, ticketID string, submitted []ReviewDisposition) error {
	rounds := rs.Rounds[ticketID]
	idx := -1
	for i := len(rounds) - 1; i >= 0; i-- {
		if len(rounds[i].SentFindings) > 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		_, err := validateWorkerDispositions(nil, submitted)
		return err
	}
	decisions, err := validateWorkerDispositions(rounds[idx].SentFindings, submitted)
	if err != nil {
		return err
	}
	for i := range rounds[idx].SentFindings {
		if decision, ok := decisions[rounds[idx].SentFindings[i].FindingID]; ok {
			copy := decision
			rounds[idx].SentFindings[i].Disposition = &copy
		}
	}
	rs.Rounds[ticketID] = rounds
	return nil
}

// recordWorkerFindingsSentLocked snapshots the current tip's full finding
// list on its round. The boolean reports whether the Worker has not yet been
// notified; callers hold d.mu.
func recordWorkerFindingsSentLocked(rs *runState, ticketID, tip string) (string, bool) {
	rounds := rs.Rounds[ticketID]
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].Tip != tip {
			continue
		}
		round := &rounds[i]
		if len(round.SentFindings) == 0 {
			round.SentFindings = workerFindingsForRound(*round)
		}
		if len(round.SentFindings) == 0 {
			return "", false
		}
		newlySent := round.SentToWorkerAt == nil
		if newlySent {
			now := time.Now().UTC()
			round.SentToWorkerAt = &now
		}
		rs.Rounds[ticketID] = rounds
		return workerReviewFeedback(tip, round.SentFindings), newlySent
	}
	return "", false
}

func (d *Daemon) handleSessionSubmit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Status       string              `json:"status"`
		Completed    []string            `json:"completed"`
		Report       string              `json:"report"`
		Summary      string              `json:"summary"`
		Verdict      string              `json:"verdict"`
		Tip          string              `json:"tip"`
		Ran          []ReviewRan         `json:"ran"`
		Findings     []ReviewFinding     `json:"findings"`
		Dispositions []ReviewDisposition `json:"dispositions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	if _, ok := d.sessions[id]; !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if _, _, reviewerType := d.findReviewerLocked(id); reviewerType != "" {
		d.mu.Unlock()
		d.handleReviewerSubmit(w, r, id, ReviewerSubmit{Verdict: req.Verdict, Tip: req.Tip, Ran: req.Ran, Findings: req.Findings})
		return
	}
	if req.Status == "" {
		d.mu.Unlock()
		http.Error(w, "status is required", http.StatusBadRequest)
		return
	}
	workerRun, workerTicket := d.findWorkerLocked(id)
	rs, ok := d.runs[workerRun]
	if workerRun == "" || !ok {
		count := d.storeSubmitLocked(id, "", nil, req.Status, req.Completed, req.Report, req.Summary)
		d.mu.Unlock()
		d.appendEvent(id, "submit", req.Status, "")
		writeJSON(w, http.StatusAccepted, map[string]any{"submits": count})
		return
	}
	switch req.Status {
	case "done":
		d.handleWorkerDoneSubmit(w, r, id, workerRun, workerTicket, rs, req.Completed, req.Report, req.Summary, req.Dispositions)
	case "blocked":
		d.handleWorkerBlockedSubmit(w, r, id, workerRun, workerTicket, rs, req.Completed, req.Report, req.Summary)
	default:
		count := d.storeSubmitLocked(id, workerTicket, rs, req.Status, req.Completed, req.Report, req.Summary)
		d.persistRun(rs)
		d.mu.Unlock()
		d.appendEvent(id, "submit", req.Status, "")
		writeJSON(w, http.StatusAccepted, map[string]any{"submits": count})
	}
}

// handleWorkerDoneSubmit gates and records one Worker done submit.
// Callers hold d.mu and stay holding it across the tip read, so two
// concurrent done submits cannot both pass the empty-resubmit check:
// rev-parse never touches the index, so holding the lock is a short,
// single-attempt read.
func (d *Daemon) handleWorkerDoneSubmit(w http.ResponseWriter, r *http.Request, id, workerRun, workerTicket string, rs *runState, completed []string, report, summary string, dispositions []ReviewDisposition) {
	if msg := refuseNotDoneText(report, summary); msg != "" {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": msg})
		return
	}
	for _, typ := range requiredReviewerTypes {
		if rev, ok := rs.Reviews[workerTicket][typ]; ok && rev.Verdict == "changes" {
			tip := ""
			if branch := rs.Branches[workerTicket]; branch != "" {
				if sha, err := runGit(rs.Repo, "rev-parse", branch); err == nil {
					tip = sha
				}
			}
			if why := refuseEmptyResubmit(rs.Reviews[workerTicket], tip); why != "" {
				d.mu.Unlock()
				writeJSON(w, http.StatusConflict, map[string]any{"error": why})
				return
			}
			break
		}
	}
	if err := recordWorkerDispositionsLocked(rs, workerTicket, dispositions); err != nil {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	if rs.Merged[workerTicket] != "" {
		count := 0
		if rec := d.submits[id]; rec != nil {
			count = rec.Count
		}
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
		d.mu.Unlock()
		d.appendEvent(id, "submit", "done", "")
		writeJSON(w, http.StatusAccepted, map[string]any{"submits": count})
		return
	}
	count := d.storeSubmitLocked(id, workerTicket, rs, "done", completed, report, summary)
	if rs.Followups == nil {
		rs.Followups = map[string]string{}
	}
	rs.Followups[workerTicket] = parseFollowups(summary)
	if rs.Blocked != nil {
		delete(rs.Blocked, workerTicket)
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.appendEvent(id, "submit", "done", "")
	// Sync so the driver's next finish sees the re-prompt already out,
	// never a stale refusal first, and prompts queue in submit order.
	// Bounded to two reviewer starts plus two steers; the cap refuses
	// fast and Tell is best effort, so the ack waits on at most that.
	d.ensureReviewers(workerRun, workerTicket)
	writeJSON(w, http.StatusAccepted, map[string]any{"submits": count})
}

// handleWorkerBlockedSubmit records one Worker blocked submit, turning a
// second submit with the same question hash into needs-you. Callers hold
// d.mu.
func (d *Daemon) handleWorkerBlockedSubmit(w http.ResponseWriter, r *http.Request, id, workerRun, workerTicket string, rs *runState, completed []string, report, summary string) {
	question := blockedQuestionText(report, summary)
	hash := blockedQuestionHash(question)
	if rs.Blocked == nil {
		rs.Blocked = map[string]blockedTrack{}
	}
	prev := rs.Blocked[workerTicket]
	var needsYou string
	if prev.Hash != "" && prev.Hash == hash {
		rs.Blocked[workerTicket] = blockedTrack{Hash: hash, Question: question, Count: prev.Count + 1}
		needsYou = "worker blocked again with the same question: " + truncateQuestion(question, 180)
		if rs.NeedsYouReason == nil {
			rs.NeedsYouReason = map[string]string{}
		}
		rs.NeedsYouReason[workerTicket] = needsYou
	} else {
		rs.Blocked[workerTicket] = blockedTrack{Hash: hash, Question: question, Count: 1}
	}
	count := d.storeSubmitLocked(id, workerTicket, rs, "blocked", completed, report, summary)
	var push bool
	var num int
	var title string
	if needsYou != "" {
		push = d.noteNeedsYouPushLocked(rs, workerTicket, needsYou)
		num, title = ticketNumTitleLocked(rs, workerTicket)
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.appendEvent(id, "submit", "blocked", "")
	if needsYou != "" && push {
		d.pushRunNeedsYou(workerRun, workerTicket, num, title, needsYou)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"submits": count})
}

// handleReviewerSubmit records one reviewer `submit` tool call: verdict, tip,
// ran and findings. The verdict rules are tool errors (400): a `pass` needs
// `ran`, a `changes` needs a blocker or major, and the verdict must name a
// tip. A tip behind the branch tip records stale and does not count toward
// the gate, but still returns 202 with `stale: true` and the current tip, so
// the reviewer can judge it without waiting for the re-prompt. A `pass` may
// carry minors without blocking; they ride the refusal text to the worker
// like any other finding once a later `changes` arrives, and the gate passes
// on a fresh pass regardless of its minors. An unknown Session answers 404
// even with an invalid payload: the reviewer check runs before validation.
// Callers hold no lock.
func (d *Daemon) handleReviewerSubmit(w http.ResponseWriter, r *http.Request, id string, sub ReviewerSubmit) {
	sub.Verdict = strings.ToLower(strings.TrimSpace(sub.Verdict))
	sub.Tip = strings.TrimSpace(sub.Tip)
	for i := range sub.Findings {
		sub.Findings[i].Severity = strings.ToLower(strings.TrimSpace(sub.Findings[i].Severity))
	}
	d.mu.Lock()
	runID, ticketID, typ := d.findReviewerLocked(id)
	rs, ok := d.runs[runID]
	sess, sok := d.sessions[id]
	if !ok || !sok || typ == "" {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	repo, branch := rs.Repo, rs.Branches[ticketID]
	lastPrompt := int64(0)
	for _, e := range sess.events {
		if e.Kind == "prompt" && e.Seq > lastPrompt {
			lastPrompt = e.Seq
		}
	}
	d.mu.Unlock()
	if err := ValidateReviewerSubmit(sub); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cur, _ := runGit(repo, "rev-parse", branch)
	stale := cur != "" && sub.Tip != cur
	patchID := d.reviewPatchIDFor(runID, ticketID, strings.TrimSpace(sub.Tip))
	now := time.Now().UTC()
	d.mu.Lock()
	rs, ok = d.runs[runID]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	retell := recordReviewerToolVerdictLocked(d, rs, id, ticketID, typ, sub, cur, patchID, now, lastPrompt)
	count := d.submits[id].Count
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	payload, _ := json.Marshal(sub)
	d.appendEvent(id, "submit", string(payload), "")
	if retell != "" {
		_ = d.runTellSession(id, retell)
	}
	resp := map[string]any{"submits": count, "verdict": sub.Verdict, "stale": stale}
	if stale && cur != "" {
		resp["currentTip"] = cur
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// findReviewerLocked names the Run, Ticket and reviewer type of a Reviewer
// Session. Empty type means the Session is no Reviewer's. Callers hold d.mu.
func (d *Daemon) findReviewerLocked(id string) (runID, ticketID, reviewerType string) {
	for _, rs := range d.runs {
		for ticket, byType := range rs.Reviewers {
			for typ, sess := range byType {
				if sess == id {
					return rs.ID, ticket, typ
				}
			}
		}
	}
	return "", "", ""
}

// findWorkerLocked names the Run and Ticket of a Worker Session. Empty means
// the Session is no Worker's. Callers hold d.mu.
func (d *Daemon) findWorkerLocked(id string) (runID, ticketID string) {
	for _, rs := range d.runs {
		for ticket, sess := range rs.Sessions {
			if sess == id {
				return rs.ID, ticket
			}
		}
	}
	return "", ""
}

// --- run persistence: stateDir/enderbloom/runs/{id}.json ---

func (d *Daemon) runFile(id string) string {
	return filepath.Join(d.stateDir, "enderbloom", "runs", id+".json")
}

// persistRun saves one Run beside the group files; callers hold d.mu.
func (d *Daemon) persistRun(rs *runState) {
	defer d.live.publish("runs", "run:"+rs.ID)
	dir := filepath.Join(d.stateDir, "enderbloom", "runs")
	_ = os.MkdirAll(dir, 0o755)
	b, _ := json.Marshal(rs)
	tmp, err := os.CreateTemp(dir, "run-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	_ = os.Rename(name, d.runFile(rs.ID))
}

// loadRuns restores saved Runs as paused: the driver does not restart
// without the user, so nothing spends tokens after a Daemon restart.
func (d *Daemon) loadRuns() {
	root := filepath.Join(d.stateDir, "enderbloom", "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, en := range entries {
		if en.IsDir() || !strings.HasSuffix(en.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, en.Name()))
		if err != nil {
			continue
		}
		var rs runState
		if err := json.Unmarshal(raw, &rs); err != nil || rs.ID == "" {
			continue
		}
		if rs.Status == runStatusRunning {
			rs.Status = runStatusPaused
		}
		if rs.Sessions == nil {
			rs.Sessions = map[string]string{}
		}
		if rs.Branches == nil {
			rs.Branches = map[string]string{}
		}
		if rs.Worktrees == nil {
			rs.Worktrees = map[string]string{}
		}
		if rs.Targets == nil {
			rs.Targets = map[string]string{}
		}
		if rs.Bases == nil {
			rs.Bases = map[string]string{}
		}
		if rs.Merged == nil {
			rs.Merged = map[string]string{}
		}
		if rs.Reviewers == nil {
			rs.Reviewers = map[string]map[string]string{}
		}
		if rs.Reviews == nil {
			rs.Reviews = map[string]map[string]storedReview{}
		}
		if rs.ReviewTips == nil {
			rs.ReviewTips = map[string]map[string]string{}
		}
		if rs.ReviewPending == nil {
			rs.ReviewPending = map[string]map[string][]string{}
		}
		if rs.Reprompted == nil {
			rs.Reprompted = map[string]int{}
		}
		if rs.ReviewStarted == nil {
			rs.ReviewStarted = map[string]map[string]time.Time{}
		}
		if rs.FormatReprompt == nil {
			rs.FormatReprompt = map[string]map[string]bool{}
		}
		if rs.Rounds == nil {
			rs.Rounds = map[string][]ticketRound{}
		}
		if rs.NeedsYouReason == nil {
			rs.NeedsYouReason = map[string]string{}
		}
		if rs.Followups == nil {
			rs.Followups = map[string]string{}
		}
		if rs.TicketPaused == nil {
			rs.TicketPaused = map[string]bool{}
		}
		if rs.TicketPauseHolds == nil {
			rs.TicketPauseHolds = map[string]bool{}
		}
		if rs.PausedRoles == nil {
			rs.PausedRoles = map[string]map[string]bool{}
		}
		if rs.RunPausedRoles == nil {
			rs.RunPausedRoles = map[string]map[string]bool{}
		}
		if rs.WorkerOverrides == nil {
			rs.WorkerOverrides = map[string]chainEntry{}
		}
		if rs.Parked == nil {
			rs.Parked = map[string]bool{}
		}
		if rs.Discarded == nil {
			rs.Discarded = map[string]bool{}
		}
		if rs.RetryGen == nil {
			rs.RetryGen = map[string]int{}
		}
		if rs.RecallOff == nil {
			rs.RecallOff = map[string]bool{}
		}
		if rs.Rows == nil {
			rs.Rows = map[string]savedRow{}
		}
		if rs.ChainExhausted == nil {
			rs.ChainExhausted = map[string]bool{}
		}
		if rs.ChainPendingSubmit == nil {
			rs.ChainPendingSubmit = map[string]bool{}
		}
		// Older Run rows could persist Worker Chain positions but not the
		// pending-submit marker. Recover it from a fallback position or a failed
		// active Worker whose last-turn error advances the Chain.
		for ticket, row := range rs.Rows {
			failedAdvancing := false
			if id := rs.Sessions[ticket]; id != "" {
				if s := d.sessions[id]; s != nil && s.meta.Status == "failed" {
					detail := lastTurnFailure(s.events)
					failedAdvancing = detail != "" && chainAdvances(errors.New(detail))
				}
			}
			if row.ChainPositions["worker"] == 0 && !failedAdvancing {
				continue
			}
			if rec, ok := rs.SubmitRecs[ticket]; ok && rec.Status == "done" && !failedAdvancing {
				continue
			}
			rs.ChainPendingSubmit[ticket] = true
		}
		rs.chainHandling = map[string]bool{}
		rs.Driver = el.NewDriverRun(nil)
		rs.notify = make(chan struct{})
		rs.unpause = make(chan struct{})
		for ticket, rec := range rs.SubmitRecs {
			if sess := rs.Sessions[ticket]; sess != "" && d.submits[sess] == nil {
				cp := rec
				d.submits[sess] = &cp
			}
		}
		cp := rs
		d.runs[cp.ID] = &cp
	}
}

// snapshotRowsLocked copies the driver's live rows into the Run: one row
// per Ticket with state, tip, last refusal and state entry time. Callers
// hold d.mu.
func (d *Daemon) snapshotRowsLocked(rs *runState) {
	if rs.Driver == nil {
		return
	}
	if rs.Rows == nil {
		rs.Rows = map[string]savedRow{}
	}
	for _, row := range rs.Driver.Snapshot() {
		saved := rs.Rows[row.ID]
		saved.State, saved.Tip, saved.LastRefusal, saved.Since = string(row.State), row.Tip, row.LastRefusal, row.Since
		rs.Rows[row.ID] = saved
		if row.State == el.DriverRowMerged && rs.NeedsYouReason != nil {
			delete(rs.NeedsYouReason, row.ID)
		}
	}
}

// snapshotAndPersist writes one driver pass to the Run file and wakes its
// SSE streams. Callers hold no lock.
func (d *Daemon) snapshotAndPersist(runID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[runID]
	if !ok {
		return
	}
	d.snapshotRowsLocked(rs)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
}

// pauseGate blocks while a Run is paused or needs User intervention and
// returns once it runs again or has been abandoned. It holds no lock while
// waiting, so Pause, Resume and Abandon never block on the driver.
func (d *Daemon) pauseGate(runID string) {
	for {
		d.mu.Lock()
		rs, ok := d.runs[runID]
		if !ok || (rs.Status != runStatusPaused && rs.Status != runStatusNeedsYou) {
			d.mu.Unlock()
			return
		}
		ch := rs.unpause
		d.mu.Unlock()
		if ch == nil {
			return
		}
		<-ch
	}
}

// needsYouGate blocks a ticket finish while a checkpoint conflict needs
// User action without extending the existing pause behavior at this seam.
func (d *Daemon) needsYouGate(runID string) {
	for {
		d.mu.Lock()
		rs, ok := d.runs[runID]
		if !ok || rs.Status != runStatusNeedsYou {
			d.mu.Unlock()
			return
		}
		ch := rs.unpause
		d.mu.Unlock()
		if ch == nil {
			return
		}
		<-ch
	}
}

// pauseRun delegates the manual pause to the Session checkpoint path.
func (d *Daemon) pauseRun(w http.ResponseWriter, r *http.Request, id string) {
	d.pauseRunSessions(w, r, id)
}

// resumeRun continues a paused, failed or needs-you Run without starting a
// second Worker for a Ticket that already has one. Anything else is 409.
func (d *Daemon) resumeRun(w http.ResponseWriter, r *http.Request, id string) {
	out, code, msg := d.resumeRunCore(id)
	switch code {
	case 0:
		writeJSON(w, http.StatusOK, map[string]any{"run": out})
	case http.StatusNotFound:
		http.NotFound(w, r)
	case http.StatusConflict:
		writeJSON(w, code, map[string]any{"error": msg})
	default:
		http.Error(w, msg, code)
	}
}

// resumeRunCore resumes a paused, failed or needs-you Run without the HTTP:
// it answers the fresh snapshot, or a status code and message. A restarted
// Daemon calls it for Runs saved with ResumeOnRestart.
func (d *Daemon) resumeRunCore(id string) (any, int, string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		return nil, http.StatusNotFound, "not found"
	}
	if rs.Status != runStatusPaused && rs.Status != runStatusFailed && rs.Status != runStatusNeedsYou {
		d.mu.Unlock()
		return nil, http.StatusConflict, "run is not paused, failed or needs-you"
	}
	spec, repo, specBranch := rs.Spec, rs.Repo, rs.SpecBranch
	tickets := append([]staticTicket(nil), rs.Tickets...)
	d.mu.Unlock()
	specRun, err := el.ResolveSpec(spec)
	if err != nil {
		// The Run exists; its spec no longer resolves. That is Daemon
		// state, not a missing Run.
		return nil, http.StatusInternalServerError, err.Error()
	}
	files, err := el.ReadDriverFiles(specRun)
	if err != nil {
		return nil, http.StatusInternalServerError, err.Error()
	}
	// Ticket completion is authoritative on the integration branch: a final
	// Needs-you Resume must seed the driver from its done marks, not the stale
	// Status values in the main checkout.
	if specBranch != "" {
		for _, ticket := range tickets {
			rel, relErr := filepath.Rel(repo, ticket.Path)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if body, showErr := runGit(repo, "show", specBranch+":"+filepath.ToSlash(rel)); showErr == nil {
				files[ticket.Path] = body
			}
		}
	}
	max := d.MaxRunning()
	d.mu.Lock()
	rs, ok = d.runs[id]
	if !ok {
		d.mu.Unlock()
		return nil, http.StatusNotFound, "not found"
	}
	if rs.Status != runStatusPaused && rs.Status != runStatusFailed && rs.Status != runStatusNeedsYou {
		d.mu.Unlock()
		return nil, http.StatusConflict, "run is not paused, failed or needs-you"
	}
	rs.SpecRun = specRun
	preparePausedRunChainFailuresLocked(d, rs)
	rs.Status = runStatusRunning
	rs.ResumeOnRestart = false
	// A failed Run or final-gate Needs-you resumes from saved state: merged
	// Tickets and their Sessions stay, and the gate diagnostics clear.
	rs.Error = ""
	rs.FinalMergeFiles = nil
	if rs.unpause != nil {
		close(rs.unpause)
	}
	rs.unpause = make(chan struct{})
	var world el.DriverWorld
	startDriver := !rs.driving
	if startDriver {
		rs.driving = true
		world = d.webDriverWorld(rs, files, max)
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.reconcileRunChainFailures(id)
	if err := d.resumePausedRunRoles(id, ""); err != nil {
		d.mu.Lock()
		if rs := d.runs[id]; rs != nil {
			rs.Status = runStatusPaused
			rs.unpause = make(chan struct{})
			if startDriver {
				rs.driving = false
			}
			d.persistRun(rs)
			d.broadcastRunLocked(rs)
		}
		d.mu.Unlock()
		return nil, http.StatusConflict, err.Error()
	}
	out := d.runJSON(rs)
	if startDriver {
		go d.driveRun(rs.ID, world)
	}
	return out, 0, ""
}

// runTicketStatic names one Run ticket for the per-ticket actions. Callers
// hold d.mu.
func runTicketStatic(rs *runState, ticketID string) (staticTicket, bool) {
	for _, t := range rs.Tickets {
		if t.ID == ticketID {
			return t, true
		}
	}
	return staticTicket{}, false
}

// ticketActionLocked validates a per-ticket action: the Run exists, the
// ticket belongs to it and the Run is still live (running, paused or
// needs-you). A Needs-you Run allows ticket actions so unresolved Tickets
// can be retried, unparked or discarded before Resume. Parked and discarded
// tickets run no action except retry (which unparks) and idempotent repeats.
// Callers hold d.mu and get the failure status and message, or zero when the
// action may run.
func ticketActionLocked(rs *runState, ticketID, action string) (int, string) {
	if _, ok := runTicketStatic(rs, ticketID); !ok {
		return http.StatusNotFound, "unknown ticket"
	}
	if rs.Status != runStatusRunning && rs.Status != runStatusPaused && rs.Status != runStatusNeedsYou {
		return http.StatusConflict, "run is finished"
	}
	if rs.Merged[ticketID] != "" {
		return http.StatusConflict, "ticket is already merged"
	}
	if rs.Status == runStatusPaused && action == "retry" {
		return http.StatusConflict, "run is paused: resume it first, a retry does nothing while paused"
	}
	if rs.TicketPaused[ticketID] && (action == "steer" || action == "retry") {
		return http.StatusConflict, "ticket is paused"
	}
	if rs.Discarded[ticketID] && action != "discard" {
		return http.StatusConflict, "ticket is discarded"
	}
	return 0, ""
}

// writeTicketRun persists and publishes a ticket action, then releases d.mu
// before building the response snapshot because that includes git data.
// Callers hold d.mu on entry; this helper consumes the lock.
func (d *Daemon) writeTicketRun(w http.ResponseWriter, rs *runState) {
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	out := d.runJSON(rs)
	writeJSON(w, http.StatusOK, map[string]any{"run": out})
}

// steerTicket steers one ticket's worker through the driver tell, so the
// refusal the row shows stays in sync. Workers only: a ticket with no
// worker yet, or a finished one, is 409.
func (d *Daemon) steerTicket(w http.ResponseWriter, r *http.Request, id, ticketID string) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if code, msg := ticketActionLocked(rs, ticketID, "steer"); code != 0 {
		d.mu.Unlock()
		writeJSON(w, code, map[string]any{"error": msg})
		return
	}
	d.mu.Unlock()
	if err := d.runTellWorker(id, ticketID, req.Text, d.senderOf(r)); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	d.mu.Lock()
	rs, ok = d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	d.writeTicketRun(w, rs)
}

// retryTicket starts one ticket over: it clears the needs-you banner and
// any park, bumps the retry generation the driver honors on its next pass
// (dropping the backoff, the refusal streak and the stall-nudge count),
// and steers the optional text to the worker. The driver keeps polling on
// its 500ms cadence, so the retry runs at once.
func (d *Daemon) retryTicket(w http.ResponseWriter, r *http.Request, id, ticketID string) {
	var req struct {
		Text string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if code, msg := ticketActionLocked(rs, ticketID, "retry"); code != 0 {
		d.mu.Unlock()
		writeJSON(w, code, map[string]any{"error": msg})
		return
	}
	if rs.NeedsYouReason == nil {
		rs.NeedsYouReason = map[string]string{}
	}
	delete(rs.NeedsYouReason, ticketID)
	if rs.ChainExhausted != nil {
		delete(rs.ChainExhausted, ticketID)
	}
	if rs.pushedNeedsYou != nil {
		delete(rs.pushedNeedsYou, ticketID)
	}
	if rs.Blocked != nil {
		delete(rs.Blocked, ticketID)
	}
	if rs.Parked != nil {
		delete(rs.Parked, ticketID)
	}
	if rs.RetryGen == nil {
		rs.RetryGen = map[string]int{}
	}
	rs.RetryGen[ticketID]++
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	broadcast := d.runJSON(rs)
	// The retry itself landed above; only an optional steer can still
	// miss, and the snapshot below still answers it.
	steerErr := ""
	if strings.TrimSpace(req.Text) != "" {
		if err := d.runTellWorker(id, ticketID, req.Text, d.senderOf(r)); err != nil {
			steerErr = err.Error()
		}
	}
	if steerErr != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": steerErr, "run": broadcast})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": broadcast})
}

// parkTicket parks one ticket: the driver never starts or finishes it and
// only its dependents wait. The worktree and branch stay, so a retry
// resumes it where it stood. Repeats are idempotent.
func (d *Daemon) parkTicket(w http.ResponseWriter, r *http.Request, id, ticketID string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if code, msg := ticketActionLocked(rs, ticketID, "park"); code != 0 {
		d.mu.Unlock()
		writeJSON(w, code, map[string]any{"error": msg})
		return
	}
	if rs.Parked == nil {
		rs.Parked = map[string]bool{}
	}
	rs.Parked[ticketID] = true
	d.writeTicketRun(w, rs)
}

// discardTicket discards one ticket: its worktree and branch go away and
// the driver never touches it again. The Worker session stays for its
// transcript. Repeats are idempotent.
func (d *Daemon) discardTicket(w http.ResponseWriter, r *http.Request, id, ticketID string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if _, ok := runTicketStatic(rs, ticketID); !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusRunning && rs.Status != runStatusPaused && rs.Status != runStatusNeedsYou {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is finished"})
		return
	}
	if rs.Merged[ticketID] != "" {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "ticket is already merged"})
		return
	}
	repo, branch, worktree := rs.Repo, rs.Branches[ticketID], rs.Worktrees[ticketID]
	if rs.Discarded == nil {
		rs.Discarded = map[string]bool{}
	}
	rs.Discarded[ticketID] = true
	if rs.Parked != nil {
		delete(rs.Parked, ticketID)
	}
	d.writeTicketRun(w, rs)
	// Best effort like the start cleanup: the discard stands even when git
	// has nothing left to remove.
	if worktree != "" {
		_, _ = runGit(repo, "worktree", "remove", "--force", worktree)
	}
	if branch != "" {
		_, _ = runGit(repo, "branch", "-D", branch)
	}
}
