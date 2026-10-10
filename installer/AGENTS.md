# AGENTS.md — installer

## Purpose

The install TUI that `install.sh` runs. It asks which harnesses to install
(pig, claude) and whether to enable the code graph MCP, then builds the two
binaries, writes emberloop settings and the pig MCP entry, links the pig extension, and
ends on the plain terminal with `el deps` and `el skills install`.

## Ownership

Owns this directory as a separate Go module: `main.go` (flags, TUI or plain
run), `tui.go` (the Bubble Tea screens), `steps.go` (the step list), `config.go`
(pure merges for `mcp.json`, settings and `go.work`).

## Local Contracts

- Every step is idempotent: a second run changes nothing.
- User files are merged, never replaced: `config.go` keeps unknown keys.
- A step that cannot finish returns `warning` when the install can go on
  without it; any other error stops the run.
- `--dry-run` touches nothing outside the build output directory it names.

## Verification

`cd installer && go vet ./... && go test ./...`; part of `.emberloop/check`.

## Child DOX Index

No child docs.
