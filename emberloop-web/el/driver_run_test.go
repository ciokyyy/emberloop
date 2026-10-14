package el

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The run pane shares one in-memory run state with the driver loop: the loop
// publishes every pass, the pane reads snapshots. These tests go through the
// one driver seam with an injected world and a fake clock, asserting on calls
// and run state, never on internals.
func TestDriverRunPublishesRowsForEveryState(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-second.md": ticketDoc("03-third"),
		"03-third.md":  ticketDoc("01-first"),
	})
	world.Now = func() time.Time { return now }
	world.MaxTickets = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "tip-" + id }
	world.Reviews = func(id string) map[string]DriverReview {
		if id == "02-second" {
			return map[string]DriverReview{
				reviewerStandards: {Verdict: verdictPass, At: "tip-02-second"},
				reviewerSpec:      {Verdict: verdictChanges, At: "tip-02-second"},
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
	run := NewDriverRun(world.Now)
	world.Run = run

	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.Merged) != 2 {
		t.Fatalf("merged = %v, want 01-first and 03-third", res.Merged)
	}
	rows := run.Snapshot()
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want one per ticket", len(rows))
	}
	byID := map[string]DriverRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if byID["01-first"].State != DriverRowMerged {
		t.Errorf("01-first state = %q, want merged", byID["01-first"].State)
	}
	if byID["03-third"].State != DriverRowMerged {
		t.Errorf("03-third state = %q, want merged", byID["03-third"].State)
	}
	inReview := byID["02-second"]
	if inReview.State != DriverRowInReview {
		t.Errorf("02-second state = %q, want in-review", inReview.State)
	}
	if inReview.Worker == "" {
		t.Errorf("02-second worker = %q, want the worker", inReview.Worker)
	}
	if inReview.Verdicts[reviewerSpec].Verdict != verdictChanges {
		t.Errorf("02-second spec verdict = %+v, want changes at the tip", inReview.Verdicts[reviewerSpec])
	}
	if inReview.Since.IsZero() {
		t.Errorf("02-second Since is zero, want the state entry time")
	}
}

// A ticket the gate holds back but no reviewer has seen reads running; a
// blocked ticket that never started reads queued with no worker.
func TestDriverRunMarksRunningAndQueued(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md":   ticketDoc("None"),
		"02-blocked.md": ticketDoc("01-first"),
	})
	world.Now = func() time.Time { return now }
	world.MaxTickets = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "" }
	world.Reviews = func(id string) map[string]DriverReview { return nil }
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{}, nil
	}
	run := NewDriverRun(world.Now)
	world.Run = run

	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	byID := map[string]DriverRow{}
	for _, r := range run.Snapshot() {
		byID[r.ID] = r
	}
	if byID["01-first"].State != DriverRowRunning {
		t.Errorf("01-first state = %q, want running", byID["01-first"].State)
	}
	if byID["01-first"].Worker == "" {
		t.Errorf("01-first worker is empty, want the started worker")
	}
	if byID["02-blocked"].State != DriverRowQueued {
		t.Errorf("02-blocked state = %q, want queued", byID["02-blocked"].State)
	}
	if byID["02-blocked"].Worker != "" {
		t.Errorf("queued ticket carries worker %q, want none", byID["02-blocked"].Worker)
	}
}

// The same refusal three times in a row marks the ticket needs-you with the
// refusal text on the row.
func TestDriverRunMarksNeedsYouWithTheRefusal(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.Now = func() time.Time { return now }
	world.MaxTickets = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	submits := 0
	var told []string
	world.Tell = func(id, text string) error {
		told = append(told, text)
		if len(told) <= 3 {
			// The worker retries after the first three tells, then stalls:
			// no more submits, so the run holds after the fourth finish.
			submits++
		}
		return nil
	}
	world.Submits = func(id string) int { return submits }
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: false, Detail: "conflict in work.go"}, nil
	}
	run := NewDriverRun(world.Now)
	world.Run = run

	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(res.NeedsYou) != 1 {
		t.Fatalf("NeedsYou = %v, want the ticket", res.NeedsYou)
	}
	rows := run.Snapshot()
	if len(rows) != 1 || rows[0].State != DriverRowNeedsYou {
		t.Fatalf("row state = %v, want one needs-you row", rows)
	}
	if rows[0].LastRefusal != "conflict in work.go" {
		t.Errorf("LastRefusal = %q, want the refusal text", rows[0].LastRefusal)
	}
}

// While finish runs, the ticket reads merging: the snapshot taken inside the
// finish call itself shows it.
func TestDriverRunMarksMergingWhileFinishRuns(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.Now = func() time.Time { return now }
	world.MaxTickets = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	run := NewDriverRun(world.Now)
	world.Run = run
	var during DriverRowState
	world.Finish = func(id string) (DriverFinish, error) {
		for _, r := range run.Snapshot() {
			if r.ID == id {
				during = r.State
			}
		}
		return DriverFinish{Merged: true, SHA: "sha"}, nil
	}

	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if during != DriverRowMerging {
		t.Errorf("state during finish = %q, want merging", during)
	}
	if got := run.Snapshot()[0].State; got != DriverRowMerged {
		t.Errorf("final state = %q, want merged", got)
	}
}

// Parked and discarded tickets read parked and discarded with no worker.
func TestDriverRunMarksParkedAndDiscarded(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md":  ticketDoc("None"),
		"02-parked.md": ticketDoc("None"),
		"03-gone.md":   ticketDoc("None"),
	})
	world.Now = func() time.Time { return now }
	world.MaxTickets = 3
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Parked = func(id string) bool { return id == "02-parked" }
	world.Discarded = func(id string) bool { return id == "03-gone" }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview { return nil }
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha"}, nil
	}
	run := NewDriverRun(world.Now)
	world.Run = run

	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	byID := map[string]DriverRow{}
	for _, r := range run.Snapshot() {
		byID[r.ID] = r
	}
	if byID["02-parked"].State != DriverRowParked {
		t.Errorf("02-parked state = %q, want parked", byID["02-parked"].State)
	}
	if byID["02-parked"].Worker != "" {
		t.Errorf("parked ticket carries worker %q, want none", byID["02-parked"].Worker)
	}
	if byID["03-gone"].State != DriverRowDiscarded {
		t.Errorf("03-gone state = %q, want discarded", byID["03-gone"].State)
	}
	if byID["03-gone"].Worker != "" {
		t.Errorf("discarded ticket carries worker %q, want none", byID["03-gone"].Worker)
	}
}

// A state change re-dates Since: the ticket starts running at the base
// clock, the reviewers arrive after the clock advances, and the in-review
// row carries the later time.
func TestDriverRunRedatesSinceOnStateChange(t *testing.T) {
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	later := base.Add(5 * time.Minute)
	nowCalls := 0
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	// The fake clock advances past its first read: the first pass dates
	// running at the base time, every later pass at the later time. Gating
	// on clock reads keeps the verdict timing independent of how many times
	// a pass reads the reviews.
	world.Now = func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return base
		}
		return later
	}
	world.MaxTickets = 1
	world.StartWorker = func(tk DriverTicket) error { return nil }
	world.Tip = func(id string) string { return "tip" }
	world.Reviews = func(id string) map[string]DriverReview {
		if nowCalls <= 1 {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
		}
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha"}, nil
	}
	run := NewDriverRun(world.Now)
	world.Run = run

	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	rows := run.Snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want one", rows)
	}
	if rows[0].State != DriverRowInReview {
		t.Fatalf("state = %q, want in-review", rows[0].State)
	}
	if !rows[0].Since.Equal(later) {
		t.Errorf("Since = %v, want the state entry time %v", rows[0].Since, later)
	}
}

// The park command writes Status parked into the ticket file: the driver's
// parked hook reads it on the next pass.
func TestParkDriverTicketFileParksTheTicket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-first.md")
	doc := "# 01: Test ticket\n\nStatus: ready-for-agent\n\n**Blocked by:** None\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	if err := ParkDriverTicketFile(path); err != nil {
		t.Fatalf("ParkDriverTicketFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if !driverFileParked(string(raw)) {
		t.Errorf("parked file = %q, want Status parked", raw)
	}
	if strings.Contains(string(raw), "ready-for-agent") {
		t.Errorf("parked file keeps the old status: %q", raw)
	}
}

// Without a Status line the park command adds one, so prose-only tickets
// still park.
func TestParkDriverTicketFileAddsAStatusLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-first.md")
	if err := os.WriteFile(path, []byte(ticketDoc("None")), 0o644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	if err := ParkDriverTicketFile(path); err != nil {
		t.Fatalf("ParkDriverTicketFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if !driverFileParked(string(raw)) {
		t.Errorf("parked file = %q, want Status parked", raw)
	}
}

func TestParkDriverTicketFileRefusesMerged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-first.md")
	doc := "# 01: Test ticket\n\nStatus: done\n\n**Blocked by:** None\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	if err := ParkDriverTicketFile(path); err == nil {
		t.Fatal("ParkDriverTicketFile on a merged ticket must fail")
	} else if !strings.Contains(err.Error(), "already merged") {
		t.Fatalf("park error = %q, want the already-merged refusal", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if string(raw) != doc {
		t.Errorf("refused park rewrote the file: %q", raw)
	}
}

// Parking an already-parked ticket is a no-op success, so a repeated p never
// fails.
func TestParkDriverTicketFileKeepsParked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-first.md")
	doc := "# 01: Test ticket\n\nStatus: parked\n\n**Blocked by:** None\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	if err := ParkDriverTicketFile(path); err != nil {
		t.Fatalf("ParkDriverTicketFile on a parked ticket: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if !driverFileParked(string(raw)) {
		t.Errorf("parked file = %q, want Status parked", raw)
	}
}

// A zero pass time falls back to the run's injected clock, so a run built
// over a fake clock never dates rows at the zero time.
func TestDriverRunZeroTimeUsesInjectedClock(t *testing.T) {
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	run := NewDriverRun(func() time.Time { return at })
	order := []DriverTicket{{ID: "01-first", Path: "/spec/01-first.md", Num: 1}}
	run.set(order, func(tk DriverTicket) DriverRow {
		return DriverRow{ID: tk.ID, Path: tk.Path, Num: tk.Num, State: DriverRowQueued}
	}, time.Time{})
	rows := run.Snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want one", rows)
	}
	if !rows[0].Since.Equal(at) {
		t.Errorf("Since = %v, want the injected clock %v", rows[0].Since, at)
	}
}

// Publishing never holds the run lock across the world: a builder that
// snapshots (any world lock taken before a snapshot, as the web Run read
// does) must not deadlock.
func TestDriverRunSetBuildsWithoutHoldingTheLock(t *testing.T) {
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	run := NewDriverRun(func() time.Time { return at })
	order := []DriverTicket{{ID: "01-first", Path: "/spec/01-first.md", Num: 1}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		run.set(order, func(tk DriverTicket) DriverRow {
			_ = run.Snapshot()
			return DriverRow{ID: tk.ID, Path: tk.Path, Num: tk.Num, State: DriverRowQueued}
		}, at)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("set holds its lock across the builder, deadlocking a snapshot inside it")
	}
	rows := run.Snapshot()
	if len(rows) != 1 || rows[0].ID != "01-first" {
		t.Fatalf("rows = %v, want the published row", rows)
	}
}
