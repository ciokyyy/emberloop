// Package tui is the emberloop dashboard: a bubbletea terminal UI in its own Go
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
	"sync"
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
// under its repo. MachineRSS is the summed current RSS of every running
// agent, for the header's machine-wide load.
type Snap struct {
	Repos      []RepoSnap
	MachineRSS int64
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
// Orchestrator is the run's own agent (nil without a run); Totals sums it
// with every ticket's agents, so the spec row shows live tokens, reported
// cost and RSS.
type SpecSnap struct {
	Slug         string
	Spec         string
	Branch       string
	HasRun       bool
	Running      bool
	Merged       int
	Total        int
	Tickets      []TicketSnap
	Orchestrator *AgentSnap
	Totals       Totals
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
	TicketCancelled        TicketStatus = "cancelled"
	TicketFailed           TicketStatus = "failed"
)

// TicketSnap is one ticket file with its derived status. CodeVerdict and
// SpecVerdict carry the reviewers' verdicts while in review (`pass`,
// `changes`, or "" while pending); they are "" in every other status.
// Title and Body are the ticket's Markdown (title from its first heading)
// for the detail view; Agents lists every attached agent (worker,
// reviewers, scouts) with harness, model, tokens, cost and RSS; Totals
// sums them for the detail's total row.
type TicketSnap struct {
	Path        string
	Name        string
	Status      TicketStatus
	CodeVerdict string
	SpecVerdict string
	Title       string
	Body        string
	Agents      []AgentSnap
	Totals      Totals
}

// workerRecord is the slice of workers.json Snapshot needs. The map key is
// the worker id; Ticket is the ticket file it was spawned from, Spec the
// spec path, Pane its herdr pane, Worktree its git worktree (the reviewers
// key). Harness, Model and Effort name how it runs, for the agent row.
// Unknown fields are ignored.
type workerRecord struct {
	Ticket   string `json:"ticket"`
	Spec     string `json:"spec"`
	Pane     string `json:"pane"`
	Worktree string `json:"worktree"`
	Harness  string `json:"harness"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
}

// submitRecord is the slice of a submit file Snapshot needs: the status the
// agent chose (`done` or `blocked` for a worker).
type submitRecord struct {
	Status string `json:"status"`
}

// reviewerRecord is the slice of reviewers.json Snapshot needs: the verdict
// the reviewer last submitted (`pass` or `changes`, "" while pending), where
// it runs (Pane), how (Harness, Model, Effort), and the usage-log key
// (SessionID, the reviewer's agent id). Unknown fields are ignored.
type reviewerRecord struct {
	Pane      string `json:"pane"`
	Harness   string `json:"harness"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	SessionID string `json:"session_id"`
	Verdict   string `json:"verdict"`
}

// runRecord is the slice of runs.json Snapshot needs: the run identity plus
// the orchestrator (AgentID, Pane, Harness, Model, Effort), whose spend the
// spec row carries. Unknown fields are ignored, so the core can grow the
// record without breaking this read.
type runRecord struct {
	Slug    string `json:"slug"`
	Spec    string `json:"spec"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	Pane    string `json:"pane"`
	AgentID string `json:"agent_id"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// historyEntry is the slice of a run's ticket history Snapshot needs.
type historyEntry struct {
	Ticket  string `json:"ticket"`
	Outcome string `json:"outcome"`
}

// Reader polls the read model incrementally: usage rows are read from the
// state database past the row-id high-water mark kept across calls (see
// usageCache), so a one-second poll only folds what landed since the last
// tick. The zero value reads everything on its first call. The live binary
// holds one Reader across ticks; Snapshot is the one-shot form — a fresh
// Reader per call — which is what tests assert totals through.
//
// The live program drives loads from bubbletea Cmd goroutines, where a
// tick can start the next load while the last still runs: the mutex
// serializes overlapping Snapshots so the high-water mark never races.
type Reader struct {
	mu    sync.Mutex
	cache *usageCache
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
	return new(Reader).Snapshot(stateDir, currentRepo, herdr)
}

// Snapshot reads through the Reader's row-id high-water mark, accumulating
// new usage rows into the running per-agent totals without double-counting.
func (r *Reader) Snapshot(stateDir, currentRepo string, herdr Herdr) (Snap, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = newUsageCache()
	}
	return snapshotRead(stateDir, currentRepo, herdr, r.cache)
}

// snapshotRead is the Snapshot body over an explicit usage cache.
func snapshotRead(stateDir, currentRepo string, herdr Herdr, cache *usageCache) (Snap, error) {
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
	scouts := loadScouts(stateDir)
	terminals := loadTerminalsBySlug(stateDir, byRepo)
	// One database read per Snapshot: usage rows past the high-water mark
	// fold into the running per-agent totals before any agent row renders.
	cache.refresh(stateDir)
	var machineRSS int64
	for _, specs := range byRepo {
		for slug, s := range specs {
			ctx := agentCtx{stateDir: stateDir, runSlug: slug, live: live, workers: workers, reviewers: reviewers, scouts: scouts, cache: cache}
			s.Tickets = specTickets(s, &ctx, submits, terminals[slug])
			s.Orchestrator = specOrchestrator(slug, s, &ctx, runs)
			s.Totals = Totals{}
			if s.Orchestrator != nil {
				s.Totals.add(*s.Orchestrator)
				if s.Orchestrator.Alive && s.Orchestrator.HasRSS {
					machineRSS += s.Orchestrator.RSS
				}
			}
			for _, tk := range s.Tickets {
				for _, a := range tk.Agents {
					s.Totals.add(a)
					if a.Alive && a.HasRSS {
						machineRSS += a.RSS
					}
				}
			}
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
	return Snap{Repos: repos, MachineRSS: machineRSS}, nil
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

// agentCtx carries what ticket and spec agent rows are built from: the
// state dir, the archive run (the spec slug), live herdr panes, the worker
// and reviewer stores, the scout agent states, and the usage offsets kept
// across polls.
type agentCtx struct {
	stateDir  string
	runSlug   string
	live      map[string]string
	workers   map[string]workerRecord
	reviewers map[string]map[string]reviewerRecord
	scouts    map[string]scoutRecord
	cache     *usageCache
}

// scoutRecord is the slice of agents/<id>.json Snapshot needs for scouts:
// where the scout runs (Pane), who spawned it (ParentID), and how it runs
// (Harness, Model, Effort, recorded at launch). Reviewers also keep agent
// states, but the TUI reads them from reviewers.json, which carries their
// verdicts too. Unknown fields are ignored.
type scoutRecord struct {
	Type     string `json:"type"`
	Pane     string `json:"pane"`
	ParentID string `json:"parent_id"`
	Harness  string `json:"harness"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
}

// loadScouts reads the scout agent states: agents/<id>.json files whose
// type is scout. Workers and reviewers keep their own stores with richer
// records, so only scouts come from here. A missing directory is no
// scouts; unreadable files are skipped.
func loadScouts(stateDir string) map[string]scoutRecord {
	entries, err := os.ReadDir(filepath.Join(stateDir, "agents"))
	if err != nil {
		return nil
	}
	out := map[string]scoutRecord{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(stateDir, "agents", e.Name()))
		if err != nil {
			continue
		}
		var rec scoutRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		if rec.Type != string(AgentScout) {
			continue
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = rec
	}
	return out
}

// fillAgent completes an agent's identity with its funds, memory and
// liveness: usage sums from the incremental cache keyed by the agent id,
// the latest memory sample under the spec's run, and the herdr status of
// its pane (a pane missing from the live map is gone, never running).
func fillAgent(ctx *agentCtx, id string, base AgentSnap) AgentSnap {
	funds := ctx.cache.totals(id)
	base.ID = id
	base.Input, base.Output, base.CacheRead, base.CacheWrite, base.Reasoning = funds.Input, funds.Output, funds.CacheRead, funds.CacheWrite, funds.Reasoning
	base.Cost, base.HasCost = funds.Cost, funds.HasCost
	base.FindCalls, base.FindInput, base.FindOutput = funds.FindCalls, funds.FindInput, funds.FindOutput
	base.FindCost, base.HasFindCost = funds.FindCost, funds.HasFindCost
	if base.Pane != "" {
		if st, ok := ctx.live[base.Pane]; ok {
			base.Alive, base.Status = true, st
		}
	}
	if rss, ok := latestRSS(memoryLogPath(ctx.stateDir, ctx.runSlug, id)); ok {
		base.RSS, base.HasRSS = rss, true
	}
	return base
}

// specOrchestrator builds the run's own agent from its run record: the
// orchestrator whose spend the spec row carries. It is nil for specs with
// no run, and for runs whose record names no agent.
func specOrchestrator(slug string, s *SpecSnap, ctx *agentCtx, runs map[string]runRecord) *AgentSnap {
	if !s.HasRun {
		return nil
	}
	r, ok := runs[slug]
	if !ok || r.AgentID == "" {
		return nil
	}
	out := fillAgent(ctx, r.AgentID, AgentSnap{
		Kind: AgentOrchestrator, Harness: r.Harness, Model: r.Model, Effort: r.Effort, Pane: r.Pane,
	})
	return &out
}

// ticketContent splits a ticket file into its title (the first Markdown
// heading's text) and its raw body for the glamour detail view. A missing
// file reads empty; a file with no heading falls back to its file name.
func ticketContent(path string) (title, body string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	body = string(raw)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		title = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		title = strings.TrimSpace(strings.Trim(title, "*"))
		if title != "" {
			return title, body
		}
	}
	return filepath.Base(path), body
}

// scoutAttached reports whether a scout works on this ticket: its spawner
// chain reaches the ticket's worker. Scouts spawned by the worker name it
// directly; scouts spawned by those scouts resolve transitively, so one
// level of delegation never hides spend. A cycle or a parent outside the
// known agents attaches nowhere.
func scoutAttached(sc scoutRecord, workerID string, scouts map[string]scoutRecord) bool {
	if workerID == "" {
		return false
	}
	seen := map[string]bool{}
	for parent := sc.ParentID; parent != ""; {
		if parent == workerID {
			return true
		}
		if seen[parent] {
			return false
		}
		seen[parent] = true
		next, ok := scouts[parent]
		if !ok {
			return false
		}
		parent = next.ParentID
	}
	return false
}

// ticketAgents lists every agent attached to one ticket file: the worker,
// the worktree's reviewers (standards, spec, and any other type on
// record), and the scouts whose spawner chain reaches the worker. Reviewer
// rows sort by kind so the order is stable.
func ticketAgents(ctx *agentCtx, worker *workerRecord, workerID string, revs map[string]reviewerRecord) []AgentSnap {
	var out []AgentSnap
	if worker != nil {
		out = append(out, fillAgent(ctx, workerID, AgentSnap{
			Kind: AgentWorker, Harness: worker.Harness, Model: worker.Model, Effort: worker.Effort, Pane: worker.Pane,
		}))
	}
	kinds := make([]string, 0, len(revs))
	for kind := range revs {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		rev := revs[kind]
		out = append(out, fillAgent(ctx, rev.SessionID, AgentSnap{
			Kind: AgentKind(kind), Harness: rev.Harness, Model: rev.Model, Effort: rev.Effort, Pane: rev.Pane,
		}))
	}
	scoutIDs := make([]string, 0, len(ctx.scouts))
	for id := range ctx.scouts {
		scoutIDs = append(scoutIDs, id)
	}
	sort.Strings(scoutIDs)
	for _, id := range scoutIDs {
		sc := ctx.scouts[id]
		if !scoutAttached(sc, workerID, ctx.scouts) {
			continue
		}
		out = append(out, fillAgent(ctx, id, AgentSnap{
			Kind: AgentScout, Harness: sc.Harness, Model: sc.Model, Effort: sc.Effort, Pane: sc.Pane,
		}))
	}
	sort.Slice(out, func(i, j int) bool {
		if agentKindRank(out[i].Kind) != agentKindRank(out[j].Kind) {
			return agentKindRank(out[i].Kind) < agentKindRank(out[j].Kind)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// agentKindRank orders ticket agents the way the detail lists them: the
// worker first, then reviewers, then scouts.
func agentKindRank(k AgentKind) int {
	switch {
	case k == AgentWorker:
		return 0
	case strings.HasPrefix(string(k), "reviewer"):
		return 1
	case k == AgentScout:
		return 2
	}
	return 3
}

func specTickets(s *SpecSnap, ctx *agentCtx, submits map[string]submitRecord, terminals map[string]string) []TicketSnap {
	files := ticketFiles(s)
	out := make([]TicketSnap, 0, len(files))
	for _, f := range files {
		base := filepath.Base(f)
		worker, workerID := workerForTicket(ctx.workers, s.Spec, base)
		var submit *submitRecord
		var revs map[string]reviewerRecord
		var paneAlive bool
		var paneStatus string
		if worker != nil {
			if st, ok := ctx.live[worker.Pane]; ok {
				paneAlive, paneStatus = true, st
			}
			if sub, ok := submits[workerID]; ok {
				c := sub
				submit = &c
			}
			if worker.Worktree != "" {
				revs = ctx.reviewers[filepath.Clean(worker.Worktree)]
			}
		}
		status, code, spec := deriveTicketStatus(worker != nil, paneAlive, paneStatus, submit, revs, terminals[base])
		title, body := ticketContent(f)
		agents := ticketAgents(ctx, worker, workerID, revs)
		totals := Totals{}
		for _, a := range agents {
			totals.add(a)
		}
		out = append(out, TicketSnap{Path: f, Name: base, Status: status, CodeVerdict: code, SpecVerdict: spec, Title: title, Body: body, Agents: agents, Totals: totals})
	}
	return out
}

// deriveTicketStatus is the one function that turns a ticket's signals into
// its status. Precedence is terminal first: a merged, discarded or cancelled
// history outcome wins over everything (the worker is gone). Without a worker
// the ticket was never started. A worker whose pane is gone failed, even when
// it submitted — the agent cannot continue. A live worker that submitted reads
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
	case "cancelled":
		// A respawned ticket has a live worker again: the cancellation no
		// longer describes it, so the live signals below decide. Merged and
		// discarded stay terminal: finish deletes the worker entry.
		if !hasWorker {
			return TicketCancelled, "", ""
		}
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
// ticket basename: the last merged, discarded or cancelled history entry wins,
// and a kept-only ticket reads as no terminal. Ticket paths match by basename
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
			case "merged", "discarded", "cancelled":
				if out[slug] == nil {
					out[slug] = map[string]string{}
				}
				out[slug][filepath.Base(e.Ticket)] = e.Outcome
			}
		}
	}
	return out
}
