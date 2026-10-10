package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	el "example.com/emberloop"
)

// Run statuses: a Run is running while its driver loop polls, done when
// every Ticket merged, failed when the driver returned an error, and paused
// when loaded from disk without a driver (resume arrives in a later ticket).
const (
	runStatusRunning = "running"
	runStatusDone    = "done"
	runStatusFailed  = "failed"
	runStatusPaused  = "paused"
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
// reported and the branch tip it was given at, plus the findings. Report
// keeps the rendered `file:line severity: problem` lines for the refusal
// text; Findings keeps them structured for the rounds. A pass at an older
// tip is stale and does not count.
type storedReview struct {
	Verdict  string          `json:"verdict"`
	At       string          `json:"at"`
	Report   string          `json:"report,omitempty"`
	Findings []ReviewFinding `json:"findings,omitempty"`
}

// savedRow is one Ticket's driver row as the Run file keeps it: the state
// the driver last published, the branch tip and last refusal then, and when
// the Ticket entered the state. Live rows come from the Driver while it
// runs; these carry the table across a Daemon restart.
type savedRow struct {
	State       string    `json:"state"`
	Tip         string    `json:"tip,omitempty"`
	LastRefusal string    `json:"lastRefusal,omitempty"`
	Since       time.Time `json:"since"`
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
// the Run file, so the table survives a Daemon restart.
type runState struct {
	ID            string                             `json:"id"`
	Spec          string                             `json:"spec"`
	Repo          string                             `json:"repo"`
	Status        string                             `json:"status"`
	Harness       string                             `json:"harness"`
	Model         string                             `json:"model"`
	Effort        string                             `json:"effort,omitempty"`
	CreatedAt     time.Time                          `json:"createdAt"`
	Error         string                             `json:"error,omitempty"`
	Tickets       []staticTicket                     `json:"tickets"`
	Sessions      map[string]string                  `json:"sessions"`
	Branches      map[string]string                  `json:"branches"`
	Worktrees     map[string]string                  `json:"worktrees"`
	Targets       map[string]string                  `json:"targets"`
	Bases         map[string]string                  `json:"bases,omitempty"`
	Merged        map[string]string                  `json:"merged,omitempty"`
	Reviewers     map[string]map[string]string       `json:"reviewers,omitempty"`
	Reviews       map[string]map[string]storedReview `json:"reviews,omitempty"`
	ReviewTips    map[string]map[string]string       `json:"reviewTips,omitempty"`
	ReviewPending map[string]map[string][]string     `json:"reviewPending,omitempty"`
	Reprompted    map[string]int                     `json:"reprompted,omitempty"`
	// SubmitRecs keeps each Worker's last submit by ticket so a Daemon
	// restart does not reset the submit count below Reprompted.
	SubmitRecs map[string]submitRec `json:"submitRecs,omitempty"`
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
	// Parked holds the user's park per ticket from POST
	// /api/runs/{id}/tickets/{tid}/park: the driver never starts or
	// finishes a parked ticket and only its dependents wait. A retry
	// clears it.
	Parked map[string]bool `json:"parked,omitempty"`
	// Discarded holds the user's discard per ticket from POST
	// /api/runs/{id}/tickets/{tid}/discard: the ticket's worktree and
	// branch are removed and the driver never touches it again.
	Discarded map[string]bool `json:"discarded,omitempty"`
	// RetryGen is the user's retry generation per ticket from POST
	// /api/runs/{id}/tickets/{tid}/retry: each retry bumps it, and the
	// driver drops that ticket's needs-you mark, backoff, refusal streak
	// and stall-nudge count on its next pass.
	RetryGen map[string]int      `json:"retryGen,omitempty"`
	Rows     map[string]savedRow `json:"rows,omitempty"`
	SpecRun  el.SpecRun          `json:"-"`
	Driver   *el.DriverRun       `json:"-"`
	notify   chan struct{}       `json:"-"`
	unpause  chan struct{}       `json:"-"`
	driving  bool                `json:"-"`
}

// runPollInterval caps the driver sleep: the loop polls for Worker submits
// on this cadence instead of the CLI's slower poll.
const runPollInterval = 500 * time.Millisecond

// roleWorkerConfig reads the worker harness, model and effort from the
// settings roles. Empty when the role is missing, so the caller falls back
// to the Session default.
func roleWorkerConfig() (harness, model, effort string) {
	r, ok := readRoles()["worker"]
	if !ok || r.Harness == "" || r.Model == "" {
		return "", "", ""
	}
	if r.Harness == "claude" && !validClaudeModel(r.Model) {
		return "", "", ""
	}
	return r.Harness, r.Model, r.Effort
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

// runStagedDiff reports whether rel has staged changes: git diff --cached
// --quiet exits 0 with none, 1 with some. Other errors fail the finish.
func runStagedDiff(dir, rel string) (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--", rel)
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		return false, nil
	} else if _, ok := err.(*exec.ExitError); ok {
		return true, nil
	} else {
		return false, err
	}
}

// runGitCommitDone commits the ticket done mark, returning combined output
// with the error: git prints "nothing to commit" on stdout, which runGit
// drops, so the caller checks both.
func runGitCommitDone(dir, rel, workerID string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", "mark "+workerID+" done", "--", rel)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String() + stderr.String(), err
}

// handleRuns serves the Run collection: list Runs, or start one.
func (d *Daemon) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.mu.Lock()
		runs := make([]map[string]any, 0, len(d.runs))
		for _, rs := range d.runs {
			runs = append(runs, d.runJSONLocked(rs))
		}
		d.mu.Unlock()
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
	switch {
	case sub == "" && r.Method == "GET":
		d.mu.Lock()
		rs, ok := d.runs[id]
		if !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		out := d.runJSONLocked(rs)
		d.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"run": out})
	case sub == "events" && r.Method == "GET":
		d.serveRunEvents(w, r, id)
	case sub == "pause" && r.Method == "POST":
		d.pauseRun(w, r, id)
	case sub == "resume" && r.Method == "POST":
		d.resumeRun(w, r, id)
	case strings.HasPrefix(sub, "tickets/") && r.Method == "POST":
		rest := strings.TrimPrefix(sub, "tickets/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		switch parts[1] {
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
	var req struct {
		Spec    string `json:"spec"`
		Repo    string `json:"repo"`
		Harness string `json:"harness"`
		Model   string `json:"model"`
		Effort  string `json:"effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Spec == "" {
		http.Error(w, "spec is required", http.StatusBadRequest)
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
	harness, model, effort := roleWorkerConfig()
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
		if rs.Spec == specRun.Spec && rs.Status == runStatusRunning {
			d.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]any{"error": "a run is already active for this spec"})
			return
		}
	}
	pathByID := map[string]string{}
	for path := range files {
		if filepath.Dir(filepath.Clean(path)) == filepath.Clean(specRun.Tickets) && strings.HasSuffix(path, ".md") {
			pathByID[strings.TrimSuffix(filepath.Base(path), ".md")] = path
		}
	}
	rs := &runState{
		ID:             newID(),
		Spec:           specRun.Spec,
		Repo:           repo,
		Status:         runStatusRunning,
		Harness:        harness,
		Model:          model,
		Effort:         effort,
		CreatedAt:      time.Now().UTC(),
		Sessions:       map[string]string{},
		Branches:       map[string]string{},
		Worktrees:      map[string]string{},
		Targets:        map[string]string{},
		Bases:          map[string]string{},
		Merged:         map[string]string{},
		Reviewers:      map[string]map[string]string{},
		Reviews:        map[string]map[string]storedReview{},
		ReviewTips:     map[string]map[string]string{},
		ReviewPending:  map[string]map[string][]string{},
		Reprompted:     map[string]int{},
		ReviewStarted:  map[string]map[string]time.Time{},
		FormatReprompt: map[string]map[string]bool{},
		Rounds:         map[string][]ticketRound{},
		NeedsYouReason: map[string]string{},
		Followups:      map[string]string{},
		Rows:           map[string]savedRow{},
		SpecRun:        specRun,
		Driver:         el.NewDriverRun(nil),
		Parked:         map[string]bool{},
		Discarded:      map[string]bool{},
		RetryGen:       map[string]int{},

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
	out := d.runJSONLocked(rs)
	d.mu.Unlock()

	go d.driveRun(rs.ID, world)
	writeJSON(w, http.StatusCreated, map[string]any{"run": out})
}

// driveRun runs the core deterministic driver against the web-backed world
// to a stable state, then records the outcome on the Run.
func (d *Daemon) driveRun(id string, world el.DriverWorld) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	d.mu.Unlock()
	if !ok {
		return
	}
	res, err := el.RunDriver(rs.SpecRun, world)
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok = d.runs[id]
	if !ok {
		return
	}
	rs.driving = false
	d.snapshotRowsLocked(rs)
	if err != nil {
		rs.Status = runStatusFailed
		rs.Error = err.Error()
	} else {
		rs.Status = runStatusDone
		for _, m := range res.Merged {
			rs.Merged[m.ID] = m.SHA
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
		// runNow is the test seam for the idle and stall needs-you
		// alerts; nil reads the real clock through the driver's default.
		Now: d.runNow,
		// Parked and Discarded read the user's per-ticket actions every
		// pass, so parking or discarding mid-run takes effect at once;
		// RetryGen starts a retried ticket over on the next pass.
		Parked: func(ticketID string) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.Parked[ticketID]
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
		// HasWorker names started Tickets from the recorded Worker
		// Sessions, so a resumed Run never starts a second Worker for a
		// Ticket that already has one. MergedSHA names the merge sha of
		// an already-merged Ticket, so a resumed Run seeds it as merged
		// instead of finishing it again.
		HasWorker: func(ticketID string) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return rs.Sessions[ticketID] != ""
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
			return d.runTellWorker(rs.ID, ticketID, text)
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
			d.mu.Unlock()
			if path == "" {
				return boxes, nil
			}
			miss, err := el.TickDriverTicketFile(path, boxes)
			if err == nil {
				// Boxes are ticket-page state: publish the tick at once
				// so the UI never polls for it.
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
			if wait > runPollInterval {
				wait = runPollInterval
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
const workerReviewNote = "Do not spawn reviewers: after you submit done, the Daemon starts the spec and standards reviewers itself and sends you their findings. Spawn scouts only for read-only research."

// webStallNudge is the fixed nudge for a Worker that went idle with no submit.
const webStallNudge = "Nudge: you have not submitted. Finish the ticket and submit done with the completed checkboxes; the reviewers start after your submit."

// reviewerPrompt asks one reviewer type to judge the Ticket's worker branch
// at the given tip and reply with the verdict. Reviewers judge only: they
// never edit the worktree and never use the submit tool. The reply is the
// final assistant message of this turn: one own-line `VERDICT:
// pass|changes`, then one finding per line as `file:line severity:
// problem` (severity blocker|major|minor; a problem may continue on
// indented lines). The Daemon records the verdict at the prompted tip, so
// the reply never names it. Only the reply to this prompt counts.
func reviewerPrompt(ticketID, spec, reviewerType, branch, tip string) string {
	axis := "correctness and this repo's standards"
	if reviewerType == reviewerSpecType {
		axis = "fit to the ticket and the spec"
	}
	brief := standardsBrief
	if reviewerType == reviewerSpecType {
		brief = specBrief
	}
	return fmt.Sprintf("Review ticket %s for spec %s as %s: judge %s on branch %s at tip %s. Do not edit files; judge only. Do not use the submit tool. Reply with your verdict as the final message of this turn: one own line `VERDICT: pass` or `VERDICT: changes`, then one finding per line as `file:line severity: problem` (severity blocker, major or minor; a problem may continue on following indented lines). A pass needs no findings.\n\n%s", ticketID, spec, reviewerType, axis, branch, tip, brief)
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

// handleReviewerTurnEnd records one reviewer turn's verdict from its final
// assistant message. Only the reply to the gate prompt counts: the tail
// after the last prompt. A parseable reply lands at the prompted tip (the
// oldest owed prompt, so a mid-review worker commit reads stale, never
// fresh). An unparseable reply re-prompts the same reviewer once for the
// format; a second failure marks the ticket needs-you for the user.
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
	rs.Reviews[ticketID][typ] = storedReview{Verdict: parsed.Verdict, At: tip, Report: report, Findings: append([]ReviewFinding(nil), parsed.Findings...)}
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
		retell = reviewerPrompt(ticketID, spec, typ, branch, cur)
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
// (heldFromRound) instead of a rerun; their types need no new prompt.
// Callers hold d.mu. It returns the new round number and the types still
// needing a prompt.
func appendRoundLocked(rs *runState, ticketID, tip string, now time.Time) (int, map[string]bool) {
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
	}
	for _, typ := range requiredReviewerTypes {
		if _, ok := rd.Reviews[typ]; !ok {
			need[typ] = true
		}
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
// done again, so they keep their context across Passes. Each prompt queues
// its tip as owed; a verdict answers the oldest queued tip, so a Worker
// commit between prompts never lets a T1 judgement read fresh at T2. It
// spawns nothing before the first done submit. A refused live-Session cap
// leaves the missing reviewers for the next pass, which retries, as does a
// branch tip it cannot read. Reviewer starts never fail the Run: the driver
// keeps polling and the next pass tries again. The submit route runs this
// sync (bounded to two starts plus two steers) so prompts queue in submit
// order and the driver's next finish sees the re-prompt already out.
func (d *Daemon) ensureReviewers(runID, ticketID string) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return
	}
	// A paused Run spawns no Reviewers: the driver is frozen and the
	// Resume's driver re-runs this for every done submit it finds.
	if rs.Status == runStatusPaused {
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
	_, need := appendRoundLocked(rs, ticketID, tip, now)
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
	d.mu.Unlock()
	for _, typ := range requiredReviewerTypes {
		if !need[typ] {
			continue
		}
		if sess := existing[typ]; sess != "" {
			_ = d.runTellSession(sess, reviewerPrompt(ticketID, spec, typ, branch, tip))
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
	// A paused Run spawns no Reviewers: fail fast so the driver's next
	// pass retries after the Resume.
	if rs.Status == runStatusPaused {
		d.mu.Unlock()
		return fmt.Errorf("run is paused")
	}
	branch, worktree := rs.Branches[ticketID], rs.Worktrees[ticketID]
	spec := rs.Spec
	harness, model := rs.Harness, rs.Model
	if rh, rm := roleReviewerConfig(reviewerType); rh != "" {
		harness, model = rh, rm
	}
	d.mu.Unlock()
	if branch == "" || worktree == "" {
		return fmt.Errorf("ticket %s has no worker", ticketID)
	}
	meta := SessionMeta{
		ID:        newID(),
		Harness:   harness,
		Model:     model,
		WorkDir:   worktree,
		Status:    "idle",
		CreatedAt: time.Now().UTC(),
		ParentID:  runID,
	}
	var be Backend
	be, serr := d.startSession(meta)
	if serr != nil {
		return fmt.Errorf("could not start %s for %s: %s", reviewerType, ticketID, serr.msg)
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
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	d.persistResumeToken(be, meta.ID)
	d.appendEvent(meta.ID, "status", "", "idle")
	prompt := reviewerPrompt(ticketID, spec, reviewerType, branch, tip)
	d.mu.Lock()
	if s, ok := d.sessions[meta.ID]; ok {
		s.busy = true
		s.active++
	}
	d.mu.Unlock()
	d.appendModeEvent(meta.ID, "prompt", prompt, "", "")
	go d.runTurn(meta.ID, be, prompt, nil)
	return nil
}

// runStartWorker creates the Ticket's worktree and branch and its Worker
// Session, a Subagent of the Run, and runs the Ticket prompt on it. A
// refused live-Session cap retries briefly before failing the Run.
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
	repo, harness, model := rs.Repo, rs.Harness, rs.Model
	workerID := el.WorkerID(tk.Path)
	branch := "implement/" + workerID
	worktree := filepath.Join(repo, ".el-worktrees", workerID)
	d.mu.Unlock()

	cleanup := func() {
		_, _ = runGit(repo, "worktree", "remove", "--force", worktree)
		_, _ = runGit(repo, "branch", "-D", branch)
	}
	// A Pause landing anywhere below never records a Worker: the
	// record-time check drops the attempt and waits for the Resume, then
	// the start runs again from a clean worktree.
	for {
		target, err := runGit(repo, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			target = ""
		}
		base := "HEAD"
		if sha, err := runGit(repo, "rev-parse", "HEAD"); err == nil {
			base = sha
		} else {
			return fmt.Errorf("could not read HEAD in %s: %w", repo, err)
		}
		if _, err := runGit(repo, "worktree", "add", "-b", branch, worktree, base); err != nil {
			return err
		}
		meta := SessionMeta{
			ID:        newID(),
			Harness:   harness,
			Model:     model,
			WorkDir:   worktree,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
			ParentID:  runID,
		}
		var be Backend
		for i := 0; ; i++ {
			var serr *startError
			be, serr = d.startSession(meta)
			if serr == nil {
				break
			}
			if serr.code != http.StatusTooManyRequests || i >= 60 {
				cleanup()
				return fmt.Errorf("could not start worker for %s: %s", tk.ID, serr.msg)
			}
			// Freeze the cap wait while paused instead of burning retries.
			d.pauseGate(runID)
			time.Sleep(runPollInterval)
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
		if rs.Status == runStatusPaused {
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
		prompt := fmt.Sprintf("Implement ticket %s for spec %s. Ticket file: %s. Work on branch %s in this worktree. When the work is done, submit done with the completed checkboxes. %s", tk.ID, spec, tk.Path, branch, workerReviewNote)
		d.mu.Lock()
		if s, ok := d.sessions[meta.ID]; ok {
			s.busy = true
			s.active++
		}
		d.mu.Unlock()
		d.appendModeEvent(meta.ID, "prompt", prompt, "", "")
		go d.runTurn(meta.ID, be, prompt, nil)
		return nil
	}
}

// runTellWorker steers the Ticket's Worker Session with the driver's text:
// a refused merge goes back to the Worker unchanged.
func (d *Daemon) runTellWorker(runID, ticketID, text string) error {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("unknown run %q", runID)
	}
	sessionID := rs.Sessions[ticketID]
	d.mu.Unlock()
	if sessionID == "" {
		return fmt.Errorf("ticket %s has no worker", ticketID)
	}
	if code, msg := d.resumeReleased(sessionID); code != 0 {
		return fmt.Errorf("could not reach worker for %s: %s", ticketID, msg)
	}
	if _, code := d.routeLive(sessionID, "steer", text); code != http.StatusAccepted {
		return fmt.Errorf("could not reach worker for %s", ticketID)
	}
	// The tell carries the refusal the row shows: write the pass now, so
	// the file holds it even when the next pass never sleeps.
	d.snapshotAndPersist(runID)
	return nil
}

// runFinishWorker merges a submitted done through the reviewer gate and the
// shared core checks: without both Passes at the current tip, or with a
// dirty worktree or a rebase conflict, the Worker stays alive with the
// refusal. A Ticket with no verdict on the current tip is still under
// review, not refused:
// the finish reports nothing and the driver polls again. The gate judges
// the pre-rebase tip: the rebase replays the reviewed commits onto the
// target without changing their content. On a merge the
// ticket is marked done and committed on the target, and the worktree is
// removed; the Worker Session stays for its transcript.
func (d *Daemon) runFinishWorker(runID, ticketID string) (el.DriverFinish, error) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return el.DriverFinish{}, fmt.Errorf("unknown run %q", runID)
	}
	repo, branch, worktree, target, _, path := rs.Repo, rs.Branches[ticketID], rs.Worktrees[ticketID], rs.Targets[ticketID], rs.Bases[ticketID], ticketPath(rs, ticketID)
	submitStatus := ""
	if rec, ok := d.submits[rs.Sessions[ticketID]]; ok && rec != nil {
		submitStatus = rec.Status
	}
	workerID := el.WorkerID(ticketID)
	d.mu.Unlock()
	if branch == "" || worktree == "" {
		return el.DriverFinish{}, fmt.Errorf("ticket %s has no worker", ticketID)
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
		d.mu.Lock()
		if rs, ok := d.runs[runID]; ok {
			if rounds := rs.Rounds[ticketID]; len(rounds) > 0 {
				idx := len(rounds) - 1
				for i := len(rounds) - 1; i >= 0; i-- {
					if rounds[i].Tip == tip {
						idx = i
						break
					}
				}
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
				now := time.Now().UTC()
				rs.Rounds[ticketID][idx].Declined = declined
				rs.Rounds[ticketID][idx].SentToWorkerAt = &now
				d.persistRun(rs)
				d.broadcastRunLocked(rs)
			}
		}
		d.mu.Unlock()
		return el.DriverFinish{Merged: false, Detail: why}, nil
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
		changed, err := el.MarkDriverTicketDone(path)
		if err != nil {
			// A ticket without a Status line is not one finish completes,
			// the way the CLI completion leaves it alone: the merge still
			// stands and the boxes already ticked stay.
			if !strings.Contains(err.Error(), "has no status line") {
				return el.DriverFinish{}, err
			}
		} else if changed {
			rel, err := filepath.Rel(repo, path)
			if err != nil {
				return el.DriverFinish{}, err
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return el.DriverFinish{}, fmt.Errorf("ticket %s is outside the repo", ticketID)
			}
			if _, err := runGit(repo, "add", "--", rel); err != nil {
				return el.DriverFinish{}, err
			}
			if staged, err := runStagedDiff(repo, rel); err != nil {
				return el.DriverFinish{}, err
			} else if staged {
				if out, err := runGitCommitDone(repo, rel, workerID); err != nil {
					if !strings.Contains(out+err.Error(), "nothing to commit") {
						return el.DriverFinish{}, err
					}
				}
			}
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
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	}
	d.mu.Unlock()
	return el.DriverFinish{Merged: true, SHA: sha}, nil
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

// runJSONLocked renders one Run with its live Ticket table: state, Worker
// Session link, both Reviewer verdicts with tip freshness, and the last
// refusal per Ticket. Callers hold d.mu.
// runJSONLocked renders one Run with its live Ticket table: state,
// Worker Session link, both Reviewer verdicts with tip freshness, and the
// last refusal per Ticket, plus everything the ticket page needs without
// polling: boxes/body/title re-read from the ticket file at snapshot time,
// branch, tip, state entry time, needs-you reason, follow-ups, agents
// (worker, both reviewers, worker-spawned scouts), the gate and the
// append-only rounds. Callers hold d.mu.
func (d *Daemon) runJSONLocked(rs *runState) map[string]any {
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
		if needsYou != "" && state != "merged" && state != "discarded" && state != "parked" && state != "needs-you" {
			// A reviewer-format failure needs the user while the driver
			// still shows the ticket running: surface it as needs-you.
			state = "needs-you"
		}
		// A user park or discard shows at once, ahead of the driver's
		// next pass publishing the same row.
		if state != "merged" {
			if rs.Discarded[t.ID] {
				state = "discarded"
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
		body := readTicketFile(ticketPath(rs, t.ID))
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
			"rounds": rounds,
		})
	}
	return map[string]any{
		"id": rs.ID, "spec": rs.Spec, "title": el.SpecTitle(rs.Spec), "repo": rs.Repo, "status": rs.Status,
		"harness": rs.Harness, "model": rs.Model, "effort": rs.Effort,
		"createdAt": rs.CreatedAt, "error": rs.Error, "tickets": tickets,
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

// serveRunEvents streams the live Ticket table over SSE, resending the Run
// whenever the driver publishes new rows.
func (d *Daemon) serveRunEvents(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
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
		d.mu.Lock()
		rs, ok = d.runs[id]
		var body string
		var ch chan struct{}
		if ok {
			body = mustJSON(d.runJSONLocked(rs))
			ch = rs.notify
		}
		d.mu.Unlock()
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
// checkbox line texts) or a Reviewer's submit (pass or changes with the
// findings in the report). The driver loop reads both: it ticks the boxes
// and merges a done submit once the gate passes. A Worker's done submit
// also spawns its Reviewers, once, and re-prompts them on later dones.
// handleSessionSubmit records a Worker's submit: its status, completed
// checkbox line texts and submit summary (follow-ups come from the summary
// after Follow-ups:). The driver loop ticks the boxes and merges a done
// submit once the gate passes. A Worker's done submit also spawns its
// Reviewers, once, and re-prompts them on later dones. Reviewers never
// submit: they reply with VERDICT in their turn, so a submit to a reviewer
// session is 409. Workers keep submit.
func (d *Daemon) handleSessionSubmit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Status    string   `json:"status"`
		Completed []string `json:"completed"`
		Report    string   `json:"report"`
		Summary   string   `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		http.Error(w, "status is required", http.StatusBadRequest)
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
		writeJSON(w, http.StatusConflict, map[string]any{"error": "reviewer sessions reply with VERDICT: pass|changes in their turn, not submit"})
		return
	}
	rec := d.submits[id]
	if rec == nil {
		rec = &submitRec{}
		d.submits[id] = rec
	}
	rec.Count++
	rec.Status = req.Status
	rec.Completed = append([]string(nil), req.Completed...)
	rec.Report = req.Report
	rec.Summary = req.Summary
	count := rec.Count
	var workerRun, workerTicket string
	if rec.Status == "done" {
		workerRun, workerTicket = d.findWorkerLocked(id)
		if workerRun != "" {
			if rs, ok := d.runs[workerRun]; ok {
				if rs.Followups == nil {
					rs.Followups = map[string]string{}
				}
				rs.Followups[workerTicket] = parseFollowups(req.Summary)
				d.persistRun(rs)
				d.broadcastRunLocked(rs)
			}
		}
	}
	if workerRun == "" {
		workerRun, workerTicket = d.findWorkerLocked(id)
	}
	if rs, ok := d.runs[workerRun]; ok && workerRun != "" {
		// A count reset by a Daemon restart must still outrun what the
		// reviewers were already re-prompted for.
		if n := rs.Reprompted[workerTicket]; rec.Count <= n {
			rec.Count = n + 1
			count = rec.Count
		}
		if rs.SubmitRecs == nil {
			rs.SubmitRecs = map[string]submitRec{}
		}
		rs.SubmitRecs[workerTicket] = *rec
		d.persistRun(rs)
	}
	d.mu.Unlock()
	d.appendEvent(id, "submit", req.Status, "")
	if workerRun != "" && req.Status == "done" {
		// Sync so the driver's next finish sees the re-prompt already out,
		// never a stale refusal first, and prompts queue in submit order.
		// Bounded to two reviewer starts plus two steers; the cap refuses
		// fast and Tell is best effort, so the ack waits on at most that.
		d.ensureReviewers(workerRun, workerTicket)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"submits": count})
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
		if rs.Parked == nil {
			rs.Parked = map[string]bool{}
		}
		if rs.Discarded == nil {
			rs.Discarded = map[string]bool{}
		}
		if rs.RetryGen == nil {
			rs.RetryGen = map[string]int{}
		}
		if rs.Rows == nil {
			rs.Rows = map[string]savedRow{}
		}
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
		rs.Rows[row.ID] = savedRow{State: string(row.State), Tip: row.Tip, LastRefusal: row.LastRefusal, Since: row.Since}
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

// pauseGate blocks while the Run is paused and returns once it runs again.
// It holds no lock while waiting, so Pause and Resume never block on the
// driver. An unknown Run returns at once; its caller reports it.
func (d *Daemon) pauseGate(runID string) {
	for {
		d.mu.Lock()
		rs, ok := d.runs[runID]
		if !ok || rs.Status != runStatusPaused {
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

// pauseRun freezes a running Run: its driver starts nothing new while
// paused (the next start and the next poll both wait) and running Worker
// Sessions stay live and untouched. Anything else is 409.
func (d *Daemon) pauseRun(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusRunning {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is not running"})
		return
	}
	rs.Status = runStatusPaused
	d.persistRun(rs)
	out := d.runJSONLocked(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"run": out})
}

// resumeRun continues a paused Run without starting a second Worker for a
// Ticket that already has one: started Tickets keep their Sessions and
// merged ones seed from their shas. Anything else is 409.
func (d *Daemon) resumeRun(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusPaused {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is not paused"})
		return
	}
	spec := rs.Spec
	d.mu.Unlock()
	specRun, err := el.ResolveSpec(spec)
	if err != nil {
		// The Run exists; its spec no longer resolves. That is Daemon
		// state, not a missing Run.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	files, err := el.ReadDriverFiles(specRun)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	max := d.MaxRunning()
	d.mu.Lock()
	rs, ok = d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status != runStatusPaused {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is not paused"})
		return
	}
	rs.SpecRun = specRun
	rs.Status = runStatusRunning
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
	out := d.runJSONLocked(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	if startDriver {
		go d.driveRun(rs.ID, world)
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": out})
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
// ticket belongs to it and the Run is still live (running or paused).
// Parked and discarded tickets run no action except retry (which unparks)
// and idempotent repeats. Callers hold d.mu and get the failure status and
// message, or zero when the action may run.
func ticketActionLocked(rs *runState, ticketID, action string) (int, string) {
	if _, ok := runTicketStatic(rs, ticketID); !ok {
		return http.StatusNotFound, "unknown ticket"
	}
	if rs.Status != runStatusRunning && rs.Status != runStatusPaused {
		return http.StatusConflict, "run is finished"
	}
	if rs.Discarded[ticketID] && action != "discard" {
		return http.StatusConflict, "ticket is discarded"
	}
	return 0, ""
}

// writeTicketRun answers a per-ticket action with the fresh snapshot;
// callers hold d.mu.
func (d *Daemon) writeTicketRun(w http.ResponseWriter, rs *runState) {
	d.persistRun(rs)
	out := d.runJSONLocked(rs)
	d.broadcastRunLocked(rs)
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
	if err := d.runTellWorker(id, ticketID, req.Text); err != nil {
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
	d.mu.Unlock()
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
	if rs.pushedNeedsYou != nil {
		delete(rs.pushedNeedsYou, ticketID)
	}
	if rs.Parked != nil {
		delete(rs.Parked, ticketID)
	}
	if rs.RetryGen == nil {
		rs.RetryGen = map[string]int{}
	}
	rs.RetryGen[ticketID]++
	d.persistRun(rs)
	broadcast := d.runJSONLocked(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	// The retry itself landed above; only an optional steer can still
	// miss, and the snapshot below still answers it.
	steerErr := ""
	if strings.TrimSpace(req.Text) != "" {
		if err := d.runTellWorker(id, ticketID, req.Text); err != nil {
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
	d.mu.Unlock()
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
	if rs.Status != runStatusRunning && rs.Status != runStatusPaused {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is finished"})
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
	d.mu.Unlock()
	// Best effort like the start cleanup: the discard stands even when git
	// has nothing left to remove.
	if worktree != "" {
		_, _ = runGit(repo, "worktree", "remove", "--force", worktree)
	}
	if branch != "" {
		_, _ = runGit(repo, "branch", "-D", branch)
	}
}
