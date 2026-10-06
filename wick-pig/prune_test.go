package wick

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/wick/statedb"
)

var errPruneHerdrDown = errors.New("herdr down")

// Seam (ticket 07): prune planning on a fixture archive — a dry run by --run
// lists the selected runs and their size, and deletes nothing.
func TestPruneDryRunByRunListsSizeAndDeletesNothing(t *testing.T) {
	pruneFixture(t)

	now := time.Now().UTC()
	res, err := PruneRuns(pruneDeadExec(t), now, PruneOptions{Runs: []string{"old-run"}})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if res.Applied {
		t.Error("a prune without --apply is a dry run and must not apply")
	}
	if len(res.Runs) != 1 || res.Runs[0].Slug != "old-run" {
		t.Fatalf("selected = %+v, want one run old-run", res.Runs)
	}
	if res.Runs[0].Bytes <= 0 {
		t.Errorf("a selected run must report its size, got %d", res.Runs[0].Bytes)
	}
	if res.TotalBytes < res.Runs[0].Bytes {
		t.Errorf("total %d must cover the run %d", res.TotalBytes, res.Runs[0].Bytes)
	}
	// Nothing is gone: the archive, history and usage log survive a dry run.
	for _, p := range []string{
		SessionFilePath("old-run", "agent-old"),
		TranscriptCopyPath("old-run", "agent-old"),
		MemoryLogPath("old-run", "agent-old"),
		historyPath("old-run"),
		UsageLogPath("agent-old"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("dry run must delete nothing, missing %s: %v", p, err)
		}
	}
}

// pruneDeadExec answers `herdr pane list` the way the repo's herdr fakes do:
// valid JSON with no panes alive, so nothing is skipped as running.
func pruneDeadExec(t *testing.T) Exec {
	t.Helper()
	return func(cmd string, args []string, o ExecOpts) (Result, error) {
		if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
			return Result{Code: 0, Stdout: `{"result":{"panes":[]}}`}, nil
		}
		return Result{Code: 1, Stderr: `no such command`}, nil
	}
}

// pruneLiveExec answers `pane list` with pane alive, the way herdr reports a
// session that is still running.
func pruneLiveExec(pane string) Exec {
	return func(cmd string, args []string, o ExecOpts) (Result, error) {
		if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
			return Result{Code: 0, Stdout: `{"result":{"panes":[{"pane_id":"` + pane + `"}]}}`}, nil
		}
		return Result{Code: 1, Stderr: `no such command`}, nil
	}
}

// pruneBrokenExec stands in for a herdr that cannot be asked: every call
// fails, so liveness is unknown.
func pruneBrokenExec(cmd string, args []string, o ExecOpts) (Result, error) {
	return Result{}, errPruneHerdrDown
}

// pruneFixture builds a two-run archive in an isolated state dir: old-run
// (ended long ago) and new-run (just started), each with a session snapshot,
// a transcript copy, memory samples, a usage log, ticket history and a run
// record. Blobs are shared where the sessions agree, distinct where they do
// not, so blob pruning has something to keep and something to drop.
func pruneFixture(t *testing.T) {
	t.Helper()
	isolateState(t)
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Format(time.RFC3339)

	mustSnapshot(t, "old-run", "agent-old", old)
	mustSnapshot(t, "new-run", "agent-new", recent)

	writePruneFile(t, TranscriptCopyPath("old-run", "agent-old"), "{\"n\":1}\n")
	writePruneFile(t, TranscriptCopyPath("new-run", "agent-new"), "{\"n\":1}\n{\"n\":2}\n")
	if err := AppendMemorySamples(MemoryLogPath("old-run", "agent-old"),
		[]MemorySample{{Agent: "agent-old", At: old, PID: 10, Name: "pig", RSS: 1024}}); err != nil {
		t.Fatal(err)
	}
	if err := AppendMemorySamples(MemoryLogPath("new-run", "agent-new"),
		[]MemorySample{{Agent: "agent-new", At: recent, PID: 11, Name: "pig", RSS: 2048}}); err != nil {
		t.Fatal(err)
	}
	writePruneFile(t, UsageLogPath("agent-old"), "{\"agent\":\"agent-old\"}\n")
	writePruneFile(t, usageTailPath("agent-old"), "{\"offset\":1}\n")
	writePruneFile(t, UsageLogPath("agent-new"), "{\"agent\":\"agent-new\"}\n")
	if err := saveHistory("old-run", []TicketHistory{{Ticket: "t/01.md", Outcome: "merged", Ended: old}}); err != nil {
		t.Fatal(err)
	}
	if err := saveHistory("new-run", []TicketHistory{{Ticket: "t/02.md", Outcome: "merged", Ended: recent}}); err != nil {
		t.Fatal(err)
	}
	if err := updateRuns(func(store map[string]RunRecord) error {
		store["old-run"] = RunRecord{Slug: "old-run", Spec: "/s/spec.md", Repo: "/repo",
			Branch: "spec/s", AgentID: "agent-old", Harness: "pig", Started: old, Ended: old}
		store["new-run"] = RunRecord{Slug: "new-run", Spec: "/s/spec.md", Repo: "/repo",
			Branch: "spec/s", AgentID: "agent-new", Harness: "pig", Started: recent}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func mustSnapshot(t *testing.T, run, agent, started string) {
	t.Helper()
	if _, err := SnapshotSession(SessionSnapshot{
		Run: run, Agent: agent, Harness: "pig", Role: "review", Started: started,
		SystemPrompt: "shared prompt", ToolsJSON: []byte(`[{"name":"read"}]`),
		FlagsJSON: FlagsJSON("pig", "m1", "high"),
	}); err != nil {
		t.Fatalf("SnapshotSession(%s/%s): %v", run, agent, err)
	}
}

func writePruneFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Seam (ticket 07): apply — pruning one run removes its transcripts, usage
// logs, memory logs and history, and leaves the other run alone.
func TestPruneApplyDeletesTheSelectionAndKeepsTheRest(t *testing.T) {
	pruneFixture(t)

	res, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if !res.Applied {
		t.Error("a prune with --apply must apply")
	}
	for _, p := range []string{
		SessionFilePath("old-run", "agent-old"),
		TranscriptCopyPath("old-run", "agent-old"),
		MemoryLogPath("old-run", "agent-old"),
		historyPath("old-run"),
		UsageLogPath("agent-old"),
		usageTailPath("agent-old"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must be gone, stat err = %v", p, err)
		}
	}
	if store, err := loadRuns(); err != nil || len(store) != 1 || store["new-run"].Slug != "new-run" {
		t.Errorf("the run record of old-run must be gone and new-run kept: %+v, %v", store, err)
	}
	// The other run survives whole.
	for _, p := range []string{
		SessionFilePath("new-run", "agent-new"),
		TranscriptCopyPath("new-run", "agent-new"),
		historyPath("new-run"),
		UsageLogPath("agent-new"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("new-run must survive, missing %s: %v", p, err)
		}
	}
}

// Seam (ticket 07): blobs — pruning drops the blobs nothing references and
// keeps the ones a surviving run still references.
func TestPruneRemovesOrphanedBlobsAndKeepsSharedOnes(t *testing.T) {
	pruneFixture(t)
	// A blob only old-run references: pruning old-run must drop it.
	onlyOld, err := StoreBlob([]byte("prompt only old-run uses"))
	if err != nil {
		t.Fatal(err)
	}
	rec := readSession(t, "old-run", "agent-old")
	rec.SystemPrompt = onlyOld
	raw := mustMarshal(t, rec)
	writePruneFile(t, SessionFilePath("old-run", "agent-old"), string(raw))
	// The shared prompt blob both sessions reference must survive.
	shared := readSession(t, "new-run", "agent-new").SystemPrompt

	if _, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true}); err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if _, err := os.Stat(BlobPath(onlyOld)); !os.IsNotExist(err) {
		t.Errorf("the orphaned blob must be gone, stat err = %v", err)
	}
	if _, err := os.Stat(BlobPath(shared)); err != nil {
		t.Errorf("a blob the surviving run references must stay: %v", err)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

// Seam (ticket 07): running runs — a run whose orchestrator pane is still
// alive is never touched, even with --apply.
func TestPruneNeverTouchesARunningRun(t *testing.T) {
	pruneFixture(t)
	if err := updateRuns(func(store map[string]RunRecord) error {
		rec := store["old-run"]
		rec.Pane = "w9:p9"
		store["old-run"] = rec
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	res, err := PruneRuns(pruneLiveExec("w9:p9"), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if len(res.Runs) != 0 {
		t.Errorf("a running run must not be selected, got %+v", res.Runs)
	}
	if len(res.SkippedRunning) != 1 || res.SkippedRunning[0] != "old-run" {
		t.Errorf("skipped = %+v, want [old-run]", res.SkippedRunning)
	}
	for _, p := range []string{
		SessionFilePath("old-run", "agent-old"),
		historyPath("old-run"),
		UsageLogPath("agent-old"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("a running run must survive, missing %s: %v", p, err)
		}
	}
}

// Seam (ticket 07): age — --older-than selects only the runs whose end (or
// start, when unfinished) is that far in the past.
func TestPruneOlderThanSelectsOnlyOldRuns(t *testing.T) {
	pruneFixture(t)
	now := time.Now().UTC()
	res, err := PruneRuns(pruneDeadExec(t), now, PruneOptions{OlderThan: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if len(res.Runs) != 1 || res.Runs[0].Slug != "old-run" {
		t.Fatalf("older-than 30d = %+v, want [old-run]", res.Runs)
	}
	fresh, err := PruneRuns(pruneDeadExec(t), now, PruneOptions{OlderThan: 365 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if len(fresh.Runs) != 0 {
		t.Errorf("older-than 365d = %+v, want none", fresh.Runs)
	}
}

// Seam (ticket 07): age parsing — --older-than takes Go durations and
// day/week counts, and refuses the rest.
func TestParseOlderThanTakesDurationsDaysAndWeeks(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"30d": 30 * 24 * time.Hour,
		"2w":  2 * 7 * 24 * time.Hour,
		"72h": 72 * time.Hour,
		"90m": 90 * time.Minute,
	} {
		got, err := ParseOlderThan(in)
		if err != nil || got != want {
			t.Errorf("ParseOlderThan(%q) = %v, %v; want %v, nil", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "soon", "0d", "-3d", "0", "10x"} {
		if _, err := ParseOlderThan(bad); err == nil {
			t.Errorf("ParseOlderThan(%q) must fail", bad)
		}
	}
}

// Seam (ticket 07): stats CLI — `stats prune` dry runs by default, listing
// runs and size; --apply deletes; --dry-run overrides --apply.
func TestStatsPruneCLIDryRunsByDefaultAndAppliesOnRequest(t *testing.T) {
	pruneFixture(t)
	var out strings.Builder
	if err := RunStats([]string{"prune", "--run", "old-run"}, &out); err != nil {
		t.Fatalf("RunStats: %v", err)
	}
	if !strings.Contains(out.String(), "old-run") || !strings.Contains(out.String(), "dry run") {
		t.Errorf("a default prune must dry run and name the run, got:\n%s", out.String())
	}
	if _, err := os.Stat(SessionFilePath("old-run", "agent-old")); err != nil {
		t.Errorf("a dry run must delete nothing: %v", err)
	}
	out.Reset()
	if err := RunStats([]string{"prune", "--run", "old-run", "--apply"}, &out); err != nil {
		t.Fatalf("RunStats --apply: %v", err)
	}
	if strings.Contains(out.String(), "dry run") {
		t.Errorf("an --apply prune must not call itself a dry run:\n%s", out.String())
	}
	if _, err := os.Stat(SessionFilePath("old-run", "agent-old")); !os.IsNotExist(err) {
		t.Errorf("an --apply prune must delete, stat err = %v", err)
	}
	// --dry-run wins over --apply: new-run survives.
	out.Reset()
	if err := RunStats([]string{"prune", "--run", "new-run", "--apply", "--dry-run"}, &out); err != nil {
		t.Fatalf("RunStats --dry-run: %v", err)
	}
	if _, err := os.Stat(SessionFilePath("new-run", "agent-new")); err != nil {
		t.Errorf("--dry-run must override --apply: %v", err)
	}
}

// The stats CLI refuses what it does not know, with usage.
func TestStatsCLIRefusesUnknownCommandsAndFlags(t *testing.T) {
	var out strings.Builder
	if err := RunStats([]string{"bogus"}, &out); err == nil {
		t.Error("an unknown stats command must fail")
	}
	if err := RunStats([]string{"prune", "--bogus"}, &out); err == nil {
		t.Error("an unknown prune flag must fail")
	}
	if err := RunStats([]string{"prune", "--older-than", "soon"}, &out); err == nil {
		t.Error("a bad --older-than must fail")
	}
}

// Seam (ticket 07): unknown liveness — when herdr cannot be asked, a dry run
// still lists (nothing is at risk) but an apply refuses rather than deletes
// blind. Either way nothing is deleted.
func TestPruneWithUnknownLivenessRefusesToApply(t *testing.T) {
	pruneFixture(t)
	if err := updateRuns(func(store map[string]RunRecord) error {
		rec := store["old-run"]
		rec.Pane = "w9:p9"
		store["old-run"] = rec
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	dry, err := PruneRuns(pruneBrokenExec, time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}})
	if err != nil {
		t.Fatalf("a dry run with unknown liveness must still list: %v", err)
	}
	if !dry.LivenessUnknown || len(dry.Runs) != 1 {
		t.Errorf("dry = %+v, want one run flagged unknown", dry)
	}
	if _, err := PruneRuns(pruneBrokenExec, time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true}); err == nil {
		t.Error("an apply with unknown liveness must refuse")
	}
	if _, err := os.Stat(SessionFilePath("old-run", "agent-old")); err != nil {
		t.Errorf("a refused apply must delete nothing: %v", err)
	}
}

// Seam (ticket 07): shared agents — a usage log belonging to an agent of both
// a pruned and a kept run survives the prune.
func TestPruneKeepsUsageLogsSharedWithAKeptRun(t *testing.T) {
	pruneFixture(t)
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	mustSnapshot(t, "new-run", "agent-old", old)
	writePruneFile(t, TranscriptCopyPath("new-run", "agent-old"), "{\"n\":9}\n")
	if _, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true}); err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if _, err := os.Stat(UsageLogPath("agent-old")); err != nil {
		t.Errorf("a usage log shared with the kept run must survive: %v", err)
	}
	if _, err := os.Stat(usageTailPath("agent-old")); err != nil {
		t.Errorf("shared tail state must survive too: %v", err)
	}
	if _, err := os.Stat(UsageLogPath("agent-new")); err != nil {
		t.Errorf("the kept run's own log must survive: %v", err)
	}
}

// Seam (ticket 07): combined filters — --run and --older-than intersect, and
// an apply reports the bytes it freed instead of measuring afterwards.
func TestPruneFiltersCombineAndApplyReportsBytes(t *testing.T) {
	pruneFixture(t)
	now := time.Now().UTC()
	both, err := PruneRuns(pruneDeadExec(t), now,
		PruneOptions{Runs: []string{"old-run", "new-run"}, OlderThan: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if len(both.Runs) != 1 || both.Runs[0].Slug != "old-run" {
		t.Fatalf("run+age = %+v, want only old-run", both.Runs)
	}
	applied, err := PruneRuns(pruneDeadExec(t), now, PruneOptions{Apply: true})
	if err != nil {
		t.Fatalf("PruneRuns apply: %v", err)
	}
	if !applied.Applied || len(applied.Runs) != 2 {
		t.Fatalf("bare apply = %+v, want both runs", applied)
	}
	if applied.BlobBytes <= 0 {
		t.Errorf("an apply must report its orphaned blob bytes, got %+v", applied)
	}
	var runs int64
	for _, r := range applied.Runs {
		runs += r.Bytes
	}
	if applied.TotalBytes != runs+applied.BlobBytes {
		t.Errorf("total %d must cover runs %d plus blobs %d", applied.TotalBytes, runs, applied.BlobBytes)
	}
}

// Seam (ticket 07): record-less runs — an adhoc-style archive with no run
// record is dated by activity, not by its session start, so a live session
// started long ago is passed over while a stale one is selected.
func TestPruneDatesRecordLessRunsByActivity(t *testing.T) {
	isolateState(t)
	now := time.Now().UTC()
	old := now.Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	mustSnapshot(t, "adhoc", "agent-a", old)
	writePruneFile(t, TranscriptCopyPath("adhoc", "agent-a"), "{\"n\":1}\n")
	fresh, err := PruneRuns(pruneDeadExec(t), now, PruneOptions{OlderThan: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if len(fresh.Runs) != 0 {
		t.Fatalf("a live record-less archive must be passed over, got %+v", fresh.Runs)
	}
	stale := now.Add(-60 * 24 * time.Hour)
	_ = filepath.WalkDir(archiveRoot(), func(p string, d os.DirEntry, err error) error {
		if err == nil {
			_ = os.Chtimes(p, stale, stale)
		}
		return nil
	})
	aged, err := PruneRuns(pruneDeadExec(t), now, PruneOptions{OlderThan: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if len(aged.Runs) != 1 || aged.Runs[0].Slug != "adhoc" {
		t.Fatalf("a stale record-less archive must be selected, got %+v", aged.Runs)
	}
}

// Seam (ticket 03): prune deletes a run's usage rows as well as its files —
// pruning old-run removes agent-old's rows and keeps agent-new's, and a row
// belonging to an agent of both a pruned and a kept run survives.
func TestPruneApplyDeletesUsageRowsAndKeepsSharedOnes(t *testing.T) {
	pruneFixture(t)
	insertPruneRow(t, "agent-old", 100)
	insertPruneRow(t, "agent-new", 200)

	if _, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true}); err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if got := pruneRows(t, "agent-old"); len(got) != 0 {
		t.Errorf("pruned rows = %+v, want none", got)
	}
	if got := pruneRows(t, "agent-new"); len(got) != 1 || got[0].Input != 200 {
		t.Errorf("kept rows = %+v, want the surviving agent's row", got)
	}

	// Sharing works at the row level too: agent-old joins the kept run, so
	// pruning old-run must leave its rows alone next time.
	pruneFixture(t)
	insertPruneRow(t, "agent-old", 100)
	mustSnapshot(t, "new-run", "agent-old", time.Now().UTC().Format(time.RFC3339))
	if _, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true}); err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if got := pruneRows(t, "agent-old"); len(got) != 1 {
		t.Errorf("shared rows = %+v, want the row kept with the surviving run", got)
	}
}

// Seam (ticket 09): an applied prune with no run selected still collects
// expired overflow rows.
func TestPruneApplyWithNoRunsStillPurgesExpired(t *testing.T) {
	pruneFixture(t)
	freshID := insertPruneCtx(t, "agent-new", "fresh blob body\n", time.Now().UTC())
	expiredID := insertPruneCtx(t, "agent-new", "expired blob body\n", time.Now().UTC().Add(-25*time.Hour))

	res, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"no-such-run"}, Apply: true})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if !res.Applied || res.CtxPurged != 1 {
		t.Errorf("applied empty prune purged %d rows, want 1: %+v", res.CtxPurged, res)
	}
	db, err := statedb.Open(dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, ok, _ := statedb.GetCtxBlob(db, expiredID); ok {
		t.Errorf("expired blob %d survived an applied prune", expiredID)
	}
	if _, ok, _ := statedb.GetCtxBlob(db, freshID); !ok {
		t.Errorf("fresh blob %d is gone after an applied prune", freshID)
	}
}

// Seam (ticket 09): prune deletes a pruned run's overflow blobs and purges
// expired ones — agent-old's blob goes with old-run, agent-new's stays, and
// an expired blob of the surviving session is purged too.
func TestPruneApplyDeletesCtxBlobsAndPurgesExpired(t *testing.T) {
	pruneFixture(t)
	oldID := insertPruneCtx(t, "agent-old", "fresh blob body old\n", time.Now().UTC())
	newID := insertPruneCtx(t, "agent-new", "fresh blob body new\n", time.Now().UTC())
	expiredID := insertPruneCtx(t, "agent-new", "expired blob body\n", time.Now().UTC().Add(-25*time.Hour))

	res, err := PruneRuns(pruneDeadExec(t), time.Now().UTC(), PruneOptions{Runs: []string{"old-run"}, Apply: true})
	if err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}
	if res.CtxPurged != 1 {
		t.Errorf("prune purged %d expired rows, want 1: %+v", res.CtxPurged, res)
	}
	db, err := statedb.Open(dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, ok, _ := statedb.GetCtxBlob(db, oldID); ok {
		t.Errorf("blob %d of pruned agent-old survived", oldID)
	}
	if _, ok, _ := statedb.GetCtxBlob(db, newID); !ok {
		t.Errorf("blob %d of surviving agent-new is gone", newID)
	}
	if _, ok, _ := statedb.GetCtxBlob(db, expiredID); ok {
		t.Errorf("expired blob %d survived the prune", expiredID)
	}
}

func insertPruneCtx(t *testing.T, session, body string, at time.Time) int64 {
	t.Helper()
	db, err := statedb.Open(dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := statedb.InsertCtxBlob(db, statedb.CtxBlob{Session: session, Source: "prune-test", Bytes: body, CreatedAt: at.Unix()}, statedb.ChunkText(body))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertPruneRow(t *testing.T, agent string, input int64) {
	t.Helper()
	db, err := statedb.Open(dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := statedb.InsertUsageRow(db, statedb.UsageRow{
		Agent: agent, At: time.Now().UTC().Format(time.RFC3339),
		Harness: "pig", Kind: statedb.KindRequest,
		Input: input, DedupeKey: "prune-test:" + agent,
	}); err != nil {
		t.Fatal(err)
	}
}

func pruneRows(t *testing.T, agent string) []statedb.UsageRow {
	t.Helper()
	// A prune that removed every row may leave no database behind at all.
	if _, err := os.Stat(dbPath()); os.IsNotExist(err) {
		return nil
	}
	db, err := statedb.Open(dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := statedb.UsageRowsForAgent(db, agent)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
