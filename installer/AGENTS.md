# AGENTS.md — installer

## Purpose

The install TUI that `install.sh` runs. It asks which harnesses to install
(pig, claude) and whether to enable the code graph MCP, then builds the three
binaries, installs herdr, links the herdr plugin, binds the compress key,
writes emberloop settings and the pig MCP entry, links the pig extensions, and
ends on the plain terminal with `emberloop deps` and `emberloop skills install`.

## Ownership

Owns this directory as a separate Go module: `main.go` (flags, TUI or plain
run), `tui.go` (the Bubble Tea screens), `steps.go` (the step list), `config.go`
(pure merges for the herdr config, `mcp.json`, settings and `go.work`).

## Local Contracts

- Every step is idempotent: a second run changes nothing.
- User files are merged, never replaced: `config.go` keeps unknown keys, and
  the herdr config gets a `.bak` copy before the first edit.
- A step that cannot finish returns `warning` when the install can go on
  without it; any other error stops the run.
- `--dry-run` touches nothing outside the build output directory it names.

## Verification

`cd installer && go vet ./... && go test ./...`; part of `.emberloop/check`.

## Child DOX Index

No child docs.
