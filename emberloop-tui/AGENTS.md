# AGENTS.md — emberloop-tui

## Purpose

The emberloop dashboard: `emberloop tui` opens it in the current pane. The Runs
tab lists specs grouped by repo (current first), each with running/ended
status, merged/total progress, live tokens, reported cost and memory, with
the selected spec's tickets beside them carrying a derived status glyph
each, refreshed every second. Enter or double-click opens a ticket's
detail: its Markdown plus every attached agent (worker, reviewers,
scouts) with harness, model, tokens, reported cost and RSS, with ticket
totals; the header carries the machine-wide RSS of all running agents.
Enter on an agent (or a spec's orchestrator) or a double-click focuses
its herdr tab and pane through `tab focus` plus `agent focus`, leaving
the TUI running in its own pane; a pane herdr no longer lists shows a
gone message instead of focusing anything.
The Settings tab edits the role slots, decision model and threshold
through the global settings load/save functions, with model choices from
each harness's cached `Models()` and the archive size beside them.

## Ownership

Owns this directory as a separate Go module (`snapshot.go` and its test,
`costs.go` with the usage/memory sums and its tests, `db_test.go` with the
shared fixture-database helpers the usage tests build on, `model.go` and its
test, `run.go` (the driver run pane: ticket table on top from the shared
driver run state, the agents view below; `RunLoader`, `RunParker` and jump
`CommandRunner` seams, `RunLiveRows` join, `RunDoneMsg` end) and its test,
`herdr.go` (pane-list parse, liveness, and the `focusAgentPane`
jump primitive) and its test, `settings.go` and its test,
`cmd/emberloop-tui/main.go` (dashboard plus `--driver-run`, which runs the
live driver loop in-process against the shared state)). Ticket status derivation lives in `Snapshot`;
status glyphs and colours live in `model.go`'s view layer (run-pane state
text in `run.go`'s view layer). The Settings tab edits through the
`SettingsBackend` seam: the live backend calls the core's settings load/save,
harness `Models()` cache and archive size (this module may import the core;
never the reverse). Pane jumps run through the jump `CommandRunner` both
models share: the live binary passes os/exec, tests a fake over a canned
pane list. The run pane's `p` parks through its `RunParker` seam: the live
mode passes the driver's park command, tests a fake over canned rows.

## Local Contracts

- The core stays dependency-free: `emberloop` never imports this module. The
  `tui` subcommand execs the installed `emberloop-tui` binary beside it (or on
  PATH); `emberloop install` ships all three binaries.
- Running is decided by the orchestrator pane being alive in herdr, never by
  the run record's end time. A herdr failure keeps the last snapshot and
  retries; merged never exceeds total.
- Usage rows are read incrementally through one live `Reader`'s row-id
  high-water mark over the state database opened read-only; costs are
  harness-reported only, never estimated.
- Jumping only ever focuses: `tab focus` plus `agent focus` through the
  jump seam, never a close, move or split; a pane herdr no longer lists
  shows a gone message instead.
- UI toolkit lock (bubbletea v1 line throughout): bubbletea v1.3.10 (program,
  `tea.Tick` polling, no extra dep), bubbles v1.0.0 (list for runs/tickets,
  viewport for ticket Markdown, table for agent rows, textinput for the
  free-text model id, progress for the merged/total bar), lipgloss
  v1.1.1-0.20250404203927-76690c660834 (styling, tree for spec → ticket →
  agent), bubblezone v1.0.0 (named click regions; double-click detection
  stays ours — last click zone plus time). No huh: the Settings tab
  hand-rolls its rows on textinput and bubblezone so keys, clicks and `r`
  share the tab's Update loop. glamour v1.0.0 (ticket Markdown). Skipped:
  ntcharts, evertras/bubble-table, tview/gocui/ratatui ports.

## Work Guidance

- Build and test from this directory: `go build ./...`, `go vet ./...`,
  `go test ./...`. Never launch the binary from a test: drive the model
  through the `Loader` seam and liveness through the `Herdr` and
  `CommandRunner` seams with fixture state dirs; drive Settings edits
  through the `SettingsBackend` seam with the shared load/save pointed at
  a temp file and stubbed model lists.
- Behavioural change needs a test through `Snapshot`, the run loader, or
  model messages, not internals; expected values come from the fixture,
  never recomputed the way the code does. Enter and double-click jumps assert
  through a fake jump runner over a canned pane list, plus the gone
  message for a pane the list no longer names.

 ## Verification

 `cd emberloop-tui && go vet ./... && go test ./...`. The repo check
 (`.emberloop/check`) runs this module and `emberloop/`.

 ## Child DOX Index

 No child docs.
