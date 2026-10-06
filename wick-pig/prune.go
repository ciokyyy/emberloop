package wick

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/wick/statedb"
)

// Pruning deletes archived run data on purpose, never automatically: `wick-pig
// stats prune` takes a run filter (--run) or an age filter (--older-than), dry
// runs by default, and only deletes with --apply. A dry run lists the runs
// that would go and how much space they would free. Runs whose orchestrator
// pane is still alive in herdr are never touched; when herdr cannot be asked,
// an apply refuses rather than guesses.
//
// Per selected run, pruning removes the archive directory (session records,
// transcript copies, offsets, memory logs), the run's ticket history, its run
// record, and the usage logs (plus tail state) of agents that belong only to
// pruned runs. Blobs no remaining session references are removed; blobs a kept
// run still references stay.
//
// Liveness needs a run record: only implement runs record their orchestrator
// pane, so a record-less archive (adhoc sessions share one slug) has no pane
// to check. A live session keeps appending to its archive files, so an age
// filter dates it by activity and passes over it in practice; an explicit
// --run names it deliberately and prunes it.

// PruneOptions selects what to prune. Runs names run slugs (the key FinishRun
// takes); OlderThan keeps only runs whose end (or start, when unfinished)
// is that far in the past. Both filters combine: a run must match every
// filter given. No filter selects every run. Apply deletes; without it the
// prune is a dry run that reports what would go.
type PruneOptions struct {
	Runs      []string
	OlderThan time.Duration
	Apply     bool
}

// PruneRunPlan is one selected run and its reclaimable size.
type PruneRunPlan struct {
	Slug   string
	Bytes  int64
	Agents []string
}

// PruneResult is what a prune (dry or applied) found. Runs holds the runs
// selected after running-run protection; SkippedRunning names the runs left
// alone because their orchestrator pane is still alive. BlobFiles counts the
// orphaned blobs selected alongside the runs, and TotalBytes covers runs and
// blobs together. CtxPurged counts overflow-store rows older than 24 hours
// purged by an applied prune, even when no run was selected. Applied reports
// whether the prune ran with --apply rather than as a dry run.
// LivenessUnknown reports that herdr could not be asked which panes are
// alive; a dry run still lists, but an apply refuses.
type PruneResult struct {
	Runs            []PruneRunPlan
	SkippedRunning  []string
	BlobFiles       int
	BlobBytes       int64
	TotalBytes      int64
	CtxPurged       int64
	Applied         bool
	LivenessUnknown bool
}

// PlanPrune selects the runs opts matches and sizes them, without checking
// liveness and without deleting anything. PruneRuns is the entry point that
// protects running runs and applies the plan.
func PlanPrune(now time.Time, opts PruneOptions) (PruneResult, error) {
	slugs, err := listPruneSlugs()
	if err != nil {
		return PruneResult{}, err
	}
	want := map[string]bool{}
	for _, r := range opts.Runs {
		want[sanitizeArchiveName(r)] = true
	}
	var runs []PruneRunPlan
	for _, slug := range slugs {
		if len(want) > 0 && !want[sanitizeArchiveName(slug)] {
			continue
		}
		if opts.OlderThan > 0 {
			at, ok := pruneRunTime(slug)
			if !ok || now.Sub(at) <= opts.OlderThan {
				continue
			}
		}
		agents := pruneAgents(slug)
		runs = append(runs, PruneRunPlan{Slug: slug, Bytes: pruneRunBytes(slug, agents), Agents: agents})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Slug < runs[j].Slug })
	files, blobBytes, total := sizeSelection(slugs, runs)
	return PruneResult{Runs: runs, BlobFiles: files, BlobBytes: blobBytes, TotalBytes: total}, nil
}

// sizeSelection sizes a selection: the orphaned blobs are the ones no kept
// run (every run outside the selection) references, and the total covers the
// runs and those blobs together.
func sizeSelection(all []string, selected []PruneRunPlan) (files int, blobBytes, total int64) {
	names, _ := orphanBlobNames(keptSlugs(all, selected))
	blobBytes = blobBytesOf(names)
	total = blobBytes
	for _, r := range selected {
		total += r.Bytes
	}
	return len(names), blobBytes, total
}

// PruneRuns plans the prune, sets aside the runs whose orchestrator pane is
// still alive, and deletes the rest when opts.Apply is set. A dry run returns
// the plan with Applied false and deletes nothing. Liveness comes from one
// `herdr pane list`, the same membership read the grid and the read-only
// launcher use; when herdr cannot be asked, a dry run still lists (nothing is
// at risk) but an apply refuses rather than deletes blind.
func PruneRuns(run Exec, now time.Time, opts PruneOptions) (PruneResult, error) {
	plan, err := PlanPrune(now, opts)
	if err != nil {
		return PruneResult{}, err
	}
	records := map[string]RunRecord{}
	if store, err := loadRuns(); err == nil {
		records = store
	} else if !os.IsNotExist(err) {
		return PruneResult{}, err
	}
	needsLiveness := false
	for _, r := range plan.Runs {
		if rec, ok := records[r.Slug]; ok && rec.Pane != "" {
			needsLiveness = true
			break
		}
	}
	if needsLiveness {
		live, err := livePanes(run)
		if err != nil {
			if !opts.Apply {
				plan.LivenessUnknown = true
				return plan, nil
			}
			return PruneResult{}, fmt.Errorf("cannot check which panes are alive: %v; refusing the prune", err)
		}
		var keep []PruneRunPlan
		for _, r := range plan.Runs {
			if rec, ok := records[r.Slug]; ok && rec.Pane != "" && live[rec.Pane] {
				plan.SkippedRunning = append(plan.SkippedRunning, r.Slug)
				continue
			}
			keep = append(keep, r)
		}
		sort.Strings(plan.SkippedRunning)
		plan.Runs = keep
	}
	if !opts.Apply {
		plan.Applied = false
		if len(plan.Runs) == 0 {
			// Nothing selected after running-run protection: blob collection
			// rides along with a run prune, so an empty selection frees
			// nothing on its own.
			plan.BlobFiles, plan.BlobBytes, plan.TotalBytes = 0, 0, 0
			return plan, nil
		}
		// A skipped running run stays, so its blobs are not orphaned: recount
		// without the skipped runs' references removed.
		plan.BlobFiles, plan.BlobBytes, plan.TotalBytes = sizeSelection(mustPruneSlugs(), plan.Runs)
		return plan, nil
	}
	plan.Applied = true
	if len(plan.Runs) == 0 {
		// No run selected, but an applied prune still collects expired
		// overflow rows: the ticket purges them by prune, not only by open.
		purged, perr := purgeExpiredCtxRows()
		plan.BlobFiles, plan.BlobBytes, plan.TotalBytes = 0, 0, 0
		plan.CtxPurged = purged
		return plan, perr
	}
	kept := keptSlugs(mustPruneSlugs(), plan.Runs)
	blobNames, _ := orphanBlobNames(kept)
	// Snapshot the reported sizes before deleting: afterwards the files are
	// gone and there is nothing left to measure.
	plan.BlobFiles = len(blobNames)
	plan.BlobBytes = blobBytesOf(blobNames)
	plan.TotalBytes = plan.BlobBytes
	for _, r := range plan.Runs {
		plan.TotalBytes += r.Bytes
	}
	agentKept := map[string]bool{}
	for _, slug := range kept {
		for _, a := range pruneAgents(slug) {
			agentKept[a] = true
		}
	}
	var errs []error
	var droppedAgents []string
	for _, r := range plan.Runs {
		if err := deletePruneRun(r.Slug); err != nil {
			errs = append(errs, err)
		}
		for _, a := range r.Agents {
			if !agentKept[a] {
				if err := os.Remove(UsageLogPath(a)); err != nil && !os.IsNotExist(err) {
					errs = append(errs, err)
				}
				if err := os.Remove(usageTailPath(a)); err != nil && !os.IsNotExist(err) {
					errs = append(errs, err)
				}
				droppedAgents = append(droppedAgents, a)
			}
		}
	}
	if err := deletePruneUsageRows(droppedAgents); err != nil {
		errs = append(errs, err)
	}
	for _, name := range blobNames {
		if err := os.Remove(filepath.Join(blobDir(), name)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return plan, errors.Join(errs...)
}

// livePanes lists the panes herdr still has, by id. It is the one liveness
// read prune trusts: the same `pane list` membership the grid and the
// read-only launcher already read, answered by every herdr fake in the repo.
func livePanes(run Exec) (map[string]bool, error) {
	if run == nil {
		return nil, fmt.Errorf("no herdr runner")
	}
	reply, err := herdrJSON(run, []string{"pane", "list"}, 15*time.Second)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, p := range reply.Result.Panes {
		out[p.PaneID] = true
	}
	return out, nil
}

// listPruneSlugs unions every run key on disk: run records, archive
// directories and history files. A run with no record (adhoc sessions) still
// prunes by its archive.
func listPruneSlugs() ([]string, error) {
	seen := map[string]bool{}
	if store, err := loadRuns(); err == nil {
		for slug := range store {
			seen[slug] = true
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if entries, err := os.ReadDir(archiveRoot()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	histDir := filepath.Join(filepath.Dir(statePath), "history")
	if entries, err := os.ReadDir(histDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				seen[strings.TrimSuffix(e.Name(), ".json")] = true
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for slug := range seen {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out, nil
}

func mustPruneSlugs() []string {
	slugs, _ := listPruneSlugs()
	return slugs
}

// keptSlugs returns the slugs that survive after selected are removed.
func keptSlugs(all []string, selected []PruneRunPlan) []string {
	drop := map[string]bool{}
	for _, r := range selected {
		drop[sanitizeArchiveName(r.Slug)] = true
		drop[r.Slug] = true
	}
	var out []string
	for _, s := range all {
		if !drop[s] && !drop[sanitizeArchiveName(s)] {
			out = append(out, s)
		}
	}
	return out
}

// pruneAgents names the agents whose usage belongs to slug: the archived
// sessions, the history's worker and reviewers, and the run record's
// orchestrator. The archive directory name stands in when a session record
// cannot be read.
func pruneAgents(slug string) []string {
	seen := map[string]bool{}
	add := func(a string) {
		if a != "" {
			seen[a] = true
		}
	}
	dir := filepath.Join(archiveRoot(), sanitizeArchiveName(slug))
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name(), "session.json"))
			if err == nil {
				var rec SessionRecord
				if json.Unmarshal(raw, &rec) == nil && rec.Agent != "" {
					add(rec.Agent)
					continue
				}
			}
			add(e.Name())
		}
	}
	if entries, err := loadHistory(slug); err == nil {
		for _, h := range entries {
			add(h.Worker)
			for _, r := range h.Reviews {
				add(r.AgentID)
			}
		}
	}
	if store, err := loadRuns(); err == nil {
		if rec, ok := store[slug]; ok {
			add(rec.AgentID)
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// pruneRunTime is the age a run is pruned by: its end when finished, its
// start otherwise. A run with no record (adhoc archives) has no end or start
// to date it by, so it is dated by activity instead: the newest modification
// time of any file in its archive tree. A live session keeps appending to its
// transcript and memory log, so its activity stays fresh and --older-than
// passes over it; a session start would date it old from birth. ok is false
// when nothing dates the run, and an age filter then skips it rather than
// guessing.
func pruneRunTime(slug string) (at time.Time, ok bool) {
	if store, err := loadRuns(); err == nil {
		if rec, found := store[slug]; found {
			if t, err := time.Parse(time.RFC3339, rec.Ended); err == nil {
				return t, true
			}
			if t, err := time.Parse(time.RFC3339, rec.Started); err == nil {
				return t, true
			}
		}
	}
	dir := filepath.Join(archiveRoot(), sanitizeArchiveName(slug))
	if newest, found := newestFileTime(dir); found {
		return newest, true
	}
	if info, err := os.Stat(historyPath(slug)); err == nil {
		return info.ModTime().UTC(), true
	}
	return time.Time{}, false
}

// newestFileTime is the latest modification time of any file under root, for
// dating a record-less archive by activity. Directories themselves are
// ignored: appending to a transcript does not touch its directory's mtime.
func newestFileTime(root string) (time.Time, bool) {
	var newest time.Time
	found := false
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && (!found || info.ModTime().After(newest)) {
			newest, found = info.ModTime().UTC(), true
		}
		return nil
	})
	return newest, found
}

// pruneRunBytes sizes a run: its archive tree, its history file, and its
// agents' usage logs with their tail state.
func pruneRunBytes(slug string, agents []string) int64 {
	var total int64
	total += dirSize(filepath.Join(archiveRoot(), sanitizeArchiveName(slug)))
	total += fileSize(historyPath(slug))
	for _, a := range agents {
		total += fileSize(UsageLogPath(a))
		total += fileSize(usageTailPath(a))
	}
	return total
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func orphanBlobNames(kept []string) ([]string, error) {
	refs := map[string]bool{}
	for _, slug := range kept {
		dir := filepath.Join(archiveRoot(), sanitizeArchiveName(slug))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name(), "session.json"))
			if err != nil {
				continue
			}
			var rec SessionRecord
			if json.Unmarshal(raw, &rec) != nil {
				continue
			}
			for _, h := range []string{rec.SystemPrompt, rec.Tools, rec.Flags} {
				if h != "" {
					refs[strings.TrimPrefix(h, "sha256:")] = true
				}
			}
		}
	}
	entries, err := os.ReadDir(blobDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		// StoreBlob writes through blob-* temp files and renames them into
		// place; an unreferenced blob-* is an in-flight snapshot, never an
		// orphan, and deleting it would fail that StoreBlob.
		if e.IsDir() || refs[e.Name()] || strings.HasPrefix(e.Name(), "blob-") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

func blobBytesOf(names []string) int64 {
	var total int64
	for _, n := range names {
		total += fileSize(filepath.Join(blobDir(), n))
	}
	return total
}

// deletePruneRun removes one run's archive, history and run record. Usage
// logs go separately, so agents shared with a kept run survive.
// deletePruneRun removes one run's archive, history and run record. Usage
// logs go separately, so agents shared with a kept run survive. The first
// failure stops that run's own cleanup and is reported; other runs still
// prune.
func deletePruneRun(slug string) error {
	if err := os.RemoveAll(filepath.Join(archiveRoot(), sanitizeArchiveName(slug))); err != nil {
		return err
	}
	if err := os.Remove(historyPath(slug)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return updateRuns(func(store map[string]RunRecord) error {
		delete(store, slug)
		return nil
	})
}

// deletePruneUsageRows deletes the state-database rows of agents that belong
// only to pruned runs: their usage rows (the database form of the usage logs
// removed above) and their overflow-store blobs (scoped to the session, which
// is the agent id). It also purges overflow rows older than 24 hours, so a
// prune collects expired blobs even when their sessions survive. Agents
// shared with a kept run never reach here. A state with no database yet (no
// usage ever tailed) is nothing to delete, not an error.
func deletePruneUsageRows(agents []string) error {
	if _, err := os.Stat(dbPath()); os.IsNotExist(err) {
		return nil
	}
	db, err := statedb.Open(dbPath())
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := statedb.PurgeExpiredCtx(db, time.Now().UTC().Unix()); err != nil {
		return err
	}
	if len(agents) == 0 {
		return nil
	}
	if _, err := statedb.DeleteUsageForAgents(db, agents); err != nil {
		return err
	}
	_, err = statedb.DeleteCtxForSessions(db, agents)
	return err
}

// purgeExpiredCtxRows purges overflow-store rows older than 24 hours,
// returning how many went. A state with no database yet is nothing to purge,
// not an error. The due count is read first over a read-only handle: the
// migrating open below purges as a side effect, so counting after it would
// always report zero.
func purgeExpiredCtxRows() (int64, error) {
	if _, err := os.Stat(dbPath()); os.IsNotExist(err) {
		return 0, nil
	}
	var due int64
	if ro, err := statedb.OpenReadOnly(dbPath()); err == nil {
		_ = ro.QueryRow(`SELECT COUNT(*) FROM ctx_blobs WHERE created_at <= ?`,
			time.Now().UTC().Unix()-statedb.CtxTTLSeconds).Scan(&due)
		_ = ro.Close()
	}
	db, err := statedb.Open(dbPath())
	if err != nil {
		return 0, err
	}
	defer db.Close()
	if _, err := statedb.PurgeExpiredCtx(db, time.Now().UTC().Unix()); err != nil {
		return 0, err
	}
	return due, nil
}

// ParseOlderThan parses an --older-than age: a Go duration (72h, 15m) or a
// day/week count (30d, 2w). Days and weeks are what age filters are written
// in; time.ParseDuration is what handles the rest.
func ParseOlderThan(s string) (time.Duration, error) {
	t := strings.TrimSpace(s)
	if d, ok := strings.CutSuffix(t, "d"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(d))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("age %q is not a positive day count", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if w, ok := strings.CutSuffix(t, "w"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(w))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("age %q is not a positive week count", s)
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(t)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("age %q is not a positive duration (try 72h, 30d, 2w)", s)
	}
	return d, nil
}

// statsUsage is printed on a bad stats invocation. It documents the one
// decision the ticket leaves open: pruning dry runs by default, and only
// --apply deletes.
const statsUsage = `usage: wick-pig stats prune [--run <slug>]... [--older-than <age>] [--apply]

  Prune archived run data on purpose; nothing is ever deleted automatically.
  A prune dry runs by default: it lists the runs that would go and how much
  space they would free. Pass --apply to delete.

  --run <slug>      prune only this run; repeat for more than one
  --older-than <age> prune only runs ended (or started, when unfinished)
                    that long ago: a Go duration (72h) or days/weeks (30d, 2w)
  --apply           delete the selection; without it, only list
  --dry-run         list only (the default; overrides --apply)

  Both filters combine: a run must match every filter given. No filter
  selects every finished run. A run whose orchestrator pane is still alive
  in herdr is never touched, even with --apply. When herdr cannot be asked
  which panes are alive, --apply refuses instead of guessing.

  Per pruned run this removes the archive (transcripts, memory logs), the
  ticket history, the run record and the usage logs of its agents, plus the
  blobs no surviving run references.`

// RunStats serves `wick-pig stats`. Today stats has one subcommand, prune;
// argv is what follows "stats".
func RunStats(argv []string, out io.Writer) error {
	if len(argv) == 0 || argv[0] == "-h" || argv[0] == "--help" || argv[0] == "help" {
		fmt.Fprintln(out, statsUsage)
		return nil
	}
	if argv[0] != "prune" {
		return fmt.Errorf("unknown stats command %q\n%s", argv[0], statsUsage)
	}
	return runStatsPrune(argv[1:], out)
}

func runStatsPrune(argv []string, out io.Writer) error {
	var opts PruneOptions
	dry := false
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--run" || a == "-run":
			i++
			if i >= len(argv) {
				return fmt.Errorf("--run needs a slug\n%s", statsUsage)
			}
			opts.Runs = append(opts.Runs, argv[i])
		case strings.HasPrefix(a, "--run="):
			opts.Runs = append(opts.Runs, strings.TrimPrefix(a, "--run="))
		case a == "--older-than" || a == "-older-than":
			i++
			if i >= len(argv) {
				return fmt.Errorf("--older-than needs an age\n%s", statsUsage)
			}
			d, err := ParseOlderThan(argv[i])
			if err != nil {
				return fmt.Errorf("%v\n%s", err, statsUsage)
			}
			opts.OlderThan = d
		case strings.HasPrefix(a, "--older-than="):
			d, err := ParseOlderThan(strings.TrimPrefix(a, "--older-than="))
			if err != nil {
				return fmt.Errorf("%v\n%s", err, statsUsage)
			}
			opts.OlderThan = d
		case a == "--apply" || a == "-apply":
			opts.Apply = true
		case a == "--dry-run" || a == "-dry-run":
			dry = true
		case a == "--help" || a == "-h" || a == "help":
			fmt.Fprintln(out, statsUsage)
			return nil
		default:
			return fmt.Errorf("unknown flag %q\n%s", a, statsUsage)
		}
	}
	if dry {
		opts.Apply = false
	}
	res, err := PruneRuns(OSExec, time.Now().UTC(), opts)
	if err != nil {
		return err
	}
	printPruneResult(out, opts, res)
	return nil
}

func printPruneResult(out io.Writer, opts PruneOptions, res PruneResult) {
	verb := "would prune"
	tail := " (dry run; pass --apply to delete)"
	if res.Applied {
		verb, tail = "pruned", ""
	}
	if len(res.Runs) == 0 && len(res.SkippedRunning) == 0 {
		fmt.Fprintf(out, "nothing to prune%s\n", tail)
		return
	}
	fmt.Fprintf(out, "%s %d run(s), freeing %s%s\n", verb, len(res.Runs), formatBytes(res.TotalBytes), tail)
	for _, r := range res.Runs {
		fmt.Fprintf(out, "  %-30s %s\n", r.Slug, formatBytes(r.Bytes))
	}
	if res.BlobFiles > 0 {
		fmt.Fprintf(out, "  orphaned blobs: %d file(s), %s\n", res.BlobFiles, formatBytes(res.BlobBytes))
	}
	if res.CtxPurged > 0 {
		fmt.Fprintf(out, "  expired overflow rows purged: %d\n", res.CtxPurged)
	}
	for _, s := range res.SkippedRunning {
		fmt.Fprintf(out, "  skipped running run: %s\n", s)
	}
	if res.LivenessUnknown {
		fmt.Fprintf(out, "  warning: herdr could not be asked which panes are alive; --apply would refuse\n")
	}
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB"}
	f := float64(n) / 1024
	for _, u := range units {
		if f < 1024 || u == "GB" {
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%d B", n)
}
