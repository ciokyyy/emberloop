package emberloop

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// DriverRowState is one ticket's state as the run pane shows it: queued
// (never started), running (started, no reviewer verdict yet), in-review (a
// verdict at the worker's tip but the gate still closed), merging (the gate
// passed and finish is running), needs-you (the alert fired, retries
// continuing), parked (the user parked it, worktree kept), merged, or
// discarded (the user discarded it).
type DriverRowState string

const (
	DriverRowQueued    DriverRowState = "queued"
	DriverRowRunning   DriverRowState = "running"
	DriverRowInReview  DriverRowState = "in-review"
	DriverRowMerging   DriverRowState = "merging"
	DriverRowNeedsYou  DriverRowState = "needs-you"
	DriverRowParked    DriverRowState = "parked"
	DriverRowMerged    DriverRowState = "merged"
	DriverRowDiscarded DriverRowState = "discarded"
)

// DriverRow is one ticket's live row: its state, its worker and pane for the
// jump, the reviewer verdicts at the current tip, when it entered this
// state, and the last refusal text while it needs you.
type DriverRow struct {
	ID          string
	Path        string
	Num         int
	State       DriverRowState
	Worker      string
	Pane        string
	Tip         string
	Verdicts    map[string]DriverReview
	Required    []string
	Since       time.Time
	LastRefusal string
}

// DriverRun is the in-memory run state the driver loop and the run pane
// share: the loop publishes every pass, the pane reads snapshots. It holds
// no file of its own; the tickets, worker records and git stay the only
// persisted state.
type DriverRun struct {
	mu    sync.Mutex
	now   func() time.Time
	order []DriverTicket
	rows  map[string]DriverRow
}

// NewDriverRun builds the shared run state over clock. Nil reads time.Now;
// tests inject a fake.
func NewDriverRun(clock func() time.Time) *DriverRun {
	if clock == nil {
		clock = time.Now
	}
	return &DriverRun{now: clock, rows: map[string]DriverRow{}}
}

// Snapshot returns one row per ticket in start order. Copies, so the pane
// never races the loop.
func (r *DriverRun) Snapshot() []DriverRow {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DriverRow, 0, len(r.order))
	for _, tk := range r.order {
		row, ok := r.rows[tk.ID]
		if !ok {
			continue
		}
		row.Verdicts = cloneDriverVerdicts(row.Verdicts)
		row.Required = append([]string(nil), row.Required...)
		out = append(out, row)
	}
	return out
}

// cloneDriverVerdicts copies a verdict map, so snapshots share nothing.
func cloneDriverVerdicts(in map[string]DriverReview) map[string]DriverReview {
	if in == nil {
		return nil
	}
	out := make(map[string]DriverReview, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// set publishes one pass: rows for tickets the builder still names keep
// their entry time while their state holds, and re-date it on change. A zero
// pass time falls back to the run's own clock, so the injected clock always
// dates a row. The builder runs before the lock is taken, so world callbacks
// behind it may take their own locks: holding this lock across the world
// would deadlock any reader that takes those locks before snapshotting.
func (r *DriverRun) set(order []DriverTicket, build func(DriverTicket) DriverRow, now time.Time) {
	if now.IsZero() {
		now = r.now()
	}
	built := make(map[string]DriverRow, len(order))
	for _, tk := range order {
		built[tk.ID] = build(tk)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append([]DriverTicket(nil), order...)
	next := make(map[string]DriverRow, len(order))
	for _, tk := range order {
		row := built[tk.ID]
		if old, ok := r.rows[tk.ID]; ok && old.State == row.State && !old.Since.IsZero() {
			row.Since = old.Since
		} else if row.Since.IsZero() {
			row.Since = now
		}
		next[tk.ID] = row
	}
	r.rows = next
}

// ParkDriverTicketFile is the park command: it writes Status parked into the
// ticket file at path, keeping the worktree and branch. The first Status
// line is rewritten in place with its markup kept, the same rule
// ticketStatusValues reads, so writer and readers agree; a file with no
// Status line gains one on top. A merged ticket is refused: rewriting Status
// done to parked would corrupt the merge record. An already-parked ticket
// succeeds without touching the file, so a repeated park never fails. It
// takes the same per-file lock as the checkbox tick, so the driver's own
// write never races it.
func ParkDriverTicketFile(path string) error {
	mu := driverTickLock(path)
	mu.Lock()
	defer mu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	switch driverTicketState(string(raw)) {
	case driverTicketDone:
		return fmt.Errorf("ticket already merged")
	case driverTicketParked:
		return nil
	}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Status:") && !strings.HasPrefix(trimmed, "**Status:") {
			continue
		}
		if j := strings.Index(line, "Status:"); j >= 0 {
			rest := line[j+len("Status:"):]
			k := 0
			for k < len(rest) && (rest[k] == '*' || rest[k] == ' ' || rest[k] == '\t') {
				k++
			}
			sep := strings.TrimRight(rest[:k], " \t") + " "
			lines[i] = line[:j+len("Status:")] + sep + "parked"
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}
	out := "Status: parked\n\n" + string(raw)
	return os.WriteFile(path, []byte(out), 0o644)
}
