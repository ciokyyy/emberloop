package emberloop

import (
	"path/filepath"
	"strings"
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
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "mergedsha" {
		t.Errorf("merged = %v, want the live-mapped ticket", res.Merged)
	}
	if len(started) != 1 || started[0] != id {
		t.Errorf("started = %v, want [%s]", started, id)
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
