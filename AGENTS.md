# AGENTS.md — emberloop

Project preamble: emberloop is a harness extension for running parallel agent
implement/review loops. Pig and claude sessions spawn workers in git
worktrees, judge them with persistent reviewers, and merge through a gate
(`finish`). The main Go package is `emberloop/`; `emberloop-web/` is the browser UI and Daemon module; `pig-antigravity-auth/` is a separate pig
extension; `installer/` is the install TUI run by `install.sh`; `scripts/` holds
live
probes that need running agents; `.scratch/archive/retro-hardening/` holds this
run's spec and tickets. Claude sessions read the same source through
`CLAUDE.md`, which imports this file; Daemon claude Sessions load no
CLAUDE.md and get a system-prompt pointer to this file instead. The repo check is `.emberloop/check`
(`cd emberloop && go vet ./... && go test -skip TestLive ./... && cd ../emberloop-tui && go vet ./... && go test ./... && cd ../installer && go vet ./... && go test ./... && cd ../emberloop-web && go vet ./... && go test ./...`).

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
- `pig-antigravity-auth/AGENTS.md` — the Antigravity provider extension
- `installer/AGENTS.md` — the install TUI and its config merges
- `emberloop-web/AGENTS.md` — the browser UI and Daemon module (fake, pig and claude Backends; users and cookie login)
- `.scratch/archive/archived-emberloop-tui/AGENTS.md` — the archived dashboard module (not built, not tested)
- `.scratch/archive/archived-herdr-report/AGENTS.md` — the archived herdr report extension (not built, not tested)
- `.scratch/archive/archived-herdr-emberloop/AGENTS.md` — the archived herdr compress plugin (not built, not tested)
- `scripts/AGENTS.md` — the live probe scripts (live agents only, no check)
- `.scratch/archive/retro-hardening/AGENTS.md` — the spec and ticket area for this run
- `.scratch/project-init/AGENTS.md` — the spec and ticket area for project init and validate
- `.scratch/quota-tab/AGENTS.md` — the spec and ticket area for the quota tab (account quota page in `emberloop-web`)
- `.scratch/web-v1/AGENTS.md` — the spec and ticket area for web v1 in `emberloop-web` (Handoff compress, Users, Memory with Profile, Rules and Deciders)
- `.scratch/emberplugins/AGENTS.md` — the spec and ticket area for Emberplugins (runtime-added pages, jobs and servers in `emberloop-web`)
- `.scratch/archive/enderbloom-web/AGENTS.md` — the spec and ticket area for Emberloop (browser UI and Daemon, planned module `emberloop-web`)
- `.scratch/archive/web-pipeline/AGENTS.md` — the spec and ticket area for the web pipeline (deterministic Runs from the browser UI)
- `GLOSSARY.md` — Emberloop terms (Daemon, Session, Backend, Subagent, Group, Steer, Follow-up, Event, Event log)
- `docs/agent/` — pages agents read as `el://docs/<page>` (update, verify, webui); `el update` refreshes them
- `docs/adr/0001-claude-via-acp-pig-via-sdk.md` — why claude runs over ACP and pig over its Go SDK
