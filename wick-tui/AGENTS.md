# AGENTS.md — wick-tui

## Purpose

 The wick dashboard: `wick-pig tui` opens it in the current pane. The Runs
 tab lists specs grouped by repo (current first), each with running/ended
 status and merged/total progress, with the selected spec's tickets beside
 them carrying a derived status glyph each, refreshed every second. Later
 tickets add ticket detail (agents, tokens, cost, memory), pane jumping and
 the Settings tab.

## Ownership

Owns this directory as a separate Go module (`snapshot.go` and its test,
`model.go` and its test, `herdr.go` and its test, `cmd/wick-tui/main.go`).
Ticket status derivation lives in `Snapshot`; status glyphs and colours live
in `model.go`'s view layer.

## Local Contracts

- The core stays dependency-free: `wick-pig` never imports this module. The
  `tui` subcommand execs the installed `wick-tui` binary beside it (or on
  PATH); `wick-pig install` ships all three binaries.
- Running is decided by the orchestrator pane being alive in herdr, never by
  the run record's end time. A herdr failure keeps the last snapshot and
  retries; merged never exceeds total.
- UI toolkit lock (bubbletea v1 line throughout): bubbletea v1.3.10 (program,
  `tea.Tick` polling, no extra dep), bubbles v1.0.0 (list for runs/tickets,
  viewport for ticket Markdown, table for agent rows, textinput for the
  free-text model id, progress for the merged/total bar), lipgloss v1.1.0
  (styling, tree for spec → ticket → agent), bubblezone v1.0.0 (named click
  regions; double-click detection stays ours — last click zone plus time).
  huh v1.0.0 (Settings forms) and glamour v1.0.0 (ticket Markdown) join with
  their tickets. Skipped: ntcharts, evertras/bubble-table, tview/gocui/
  ratatui ports.

## Work Guidance

- Build and test from this directory: `go build ./...`, `go vet ./...`,
  `go test ./...`. Never launch the binary from a test: drive the model
  through the `Loader` seam and liveness through the `Herdr` and
  `CommandRunner` seams with fixture state dirs.
- Behavioural change needs a test through `Snapshot` or model messages, not
  internals; expected values come from the fixture, never recomputed the way
  the code does.

 ## Verification

 `cd wick-tui && go vet ./... && go test ./...`. The repo check
 (`.wick/check`) runs this module and `wick-pig/`.

 ## Child DOX Index

 No child docs.
