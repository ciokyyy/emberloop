package emberloop

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A worker idle for three minutes with no submit gets the fixed nudge: one
// tell to spawn its reviewers and submit, sent at three minutes idle. The
// nudge works: the worker reviews, submits, and the retry merges.
func TestDriverNudgesAWorkerIdleThreeMinutesWithNoSubmit(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string { return "tip" }
	submits := 0
	world.Submits = func(id string) int { return submits }
	passing := false
	world.Reviews = func(id string) map[string]DriverReview {
		if !passing {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	var tells []string
	var toldAt []time.Time
	world.Tell = func(id, text string) error {
		tells = append(tells, text)
		toldAt = append(toldAt, now)
		passing = true
		submits = 1
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-nudged"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(tells) != 1 {
		t.Fatalf("tell calls = %d, want 1: the idle worker gets one nudge", len(tells))
	}
	for _, text := range tells {
		for _, want := range []string{"spawn", "reviewer", "submit"} {
			if !strings.Contains(strings.ToLower(text), want) {
				t.Errorf("nudge %q must tell the worker to spawn its reviewers and submit", text)
			}
		}
	}
	if idle := toldAt[0].Sub(start); idle != 3*time.Minute {
		t.Errorf("nudge sent after %v idle, want 3m", idle)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-nudged" {
		t.Errorf("merged = %v, want the nudged worker's merge", res.Merged)
	}
}

// Worker activity resets the stall timer: a tip change one minute in pushes
// the nudge to four minutes, not three.
func TestDriverWorkerActivityResetsTheStallTimer(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string {
		if !now.Before(start.Add(time.Minute)) {
			return "tip-2"
		}
		return "tip-1"
	}
	passing := false
	world.Reviews = func(id string) map[string]DriverReview {
		if !passing {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip-2"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip-2"},
		}
	}
	submits := 0
	world.Submits = func(id string) int { return submits }
	var toldAt []time.Time
	world.Tell = func(id, text string) error {
		toldAt = append(toldAt, now)
		passing = true
		submits = 1
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-active"}, nil
	}
	// The nudge works: the worker submits, passes at its tip, and merges.
	// Nothing after the submit is nudged again.
	if _, err := RunDriver(spec, world); err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if len(toldAt) != 1 {
		t.Fatalf("tell calls = %d, want 1", len(toldAt))
	}
	if idle := toldAt[0].Sub(start); idle != 4*time.Minute {
		t.Errorf("nudge sent after %v, want 4m: the tip change at 1m resets the 3m timer", idle)
	}
}

// With no activity and no submit the nudge repeats every five minutes, and
// the third one raises the needs-you alert through the shared notify path:
// the ticket is marked in the run state, and no fourth nudge goes. The alert
// reaches a human, who pokes the worker: it reviews, submits, and the next
// finish merges, ending the run.
func TestDriverNudgeRepeatsEveryFiveMinutesAndThirdRaisesAlert(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string {
		if !now.Before(start.Add(9 * time.Minute)) {
			return "tip-B"
		}
		return "tip-A"
	}
	rescued := false
	world.Reviews = func(id string) map[string]DriverReview {
		if !rescued {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip-B"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip-B"},
		}
	}
	world.Submits = func(id string) int {
		if rescued {
			return 1
		}
		return 0
	}
	var toldAt []time.Time
	var firstNudge string
	world.Tell = func(id, text string) error {
		if firstNudge == "" {
			firstNudge = text
		}
		if text != firstNudge {
			t.Errorf("repeat nudge = %q, want the fixed first nudge %q", text, firstNudge)
		}
		toldAt = append(toldAt, now)
		if len(toldAt) == 3 {
			rescued = true
		}
		return nil
	}
	var notified []string
	world.Notify = func(id, text string) error {
		notified = append(notified, id+": "+text)
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-rescued"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	want := []time.Duration{3 * time.Minute, 8 * time.Minute, 13 * time.Minute}
	if len(toldAt) != len(want) {
		t.Fatalf("tell calls = %d, want %d: the nudge repeats every 5m, then the alert fires", len(toldAt), len(want))
	}
	for i, at := range toldAt {
		if idle := at.Sub(start); idle != want[i] {
			t.Errorf("nudge %d sent after %v, want %v", i+1, idle, want[i])
		}
	}
	if len(notified) != 1 || !strings.Contains(notified[0], "01-first") {
		t.Errorf("notifications = %q, want one naming 01-first", notified)
	}
	stall, ok := res.Stalls["01-first"]
	if !ok {
		t.Fatalf("run state has no stall for 01-first: %v", res.Stalls)
	}
	if stall.Nudges != 3 || !stall.NeedsYou {
		t.Errorf("stall = %+v, want 3 nudges and the needs-you alert", stall)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-rescued" {
		t.Errorf("merged = %v, want the rescued worker's merge after the alert", res.Merged)
	}
}

// The run state keeps the real reviewer spawn error read out of the stalled
// worker's transcript, and the stall alert names it. A tip change before the
// ten-minute idle alert keeps the stall path the one that raises it.
func TestDriverStallKeepsTheReviewerSpawnErrorFromTheTranscript(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string {
		if !now.Before(start.Add(9 * time.Minute)) {
			return "tip-B"
		}
		return "tip-A"
	}
	rescued := false
	world.Reviews = func(id string) map[string]DriverReview {
		if !rescued {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip-B"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip-B"},
		}
	}
	world.Submits = func(id string) int {
		if rescued {
			return 1
		}
		return 0
	}
	world.Tell = func(id, text string) error { return nil }
	world.Transcript = func(id string) string {
		return "working on the ticket\n" +
			"spawn reviewer-standards: herdr agent spawn failed: no free pane\n" +
			"retrying shortly\n" +
			"spawn reviewer-spec: herdr agent spawn could not start: timed out after 30s\n"
	}
	var notified []string
	world.Notify = func(id, text string) error {
		notified = append(notified, text)
		rescued = true
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-rescued"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	const want = "spawn reviewer-spec: herdr agent spawn could not start: timed out after 30s"
	if stall := res.Stalls["01-first"]; stall.SpawnError != want {
		t.Errorf("spawn error = %q, want the transcript's last spawn failure %q", stall.SpawnError, want)
	}
	if len(notified) != 1 || !strings.Contains(notified[0], want) {
		t.Errorf("notifications = %q, want one naming the spawn error", notified)
	}
}

// A worker that submits is never nudged: ten minutes of running with a submit
// on record ends in its merge with no tell.
func TestDriverSubmittingWorkerIsNotNudged(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string {
		if !now.Before(start.Add(10 * time.Minute)) {
			return "tip-2"
		}
		return "tip-1"
	}
	world.Reviews = func(id string) map[string]DriverReview {
		if world.Tip(id) != "tip-2" {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip-2"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip-2"},
		}
	}
	world.Submits = func(id string) int { return 1 }
	told := 0
	world.Tell = func(id, text string) error {
		told++
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-submitted"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if told != 0 {
		t.Errorf("tell calls = %d, want 0: a worker that submits is not nudged", told)
	}
	if len(res.Stalls) != 0 {
		t.Errorf("stalls = %v, want none", res.Stalls)
	}
	if len(res.Merged) != 1 || res.Merged[0].SHA != "sha-submitted" {
		t.Errorf("merged = %v, want the submitted worker's merge", res.Merged)
	}
}

// The live world reads the worker transcript for the spawn error through the
// same Transcript hook the loop scans: an unknown agent reads as no
// transcript, never a failure. Notifications go through the shared needs-you
// wiring.
func TestDriverLiveWorldNotifiesAndReadsTranscripts(t *testing.T) {
	oldState, oldNotify := statePath, notifyExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = oldState; notifyExec = oldNotify })
	repo, dir := t.TempDir(), t.TempDir()
	ticket := filepath.Join(dir, "01-first.md")
	files := map[string]string{ticket: ticketDoc("None")}
	spec := SpecRun{Spec: filepath.Join(dir, "spec.md"), Tickets: dir, Branch: "spec/driver-live-stall"}
	var notified [][]string
	notifyExec = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		notified = append(notified, args)
		return Result{Code: 0}, nil
	}
	world := liveDriverWorld(spec, repo, files)
	id := WorkerID(ticket)
	if got := world.Transcript(id); got != "" {
		t.Errorf("Transcript = %q for an unknown agent, want empty", got)
	}
	if err := world.Notify("01-first", "ticket 01-first needs you: stalled with no submit after 3 nudges"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(notified) != 1 {
		t.Fatalf("herdr calls = %d, want 1 notification", len(notified))
	}
	want := []string{"notification", "show", "emberloop: ticket 01-first needs you", "--body", "ticket 01-first needs you: stalled with no submit after 3 nudges"}
	if !equalIDs(notified[0], want) {
		t.Errorf("notify args = %q, want %q", notified[0], want)
	}
}

// A dead worker pane never aborts the run: failed nudges are recorded on the
// stall, the alert still fires through the shared notify path, and the
// recovered worker still merges.
func TestDriverFailedNudgeIsRecordedNotFatal(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string { return "tip" }
	submits := 0
	world.Submits = func(id string) int { return submits }
	passing := false
	world.Reviews = func(id string) map[string]DriverReview {
		if !passing {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip"},
		}
	}
	calls := 0
	world.Tell = func(id, text string) error {
		calls++
		if calls < 3 {
			return fmt.Errorf("unknown agent %q", id)
		}
		passing = true
		submits = 1
		return nil
	}
	var notified int
	world.Notify = func(id, text string) error {
		notified++
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-rescued"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	stall, ok := res.Stalls["01-first"]
	if !ok {
		t.Fatalf("run state has no stall for 01-first: %v", res.Stalls)
	}
	if stall.Nudges != 3 {
		t.Errorf("nudges = %d, want 3: failed deliveries still count", stall.Nudges)
	}
	if !strings.Contains(stall.NudgeError, "unknown agent") {
		t.Errorf("nudge error = %q, want the dead-pane failure", stall.NudgeError)
	}
	if !stall.NeedsYou || notified != 1 {
		t.Errorf("alert = %+v with %d notifications, want the alert and one", stall, notified)
	}
	if len(res.Merged) != 1 {
		t.Errorf("merged = %v, want the rescued merge: the run survived", res.Merged)
	}
}

// Without a submit count no submit can be ruled out, so nothing is nudged: a
// worker running ten minutes with no Submits hook merges without a nudge.
func TestDriverWithoutSubmitCountNothingIsNudged(t *testing.T) {
	spec, world, _ := driverWorld(t, map[string]string{
		"01-first.md": ticketDoc("None"),
	})
	world.MaxWorkers = 1
	start := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	now := start
	world.Now = func() time.Time { return now }
	world.Sleep = func(d time.Duration) { now = now.Add(d) }
	world.Tip = func(id string) string {
		if !now.Before(start.Add(10 * time.Minute)) {
			return "tip-2"
		}
		return "tip-1"
	}
	world.Reviews = func(id string) map[string]DriverReview {
		if world.Tip(id) != "tip-2" {
			return nil
		}
		return map[string]DriverReview{
			reviewerStandards: {Verdict: verdictPass, At: "tip-2"},
			reviewerSpec:      {Verdict: verdictPass, At: "tip-2"},
		}
	}
	told := 0
	world.Tell = func(id, text string) error {
		told++
		return nil
	}
	world.Finish = func(id string) (DriverFinish, error) {
		return DriverFinish{Merged: true, SHA: "sha-untracked"}, nil
	}
	res, err := RunDriver(spec, world)
	if err != nil {
		t.Fatalf("RunDriver: %v", err)
	}
	if told != 0 {
		t.Errorf("tell calls = %d, want 0: no submit count means no nudge", told)
	}
	if len(res.Merged) != 1 {
		t.Errorf("merged = %v, want the merge", res.Merged)
	}
}
