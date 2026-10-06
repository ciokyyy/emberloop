// Package tui is the wick dashboard: a bubbletea terminal UI in its own Go
// module so the core stays dependency-free. The deep module is the Snapshot
// read model — repos holding their specs, each with run status, ticket
// progress and per-ticket status — which later tickets extend with agents,
// cost and memory. The bubbletea model polls Snapshot every second.
package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Herdr is the liveness seam Snapshot reads through: every herdr pane still
// alive, keyed by pane id with its agent status (`idle`, `working`,
// `blocked`, `done`, `unknown`, or "" when the list carries none). Tests pass
// a fake; the binary passes one that runs `herdr pane list`. A pane missing
// from the map is gone, never running.
type Herdr func() (map[string]string, error)

// Snap is the Runs tab's read model: every known repo with its specs.
// Known repos are the run records' repos plus the current repo, current first;
// every other repo and every spec inside one sorts by path and slug. Specs
// with no run yet are discovered from the repos' spec directories on disk, so
// waiting work is visible; a recorded run whose spec file is gone stays listed
// under its repo.
type Snap struct {
	Repos []RepoSnap
}

// RepoSnap is one repo's specs.
type RepoSnap struct {
	Path  string
	Specs []SpecSnap
}

// SpecSnap is one spec's run status, ticket progress and tickets. HasRun says
// a run record exists; Running says its orchestrator pane is still alive in
// herdr. Merged/Total is merged tickets over ticket files. Tickets lists
// every ticket file with its derived status, in filename order.
type SpecSnap struct {
	Slug    string
	Spec    string
	Branch  string
	HasRun  bool
	Running bool
	Merged  int
	Total   int
	Tickets []TicketSnap
}

// TicketStatus is one ticket's derived status. The derivation lives in one
// function, deriveTicketStatus, from worker entries, herdr pane state,
// submits, reviewer records and history.
type TicketStatus string

const (
	TicketNotStarted       TicketStatus = "not-started"
	TicketWorking          TicketStatus = "working"
	TicketRetrying         TicketStatus = "retrying"
	TicketIdle             TicketStatus = "idle"
	TicketSubmittedDone    TicketStatus = "submitted-done"
	TicketSubmittedBlocked TicketStatus = "submitted-blocked"
	TicketInReview         TicketStatus = "in-review"
	TicketMerged           TicketStatus = "merged"
	TicketDiscarded        TicketStatus = "discarded"
	TicketFailed           TicketStatus = "failed"
)

// TicketSnap is one ticket file with its derived status. CodeVerdict and
// SpecVerdict carry the reviewers' verdicts while in review (`pass`,
// `changes`, or "" while pending); they are "" in every other status.
type TicketSnap struct {
	Path        string
	Name        string
	Status      TicketStatus
	CodeVerdict string
	SpecVerdict string
}

// workerRecord is the slice of workers.json Snapshot needs. The map key is
// the worker id; Ticket is the ticket file it was spawned from, Spec the
// spec path, Pane its herdr pane, Worktree its git worktree (the reviewers
// key). Unknown fields are ignored.
type workerRecord struct {
	Ticket   string `json:"ticket"`
	Spec     string `json:"spec"`
	Pane     string `json:"pane"`
	Worktree string `json:"worktree"`
}

// submitRecord is the slice of a submit file Snapshot needs: the status the
// agent chose (`done` or `blocked` for a worker).
type submitRecord struct {
	Status string `json:"status"`
}

// reviewerRecord is the slice of reviewers.json Snapshot needs: the verdict
// the reviewer last submitted (`pass` or `changes`, "" while pending).
type reviewerRecord struct {
	Verdict string `json:"verdict"`
}

// runRecord is the slice of runs.json Snapshot needs. Unknown fields are
// ignored, so the core can grow the record without breaking this read.
type runRecord struct {
	Slug   string `json:"slug"`
	Spec   string `json:"spec"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Pane   string `json:"pane"`
}

// historyEntry is the slice of a run's ticket history Snapshot needs.
type historyEntry struct {
	Ticket  string `json:"ticket"`
	Outcome string `json:"outcome"`
}

// Snapshot reads the state dir's run records and ticket histories plus the
// repos' spec directories, deciding running from live herdr panes. A herdr
// failure is returned — the caller keeps its last snapshot and retries —
// while a missing or unreadable state file reads as no runs.
//
// currentRepo is a third parameter beside the spec's Snapshot(stateDir,
// herdr) seam: the pure read model needs the current checkout to order it
// first and to discover specs with no run yet, and herdr liveness carries
// no repo notion. Callers resolve it with CurrentRepo.
func Snapshot(stateDir, currentRepo string, herdr Herdr) (Snap, error) {
	live, err := herdr()
	if err != nil {
		return Snap{}, err
	}
	current := normalizeRepo(currentRepo)
	runs := loadRunRecords(stateDir)
	mergedBySlug := loadMergedBySlug(stateDir, runs)

	byRepo := map[string]map[string]*SpecSnap{}
	ensure := func(repo, slug string) *SpecSnap {
		if byRepo[repo] == nil {
			byRepo[repo] = map[string]*SpecSnap{}
		}
		s := byRepo[repo][slug]
		if s == nil {
			s = &SpecSnap{Slug: slug}
			byRepo[repo][slug] = s
		}
		return s
	}
	// Candidate roots are every normalized record repo plus the current
	// checkout. A run homes to the deepest root containing its spec file,
	// so a record launched from a subdirectory still groups with its
	// checkout no matter which repo is current.
	roots := map[string]bool{}
	for _, r := range runs {
		if nr := normalizeRepo(r.Repo); nr != "" {
			roots[nr] = true
		}
	}
	if current != "" {
		roots[current] = true
	}
	repoOf := func(r runRecord) string {
		best := ""
		for root := range roots {
			if specUnderRepo(r.Spec, root) && len(root) > len(best) {
				best = root
			}
		}
		if best != "" {
			return best
		}
		// No candidate root contains the spec: derive it from the standard
		// layout itself (<root>/.scratch/<dir>/spec.md), so a repo whose
		// runs were all recorded from subdirectories still groups. The
		// .scratch check keeps a stale spec path from homing to a phantom
		// root; those runs fall through to the recorded repo below.
		if root := specRepoRoot(r.Spec); root != "" {
			if fi, err := os.Stat(filepath.Join(root, ".scratch")); err == nil && fi.IsDir() {
				return normalizeRepo(root)
			}
		}
		if nr := normalizeRepo(r.Repo); nr != "" {
			return nr
		}
		return current
	}
	for _, r := range runs {
		s := ensure(repoOf(r), r.Slug)
		s.Spec, s.Branch = r.Spec, r.Branch
		s.HasRun = true
		_, alive := live[r.Pane]
		s.Running = r.Pane != "" && alive
	}
	for repo := range byRepo {
		discoverSpecs(repo, byRepo[repo])
	}
	if current != "" {
		if byRepo[current] == nil {
			byRepo[current] = map[string]*SpecSnap{}
		}
		discoverSpecs(current, byRepo[current])
	}
	for _, specs := range byRepo {
		for slug, s := range specs {
			merged, total := specProgress(s, slug, mergedBySlug)
			s.Merged, s.Total = merged, total
		}
	}
	workers := loadWorkers(stateDir)
	submits := loadSubmits(stateDir)
	reviewers := loadReviewerRecords(stateDir)
	terminals := loadTerminalsBySlug(stateDir, byRepo)
	for _, specs := range byRepo {
		for slug, s := range specs {
			s.Tickets = specTickets(s, live, workers, submits, reviewers, terminals[slug])
		}
	}

	repos := make([]RepoSnap, 0, len(byRepo))
	for repo, specs := range byRepo {
		list := make([]SpecSnap, 0, len(specs))
		for _, s := range specs {
			list = append(list, *s)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Slug < list[j].Slug })
		repos = append(repos, RepoSnap{Path: repo, Specs: list})
	}
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Path == current {
			return true
		}
		if repos[j].Path == current {
			return false
		}
		return repos[i].Path < repos[j].Path
	})
	return Snap{Repos: repos}, nil
}

// normalizeRepo cleans a repo path for grouping: symlinks resolved
// best-effort (a record may point at a moved checkout), lexical clean
// always. Empty stays empty, so callers can tell "no repo" apart.
func normalizeRepo(p string) string {
	if p == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}

// specRepoRoot derives the repo root from a spec path in the standard
// layout: <root>/.scratch/<dir>/spec.md. Anything else returns "", and
// callers check the root is real before trusting it.
func specRepoRoot(spec string) string {
	if filepath.Base(spec) != "spec.md" {
		return ""
	}
	scratch := filepath.Dir(filepath.Dir(spec))
	if filepath.Base(scratch) != ".scratch" {
		return ""
	}
	return filepath.Dir(scratch)
}

// specUnderRepo reports whether a spec file sits under a repo root, so a
// run recorded from a subdirectory still groups with its checkout.
func specUnderRepo(spec, repo string) bool {
	if spec == "" || repo == "" {
		return false
	}
	rel, err := filepath.Rel(repo, normalizeRepo(spec))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// specDirSlug keys a discovered spec directory the way the core's specSlug
// keys a run record, so the two meet on one key. Keep in sync with the
// core: lowercase, anything outside [a-z0-9-_.] becomes '-', trimmed.
func specDirSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// loadRunRecords reads runs.json keyed by spec slug. A missing file is no
// runs; a corrupt one reads as no runs too — the TUI shows waiting specs
// rather than failing the whole dashboard over one bad write.
func loadRunRecords(stateDir string) map[string]runRecord {
	raw, err := os.ReadFile(filepath.Join(stateDir, "runs.json"))
	if err != nil {
		return nil
	}
	var store map[string]runRecord
	if err := json.Unmarshal(raw, &store); err != nil {
		return nil
	}
	return store
}

// loadMergedBySlug collects, per run slug, the basenames of tickets whose
// history outcome is merged. Ticket paths are matched by basename because the
// record may hold an absolute path, a repo-relative one, or just the file.
func loadMergedBySlug(stateDir string, runs map[string]runRecord) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for slug := range runs {
		raw, err := os.ReadFile(filepath.Join(stateDir, "history", slug+".json"))
		if err != nil {
			continue
		}
		var entries []historyEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if e.Outcome != "merged" || e.Ticket == "" {
				continue
			}
			if out[slug] == nil {
				out[slug] = map[string]bool{}
			}
			out[slug][filepath.Base(e.Ticket)] = true
		}
	}
	return out
}

// discoverSpecs adds every spec directory on disk under repo to specs, keyed
// by slug. A spec directory is a directory holding spec.md directly under the
// repo's .scratch tree. Entries a run record already named keep the record's
// spec path and branch; only the missing pieces fill in.
func discoverSpecs(repo string, specs map[string]*SpecSnap) {
	root := filepath.Join(repo, ".scratch")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		spec := filepath.Join(root, e.Name(), "spec.md")
		if fi, err := os.Stat(spec); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		slug := specDirSlug(e.Name())
		s := specs[slug]
		if s == nil {
			s = &SpecSnap{Slug: slug}
			specs[slug] = s
		}
		if s.Spec == "" {
			s.Spec = spec
		}
		if s.Branch == "" {
			s.Branch = "spec/" + slug
		}
	}
}

// specProgress counts a spec's ticket files and how many are merged: on-disk
// files whose status reads done, or whose basename has a merged history
// entry. Counting only on-disk files keeps merged <= total even when a file
// was deleted after its merge.
func specProgress(s *SpecSnap, slug string, mergedBySlug map[string]map[string]bool) (merged, total int) {
	files := ticketFiles(s)
	mergedSet := mergedBySlug[slug]
	for _, f := range files {
		total++
		base := filepath.Base(f)
		if mergedSet[base] {
			merged++
			continue
		}
		if raw, err := os.ReadFile(f); err == nil && ticketDone(string(raw)) {
			merged++
		}
	}
	return merged, total
}

// ticketFiles lists a spec's ticket Markdown: the .md files in its issues
// directory when there is one, else the .md files beside the spec file other
// than the spec file itself.
func ticketFiles(s *SpecSnap) []string {
	if s.Spec == "" {
		return nil
	}
	dir := filepath.Dir(s.Spec)
	if fi, err := os.Stat(filepath.Join(dir, "issues")); err == nil && fi.IsDir() {
		dir = filepath.Join(dir, "issues")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if p == s.Spec {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ticketDone reports whether a ticket file's status header reads done. Only
// the header field counts, the way finish completes it; a "Status:" mentioned
// in the body is still skipped because only a line that starts with the field
// counts.
func ticketDone(raw string) bool {
	done := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Status:") && !strings.HasPrefix(trimmed, "**Status:") {
			continue
		}
		i := strings.Index(line, "Status:")
		rest := strings.Trim(strings.TrimLeft(line[i+len("Status:"):], "* \t"), " \t")
		done = strings.EqualFold(rest, "done")
	}
	return done
}

// Reviewer type names, kept in sync with the core's built-in agent types:
// the TUI never imports the core, so these are strings, not references.
const (
	reviewerStandardsType = "reviewer-standards"
	reviewerSpecType      = "reviewer-spec"
)

func specTickets(s *SpecSnap, live map[string]string, workers map[string]workerRecord, submits map[string]submitRecord, reviewers map[string]map[string]reviewerRecord, terminals map[string]string) []TicketSnap {
	files := ticketFiles(s)
	out := make([]TicketSnap, 0, len(files))
	for _, f := range files {
		base := filepath.Base(f)
		worker, workerID := workerForTicket(workers, s.Spec, base)
		var submit *submitRecord
		var revs map[string]reviewerRecord
		var paneAlive bool
		var paneStatus string
		if worker != nil {
			if st, ok := live[worker.Pane]; ok {
				paneAlive, paneStatus = true, st
			}
			if sub, ok := submits[workerID]; ok {
				c := sub
				submit = &c
			}
			if worker.Worktree != "" {
				revs = reviewers[filepath.Clean(worker.Worktree)]
			}
		}
		status, code, spec := deriveTicketStatus(worker != nil, paneAlive, paneStatus, submit, revs, terminals[base])
		out = append(out, TicketSnap{Path: f, Name: base, Status: status, CodeVerdict: code, SpecVerdict: spec})
	}
	return out
}

// deriveTicketStatus is the one function that turns a ticket's signals into
// its status. Precedence is terminal first: a merged or discarded history
// outcome wins over everything (the worker is gone). Without a worker the
// ticket was never started. A worker whose pane is gone failed, even when it
// submitted — the agent cannot continue. A live worker that submitted reads
// submitted done or blocked, waiting for finish. A live worker with reviewer
// records is in review, carrying the code and spec verdicts ("" while a
// reviewer is still pending). Otherwise the herdr pane state decides:
// working reads working, idle or done reads idle, and anything else alive
// but unclassifiable (blocked approval UI, unknown, no agent state) reads
// retrying. A kept history entry is not terminal and is ignored here.
func deriveTicketStatus(hasWorker, paneAlive bool, paneStatus string, submit *submitRecord, reviewers map[string]reviewerRecord, terminalOutcome string) (TicketStatus, string, string) {
	switch terminalOutcome {
	case "merged":
		return TicketMerged, "", ""
	case "discarded":
		return TicketDiscarded, "", ""
	}
	if !hasWorker {
		return TicketNotStarted, "", ""
	}
	if !paneAlive {
		return TicketFailed, "", ""
	}
	if submit != nil {
		if submit.Status == "blocked" {
			return TicketSubmittedBlocked, "", ""
		}
		return TicketSubmittedDone, "", ""
	}
	if len(reviewers) > 0 {
		return TicketInReview, reviewers[reviewerStandardsType].Verdict, reviewers[reviewerSpecType].Verdict
	}
	switch paneStatus {
	case "working":
		return TicketWorking, "", ""
	case "idle", "done":
		return TicketIdle, "", ""
	default:
		return TicketRetrying, "", ""
	}
}

// workerForTicket finds the worker spawned from one ticket file: the entry
// whose ticket basename matches and whose spec is the same file (symlinks
// resolved best-effort, so a record launched from a symlinked checkout still
// meets its ticket). It returns the entry and its id (the workers.json key,
// which keys the submit file).
func workerForTicket(workers map[string]workerRecord, specPath, ticketBase string) (*workerRecord, string) {
	for id, w := range workers {
		if filepath.Base(w.Ticket) != ticketBase {
			continue
		}
		if w.Spec != "" && !sameSpecPath(w.Spec, specPath) {
			continue
		}
		c := w
		return &c, id
	}
	return nil, ""
}

// sameSpecPath reports whether two spec paths name the same file: lexical
// equality first, then symlink-resolved equality so either side may pass
// through a symlinked checkout.
func sameSpecPath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// loadWorkers reads workers.json keyed by worker id. A missing file is no
// workers; a corrupt one reads as no workers too, so one bad write shows
// tickets as not started rather than failing the dashboard.
func loadWorkers(stateDir string) map[string]workerRecord {
	raw, err := os.ReadFile(filepath.Join(stateDir, "workers.json"))
	if err != nil {
		return nil
	}
	var out map[string]workerRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// loadSubmits reads every submit file, keyed by agent id (the file name).
// Unreadable files are skipped, so one bad submit never fails the read.
func loadSubmits(stateDir string) map[string]submitRecord {
	entries, err := os.ReadDir(filepath.Join(stateDir, "submits"))
	if err != nil {
		return nil
	}
	out := map[string]submitRecord{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(stateDir, "submits", e.Name()))
		if err != nil {
			continue
		}
		var sub submitRecord
		if err := json.Unmarshal(raw, &sub); err != nil {
			continue
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = sub
	}
	return out
}

// loadReviewerRecords reads reviewers.json keyed by worktree path. A missing
// file is no reviewers; a corrupt one reads the same way.
func loadReviewerRecords(stateDir string) map[string]map[string]reviewerRecord {
	raw, err := os.ReadFile(filepath.Join(stateDir, "reviewers.json"))
	if err != nil {
		return nil
	}
	var out map[string]map[string]reviewerRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// loadTerminalsBySlug collects, per run slug, the last terminal outcome per
// ticket basename: the last merged or discarded history entry wins, and a
// kept-only ticket reads as no terminal. Ticket paths match by basename
// because the record may hold an absolute path or just the file.
func loadTerminalsBySlug(stateDir string, byRepo map[string]map[string]*SpecSnap) map[string]map[string]string {
	slugs := map[string]bool{}
	for _, specs := range byRepo {
		for slug := range specs {
			slugs[slug] = true
		}
	}
	out := map[string]map[string]string{}
	for slug := range slugs {
		raw, err := os.ReadFile(filepath.Join(stateDir, "history", slug+".json"))
		if err != nil {
			continue
		}
		var entries []historyEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if e.Ticket == "" {
				continue
			}
			switch e.Outcome {
			case "merged", "discarded":
				if out[slug] == nil {
					out[slug] = map[string]string{}
				}
				out[slug][filepath.Base(e.Ticket)] = e.Outcome
			}
		}
	}
	return out
}
