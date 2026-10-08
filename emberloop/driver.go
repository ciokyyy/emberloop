package emberloop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"example.com/emberloop/tools"
)

// DriverReview is one reviewer type's verdict as the driver sees it: the
// status and the commit it was given at. It models the same review record
// finish itself checks; commit equality is the gate, so a branch change after
// a pass reads as unreviewed again.
type DriverReview struct {
	Verdict string
	At      string
}

// DriverFinish is what the world's finish did: whether the ticket merged,
// the merge sha, or the refusal text when it did not.
type DriverFinish struct {
	Merged bool
	SHA    string
	Detail string
}

// DriverMerged is one merged ticket with the sha it merged as.
type DriverMerged struct {
	ID  string
	SHA string
}

// DriverWorld is the injected world the deterministic driver runs against:
// the ticket files, a fake clock, and the operations the driver calls. Tests
// build it from a map and a fake; the CLI fills the files from disk.
// A world without Finish is a check world: RunDriver only orders tickets and
// never starts workers. A world with it is a run world: RunDriver also runs
// the happy-path loop to a stable state.
type DriverWorld struct {
	// Files holds every ticket file by path. RunDriver only reads entries
	// under the spec's tickets directory ending in .md. A resume reads the
	// same map and writes nothing to it: Status done means merged, Status
	// parked means parked, and the driver keeps no private state file.
	Files map[string]string
	// Now is the clock the driver reads. Nil means time.Now; tests inject a
	// fake. The happy path keeps no time, but the seam stays one entry
	// point for the later tickets that retry with backoff.
	Now func() time.Time
	// StartWorker starts one ticket's worker. The graph check runs before any
	// worker would start, so check mode never calls it.
	StartWorker func(DriverTicket) error
	// MaxWorkers caps how many workers run at once. Zero means three, the
	// same default as the worker cap.
	MaxWorkers int
	// HasWorker reports whether a ticket already has a worker: its record in
	// the worker store, or its implement branch in git. A resumed run never
	// starts a second worker for such a ticket. Nil means no ticket has one.
	HasWorker func(ticketID string) bool
	// MergedSHA is the merge sha of a ticket whose file already reads Status
	// done, so a resumed run keeps it in the report. Empty means unknown, and
	// the report lists the ticket bare. Nil means every old sha is unknown.
	MergedSHA func(ticketID string) string
	// Tip is a worker's current branch tip.
	Tip func(ticketID string) string
	// Reviews holds the reviewer verdicts for a ticket's worker.
	Reviews func(ticketID string) map[string]DriverReview
	// Tell delivers text to a ticket's worker. The driver sends every refused
	// finish to the worker through it, unchanged. Nil means a world without
	// delivery: refusals only land on the report line, as before. Set it with
	// Submits: the pair is what retries a refusal after the worker's submit.
	// Without Submits there is no submit to wait for, so each distinct
	// refusal is told once and the run then holds.
	Tell func(ticketID, text string) error
	// Submits counts a ticket worker's submits. The driver reads it before
	// each Tell and calls finish again only after it advances: the worker's
	// next submit is what triggers the retry. Nil means no waiting: worlds
	// without it tell each distinct refusal once, as above.
	Submits func(ticketID string) int
	// Finish merges a ticket's worker once its gate passes.
	Finish func(ticketID string) (DriverFinish, error)
	// Completed lists the ticket checkboxes one ticket's worker submitted as
	// done, each named by its checkbox line text. Nil means the worker
	// submitted no list. Nil stays valid: progress ticks nothing.
	Completed func(ticketID string) []string
	// TickCheckboxes ticks exactly those boxes in the ticket file and leaves
	// the edit uncommitted. It reports the names with no checkbox in the
	// ticket; the driver reports those and still merges. Nil skips ticking.
	TickCheckboxes func(ticketID string, boxes []string) (unknown []string, err error)
	// Required names the reviewer types every finish needs. Nil means the
	// two built-in reviewers.
	Required []string
	// RequiredFor names the reviewer types one ticket's finish needs, so a
	// worker type with its own requires_review gates like finish gates it.
	// Nil falls back to Required.
	RequiredFor func(ticketID string) []string
	// Sleep waits between polls while tickets stay unmerged but nothing
	// changed. Nil means tests: the run ends at a stable state. A live run
	// passes time.Sleep, so the driver waits for workers instead of ending.
	Sleep func(time.Duration)
}

// DriverTicket is one ticket as the driver sees it: its id (the file base),
// its number (the leading digits), and its blockers as resolved ticket ids.
type DriverTicket struct {
	ID       string
	Num      int
	Path     string
	Blockers []string
}

// DriverResult is what the driver computed: the tickets in the order work
// would start, by ticket number among tickets whose blockers are done, plus
// the merges a run world reached and the plain report of them.
type DriverResult struct {
	Order  []DriverTicket
	Merged []DriverMerged
	Report string
	// UnknownCheckboxes holds the completed names with no checkbox in the
	// ticket, by ticket id. Reported, never merged against.
	UnknownCheckboxes map[string][]string
}

// RunDriver is the driver's one seam: the spec plus the injected world in,
// the start order, the merges and the plain report out. It reads every
// ticket's Blocked by line first, and a line that does not parse, an id that
// names no ticket, or a dependency cycle fails before any worker starts.
// A check world (no Finish) stops at the order. A run world also runs the
// happy-path loop: tickets start in ticket-number order up to MaxWorkers
// once their blockers have merged, and finish runs only after every required
// reviewer type passed at the worker's current tip. A resumed run seeds its
// state from the world: Status done tickets are already merged (their sha
// from MergedSHA), Status parked tickets sit out, and tickets with a worker
// are already started and hold their slot. It never starts a second worker
// for a ticket that has one, and writes no state file of its own.
func RunDriver(spec SpecRun, world DriverWorld) (DriverResult, error) {
	tickets, err := readDriverTickets(spec, world.Files)
	if err != nil {
		return DriverResult{}, err
	}
	order, err := orderDriverTickets(tickets)
	if err != nil {
		return DriverResult{}, err
	}
	res := DriverResult{Order: order}
	if world.Finish == nil {
		return res, nil
	}
	merged, unknown, report, err := runDriverLoop(order, world)
	if err != nil {
		return DriverResult{}, err
	}
	res.Merged, res.UnknownCheckboxes, res.Report = merged, unknown, report
	return res, nil
}

// driverPollInterval spaces a live run's polls while tickets stay unmerged
// but nothing changed.
const driverPollInterval = 5 * time.Second

// runDriverLoop runs the happy path to a stable state: start what is ready,
// finish what passed at its tip, repeat. A pass with no start, no merge, no
// tell and no observed change ends the run in tests; a live run sleeps and
// polls again instead, so it waits for workers rather than quitting on them.
// When nothing runs and nothing could start, the rest wait on parked tickets
// and no poll would change anything, so the run exits with the report.
// A refused finish goes back to the worker unchanged through tell, and
// finish runs again only after the worker's next submit. There is no attempt
// limit: every refusal is told and retried like the first. A resumed run
// seeds its state from the world first: Status done tickets are already
// merged, Status parked tickets sit out while keeping their workers, and
// tickets with a worker are already started and hold their MaxWorkers slot.
func runDriverLoop(order []DriverTicket, world DriverWorld) ([]DriverMerged, map[string][]string, string, error) {
	maxRun := world.MaxWorkers
	if maxRun <= 0 {
		maxRun = 3
	}
	merged := map[string]bool{}
	started := map[string]bool{}
	parked := map[string]bool{}
	seen := map[string]string{}
	refused := map[string]string{}
	// told counts the submits a ticket's worker had when its last refusal
	// was told to it; waiting holds the tickets whose told refusal still
	// owes its next submit. toldText holds the last text told per ticket,
	// so a world without Submits tells each distinct refusal once instead
	// of every pass forever. A ticket with no submit count never waits.
	told := map[string]int{}
	toldText := map[string]string{}
	waiting := map[string]bool{}
	// ticked holds every completed name already sent to TickCheckboxes, so
	// each box ticks once no matter how many polls the run takes.
	ticked := map[string]map[string]bool{}
	// unknown holds the completed names with no checkbox in the ticket, and
	// tickFailed the tick errors the run reported instead of failing on: a
	// run is correct without progress, so neither ever blocks a merge.
	unknown := map[string][]string{}
	tickFailed := map[string]string{}
	var out []DriverMerged
	for _, tk := range order {
		switch driverTicketState(world.Files[tk.Path]) {
		case driverTicketDone:
			merged[tk.ID] = true
			out = append(out, DriverMerged{ID: tk.ID, SHA: driverMergedSHA(world, tk.ID)})
		case driverTicketParked:
			parked[tk.ID] = true
		}
	}
	running := 0
	for _, tk := range order {
		if merged[tk.ID] || parked[tk.ID] {
			continue
		}
		if driverHasWorker(world, tk.ID) {
			started[tk.ID] = true
			running++
		}
	}
	for {
		progress, changed := false, false
		ready := func(tk DriverTicket) bool {
			if started[tk.ID] || merged[tk.ID] || parked[tk.ID] {
				return false
			}
			for _, b := range tk.Blockers {
				if !merged[b] {
					return false
				}
			}
			return true
		}
		startedNew := false
		for running < maxRun {
			next := -1
			for i, tk := range order {
				if ready(tk) && (next < 0 || tk.Num < order[next].Num || (tk.Num == order[next].Num && tk.ID < order[next].ID)) {
					next = i
				}
			}
			if next < 0 {
				break
			}
			if err := world.StartWorker(order[next]); err != nil {
				return nil, nil, "", fmt.Errorf("could not start %s: %w", order[next].ID, err)
			}
			started[order[next].ID] = true
			running++
			startedNew = true
			progress = true
		}
		if running == 0 && !startedNew {
			// Nothing runs and nothing could start: the tickets left unmerged
			// wait on parked tickets, so no poll would change anything. A live
			// run exits with the report instead of sleeping forever.
			break
		}
		for _, tk := range order {
			if !started[tk.ID] || merged[tk.ID] || parked[tk.ID] {
				continue
			}
			if fresh := driverFreshBoxes(ticked[tk.ID], world.Completed, tk.ID); len(fresh) > 0 && world.TickCheckboxes != nil {
				if ticked[tk.ID] == nil {
					ticked[tk.ID] = map[string]bool{}
				}
				for _, box := range fresh {
					ticked[tk.ID][box] = true
				}
				miss, err := world.TickCheckboxes(tk.ID, fresh)
				if err != nil {
					tickFailed[tk.ID] = err.Error()
				} else {
					if len(miss) > 0 {
						unknown[tk.ID] = append(unknown[tk.ID], miss...)
					}
					if len(miss) < len(fresh) {
						progress = true
					}
				}
			}
			// A told refusal waits for the worker's next submit: finish runs
			// again only after the submit count advances past what it was
			// when the refusal was told.
			if waiting[tk.ID] {
				if world.Submits == nil || world.Submits(tk.ID) != told[tk.ID] {
					waiting[tk.ID] = false
				} else {
					continue
				}
			}
			before := world.Tip(tk.ID)
			revs := world.Reviews(tk.ID)
			tip := world.Tip(tk.ID)
			required := driverRequired(world, tk.ID)
			if sig := tip + "\x00" + driverReviewSig(required, revs); sig != seen[tk.ID] {
				seen[tk.ID] = sig
				changed = true
			}
			if before != tip || !driverGatePass(required, revs, tip) {
				continue
			}
			fin, err := world.Finish(tk.ID)
			if err != nil {
				return nil, nil, "", fmt.Errorf("could not finish %s: %w", tk.ID, err)
			}
			if !fin.Merged {
				// A refused finish stays running; its text lands on the
				// unmerged line and goes back to the worker unchanged through
				// tell. An empty Detail clears the line back to still running
				// and is never told. A tell counts as progress, so the retry
				// after the worker's submit gets its own pass. Without a
				// submit count there is no next submit to wait for, so each
				// distinct refusal is told once and the run then holds.
				if fin.Detail == "" {
					delete(refused, tk.ID)
					delete(toldText, tk.ID)
				} else {
					refused[tk.ID] = fin.Detail
					if world.Tell != nil && (world.Submits != nil || fin.Detail != toldText[tk.ID]) {
						at := 0
						if world.Submits != nil {
							at = world.Submits(tk.ID)
						}
						if err := world.Tell(tk.ID, fin.Detail); err != nil {
							return nil, nil, "", fmt.Errorf("could not tell %s: %w", tk.ID, err)
						}
						toldText[tk.ID] = fin.Detail
						if world.Submits != nil {
							told[tk.ID], waiting[tk.ID] = at, true
						}
						progress = true
					}
				}
				continue
			}
			merged[tk.ID] = true
			running--
			out = append(out, DriverMerged{ID: tk.ID, SHA: fin.SHA})
			progress = true
		}
		if len(out)+len(parked) == len(order) {
			break
		}
		if progress || changed {
			continue
		}
		if world.Sleep == nil {
			break
		}
		world.Sleep(driverPollInterval)
	}
	return out, driverUnknownOrNil(unknown), driverReport(order, out, started, merged, parked, refused, unknown, tickFailed), nil
}

// Resume states a ticket file carries: done (merged), parked (sitting out
// with its worktree kept), or anything else (open). The parked marker is the
// same Status value the park action writes.
const (
	driverTicketOpen = iota
	driverTicketDone
	driverTicketParked
)

// driverTicketState reads one ticket file's resume state from its Status
// headers, through ticketStatusValues, the one rule for the line. Done wins
// over parked anywhere in the file, the same precedence ticketFileDone uses
// (any done reads done), so both readers agree on every file.
func driverTicketState(raw string) int {
	parked := false
	for _, value := range ticketStatusValues(raw) {
		switch {
		case strings.EqualFold(value, "done"):
			return driverTicketDone
		case strings.EqualFold(value, "parked"):
			parked = true
		}
	}
	if parked {
		return driverTicketParked
	}
	return driverTicketOpen
}

// driverHasWorker reports whether the world already holds a worker for the
// ticket. Nil means none do.
func driverHasWorker(world DriverWorld, ticketID string) bool {
	return world.HasWorker != nil && world.HasWorker(ticketID)
}

// driverMergedSHA is the old merge sha of an already-merged ticket. Empty
// means unknown, and the report lists the ticket bare.
func driverMergedSHA(world DriverWorld, ticketID string) string {
	if world.MergedSHA == nil {
		return ""
	}
	return world.MergedSHA(ticketID)
}

// driverRequired names the reviewer types one ticket's finish needs: its own
// hook, then the world's list, then the two built-in reviewers.
func driverRequired(world DriverWorld, ticketID string) []string {
	if world.RequiredFor != nil {
		if req := world.RequiredFor(ticketID); req != nil {
			return req
		}
	}
	if len(world.Required) > 0 {
		return world.Required
	}
	return defaultReviewers
}

// driverGatePass says finish may run: every required reviewer type passed at
// the tip. Anything else, a missing or stale pass, keeps the worker running.
func driverGatePass(required []string, revs map[string]DriverReview, tip string) bool {
	for _, typ := range required {
		r, ok := revs[typ]
		if !ok || r.Verdict != verdictPass || r.At != tip {
			return false
		}
	}
	return true
}

// driverReviewSig summarizes what the driver last saw of a ticket, so a new
// verdict or a new tip counts as a change worth another pass.
func driverReviewSig(required []string, revs map[string]DriverReview) string {
	var b strings.Builder
	for _, typ := range required {
		r := revs[typ]
		b.WriteString(typ)
		b.WriteString("=")
		b.WriteString(r.Verdict)
		b.WriteString("@")
		b.WriteString(r.At)
		b.WriteString(";")
	}
	return b.String()
}

// driverReport renders the plain final report: every merged ticket with its
// sha (bare when the old sha is unknown), then every ticket left unmerged,
// each with its last refusal when a finish refused it, parked when it sat
// out, never started when it never ran, still running otherwise. Unknown
// checkboxes and tick failures follow their ticket in order. Later tickets
// add the follow-ups beside them.
func driverReport(order []DriverTicket, merged []DriverMerged, started, done, parked map[string]bool, refused map[string]string, unknown map[string][]string, tickFailed map[string]string) string {
	var b strings.Builder
	for _, m := range merged {
		if m.SHA == "" {
			fmt.Fprintf(&b, "merged %s\n", m.ID)
			continue
		}
		fmt.Fprintf(&b, "merged %s as %s\n", m.ID, m.SHA)
	}
	for _, tk := range order {
		if !done[tk.ID] {
			switch {
			case parked[tk.ID]:
				fmt.Fprintf(&b, "unmerged %s: parked\n", tk.ID)
			case refused[tk.ID] != "":
				fmt.Fprintf(&b, "unmerged %s: %s\n", tk.ID, refused[tk.ID])
			case !started[tk.ID]:
				fmt.Fprintf(&b, "unmerged %s: never started\n", tk.ID)
			default:
				fmt.Fprintf(&b, "unmerged %s: still running\n", tk.ID)
			}
		}
		for _, name := range unknown[tk.ID] {
			fmt.Fprintf(&b, "unknown checkbox %q in %s\n", name, tk.ID)
		}
		if tickFailed[tk.ID] != "" {
			fmt.Fprintf(&b, "checkboxes for %s not applied: %s\n", tk.ID, tickFailed[tk.ID])
		}
	}
	return b.String()
}

// driverFreshBoxes holds the completed names the driver has not ticked yet.
// A nil Completed stays valid: no list means no fresh boxes.
func driverFreshBoxes(sent map[string]bool, completed func(string) []string, ticketID string) []string {
	if completed == nil {
		return nil
	}
	var fresh []string
	for _, box := range completed(ticketID) {
		if !sent[box] {
			fresh = append(fresh, box)
		}
	}
	return fresh
}

// driverUnknownOrNil keeps an empty unknown map nil, so runs without
// progress read exactly as runs before it.
func driverUnknownOrNil(unknown map[string][]string) map[string][]string {
	if len(unknown) == 0 {
		return nil
	}
	return unknown
}

// RunDriverCheck reads the spec's tickets from disk, checks the graph through
// RunDriver, and prints the start order, one ticket id per line. It is what
// `emberloop implement <spec> --driver --check` runs.
func RunDriverCheck(spec SpecRun, stdout io.Writer) error {
	files, err := readDriverFiles(spec)
	if err != nil {
		return err
	}
	res, err := RunDriver(spec, DriverWorld{Files: files})
	if err != nil {
		return err
	}
	for _, tk := range res.Order {
		fmt.Fprintln(stdout, tk.ID)
	}
	return nil
}

// RunDriverLive runs the whole spec through the live operations: the same
// worker spawn, reviewer verdicts and finish the LLM orchestrator uses, with
// the worker cap from settings. It prints the plain final report of merged
// tickets with their sha. The repo is already on the integration branch, and
// uncommitted edits in its checkout are left alone: the merge refuses on
// overlap, like every finish, and completeTicket in worker.go commits only
// the ticket file. The run waits for workers rather than quitting on them:
// later tickets add the needs-you alert, parking and progress rows for a run
// that stops converging.
func RunDriverLive(spec SpecRun, repo string, stdout io.Writer) error {
	files, err := readDriverFiles(spec)
	if err != nil {
		return err
	}
	if _, err := RunDriver(spec, DriverWorld{Files: files}); err != nil {
		return err
	}
	// Every worker records the spec, the way workers spawned from an
	// implement session do. Restored on return: the variable is process
	// global and the value belongs to this run.
	oldSpec, hadSpec := os.LookupEnv("EMBERLOOP_SPEC")
	_ = os.Setenv("EMBERLOOP_SPEC", spec.Spec)
	defer func() {
		if hadSpec {
			_ = os.Setenv("EMBERLOOP_SPEC", oldSpec)
		} else {
			_ = os.Unsetenv("EMBERLOOP_SPEC")
		}
	}()
	res, err := RunDriver(spec, liveDriverWorld(spec, repo, files))
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, res.Report)
	return nil
}

// liveDriverWorld wires the live operations behind the driver seam: spawns go
// through the same spawn call the orchestrator uses, tips resolve the same
// implement branches, verdicts read the same review record finish gates on,
// tells reach the worker's own pane through the same delivery the tell tool
// uses, submit counts derive from the worker's submit record, completed boxes
// read the worker's own submit, ticks land in the ticket file in this checkout
// through the per-file lock, and finish is the same merge. A resume reads the
// same three sources the seam takes: Status done and Status parked come from
// the ticket files, a running worker from its store record or its implement
// branch, and an old merge sha from the run's ticket history. Extracted so
// tests drive the mapping through the seam with fake exec and temp state.
func liveDriverWorld(spec SpecRun, repo string, files map[string]string) DriverWorld {
	paths := map[string]string{}
	for path := range files {
		if filepath.Dir(filepath.Clean(path)) == filepath.Clean(spec.Tickets) && strings.HasSuffix(path, ".md") {
			paths[strings.TrimSuffix(filepath.Base(path), ".md")] = path
		}
	}
	types := mustLoadAgentTypes(repo)
	// submitSeq counts each ticket worker's submits: the submit record holds
	// only the last one, so a new marker (anything but the turn flag, which
	// the stop gate flips on its own) advances the count.
	submitSeq := map[string]int{}
	submitMark := map[string]string{}
	return DriverWorld{
		Files:      files,
		MaxWorkers: tools.LoadSettings().EffectiveMaxWorkers(),
		HasWorker: func(ticketID string) bool {
			path := paths[ticketID]
			if path == "" {
				return false
			}
			if store, err := loadWorkersLocked(); err == nil {
				if _, ok := store[WorkerID(path)]; ok {
					return true
				}
			}
			res, err := spawnExec("git", []string{"rev-parse", "--verify", "implement/" + WorkerID(path)}, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
			return err == nil && res.Code == 0
		},
		MergedSHA: func(ticketID string) string {
			entries, err := loadHistory(specSlug(spec.Spec))
			if err != nil {
				return ""
			}
			sha := ""
			for _, h := range entries {
				if h.Outcome != outcomeMerged || filepath.Base(h.Ticket) != ticketID+".md" {
					continue
				}
				sha = h.SHA
			}
			return sha
		},
		StartWorker: func(tk DriverTicket) error {
			_, err := spawnAgents(spawnExec, repo, types, []spawnRequest{{Type: "worker", Ticket: tk.Path}})
			return err
		},
		Tip: func(ticketID string) string {
			res, err := spawnExec("git", []string{"rev-parse", "implement/" + WorkerID(paths[ticketID])}, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
			if err != nil || res.Code != 0 {
				return ""
			}
			return strings.TrimSpace(res.Stdout)
		},
		Reviews: func(ticketID string) map[string]DriverReview {
			out := map[string]DriverReview{}
			for typ, r := range reviewersOf(worktreePath(repo, WorkerID(paths[ticketID]))) {
				out[typ] = DriverReview{Verdict: r.Verdict, At: r.VerdictAt}
			}
			return out
		},
		RequiredFor: func(ticketID string) []string {
			store, err := loadWorkersLocked()
			if err != nil {
				return nil
			}
			w, ok := store[WorkerID(paths[ticketID])]
			if !ok {
				return nil
			}
			return requiredReviewers(&w)
		},
		Tell: func(ticketID, text string) error {
			id := WorkerID(paths[ticketID])
			st, ok, err := ReadAgentState(id)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("unknown agent %q", id)
			}
			if st.Pane == "" {
				return fmt.Errorf("agent %s has no pane to deliver to", id)
			}
			return Nudge(tellExec, st.Pane, text)
		},
		Submits: func(ticketID string) int {
			s, ok, err := ReadAgentSubmit(WorkerID(paths[ticketID]))
			if err != nil || !ok {
				return submitSeq[ticketID]
			}
			mark := s.At + "\x00" + s.Status + "\x00" + s.Report + "\x00" + s.Summary
			if s.Stopped {
				mark += "\x00stopped"
			}
			if mark != submitMark[ticketID] {
				submitMark[ticketID] = mark
				submitSeq[ticketID]++
			}
			return submitSeq[ticketID]
		},
		Finish: func(ticketID string) (DriverFinish, error) {
			res, err := Finish(finishExec, WorkerID(paths[ticketID]), true)
			if err != nil {
				return DriverFinish{}, err
			}
			return DriverFinish{Merged: res.Merged, SHA: res.SHA, Detail: res.Detail}, nil
		},
		Completed: func(ticketID string) []string {
			path, ok := paths[ticketID]
			if !ok {
				return nil
			}
			s, ok, err := ReadAgentSubmit(WorkerID(path))
			if err != nil || !ok {
				return nil
			}
			return s.Completed
		},
		TickCheckboxes: func(ticketID string, boxes []string) ([]string, error) {
			path, ok := paths[ticketID]
			if !ok {
				return boxes, nil
			}
			return tickDriverTicketFile(path, boxes)
		},
		Sleep: time.Sleep,
	}
}

// readDriverFiles reads every ticket file under the spec's tickets directory
// into the map the driver runs against.
func readDriverFiles(spec SpecRun) (map[string]string, error) {
	entries, err := os.ReadDir(spec.Tickets)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(spec.Tickets, e.Name()))
		if err != nil {
			return nil, err
		}
		files[filepath.Join(spec.Tickets, e.Name())] = string(raw)
	}
	return files, nil
}

// readDriverTickets parses every ticket file under the spec's tickets
// directory: the Blocked by line, then each id resolved against the tickets
// present. The first failure names the file (and the line for a bad line).
func readDriverTickets(spec SpecRun, files map[string]string) (map[string]*DriverTicket, error) {
	dir := filepath.Clean(spec.Tickets)
	var paths []string
	for path := range files {
		if filepath.Dir(filepath.Clean(path)) != dir || !strings.HasSuffix(path, ".md") {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no tickets under %s", spec.Tickets)
	}
	tickets := make(map[string]*DriverTicket, len(paths))
	for _, path := range paths {
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		num, ok := driverNumber(base)
		if !ok {
			return nil, fmt.Errorf("ticket %s has no ticket number", path)
		}
		value, line, found, err := driverBlockedValue(files[path])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if !found {
			return nil, fmt.Errorf("ticket %s has no Blocked by line", path)
		}
		refs, err := parseBlockerRefs(value)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		tickets[base] = &DriverTicket{ID: base, Num: num, Path: path}
		tickets[base].Blockers = refs
	}
	if err := resolveDriverRefs(tickets); err != nil {
		return nil, err
	}
	out := make(map[string]*DriverTicket, len(tickets))
	for id, tk := range tickets {
		out[id] = tk
	}
	return out, nil
}

// driverBlockedValue returns the text after the colon of the ticket's first
// Blocked by line, with its 1-based line number. The line is a field line: it
// starts with Blocked by (bold markers, quotes and list dashes allowed), so a
// mention deeper in the text never matches. A Blocked by line without a colon
// is found but unparsable, so it still reports its line.
func driverBlockedValue(content string) (value string, line int, found bool, err error) {
	for i, raw := range strings.Split(content, "\n") {
		field := strings.TrimSpace(raw)
		field = strings.TrimSpace(strings.TrimLeft(field, ">-"))
		field = strings.TrimLeft(field, "*_")
		if !strings.HasPrefix(field, "Blocked by") {
			continue
		}
		rest := strings.TrimSpace(field[len("Blocked by"):])
		if !strings.HasPrefix(rest, ":") {
			return "", i + 1, true, fmt.Errorf("cannot parse Blocked by line %q", strings.TrimSpace(raw))
		}
		// The line is usually bold markdown (`**Blocked by:** ...`), so the
		// value carries the closing markers: strip emphasis around it.
		value := strings.TrimSpace(strings.Trim(strings.TrimSpace(rest[1:]), "*_"))
		return value, i + 1, true, nil
	}
	return "", 0, false, nil
}

// parseBlockerRefs reads the ticket ids at the start of a Blocked by value.
// The ids come first; everything after is free text for humans. Empty, None
// and nothing mean no blockers. Parenthesized asides are free text too, so
// commas inside them never split the id list. Free text may itself hold
// commas: a comma segment with no leading id continues the free text once
// free text has started, while a segment with a leading id still adds
// blockers. A segment with no leading id before any free text fails, so a
// typo never reads as no blockers.
func parseBlockerRefs(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if first := strings.Fields(trimmed)[0]; strings.EqualFold(first, "none") || strings.EqualFold(first, "nothing") {
		return nil, nil
	}
	rest, ok := stripParens(trimmed)
	if !ok {
		return nil, fmt.Errorf("cannot parse Blocked by line %q", value)
	}
	fail := func() ([]string, error) {
		return nil, fmt.Errorf("cannot parse Blocked by line %q", value)
	}
	var refs []string
	freeText := false
	for _, segment := range strings.Split(rest, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return fail()
		}
		fields := strings.Fields(segment)
		ids := 0
		for _, field := range fields {
			if !isTicketRef(field) {
				break
			}
			refs = append(refs, field)
			ids++
		}
		if ids == 0 {
			// No leading id: free-text continuation once free text started,
			// else a line that does not parse.
			if len(refs) == 0 || !freeText {
				return fail()
			}
			continue
		}
		if ids < len(fields) {
			freeText = true
		}
	}
	return refs, nil
}

// stripParens drops every parenthesized run, which is always free text. An
// unbalanced paren leaves the line unparsable.
func stripParens(s string) (string, bool) {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return "", false
			}
			depth--
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	if depth != 0 {
		return "", false
	}
	return b.String(), true
}

// isTicketRef reports whether a field is one ticket id: digits with an
// optional -slug, like 02 or 02-ticket-graph.
func isTicketRef(field string) bool {
	head, tail, dashed := field, "", false
	if i := strings.Index(field, "-"); i >= 0 {
		head, tail, dashed = field[:i], field[i+1:], true
	}
	if head == "" || (dashed && tail == "") {
		return false
	}
	for _, r := range head {
		if r < '0' || r > '9' {
			return false
		}
	}
	if tail == "" {
		return true
	}
	for _, r := range tail {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// driverNumber reads the leading digits of a ticket id: 02-ticket-graph is
// ticket 2.
func driverNumber(id string) (int, bool) {
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(id[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// resolveDriverRefs turns each ticket's raw refs into ticket ids. A bare
// number matches the ticket with that number; a full slug matches the file
// base. Unknown and ambiguous refs fail together naming every ticket involved.
func resolveDriverRefs(tickets map[string]*DriverTicket) error {
	byNum := make(map[int][]string)
	for id, tk := range tickets {
		byNum[tk.Num] = append(byNum[tk.Num], id)
	}
	for _, ids := range byNum {
		sort.Strings(ids)
	}
	var problems []string
	ids := make([]string, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		tk := tickets[id]
		resolved := make([]string, 0, len(tk.Blockers))
		for _, ref := range tk.Blockers {
			if strings.Contains(ref, "-") {
				if _, ok := tickets[ref]; ok {
					resolved = append(resolved, ref)
					continue
				}
				problems = append(problems, fmt.Sprintf("ticket %q blocked by unknown ticket %q", id, ref))
				continue
			}
			n, _ := strconv.Atoi(ref)
			switch matches := byNum[n]; len(matches) {
			case 1:
				resolved = append(resolved, matches[0])
			case 0:
				problems = append(problems, fmt.Sprintf("ticket %q blocked by unknown ticket %q", id, ref))
			default:
				problems = append(problems, fmt.Sprintf("ticket %q blocked by ambiguous ticket %q: %s", id, ref, strings.Join(matches, ", ")))
			}
		}
		sort.Slice(resolved, func(i, j int) bool {
			if tickets[resolved[i]].Num != tickets[resolved[j]].Num {
				return tickets[resolved[i]].Num < tickets[resolved[j]].Num
			}
			return resolved[i] < resolved[j]
		})
		tk.Blockers = resolved
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// orderDriverTickets sorts tickets so every ticket follows its blockers. Among
// the tickets whose blockers are done the lowest ticket number goes first. A
// cycle fails naming the tickets in the cycle.
func orderDriverTickets(tickets map[string]*DriverTicket) ([]DriverTicket, error) {
	dependents := make(map[string][]string, len(tickets))
	pending := make(map[string]int, len(tickets))
	for id, tk := range tickets {
		pending[id] = len(tk.Blockers)
		for _, b := range tk.Blockers {
			dependents[b] = append(dependents[b], id)
		}
	}
	var ready []string
	for id, n := range pending {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	var order []DriverTicket
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			if tickets[ready[i]].Num != tickets[ready[j]].Num {
				return tickets[ready[i]].Num < tickets[ready[j]].Num
			}
			return ready[i] < ready[j]
		})
		next := ready[0]
		ready = ready[1:]
		order = append(order, *tickets[next])
		for _, dep := range dependents[next] {
			pending[dep]--
			if pending[dep] == 0 {
				ready = append(ready, dep)
			}
		}
		delete(pending, next)
	}
	if len(pending) > 0 {
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(driverCycle(tickets, pending), " -> "))
	}
	return order, nil
}

// driverCycle finds one cycle among the tickets Kahn's pass could not order,
// so the error names the tickets in the cycle rather than every ticket stuck
// behind it.
func driverCycle(tickets map[string]*DriverTicket, stuck map[string]int) []string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := make(map[string]int, len(stuck))
	var stack []string
	var cycle []string
	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		blockers := append([]string(nil), tickets[id].Blockers...)
		sort.Strings(blockers)
		for _, b := range blockers {
			if _, ok := stuck[b]; !ok {
				continue
			}
			switch color[b] {
			case grey:
				start := 0
				for stack[start] != b {
					start++
				}
				cycle = append(append([]string(nil), stack[start:]...), b)
				return true
			case white:
				if visit(b) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	stuckIDs := make([]string, 0, len(stuck))
	for id := range stuck {
		stuckIDs = append(stuckIDs, id)
	}
	sort.Strings(stuckIDs)
	for _, id := range stuckIDs {
		if color[id] == white && visit(id) {
			return cycle
		}
	}
	return stuckIDs
}

// driverTickMu guards driverTickLocks: one mutex per ticket path, so two
// workers submitting at once never write the same ticket file at the same
// time. The driver is the only writer to ticket files during a run.
var driverTickMu sync.Mutex
var driverTickLocks = map[string]*sync.Mutex{}

// driverTickLock returns the mutex for one ticket path, creating it once.
func driverTickLock(path string) *sync.Mutex {
	driverTickMu.Lock()
	defer driverTickMu.Unlock()
	if mu, ok := driverTickLocks[path]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	driverTickLocks[path] = mu
	return mu
}

// tickDriverTicketFile ticks exactly the named boxes in the ticket file at
// path and leaves the edit uncommitted. Each box names a checkbox by its
// line text; the return holds the names with no checkbox in the file.
// Workers convey boxes through submit and never edit ticket files, so the
// dirty tick overlaps no incoming diff and finish merges past it; a ticket
// the worker itself changed still hits finish's dirty-overlap refusal.
func tickDriverTicketFile(path string, boxes []string) ([]string, error) {
	mu := driverTickLock(path)
	mu.Lock()
	defer mu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ticked, unknown := applyCompletedBoxes(string(raw), boxes)
	if ticked == string(raw) {
		return unknown, nil
	}
	if err := os.WriteFile(path, []byte(ticked), 0o644); err != nil {
		return nil, err
	}
	return unknown, nil
}

// applyCompletedBoxes ticks the checkboxes named by line text: the first
// unchecked box with that text becomes [x]. A name matching an already
// ticked box is known and changes nothing; a name matching no box at all
// is unknown. Only - and * list markers count, so prose mentioning [ ]
// never ticks.
func applyCompletedBoxes(raw string, boxes []string) (string, []string) {
	lines := strings.Split(raw, "\n")
	var unknown []string
nextBox:
	for _, box := range boxes {
		want := strings.TrimSpace(box)
		for i, line := range lines {
			text, checked, ok := checkboxText(line)
			if !ok || checked || text != want {
				continue
			}
			lines[i] = strings.Replace(line, "[ ]", "[x]", 1)
			continue nextBox
		}
		for _, line := range lines {
			if text, checked, ok := checkboxText(line); ok && checked && text == want {
				continue nextBox
			}
		}
		unknown = append(unknown, box)
	}
	return strings.Join(lines, "\n"), unknown
}

// checkboxText reads one checkbox line: its text after the marker and
// whether it already ticks. ok is false for lines that hold no checkbox.
func checkboxText(line string) (text string, checked bool, ok bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "- [") && !strings.HasPrefix(trimmed, "* [") {
		return "", false, false
	}
	mark := trimmed[2:]
	switch {
	case strings.HasPrefix(mark, "[ ]"):
		return strings.TrimSpace(mark[len("[ ]"):]), false, true
	case strings.HasPrefix(mark, "[x]"), strings.HasPrefix(mark, "[X]"):
		return strings.TrimSpace(mark[len("[x]"):]), true, true
	default:
		return "", false, false
	}
}
