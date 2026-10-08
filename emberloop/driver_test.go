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
