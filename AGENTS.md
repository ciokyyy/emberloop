# AGENTS.md — emberloop

Project preamble: emberloop is a harness extension for running parallel agent
implement/review loops. Pig and claude sessions spawn workers in git
worktrees, judge them with persistent reviewers, and merge through a gate
(`finish`). The main Go package is `emberloop/`; `emberloop-tui/` is the dashboard
module; `herdr-report/` and `pig-antigravity-auth/` are separate pig
extensions, and `herdr-emberloop/` is the herdr compress plugin; `installer/` is the install TUI run by `install.sh`; `scripts/` holds
live
probes that need running agents; `.scratch/retro-hardening/` holds this
run's spec and tickets. Claude sessions read the same source through
`CLAUDE.md`, which imports this file. The repo check is `.emberloop/check`
(`cd emberloop && go vet ./... && go test -skip TestLive ./... && cd ../emberloop-tui && go vet ./... && go test ./... && cd ../installer && go vet ./... && go test ./...`).

## DOX contract

- `AGENTS.md` files are binding work contracts for their subtrees. A path
  must stay understandable from its nearest `AGENTS.md` plus every parent
  above it.
- Read before editing: read this root, walk from the root to each target
  path, and read every `AGENTS.md` on each route, following the child index
  down. The nearest doc is the local contract; parents carry repo-wide
  rules. Never rely on memory of these docs.
- Update after editing: a change that moves purpose, ownership, contracts,
  structure, workflow, inputs, outputs, or permissions must update its
  nearest owning doc, plus affected parents and children, in the same task.
  Refresh every affected child index and delete stale text on the spot.
  Trivial edits with no contract change need no doc change, but the pass
  itself still happens.
- Hierarchy: this root states project-wide rules and indexes the top level;
  children own domain detail and their own index. Closer docs are more
  specific. No child may weaken this contract.
- Style: concise, current, operational. Broad rules live in parents,
  concrete names in children. Never duplicate a rule across docs; each rule
  has one home. No history, no obvious statements, no warnings for dead
  risks.
- Closeout: re-check changed paths against the chain, update the owning
  docs, refresh indexes, run the check when code changed.

## Child DOX Index

Index paths are repo-relative. Every doc below is listed here or in the
child it belongs to, and every entry must resolve to a file.

- `emberloop/AGENTS.md` — the main Go package, its file map, and its Go
  workflow; indexes tools, MCP server, agent types, state database, and test data
- `emberloop-tui/AGENTS.md` — the dashboard module (Runs list, read model, toolkit lock)
- `herdr-report/AGENTS.md` — the herdr report extension (pig panes as
  addressable agents)
- `herdr-emberloop/AGENTS.md` — the herdr compress plugin (one key compresses
  the focused pane)
- `pig-antigravity-auth/AGENTS.md` — the Antigravity provider extension
- `installer/AGENTS.md` — the install TUI and its config merges
- `scripts/AGENTS.md` — the live probe scripts (live agents only, no check)
- `.scratch/retro-hardening/AGENTS.md` — the spec and ticket area for this run
- `.scratch/enderbloom-web/AGENTS.md` — the spec and ticket area for Enderbloom Web (browser UI and Daemon, planned module `emberloop-web`)
- `GLOSSARY.md` — Enderbloom Web terms (Daemon, Session, Backend, Subagent, Group, Steer, Follow-up, Event, Event log)
- `docs/adr/0001-claude-via-acp-pig-via-sdk.md` — why claude runs over ACP and pig over its Go SDK
