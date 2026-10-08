package emberloop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The driver seam is RunDriver over an injected world: the spec plus ticket
// files, a fake clock, and a hook that records worker starts. Every test here
// goes through that seam and asserts on the returned order and the recorded
// calls, never on the parser directly.
// driverWorld builds a world whose ticket files live under one temp dir. The
// start hook records worker starts, so every check test also proves no worker
// starts, including on failure.
func driverWorld(t *testing.T, docs map[string]string) (SpecRun, DriverWorld, *[]string) {
	t.Helper()
	dir := t.TempDir()
	files := make(map[string]string, len(docs))
	for name, doc := range docs {
		files[filepath.Join(dir, name)] = doc
	}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-test"}
	var started []string
	world := DriverWorld{
		Files: files,
		Now:   func() time.Time { return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC) },
		StartWorker: func(tk DriverTicket) error {
			started = append(started, tk.ID)
			return nil
		},
	}
	return spec, world, &started
}

func ticketDoc(blocked string) string {
	return "# 00: Test ticket\n\n**Blocked by:** " + blocked + "\n"
}

func driverOrderIDs(t *testing.T, res DriverResult) []string {
	t.Helper()
	out := make([]string, 0, len(res.Order))
	for _, tk := range res.Order {
		out = append(out, tk.ID)
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A chain with a fan-out merges in dependency order: each ticket starts only
// after its blockers, and among ready tickets the lowest number goes first.
func TestDriverCheckOrdersChainAndFanoutByTicketNumber(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None (can start immediately)"),
		"02-second.md": ticketDoc("01"),
		"03-third.md":  ticketDoc("01"),
		"04-fourth.md": ticketDoc("02, 03"),
	})
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	want := []string{"01-first", "02-second", "03-third", "04-fourth"}
	if got := driverOrderIDs(t, res); !equalIDs(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if len(*started) != 0 {
		t.Errorf("check started workers %v; none may start", *started)
	}
}

// Ids come first and trailing free text is ignored: `02 Ticket graph` blocks
// on 02, and `None (can start immediately)` means no blockers. Full slugs and
// parenthesized asides resolve the same way.
func TestDriverCheckReadsIdsFirstAndIgnoresFreeText(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-base.md":  ticketDoc("None (can start immediately)"),
		"02-graph.md": ticketDoc("01 Ticket graph"),
		"03-loop.md":  ticketDoc("02-graph (the ticket graph run; both touch the driver)"),
		"04-free.md":  ticketDoc("nothing"),
	})
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	want := []string{"01-base", "02-graph", "03-loop", "04-free"}
	if got := driverOrderIDs(t, res); !equalIDs(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// A mention of `Blocked by` deeper in the text is not the field line: the
// real line below it still decides the blockers.
func TestDriverCheckIgnoresABlockedByMentionInProse(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": "# 02: Test ticket\n\nParses each `Blocked by` line first.\n\n**Blocked by:** 01-first\n",
	})
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	want := []string{"01-first", "02-second"}
	if got := driverOrderIDs(t, res); !equalIDs(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// Free text may hold commas of its own: a comma segment with no leading id
// continues the free text once it has started, while a segment with a leading
// id still adds blockers.
func TestDriverCheckIgnoresCommasInsideFreeText(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("01"),
		"03-third.md":  ticketDoc("01 Happy path, 02 Park, discard, follow-ups"),
	})
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	want := []string{"01-first", "02-second", "03-third"}
	if got := driverOrderIDs(t, res); !equalIDs(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if blockers := res.Order[2].Blockers; !equalIDs(blockers, []string{"01-first", "02-second"}) {
		t.Errorf("03-third blockers = %v, want [01-first 02-second]", blockers)
	}
}

// A Blocked by line with no leading ticket id fails naming the file and line,
// before any worker starts.
func TestDriverCheckFailsOnAnUnparsableLineNamingFileAndLine(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": "# 02: Test ticket\n\n**Blocked by:** someday maybe\n",
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail on an unparsable Blocked by line")
	}
	path := filepath.Join(spec.Tickets, "02-second.md")
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), ":3:") {
		t.Errorf("error must name the file and line, got: %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("a bad graph started workers %v; none may start", *started)
	}
}

// A Blocked by line without a colon is also unparsable, naming file and line.
func TestDriverCheckFailsOnABlockedByLineWithoutAColon(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": "# 01: Test ticket\n\n**Blocked by** None\n",
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail on a Blocked by line without a colon")
	}
	if !strings.Contains(err.Error(), filepath.Join(spec.Tickets, "01-first.md")) {
		t.Errorf("error must name the file, got: %v", err)
	}
}

// A ticket without any Blocked by line fails naming the file.
func TestDriverCheckFailsWhenABlockedByLineIsMissing(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"01-first.md": "# 01: Test ticket\n\nNo blockers here.\n",
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail when the Blocked by line is missing")
	}
	if !strings.Contains(err.Error(), filepath.Join(spec.Tickets, "01-first.md")) {
		t.Errorf("error must name the file, got: %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("a bad graph started workers %v; none may start", *started)
	}
}

// An id that names no ticket fails naming both the unknown id and the ticket
// that references it, before any worker starts.
func TestDriverCheckFailsOnAnUnknownIdNamingBothTickets(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("01, 09 Typo here"),
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail on an unknown ticket id")
	}
	if !strings.Contains(err.Error(), "09") || !strings.Contains(err.Error(), "02-second") {
		t.Errorf("error must name the unknown id and the referencing ticket, got: %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("a bad graph started workers %v; none may start", *started)
	}
}

// A dependency cycle fails naming the tickets in the cycle, before any worker
// starts.
func TestDriverCheckFailsOnACycleNamingTheTickets(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("03-third"),
		"03-third.md":  ticketDoc("02-second"),
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail on a dependency cycle")
	}
	if !strings.Contains(err.Error(), "02-second") || !strings.Contains(err.Error(), "03-third") {
		t.Errorf("error must name the tickets in the cycle, got: %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("a cyclic graph started workers %v; none may start", *started)
	}
}

// A ticket blocked by itself is a one-ticket cycle and fails the same way.
func TestDriverCheckFailsWhenATicketBlocksItself(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("01-first"),
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail when a ticket blocks itself")
	}
	if !strings.Contains(err.Error(), "01-first") {
		t.Errorf("error must name the ticket, got: %v", err)
	}
}

// A dangling dash is no ticket id: `02-` fails naming the file and line.
func TestDriverCheckFailsOnADanglingDash(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("02-"),
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail on a dangling dash")
	}
	if !strings.Contains(err.Error(), filepath.Join(spec.Tickets, "02-second.md")) {
		t.Errorf("error must name the file, got: %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("a bad graph started workers %v; none may start", *started)
	}
}

// A bare number matching two tickets is ambiguous and fails naming both.
func TestDriverCheckFailsOnAnAmbiguousNumber(t *testing.T) {
	spec, world, started := driverWorld(t, map[string]string{
		"02-alpha.md": ticketDoc("None"),
		"02-beta.md":  ticketDoc("None"),
		"03-third.md": ticketDoc("02"),
	})
	_, err := RunDriver(spec, world)
	if err == nil {
		t.Fatal("RunDriver must fail on an ambiguous ticket number")
	}
	if !strings.Contains(err.Error(), "02-alpha") || !strings.Contains(err.Error(), "02-beta") {
		t.Errorf("error must name both tickets, got: %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("a bad graph started workers %v; none may start", *started)
	}
}

// A run world is a check world plus the fake run operations: the worker tips,
// the reviewer verdicts at those tips, and a finish that merges. The fakes
// record every call, so run tests assert on calls and run state.
type runFake struct {
	started  []string
	finished []string
	active   int
	maxSeen  int
	tips     map[string]string
	reviews  map[string]map[string]DriverReview
	shas     map[string]string
}

// runWorld builds a spec plus a run world over constant tips with both
// reviewers passed at the tip, merging each ticket as sha-<id>.
func runWorld(t *testing.T, docs map[string]string, maxWorkers int) (SpecRun, DriverWorld, *runFake) {
	t.Helper()
	spec, world, _ := driverWorld(t, docs)
	fake := &runFake{tips: map[string]string{}, reviews: map[string]map[string]DriverReview{}, shas: map[string]string{}}
	for name := range docs {
		id := strings.TrimSuffix(name, ".md")
		tip := "tip-" + id
		fake.tips[id] = tip
		fake.reviews[id] = map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: tip},
			reviewerSpec:      {Verdict: verdictPass, At: tip},
		}
		fake.shas[id] = "sha-" + id
	}
	world.MaxWorkers = maxWorkers
	world.StartWorker = func(tk DriverTicket) error {
		fake.started = append(fake.started, tk.ID)
		fake.active++
		if fake.active > fake.maxSeen {
			fake.maxSeen = fake.active
		}
		return nil
	}
	world.Tip = func(id string) string { return fake.tips[id] }
	world.Reviews = func(id string) map[string]DriverReview { return fake.reviews[id] }
	world.Finish = func(id string) (DriverFinish, error) {
		fake.finished = append(fake.finished, id)
		fake.active--
		return DriverFinish{Merged: true, SHA: fake.shas[id]}, nil
	}
	return spec, world, fake
}

func indexOfID(ids []string, want string) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return -1
}

// A chain with a fan-out merges in dependency order and never exceeds
// max_workers: 04 starts only after 02 and 03 merged, and at most two
// workers run at once when the cap is two.
func TestDriverMergesChainAndFanoutInDependencyOrderWithinCap(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("01"),
		"03-third.md":  ticketDoc("01"),
		"04-fourth.md": ticketDoc("02, 03"),
	}, 2)
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 4 {
		t.Fatalf("merged = %v, want all 4 tickets", res.Merged)
	}
	for _, m := range res.Merged {
		if want := "sha-" + m.ID; m.SHA != want {
			t.Errorf("merged sha for %s = %q, want %q", m.ID, m.SHA, want)
		}
	}
	if fake.maxSeen > 2 {
		t.Errorf("max concurrent workers = %d, want at most 2", fake.maxSeen)
	}
	if indexOfID(fake.started, "01-first") != 0 {
		t.Errorf("start order = %v, want 01-first first", fake.started)
	}
	if start04 := indexOfID(fake.started, "04-fourth"); start04 < 0 {
		t.Errorf("04-fourth never started: %v", fake.started)
	} else {
		for _, dep := range []string{"02-second", "03-third"} {
			if fin := indexOfID(fake.finished, dep); fin < 0 || start04 < fin {
				t.Errorf("04-fourth started at %d but %s finished at %d: started=%v finished=%v", start04, dep, fin, fake.started, fake.finished)
			}
		}
	}
}

// Finish runs only after both reviewers passed at the tip: A merges, while
// B waits on a missing spec pass and C on a spec reviewer asking for
// changes. Neither B nor C is ever finished.
func TestDriverFinishWaitsForBothPassesAtTheTip(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("None"),
		"03-third.md":  ticketDoc("None"),
	}, 3)
	tip2, tip3 := fake.tips["02-second"], fake.tips["03-third"]
	fake.reviews["02-second"] = map[string]DriverReview{
		reviewerStandards: {Verdict: verdictPass, At: tip2},
	}
	fake.reviews["03-third"] = map[string]DriverReview{
		reviewerStandards: {Verdict: verdictPass, At: tip3},
		reviewerSpec:      {Verdict: verdictChanges, At: tip3},
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "01-first" {
		t.Fatalf("merged = %v, want only 01-first", res.Merged)
	}
	if len(fake.finished) != 1 || fake.finished[0] != "01-first" {
		t.Errorf("finished = %v, want only 01-first", fake.finished)
	}
	for _, line := range []string{"merged 01-first as sha-01-first", "unmerged 02-second", "unmerged 03-third"} {
		if !strings.Contains(res.Report, line) {
			t.Errorf("report misses %q:\n%s", line, res.Report)
		}
	}
}

// A branch change after a pass reads as unreviewed again: the tip moves from
// t1 to t2 between the two tip reads, so the pass at t1 never finishes, and
// finish runs only once both reviewers re-pass at t2.
func TestDriverTreatsAPostPassChangeAsUnreviewed(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	tips := []string{"t1", "t2", "t2", "t2", "t2", "t2"}
	calls, lastTip := 0, ""
	world.MaxWorkers = 1
	var finishedAt []string
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string {
		lastTip = tips[calls]
		if calls < len(tips)-1 {
			calls++
		}
		return lastTip
	}
	world.Reviews = func(id string) map[string]DriverReview {
		at := "t1"
		if calls >= 3 {
			at = "t2"
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: at},
			reviewerSpec:      {Verdict: verdictPass, At: at},
		}
	}
	world.Finish = func(id string) (DriverFinish, error) {
		finishedAt = append(finishedAt, lastTip)
		return DriverFinish{Merged: true, SHA: "sha-new"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(finishedAt) != 1 || finishedAt[0] != "t2" {
		t.Errorf("finish ran at %v, want exactly once at the current tip t2", finishedAt)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-new" {
		t.Errorf("merged = %v, want the re-passed tip", res.Merged)
	}
}

// The live world maps the seam onto the same operations the orchestrator
// uses: tips resolve the implement branches, verdicts read the review record
// the merge gate reads, and the required types come from the worker's own
// record. A run over that world merges through the seam.
func TestDriverLiveWorldMapsTipsReviewsAndRequiredTypes(t *testing.T) {
	oldState, oldExec := statePath, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; spawnExec = oldExec })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live"}
	id := WorkerID(ticket)
	tree := worktreePath(repo, id)
	if err := mergeReviewerVerdict(tree, reviewerStandards, verdictPass, "tipsha", "tipsha", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := mergeReviewerVerdict(tree, reviewerSpec, verdictPass, "tipsha", "tipsha", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := save(map[string]Worker{id: {ID: id, Ticket: ticket, Repo: repo, AgentType: "no-such-type"}}); err != nil {
		t.Fatal(err)
	}
	spawnExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" && len(args) == 2 && args[0] == "rev-parse" && args[1] == "implement/"+id {
			return Result{Code: 0, Stdout: "tipsha\n"}, nil
		}
		return Result{Code: 1, Stderr: "unexpected call"}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	world.MaxWorkers = 1
	world.Sleep = nil
	var started []string
	world.StartWorker = func(tk DriverTicket) error {
		started = append(started, tk.ID)
		return nil
	}
	world.Finish = func(ticketID string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "mergedsha"}, nil
	}
	if got := world.Tip(id); got != "tipsha" {
		t.Errorf("Tip = %q, want the implement branch sha", got)
	}
	if got := world.Reviews(id); got[reviewerStandards].At != "tipsha" || got[reviewerSpec].Verdict != verdictPass {
		t.Errorf("Reviews = %v, want both passes at the tip", got)
	}
	if got := world.RequiredFor(id); !equalIDs(got, defaultReviewers) {
		t.Errorf("RequiredFor = %v, want the built-in fallback for an unknown type", got)
	}
	if !world.HasWorker(id) {
		t.Errorf("HasWorker(%s) = false, want the recorded worker", id)
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "mergedsha" {
		t.Errorf("merged = %v, want the live-mapped ticket", res.Merged)
	}
	if len(started) != 0 {
		t.Errorf("started = %v, want no second worker for the recorded one", started)
	}
}

// The live world reads a resume from the worker store, the implement branch
// and the run history: a store record counts as a worker, a bare branch with
// no record counts too, and the last merged history entry gives the old sha.
func TestDriverLiveWorldReadsResumeFromStoreBranchAndHistory(t *testing.T) {
	oldState, oldExec := statePath, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; spawnExec = oldExec })
	repo, dir := t.TempDir(), t.TempDir()
	stored := filepath.Join(dir, "01-first.md")
	branched := filepath.Join(dir, "02-second.md")
	missing := filepath.Join(dir, "03-third.md")
	files := map[string]string{stored: ticketDoc("None"), branched: ticketDoc("None"), missing: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-resume"}
	if err := save(map[string]Worker{WorkerID(stored): {ID: WorkerID(stored), Ticket: stored, Repo: repo}}); err != nil {
		t.Fatal(err)
	}
	if err := saveHistory(specSlug(spec.Spec), []TicketHistory{
		{Ticket: stored, Outcome: outcomeMerged, SHA: "old-sha"},
		{Ticket: stored, Outcome: outcomeMerged, SHA: "new-sha"},
	}); err != nil {
		t.Fatal(err)
	}
	spawnExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == "git" && len(args) == 3 && args[0] == "rev-parse" && args[1] == "--verify" && args[2] == "implement/"+WorkerID(branched) {
			return Result{Code: 0, Stdout: "tipsha\n"}, nil
		}
		return Result{Code: 1, Stderr: "no such branch"}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	if !world.HasWorker(WorkerID(stored)) {
		t.Errorf("HasWorker(%s) = false, want the recorded worker", WorkerID(stored))
	}
	if !world.HasWorker(WorkerID(branched)) {
		t.Errorf("HasWorker(%s) = false, want the bare implement branch", WorkerID(branched))
	}
	if world.HasWorker(WorkerID(missing)) {
		t.Errorf("HasWorker(%s) = true, want nothing without a record or a branch", WorkerID(missing))
	}
	if got := world.MergedSHA(WorkerID(stored)); got != "new-sha" {
		t.Errorf("MergedSHA = %q, want the last merged history entry", got)
	}
	if got := world.MergedSHA(WorkerID(missing)); got != "" {
		t.Errorf("MergedSHA = %q, want empty without history", got)
	}
}

// A refused finish keeps its text on the unmerged line instead of reading as
// still running.
func TestDriverReportCarriesARefusedFinishText(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: false, Detail: "rebase onto spec/x conflicted in a.go"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 0 {
		t.Fatalf("merged = %v, want nothing", res.Merged)
	}
	want := "unmerged 01-first: rebase onto spec/x conflicted in a.go\n"
	if res.Report != want {
		t.Errorf("report = %q, want %q", res.Report, want)
	}
}

// The final report lists every merged ticket with its sha, one per line.
func TestDriverReportListsEveryMergedTicketWithSha(t *testing.T) {
	spec, world, _ := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("01"),
	}, 2)
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	want := "merged 01-first as sha-01-first\nmerged 02-second as sha-02-second\n"
	if res.Report != want {
		t.Errorf("report = %q, want %q", res.Report, want)
	}
}

// A refused finish reaches the worker byte for byte through tell, and finish
// runs again after the worker's next submit: the fixed ticket then merges.
// The refusal text keeps its newlines and spacing exactly.
func TestDriverRefusalReachesWorkerAndRefinishMergesAfterSubmit(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	tip := "tip-1"
	world.Tip = func(id string) string { return tip }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: tip},
			reviewerSpec:      {Verdict: verdictPass, At: tip},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go, b.go; the rebase was aborted, the branch is unchanged\n\tsecond line  with  spacing"
	var told []string
	submits := 0
	finished := 0
	world.Tell = func(id, text string) error {
		if id != "01-first" {
			t.Errorf("Tell ticket = %q, want 01-first", id)
		}
		told = append(told, text)
		// The worker fixes the conflict, is re-reviewed at the new tip, and
		// submits.
		tip = "tip-2"
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if finished == 1 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(told) != 1 || told[0] != refusal {
		t.Errorf("told = %q, want exactly the refusal text %q", told, refusal)
	}
	if finished != 2 {
		t.Errorf("finish calls = %d, want 2: one refusal and one retry after the submit", finished)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-fixed" {
		t.Errorf("merged = %v, want the fixed ticket", res.Merged)
	}
	if !strings.Contains(res.Report, "merged 01-first as sha-fixed") {
		t.Errorf("report misses the merge:\n%s", res.Report)
	}
}

// Without a next submit from the worker, finish never runs again: one
// refusal means one tell and one finish, not a busy retry loop.
func TestDriverWaitsForTheWorkerSubmitBeforeFinishingAgain(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	told, finished := 0, 0
	world.Tell = func(id, text string) error {
		told++
		// The worker never submits: no fix, no new tip, no new submit.
		return nil
	}
	world.Submits = func(id string) int { return 0 }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		return DriverFinish{Merged: false, Detail: "rebase onto spec/x conflicted in a.go"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if told != 1 {
		t.Errorf("tell calls = %d, want 1: the refusal is told once, then the driver waits", told)
	}
	if finished != 1 {
		t.Errorf("finish calls = %d, want 1: no submit means no retry", finished)
	}
	if len(res.Merged) != 0 {
		t.Errorf("merged = %v, want nothing", res.Merged)
	}
	if !strings.Contains(res.Report, "unmerged 01-first: rebase onto spec/x conflicted in a.go") {
		t.Errorf("report keeps the refusal:\n%s", res.Report)
	}
}

// Every real refusal kind loops the same way until the world lets it merge:
// a dirty worktree, an overlapping dirty integration checkout, and an
// aborted rebase conflict. Each text below is what finish really refuses
// with. The worker fixes it, submits, and the retry merges.
func TestDriverEachRefusalKindLoopsUntilTheWorldLetsItMerge(t *testing.T) {
	refusals := map[string]string{
		"dirty worktree":             "the worktree has uncommitted changes, which would be lost; have it commit them and call spawn of its reviewers again",
		"overlapping dirty checkout": "the integration checkout has uncommitted changes overlapping the incoming change in a.go, b.go; commit or stash them and finish again",
		"rebase conflict":            "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged",
	}
	for kind, refusal := range refusals {
		t.Run(kind, func(t *testing.T) {
			spec, world, _ := driverWorld(t, map[string]string{
				"01-first.md": ticketDoc("None"),
			})
			world.MaxWorkers = 1
			tip := "tip-1"
			world.Tip = func(id string) string { return tip }
			world.Reviews = func(id string) map[string]DriverReview {
				return map[string]DriverReview{
					reviewerStandards: {Verdict: verdictPass, At: tip},
					reviewerSpec:      {Verdict: verdictPass, At: tip},
				}
			}
			var told []string
			submits := 0
			finished := 0
			world.Tell = func(id, text string) error {
				told = append(told, text)
				tip = "tip-2"
				submits++
				return nil
			}
			world.Submits = func(id string) int { return submits }
			world.Finish = func(id string) (DriverFinish, error) {
				finished++
				if finished == 1 {
					return DriverFinish{Merged: false, Detail: refusal}, nil
				}
				return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
			}
			res, err := RunDriver(spec, world)
			if err != nil {
				t.Fatalf("RunDriver: %v", err)
			}
			if len(told) != 1 || told[0] != refusal {
				t.Errorf("told = %q, want the %s text %q", told, kind, refusal)
			}
			if finished != 2 {
				t.Errorf("finish calls = %d, want 2: refusal then retry after the submit", finished)
			}
			if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-fixed" {
				t.Errorf("merged = %v, want the fixed ticket", res.Merged)
			}
		})
	}
}

// There is no attempt limit: five refusals in a row are each told and
// retried like the first, and the sixth finish merges.
func TestDriverHandlesAFifthRefusalLikeTheFirst(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	var told []string
	submits := 0
	finished := 0
	world.Tell = func(id, text string) error {
		told = append(told, text)
		// The worker tries again and submits, but the conflict is still
		// there until the sixth finish.
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if finished <= 5 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if finished != 6 {
		t.Fatalf("finish calls = %d, want 6: five refusals and the merging retry", finished)
	}
	if len(told) != 5 {
		t.Fatalf("tell calls = %d, want 5: every refusal reaches the worker", len(told))
	}
	for i, text := range told {
		if text != refusal {
			t.Errorf("tell %d = %q, want the refusal text %q", i+1, text, refusal)
		}
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-fixed" {
		t.Errorf("merged = %v, want the fixed ticket", res.Merged)
	}
}

// The live world tells the worker's own pane the refusal verbatim and counts
// its submits from the submit record: an unchanged record reads as the same
// count, a new one as the next.
func TestDriverLiveWorldTellsPaneVerbatimAndCountsSubmits(t *testing.T) {
	oldState, oldTell := statePath, tellExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; tellExec = oldTell })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-tell"}
	id := WorkerID(ticket)
	if err := WriteAgentState(AgentState{ID: id, Type: "worker", Pane: "pane-1"}); err != nil {
		t.Fatal(err)
	}
	var prompted []string
	tellExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if len(args) == 4 && args[0] == "agent" && args[1] == "prompt" && args[2] == "pane-1" {
			prompted = append(prompted, args[3])
			return Result{Code: 0}, nil
		}
		return Result{Code: 1, Stderr: "unexpected call"}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	if err := world.Tell(id, refusal); err != nil {
		t.Fatalf("Tell: %v", err)
	}
	if len(prompted) != 1 || prompted[0] != refusal {
		t.Errorf("prompted = %q, want the refusal verbatim", prompted)
	}
	if got := world.Submits(id); got != 0 {
		t.Errorf("Submits = %d before any submit, want 0", got)
	}
	submit := func(report string) {
		if err := WriteAgentSubmit(AgentSubmit{AgentID: id, Type: "worker", Status: "done", Report: report, At: "2026-10-08T12:00:00Z", ThisTurn: true}); err != nil {
			t.Fatal(err)
		}
	}
	submit("fixed the conflict")
	if got := world.Submits(id); got != 1 {
		t.Errorf("Submits = %d after one submit, want 1", got)
	}
	if got := world.Submits(id); got != 1 {
		t.Errorf("Submits = %d on a re-read with no new submit, want 1", got)
	}
	submit("fixed the conflict again")
	if got := world.Submits(id); got != 2 {
		t.Errorf("Submits = %d after a second submit, want 2", got)
	}
}

// A world with Tell but no submit count still ends: the distinct refusal is
// told once, then the run holds on the refusal line instead of telling
// every pass forever.
func TestDriverTellWithoutSubmitsTellsOnceAndHolds(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	told, finished := 0, 0
	world.Tell = func(id, text string) error {
		told++
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		return DriverFinish{Merged: false, Detail: "rebase onto spec/x conflicted in a.go"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if told != 1 {
		t.Errorf("tell calls = %d, want 1: the refusal is told once, then the run holds", told)
	}
	if finished != 2 {
		t.Errorf("finish calls = %d, want 2: the run holds instead of retrying with no submit", finished)
	}
	if len(res.Merged) != 0 {
		t.Errorf("merged = %v, want nothing", res.Merged)
	}
	if !strings.Contains(res.Report, "unmerged 01-first: rebase onto spec/x conflicted in a.go") {
		t.Errorf("report keeps the refusal:\n%s", res.Report)
	}
}

// A worker's submit lists completed checkboxes by line text: the driver
// ticks exactly those boxes through the world and the ticket still merges.
func TestDriverTicksSubmittedCheckboxes(t *testing.T) {
	spec, world, _ := runWorld(t, map[string]string{
		"01-first.md": "# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n- [ ] second box\n",
	}, 1)
	files := map[string]string{"01-first": "- [ ] first box\n- [ ] second box\n"}
	type tickCall struct {
		ticket string
		boxes  []string
	}
	var calls []tickCall
	world.Completed = func(id string) []string {
		if id == "01-first" {
			return []string{"first box"}
		}
		return nil
	}
	world.TickCheckboxes = func(id string, boxes []string) ([]string, error) {
		calls = append(calls, tickCall{id, append([]string(nil), boxes...)})
		for _, box := range boxes {
			files[id] = strings.ReplaceAll(files[id], "[ ] "+box, "[x] "+box)
		}
		return nil, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(calls) != 1 || calls[0].ticket != "01-first" || !equalIDs(calls[0].boxes, []string{"first box"}) {
		t.Fatalf("tick calls = %+v, want exactly [{01-first [first box]}]", calls)
	}
	if want := "- [x] first box\n- [ ] second box\n"; files["01-first"] != want {
		t.Errorf("ticked file = %q, want %q", files["01-first"], want)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "01-first" {
		t.Errorf("merged = %v, want [01-first]: progress must still merge", res.Merged)
	}
}

// A completed name with no checkbox in the ticket is reported and never
// blocks the merge.
func TestDriverReportsAnUnknownCheckboxAndStillMerges(t *testing.T) {
	spec, world, _ := runWorld(t, map[string]string{
		"01-first.md": "# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n",
	}, 1)
	world.Completed = func(id string) []string { return []string{"no such box"} }
	world.TickCheckboxes = func(id string, boxes []string) ([]string, error) {
		return boxes, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "01-first" {
		t.Fatalf("merged = %v, want [01-first]: an unknown box must not fail the merge", res.Merged)
	}
	if want := `unknown checkbox "no such box" in 01-first`; !strings.Contains(res.Report, want) {
		t.Errorf("report misses %q:\n%s", want, res.Report)
	}
}

// The live tick edits the ticket file in the integration checkout and
// leaves the edit uncommitted: the box ticks, the rest stays, and git
// reports the file modified but uncommitted.
func TestTickDriverTicketFileTicksBoxesAndLeavesEditUncommitted(t *testing.T) {
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "emberloop@test.local")
	gitOut(t, repo, "config", "user.name", "emberloop")
	ticket := filepath.Join(repo, "01-first.md")
	writeFile(t, ticket, "# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n- [ ] second box\n")
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "track the ticket")
	miss, err := tickDriverTicketFile(ticket, []string{"first box"})
	if err != nil {
		t.Fatalf("tickDriverTicketFile: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("unknown = %v, want none", miss)
	}
	got, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	want := "# 01: Test ticket\n\n**Blocked by:** None\n\n- [x] first box\n- [ ] second box\n"
	if string(got) != want {
		t.Errorf("ticked ticket = %q, want %q", got, want)
	}
	if status := gitOut(t, repo, "status", "--porcelain", "--", "01-first.md"); !strings.Contains(status, "01-first.md") {
		t.Errorf("git status = %q, want the tick left dirty in the checkout", status)
	}
	if staged := gitOut(t, repo, "diff", "--cached", "--name-only", "--", "01-first.md"); staged != "" {
		t.Errorf("staged = %q, want the tick left uncommitted and unstaged", staged)
	}
	miss, err = tickDriverTicketFile(ticket, []string{"no such box"})
	if err != nil {
		t.Fatalf("tickDriverTicketFile: %v", err)
	}
	if !equalIDs(miss, []string{"no such box"}) {
		t.Errorf("unknown = %v, want [no such box]", miss)
	}
}

// Two workers submitting at once never write the same ticket file at the
// same time: concurrent ticks of one file both land.
func TestTickDriverTicketFileSerializesConcurrentTicks(t *testing.T) {
	dir := t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	writeFile(t, ticket, "- [ ] first box\n- [ ] second box\n")
	var wg sync.WaitGroup
	for _, box := range []string{"first box", "second box"} {
		wg.Add(1)
		go func(box string) {
			defer wg.Done()
			if _, err := tickDriverTicketFile(ticket, []string{box}); err != nil {
				t.Error(err)
			}
		}(box)
	}
	wg.Wait()
	got, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- [x] first box\n- [x] second box\n"; string(got) != want {
		t.Errorf("ticked ticket = %q, want %q", got, want)
	}
}

// The live world reads completed boxes from the worker's own submit and
// ticks them in the ticket file: the seam from submit to file in one run.
func TestDriverLiveWorldReadsSubmitsAndTicksTicketFile(t *testing.T) {
	oldState, oldExec := statePath, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; spawnExec = oldExec })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	if err := os.WriteFile(ticket, []byte("# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n- [ ] second box\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{ticket: "# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n- [ ] second box\n"}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-submit"}
	id := WorkerID(ticket)
	if err := WriteAgentSubmit(AgentSubmit{AgentID: id, Type: "worker", Status: "done", Summary: "did it", Completed: []string{"first box"}, At: "2026-10-08T12:00:00Z", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	world := liveDriverWorld(spec, repo, files)
	if got := world.Completed("01-first"); !equalIDs(got, []string{"first box"}) {
		t.Fatalf("Completed = %v, want [first box]", got)
	}
	if _, err := world.TickCheckboxes("01-first", []string{"first box"}); err != nil {
		t.Fatalf("TickCheckboxes: %v", err)
	}
	got, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	if want := "# 01: Test ticket\n\n**Blocked by:** None\n\n- [x] first box\n- [ ] second box\n"; string(got) != want {
		t.Errorf("ticked ticket = %q, want %q", got, want)
	}
}

// Two boxes sharing one line text still tick: the first unchecked one with
// that text becomes [x], even past an already ticked same-text box.
func TestTickDriverTicketFileTicksPastATickedDuplicate(t *testing.T) {
	dir := t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	writeFile(t, ticket, "- [x] same text\n- [ ] same text\n")
	miss, err := tickDriverTicketFile(ticket, []string{"same text"})
	if err != nil {
		t.Fatalf("tickDriverTicketFile: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("unknown = %v, want none", miss)
	}
	got, err := os.ReadFile(ticket)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- [x] same text\n- [x] same text\n"; string(got) != want {
		t.Errorf("ticked ticket = %q, want %q", got, want)
	}
}

// A tick failure is reported and never blocks the merge.
func TestDriverReportsATickFailureAndStillMerges(t *testing.T) {
	spec, world, _ := runWorld(t, map[string]string{
		"01-first.md": "# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n",
	}, 1)
	world.Completed = func(id string) []string { return []string{"first box"} }
	world.TickCheckboxes = func(id string, boxes []string) ([]string, error) {
		return nil, fmt.Errorf("disk gone")
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "01-first" {
		t.Fatalf("merged = %v, want [01-first]: a tick failure must not fail the merge", res.Merged)
	}
	if want := "checkboxes for 01-first not applied: disk gone"; !strings.Contains(res.Report, want) {
		t.Errorf("report misses %q:\n%s", want, res.Report)
	}
}

// A tick error retries on the next pass: the box stays unticked until the
// tick succeeds, and a later success clears the failure from the report.
func TestDriverTickErrorRetriesUntilItSucceeds(t *testing.T) {
	spec, world, _ := runWorld(t, map[string]string{
		"01-first.md": "# 01: Test ticket\n\n**Blocked by:** None\n\n- [ ] first box\n",
	}, 1)
	world.Completed = func(id string) []string { return []string{"first box"} }
	tickCalls := 0
	world.TickCheckboxes = func(id string, boxes []string) ([]string, error) {
		tickCalls++
		if tickCalls == 1 {
			return nil, fmt.Errorf("disk gone")
		}
		return nil, nil
	}
	submits := 0
	finished := 0
	world.Tell = func(id, text string) error {
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if finished == 1 {
			return DriverFinish{Merged: false, Detail: "boom"}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-01-first"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "01-first" {
		t.Fatalf("merged = %v, want [01-first]", res.Merged)
	}
	if tickCalls != 2 {
		t.Fatalf("tick calls = %d, want 2: a tick error must retry on the next pass", tickCalls)
	}
	if strings.Contains(res.Report, "not applied") {
		t.Errorf("report keeps a tick failure after a later success:\n%s", res.Report)
	}
}

// A resumed run finds the merged ticket in the ticket file and carries on:
// 01-first reads Status done, so it never starts again, its old sha stays in
// the report, and 02-second (blocked by 01) still starts and merges. Stray
// earlier Status lines never hide the marker: done wins over parked anywhere
// in the file.
func TestDriverResumeSkipsMergedAndFinishesRest(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  "# 01: Test ticket\n\nStatus: ready\n\nStatus: parked\n\nStatus: done\n\n**Blocked by:** None\n",
		"02-second.md": ticketDoc("01"),
	}, 2)
	world.MergedSHA = func(id string) string {
		if id == "01-first" {
			return "sha-01-first"
		}
		return ""
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if indexOfID(fake.started, "01-first") >= 0 {
		t.Errorf("resumed run restarted merged ticket 01-first: started=%v", fake.started)
	}
	if len(res.Merged) != 2 || res.Merged[0].ID != "01-first" || res.Merged[0].SHA != "sha-01-first" {
		t.Errorf("merged = %v, want 01-first with its old sha first", res.Merged)
	}
	if !strings.Contains(res.Report, "merged 01-first as sha-01-first\n") {
		t.Errorf("report misses the resumed merge:\n%s", res.Report)
	}
}

// A ticket that already has a worker gets no second worker: 01-first is
// running, so it never starts again, but its pass at the tip still finishes
// it, freeing the MaxWorkers slot for 02-second.
func TestDriverResumeStartsNoSecondWorkerForRunning(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("01"),
	}, 1)
	world.HasWorker = func(id string) bool { return id == "01-first" }
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if indexOfID(fake.started, "01-first") >= 0 {
		t.Errorf("resumed run started a second worker for 01-first: started=%v", fake.started)
	}
	if !equalIDs(fake.finished, []string{"01-first", "02-second"}) {
		t.Errorf("finished = %v, want both tickets finished in order", fake.finished)
	}
	if len(res.Merged) != 2 {
		t.Errorf("merged = %v, want both tickets", res.Merged)
	}
}

// A parked ticket keeps its worker but the driver leaves it alone: 02-park
// reads Status parked, so it never starts or finishes, 03-second (blocked by
// it) waits, and every other ticket keeps running.
func TestDriverResumeRecognisesParkedAndBlocksOnlyDependents(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-park.md":   "# 02: Test ticket\n\nStatus: parked\n\n**Blocked by:** None\n",
		"03-second.md": ticketDoc("02-park"),
		"04-free.md":   ticketDoc("None"),
	}, 3)
	world.HasWorker = func(id string) bool { return id == "02-park" }
	// Nothing left can run or start, so a live run must exit with the report
	// instead of polling: any sleep here fails the run.
	world.Sleep = func(time.Duration) { panic("driver slept with nothing left to wait for") }
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if indexOfID(fake.started, "02-park") >= 0 {
		t.Errorf("resumed run started parked ticket 02-park: started=%v", fake.started)
	}
	if indexOfID(fake.finished, "02-park") >= 0 {
		t.Errorf("resumed run finished parked ticket 02-park: finished=%v", fake.finished)
	}
	if indexOfID(fake.started, "03-second") >= 0 {
		t.Errorf("dependent 03-second started while 02-park stays parked: started=%v", fake.started)
	}
	if !equalIDs(fake.started, []string{"01-first", "04-free"}) {
		t.Errorf("started = %v, want only the unblocked tickets", fake.started)
	}
	if !strings.Contains(res.Report, "unmerged 02-park: parked\n") {
		t.Errorf("report misses the parked row:\n%s", res.Report)
	}
	if !strings.Contains(res.Report, "unmerged 03-second: never started\n") {
		t.Errorf("report misses the blocked dependent:\n%s", res.Report)
	}
}

// The driver holds no state file of its own: a resumed run reads the injected
// world and leaves it untouched. A merged ticket whose old sha is unknown
// still lists bare in the report.
func TestDriverResumeLeavesTheInjectedWorldUntouched(t *testing.T) {
	spec, world, _ := runWorld(t, map[string]string{
		"01-first.md":  "# 01: Test ticket\n\nStatus: done\n\n**Blocked by:** None\n",
		"02-second.md": ticketDoc("01"),
	}, 2)
	world.HasWorker = func(id string) bool { return false }
	world.MergedSHA = func(id string) string { return "" }
	before := map[string]string{}
	for path, raw := range world.Files {
		before[path] = raw
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	for path, raw := range before {
		if world.Files[path] != raw {
			t.Errorf("driver rewrote ticket file %s", path)
		}
	}
	if len(world.Files) != len(before) {
		t.Errorf("driver added files to the world: %d, want %d", len(world.Files), len(before))
	}
	if !strings.Contains(res.Report, "merged 01-first\n") {
		t.Errorf("report misses the bare resumed merge:\n%s", res.Report)
	}
}

// Three identical refusals in a row mark the ticket "needs you" and send one
// herdr notification. The retry after the alert still runs, and further
// identical refusals send no second notification.
func TestDriverThreeIdenticalRefusalsRaiseNeedsYouOnce(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	var told []string
	submits := 0
	finished := 0
	var notified []string
	world.Tell = func(id, text string) error {
		told = append(told, text)
		if len(told) <= 3 {
			// The worker tries again after each of the first three tells,
			// then stalls: no more submits.
			submits++
		}
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		return DriverFinish{Merged: false, Detail: refusal}, nil
	}
	world.Notify = func(id, message string) error {
		notified = append(notified, id+": "+message)
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.NeedsYou) != 1 || res.NeedsYou[0] != "01-first" {
		t.Errorf("NeedsYou = %v, want [01-first]", res.NeedsYou)
	}
	if len(notified) != 1 {
		t.Fatalf("notify calls = %d, want 1: the alert sends one notification", len(notified))
	}
	if !strings.Contains(notified[0], "01-first") || !strings.Contains(notified[0], refusal) {
		t.Errorf("notification = %q, want the ticket and the refusal text", notified[0])
	}
	if finished != 4 {
		t.Errorf("finish calls = %d, want 4: three to raise the alert and one retry after it", finished)
	}
	if len(told) != 4 {
		t.Errorf("tell calls = %d, want 4: every refusal still reaches the worker", len(told))
	}
	if len(res.Merged) != 0 {
		t.Errorf("merged = %v, want nothing: the worker stalled", res.Merged)
	}
}

// Two identical refusals do not raise the alert: the ticket merges on the
// retry with no notification and no needs-you mark.
func TestDriverTwoIdenticalRefusalsDoNotRaiseNeedsYou(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	finished := 0
	notified := 0
	world.Tell = func(id, text string) error {
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if finished <= 2 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	world.Notify = func(id, message string) error {
		notified++
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if notified != 0 {
		t.Errorf("notify calls = %d, want 0: two refusals raise no alert", notified)
	}
	if len(res.NeedsYou) != 0 {
		t.Errorf("NeedsYou = %v, want nothing", res.NeedsYou)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-fixed" {
		t.Errorf("merged = %v, want the fixed ticket", res.Merged)
	}
}

// Ten minutes with no worker activity marks the ticket "needs you" and sends
// one notification on the fake clock. The slow worker still merges after it
// recovers, so the alert never stops the ticket.
func TestDriverIdleWorkerRaisesNeedsYouAndStillMerges(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-slow.md": ticketDoc("None"),
		"02-fast.md": ticketDoc("None"),
	})
	world.MaxWorkers = 2
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string { return "tip-" + id }
	world.Reviews = func(id string) map[string]DriverReview {
		if id == "01-slow" && now.Before(start.Add(11*time.Minute)) {
			// The slow worker shows one pass and then goes quiet.
			return map[string]DriverReview{
				reviewerStandards: {Verdict: verdictPass, At: "tip-01-slow"},
			}
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip-" + id},
			reviewerSpec:      {Verdict: verdictPass, At: "tip-" + id},
		}
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-" + id}, nil
	}
	var notified []string
	world.Notify = func(id, message string) error {
		notified = append(notified, id+": "+message)
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(notified) != 1 {
		t.Fatalf("notify calls = %d, want 1: the idle ticket alerts once", len(notified))
	}
	if !strings.Contains(notified[0], "01-slow") || !strings.Contains(notified[0], "10 minutes") {
		t.Errorf("notification = %q, want the idle ticket and the 10 minutes", notified[0])
	}
	if len(res.Merged) != 2 {
		t.Errorf("merged = %v, want both tickets: the slow one recovers", res.Merged)
	}
}

// Retries continue after the alert with a growing delay: the gaps between
// finish calls grow once the ticket is marked needs-you.
func TestDriverNeedsYouRetriesWithGrowingDelay(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	var finishAt []time.Time
	notified := 0
	world.Tell = func(id, text string) error {
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finishAt = append(finishAt, now)
		if len(finishAt) <= 4 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	world.Notify = func(id, message string) error {
		notified++
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(finishAt) != 5 {
		t.Fatalf("finish calls = %d, want 5: retries continue after the alert", len(finishAt))
	}
	firstGap := finishAt[3].Sub(finishAt[2])
	secondGap := finishAt[4].Sub(finishAt[3])
	if firstGap < time.Minute {
		t.Errorf("first post-alert gap = %v, want at least a minute", firstGap)
	}
	if secondGap <= firstGap {
		t.Errorf("gaps = %v then %v, want the delay to keep growing", firstGap, secondGap)
	}
	if notified != 1 {
		t.Errorf("notify calls = %d, want 1: one alert across all five finishes", notified)
	}
	if len(res.Merged) != 1 {
		t.Errorf("merged = %v, want the fixed ticket", res.Merged)
	}
}

// One ticket in "needs you" stops nothing else: the stuck ticket alerts and
// waits while the other ticket merges.
func TestDriverOtherTicketsRunWhileOneNeedsYou(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-stuck.md": ticketDoc("None"),
		"02-quick.md": ticketDoc("None"),
	})
	world.MaxWorkers = 2
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	told := 0
	var notified []string
	world.Tell = func(id, text string) error {
		told++
		if told <= 3 {
			submits++
		}
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		if id == "02-quick" {
			return DriverFinish{Merged: true, SHA: "sha-02-quick"}, nil
		}
		return DriverFinish{Merged: false, Detail: refusal}, nil
	}
	world.Notify = func(id, message string) error {
		notified = append(notified, id)
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "02-quick" {
		t.Errorf("merged = %v, want only 02-quick", res.Merged)
	}
	if len(res.NeedsYou) != 1 || res.NeedsYou[0] != "01-stuck" {
		t.Errorf("NeedsYou = %v, want [01-stuck]", res.NeedsYou)
	}
	if len(notified) != 1 || notified[0] != "01-stuck" {
		t.Errorf("notified = %v, want one alert for 01-stuck", notified)
	}
}

// The live world sends the needs-you alert through herdr's notification show:
// the title names the ticket and the body carries the alert text.
func TestDriverLiveWorldNotifiesThroughHerdr(t *testing.T) {
	oldState, oldNotify := statePath, notifyExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; notifyExec = oldNotify })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-notify"}
	var calls [][]string
	notifyExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		calls = append(calls, append([]string{cmd}, args...))
		return Result{Code: 0}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	message := "ticket 01-first needs you: the same refusal 3 times in a row: conflicted"
	if err := world.Notify("01-first", message); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("herdr calls = %d, want 1", len(calls))
	}
	want := []string{"notification", "show", "emberloop: ticket 01-first needs you", "--body", message}
	got := calls[0][1:]
	if len(got) != len(want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A notification herdr rejects never stops the run: the stuck ticket stays
// marked, the other ticket merges, and the report records the failed
// notification beside the refusal.
func TestDriverFailedNotificationKeepsTheRunGoing(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-stuck.md": ticketDoc("None"),
		"02-quick.md": ticketDoc("None"),
	})
	world.MaxWorkers = 2
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	told := 0
	world.Tell = func(id, text string) error {
		told++
		if told <= 3 {
			submits++
		}
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		if id == "02-quick" {
			return DriverFinish{Merged: true, SHA: "sha-02-quick"}, nil
		}
		return DriverFinish{Merged: false, Detail: refusal}, nil
	}
	world.Notify = func(id, message string) error {
		return errors.New("herdr down")
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "02-quick" {
		t.Errorf("merged = %v, want only 02-quick", res.Merged)
	}
	if len(res.NeedsYou) != 1 || res.NeedsYou[0] != "01-stuck" {
		t.Errorf("NeedsYou = %v, want [01-stuck]", res.NeedsYou)
	}
	for _, line := range []string{"unmerged 01-stuck: " + refusal, "notify 01-stuck failed: herdr down"} {
		if !strings.Contains(res.Report, line) {
			t.Errorf("report misses %q:\n%s", line, res.Report)
		}
	}
}

// A notification that fails once goes out on retry: two calls, one delivery,
// and no failure line on the report.
func TestDriverNotificationRetriesUntilItSucceeds(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	finished := 0
	calls := 0
	world.Tell = func(id, text string) error {
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if finished <= 4 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	world.Notify = func(id, message string) error {
		calls++
		if calls == 1 {
			return errors.New("herdr down")
		}
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if calls != 2 {
		t.Errorf("notify calls = %d, want 2: one failure and one delivery", calls)
	}
	if strings.Contains(res.Report, "notify 01-first failed") {
		t.Errorf("report keeps a failure line after the retry succeeded:\n%s", res.Report)
	}
	if len(res.Merged) != 1 {
		t.Errorf("merged = %v, want the fixed ticket", res.Merged)
	}
}

// Failed notifications wait out the same growing delay as the retries: with
// herdr down for the whole run the driver attempts exactly three times (at
// the mark, a minute later, three minutes later) instead of every poll.
func TestDriverNotificationRetriesWaitOutTheBackoff(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	var notifyAt []time.Time
	calls := 0
	world.Tell = func(id, text string) error {
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		calls++
		if calls <= 4 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	world.Notify = func(id, message string) error {
		notifyAt = append(notifyAt, now)
		if len(notifyAt) <= 2 {
			return errors.New("herdr down")
		}
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(notifyAt) != 3 {
		t.Fatalf("notify calls = %d, want 3: the mark plus two delayed retries", len(notifyAt))
	}
	if gap := notifyAt[1].Sub(notifyAt[0]); gap < time.Minute {
		t.Errorf("first notify retry gap = %v, want at least a minute", gap)
	}
	if gap := notifyAt[2].Sub(notifyAt[1]); gap <= notifyAt[1].Sub(notifyAt[0]) {
		t.Errorf("notify gaps keep growing, got %v then %v", notifyAt[1].Sub(notifyAt[0]), gap)
	}
	if len(res.Merged) != 1 {
		t.Errorf("merged = %v, want the fixed ticket", res.Merged)
	}
}

// A ticket that merges after its notification failed carries no failure line:
// the merge clears the pending error with the mark.
func TestDriverMergeDropsAPendingNotifyFailure(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	submits := 0
	finished := 0
	world.Tell = func(id, text string) error {
		submits++
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if finished <= 3 {
			return DriverFinish{Merged: false, Detail: refusal}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-fixed"}, nil
	}
	world.Notify = func(id, message string) error {
		return errors.New("herdr down")
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 {
		t.Fatalf("merged = %v, want the fixed ticket", res.Merged)
	}
	if strings.Contains(res.Report, "notify 01-first failed") {
		t.Errorf("report keeps a failure line for a merged ticket:\n%s", res.Report)
	}
}

// A parked ticket never starts and blocks only the tickets that depend on
// it: 02-park keeps its worktree so the driver leaves it alone, 04-behind
// waits on it, and 01 and 03 merge around them.
func TestDriverParkedTicketNeverStartsAndBlocksOnlyDependents(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-parked.md": ticketDoc("01"),
		"03-free.md":   ticketDoc("01"),
		"04-behind.md": ticketDoc("02-parked"),
	}, 3)
	world.Parked = func(id string) bool { return id == "02-parked" }
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if indexOfID(fake.started, "02-parked") >= 0 {
		t.Errorf("parked ticket started: %v", fake.started)
	}
	if indexOfID(fake.started, "04-behind") >= 0 {
		t.Errorf("dependent of a parked ticket started: %v", fake.started)
	}
	if indexOfID(fake.finished, "02-parked") >= 0 {
		t.Errorf("parked ticket finished, its worktree and branch must stay: %v", fake.finished)
	}
	if len(res.Merged) != 2 {
		t.Errorf("merged = %v, want 01-first and 03-free", res.Merged)
	}
	for _, line := range []string{"merged 01-first as sha-01-first", "merged 03-free as sha-03-free", "unmerged 02-parked: parked", "unmerged 04-behind: never started"} {
		if !strings.Contains(res.Report, line) {
			t.Errorf("report misses %q:\n%s", line, res.Report)
		}
	}
}

// Parking mid-run stops the driver working the ticket but keeps its worktree
// and branch: no finish and no tell after the park, and its worker slot goes
// to the next ticket, which merges around it.
func TestDriverParkingMidRunKeepsWorktreeAndFreesTheSlot(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("None"),
	}, 1)
	parked := false
	world.StartWorker = func(tk DriverTicket) error {
		fake.started = append(fake.started, tk.ID)
		fake.active++
		if fake.active > fake.maxSeen {
			fake.maxSeen = fake.active
		}
		// The user presses p on 01-first the moment its worker exists.
		if tk.ID == "01-first" {
			parked = true
		}
		return nil
	}
	world.Parked = func(id string) bool { return parked && id == "01-first" }
	var told []string
	world.Tell = func(id, text string) error {
		told = append(told, id+": "+text)
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if indexOfID(fake.finished, "01-first") >= 0 {
		t.Errorf("parked ticket finished, its worktree and branch must stay: %v", fake.finished)
	}
	if len(told) != 0 {
		t.Errorf("parked ticket told %v; the driver leaves it alone", told)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "02-second" {
		t.Errorf("merged = %v, want 02-second merging into the freed slot", res.Merged)
	}
	for _, line := range []string{"merged 02-second as sha-02-second", "unmerged 01-first: parked"} {
		if !strings.Contains(res.Report, line) {
			t.Errorf("report misses %q:\n%s", line, res.Report)
		}
	}
}

// The driver never discards: refusals are told and retried, parked tickets
// are left alone, and no run calls the user's discard op.
func TestDriverNeverDiscardsOnItsOwn(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-parked.md": ticketDoc("None"),
	})
	world.MaxWorkers = 2
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	var told []string
	world.Tell = func(id, text string) error {
		told = append(told, text)
		return nil
	}
	finished := 0
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		if id == "01-first" && finished <= 2 {
			return DriverFinish{Merged: false, Detail: "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"}, nil
		}
		return DriverFinish{Merged: true, SHA: "sha-" + id}, nil
	}
	world.Parked = func(id string) bool { return id == "02-parked" }
	var discarded []string
	world.Discard = func(id string) error {
		discarded = append(discarded, id)
		return nil
	}
	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(discarded) != 0 {
		t.Errorf("driver discarded %v; only the user discards", discarded)
	}
	if len(told) == 0 {
		t.Errorf("no refusal told; refusals must reach the worker, not the discard")
	}
}

// Worker follow-ups reach the final report verbatim: the merged ticket's
// note and the unmerged ticket's note alike, punctuation and all.
func TestDriverFollowUpsAppearVerbatimInTheReport(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("None"),
	}, 2)
	tip2 := fake.tips["02-second"]
	fake.reviews["02-second"] = map[string]DriverReview{
		reviewerStandards: {Verdict: verdictPass, At: tip2},
	}
	world.Followups = func(id string) string {
		if id == "01-first" {
			return "the rebase helper misfires on empty repos, worth a look"
		}
		return "still needs a reviewer pass at the tip, parked for now"
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	for _, line := range []string{
		"merged 01-first as sha-01-first",
		"follow-ups 01-first: the rebase helper misfires on empty repos, worth a look",
		"unmerged 02-second: still running",
		"follow-ups 02-second: still needs a reviewer pass at the tip, parked for now",
	} {
		if !strings.Contains(res.Report, line) {
			t.Errorf("report misses %q:\n%s", line, res.Report)
		}
	}
}

// A parked ticket keeps its last refusal on the report: refused once, told,
// then parked, the run never finishes it again and the line names both.
func TestDriverParkedTicketKeepsItsLastRefusal(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	refusal := "rebase onto spec/x conflicted in a.go; the rebase was aborted, the branch is unchanged"
	parked := false
	world.Tell = func(id, text string) error {
		// The user parks the ticket instead of letting the worker retry.
		parked = true
		return nil
	}
	world.Parked = func(id string) bool { return parked }
	finished := 0
	world.Finish = func(id string) (DriverFinish, error) {
		finished++
		return DriverFinish{Merged: false, Detail: refusal}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if finished != 1 {
		t.Errorf("finish calls = %d, want 1: a parked ticket is never finished again", finished)
	}
	if want := "unmerged 01-first: parked: " + refusal; !strings.Contains(res.Report, want) {
		t.Errorf("report misses %q:\n%s", want, res.Report)
	}
}

// The live world reads the park from the ticket file: a parked Status parks
// the ticket, any other status does not, and the driver is still the only
// writer the run needs.
func TestDriverLiveWorldReadsParkedStatusFromTheTicketFile(t *testing.T) {
	dir := t.TempDir()
	running := filepath.Join(dir, "01-first.md")
	parked := filepath.Join(dir, "02-parked.md")
	files := map[string]string{
		running: "# 01: Test ticket\n\n**Status:** ready-for-agent\n\n**Blocked by:** None\n",
		parked:  "# 02: Test ticket\n\n**Status:** parked\n\n**Blocked by:** 01-first\n",
	}
	for path, raw := range files {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-park"}
	world := liveDriverWorld(spec, t.TempDir(), files)
	if world.Parked == nil {
		t.Fatal("live world wires no Parked hook")
	}
	if world.Parked("01-first") {
		t.Errorf("01-first reads parked with a ready-for-agent status")
	}
	if !world.Parked("02-parked") {
		t.Errorf("02-parked with a parked status does not read parked")
	}
	if world.Parked("09-missing") {
		t.Errorf("an unknown ticket reads parked")
	}
}

// The live world reads follow-ups from the worker's submit: the text its
// summary named after Follow-ups:, verbatim, and nothing when it named none.
func TestDriverLiveWorldReadsFollowUpsFromTheWorkerSubmit(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	dir := t.TempDir()
	withNote := filepath.Join(dir, "01-first.md")
	withoutNote := filepath.Join(dir, "02-second.md")
	files := map[string]string{
		withNote:    ticketDoc("None"),
		withoutNote: ticketDoc("None"),
	}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-followups"}
	world := liveDriverWorld(spec, t.TempDir(), files)
	if world.Followups == nil {
		t.Fatal("live world wires no Followups hook")
	}
	note := "the rebase helper misfires on empty repos, worth a look"
	if err := WriteAgentSubmit(AgentSubmit{AgentID: WorkerID(withNote), Type: "worker", Status: "done", Summary: "done. Follow-ups: " + note, At: "2026-10-08T12:00:00Z", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	if got := world.Followups("01-first"); got != note {
		t.Errorf("Followups = %q, want the note verbatim %q", got, note)
	}
	if got := world.Followups("02-second"); got != "" {
		t.Errorf("Followups without a submit = %q, want empty", got)
	}
}

// The live discard removes the worker and its worktree on request and leaves
// the ticket file untouched: the same finish with merge=false the tool runs.
func TestDriverLiveWorldDiscardsTheWorkerAndItsWorktree(t *testing.T) {
	repo, tree := reviewRepo(t)
	ticket := filepath.Join(t.TempDir(), "07-x.md")
	writeFile(t, ticket, "# 07-x: example\n\n**Status:** ready-for-agent\n\n**Blocked by:** None\n")
	pointWorkerAtTicket(t, ticket)
	f := newFakeReviewers(answers("pass", "pass"))
	old := finishExec
	finishExec = f.run
	t.Cleanup(func() { finishExec = old })
	dir := filepath.Dir(ticket)
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-discard"}
	world := liveDriverWorld(spec, repo, files)
	if world.Discard == nil {
		t.Fatal("live world wires no Discard hook")
	}
	if err := world.Discard("07-x"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if store, err := load(); err != nil {
		t.Fatal(err)
	} else if _, ok := store["07-x"]; ok {
		t.Errorf("worker 07-x still recorded after a discard")
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("worktree still exists after a discard: %v", err)
	}
	if err := world.Discard("09-missing"); err == nil {
		t.Errorf("discarding an unknown ticket must fail")
	}
}

// A ticket the user discards mid-run stops holding the run: no finish and
// no tell after the discard, its slot frees for the next ticket, its
// dependents wait, and the report names it discarded.
func TestDriverDiscardedTicketFreesSlotAndReadsDiscarded(t *testing.T) {
	spec, world, fake := runWorld(t, map[string]string{
		"01-gone.md":   ticketDoc("None"),
		"02-free.md":   ticketDoc("None"),
		"03-behind.md": ticketDoc("01-gone"),
	}, 1)
	gone := false
	world.StartWorker = func(tk DriverTicket) error {
		fake.started = append(fake.started, tk.ID)
		fake.active++
		if fake.active > fake.maxSeen {
			fake.maxSeen = fake.active
		}
		// The user discards 01-gone the moment its worker exists.
		if tk.ID == "01-gone" {
			gone = true
		}
		return nil
	}
	world.Discarded = func(id string) bool { return gone && id == "01-gone" }
	var told []string
	world.Tell = func(id, text string) error {
		told = append(told, id+": "+text)
		return nil
	}
	var discarded []string
	world.Discard = func(id string) error {
		discarded = append(discarded, id)
		return nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if indexOfID(fake.finished, "01-gone") >= 0 {
		t.Errorf("discarded ticket finished, its worker is gone: %v", fake.finished)
	}
	if len(told) != 0 {
		t.Errorf("discarded ticket told %v; the driver leaves it alone", told)
	}
	if len(discarded) != 0 {
		t.Errorf("driver discarded %v; only the user discards", discarded)
	}
	if len(res.Merged) != 1 || res.Merged[0].ID != "02-free" {
		t.Errorf("merged = %v, want 02-free merging into the freed slot", res.Merged)
	}
	if indexOfID(fake.started, "03-behind") >= 0 {
		t.Errorf("dependent of a discarded ticket started: %v", fake.started)
	}
	for _, line := range []string{"merged 02-free as sha-02-free", "unmerged 01-gone: discarded", "unmerged 03-behind: never started"} {
		if !strings.Contains(res.Report, line) {
			t.Errorf("report misses %q:\n%s", line, res.Report)
		}
	}
}

// The live world sees the user's discard in the worker store: a ticket with
// no worker record reads discarded, one with a record does not.
func TestDriverLiveWorldSeesADiscardByItsMissingWorkerRecord(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	dir := t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-discarded"}
	world := liveDriverWorld(spec, t.TempDir(), files)
	if world.Discarded == nil {
		t.Fatal("live world wires no Discarded hook")
	}
	if !world.Discarded("01-first") {
		t.Errorf("ticket with no worker record does not read discarded")
	}
	if err := save(map[string]Worker{WorkerID(ticket): {ID: WorkerID(ticket)}}); err != nil {
		t.Fatal(err)
	}
	if world.Discarded("01-first") {
		t.Errorf("ticket with a worker record reads discarded")
	}
	if world.Discarded("09-missing") {
		t.Errorf("an unknown ticket reads discarded")
	}
}

// Follow-ups: none means no follow-ups: the report carries no line for it.
func TestDriverLiveWorldReadsFollowUpsNoneAsEmpty(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	dir := t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-followups-none"}
	world := liveDriverWorld(spec, t.TempDir(), files)
	if err := WriteAgentSubmit(AgentSubmit{AgentID: WorkerID(ticket), Type: "worker", Status: "done", Summary: "done. Follow-ups: none", At: "2026-10-08T12:00:00Z", ThisTurn: true}); err != nil {
		t.Fatal(err)
	}
	if got := world.Followups("01-first"); got != "" {
		t.Errorf("Followups none = %q, want empty", got)
	}
}

// A queued ticket is not a discard: spawn queues over the cap with no worker
// record, so the driver must keep waiting for it instead of dropping it.
func TestDriverLiveWorldQueuedTicketDoesNotReadDiscarded(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	dir := t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-queued"}
	world := liveDriverWorld(spec, t.TempDir(), files)
	if err := queueWorker(queuedWorker{ID: WorkerID(ticket), Type: "worker", Ticket: ticket, Repo: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if world.Discarded("01-first") {
		t.Errorf("queued ticket reads discarded; a queued worker has no record yet")
	}
}

// History tells a discard from a pruned pane: a cancelled entry keeps its
// worktree for a respawn, only a discarded entry reads discarded.
func TestDriverLiveWorldCancelledHistoryDoesNotReadDiscarded(t *testing.T) {
	oldState := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState })
	dir := t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-history"}
	world := liveDriverWorld(spec, t.TempDir(), files)
	slug := specSlug(spec.Spec)
	if err := saveHistory(slug, []TicketHistory{{Ticket: ticket, Worker: WorkerID(ticket), Outcome: outcomeCancelled}}); err != nil {
		t.Fatal(err)
	}
	if world.Discarded("01-first") {
		t.Errorf("cancelled history reads discarded; only a discard removes work on request")
	}
	if err := saveHistory(slug, []TicketHistory{{Ticket: ticket, Worker: WorkerID(ticket), Outcome: outcomeCancelled}, {Ticket: ticket, Worker: WorkerID(ticket), Outcome: outcomeDiscarded}}); err != nil {
		t.Fatal(err)
	}
	if !world.Discarded("01-first") {
		t.Errorf("discarded history does not read discarded")
	}
}

// A worker that submitted while its reviewers passed an older tree gets
// finish's refusal told to it: the driver gate alone would skip finish and
// the stall nudge needs zero submits, so nothing else would move the ticket.
func TestDriverTellsASubmittedWorkerWhoseReviewPassesAreStale(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "t2" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "t1"},
			reviewerSpec:      {Verdict: verdictPass, At: "t1"},
		}
	}
	world.Submits = func(id string) int { return 1 }
	var told []string
	world.Tell = func(id, text string) error { told = append(told, text); return nil }
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Detail: "passed at t1, not at the tip t2"}, nil
	}
	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(told) != 1 || told[0] != "passed at t1, not at the tip t2" {
		t.Errorf("told = %v, want the stale-pass refusal once", told)
	}
}

// A recorded worker whose pane is gone is not a worker: a resumed run starts
// it again instead of waiting on a pane that can no longer act.
func TestDriverLiveWorldTreatsAWorkerWithADeadPaneAsNoWorker(t *testing.T) {
	oldState, oldExec := statePath, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; spawnExec = oldExec })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-resume"}
	if err := save(map[string]Worker{WorkerID(ticket): {ID: WorkerID(ticket), Ticket: ticket, Repo: repo, Pane: "w:p1", Started: "2020-01-01T00:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	spawnExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
			return Result{Code: 1, Stderr: `{"error":{"code":"pane_not_found"}}`}, nil
		}
		return Result{Code: 0, Stdout: "tipsha\n"}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	if world.HasWorker(WorkerID(ticket)) {
		t.Errorf("HasWorker = true, want false for a worker whose pane is gone")
	}
}

// A worktree left on disk with no worker record is a pruned worker: the run
// starts it again, while a bare branch still reads as a worker already done.
func TestDriverLiveWorldTreatsALeftoverWorktreeAsNoWorker(t *testing.T) {
	oldState, oldExec := statePath, spawnExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; spawnExec = oldExec })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-resume"}
	spawnExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		return Result{Code: 0, Stdout: "tipsha\n"}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	if !world.HasWorker(WorkerID(ticket)) {
		t.Fatalf("HasWorker = false, want the bare implement branch")
	}
	if err := os.MkdirAll(worktreePath(repo, WorkerID(ticket)), 0o755); err != nil {
		t.Fatal(err)
	}
	if world.HasWorker(WorkerID(ticket)) {
		t.Errorf("HasWorker = true, want false for a worktree with no record")
	}
}
