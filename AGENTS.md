# AGENTS.md — emberloop

Project preamble: Emberloop is a browser UI and local Daemon (`emberloop-web/`,
one Go module) that runs coding-agent Sessions and deterministic implement/
review Runs. `emberloop-web/el/` is the el library behind it: the el MCP tools,
Run driver, session archive and usage tailer, and operator CLI with its update
path. `pig-antigravity-auth/` is a separate pig extension; `installer/` is the
install TUI run by `install.sh`; `scripts/` holds live probes that need running
agents; `.scratch/workspace-overhaul/` holds the combined workspace spec and
ticket map. Claude sessions read the same source through `CLAUDE.md`, which
imports this file; Daemon Claude Sessions load no `CLAUDE.md` and get a system-
prompt pointer to this file instead. The repo check is `.emberloop/check`
(`cd emberloop-web && go vet ./... && go test -timeout 15m -skip TestLive ./... && cd ../installer && go vet ./... && go test ./...`).

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

- `emberloop-web/AGENTS.md` — browser UI and Daemon seams: summary-only Runs,
  git caching, Ticket-to-Session index and non-blocking boot; cache-only quota
  API, subscriber-gated `quota` live refresher and percent-first provider cards;
  provider status/connect flows; Project Dashboard and stable scope routes;
  Jev draft Project picks; per-concern Settings and model/effort chains; admin-
  only host-global Memory settings, hybrid recall/injection and nightly stages;
  Profile patch push actions, inline Session approval, context-sensitive rejected-
  patch dedup, one-time 48-hour reminders and opt-in daily Memory digests; Memory
  page APIs, panic-safe Run-test cleanup and the no-cgo QuickJS SSR spike.
- `pig-antigravity-auth/AGENTS.md` — the Antigravity provider extension
- `installer/AGENTS.md` — the install TUI and its config merges
- `.scratch/archive/archived-emberloop-tui/AGENTS.md` — the archived dashboard module (not built, not tested)
- `.scratch/archive/archived-herdr-report/AGENTS.md` — the archived herdr report extension (not built, not tested)
- `.scratch/archive/archived-herdr-emberloop/AGENTS.md` — the archived herdr compress plugin (not built, not tested)
- `scripts/AGENTS.md` — the probe scripts (live agents only) and the offline fakellm server (no check)
- `.scratch/archive/retro-hardening/AGENTS.md` — the spec and ticket area for this run
- `.scratch/project-init/AGENTS.md` — the spec and ticket area for project init and validate
- `.scratch/quota-tab/AGENTS.md` — the quota tab history (original spec, done tickets 01 and 02); open quota work lives in workspace-overhaul
- `.scratch/projects-providers-settings/AGENTS.md` — the spec area for Speed, Providers, Project scope, the draft Project pick and the Settings redesign (spec only; no tickets yet)
- `.scratch/workspace-overhaul/AGENTS.md` — the combined spec of the 2026-10-13 grilling Session (Quota, Speed, Providers, Project scope, draft Project pick, Settings redesign, stall-proof Run tests; tickets for all Parts, Quota included)
- `.scratch/web-v1/AGENTS.md` — the spec and ticket area for web v1 in `emberloop-web` (Handoff compress, Users, Memory with Profile, Rules and Deciders)
- `.scratch/run-upgrade/AGENTS.md` — the spec and ticket area for the Run upgrade (Reviewer submit tool, Chains and Switch, spec branches, hygiene guards, `find` graph hits)
- `.scratch/memory-v2/AGENTS.md` — the spec and ticket area for Memory v2 (automatic extraction, embedding-backed Memories and hybrid recall, deduplication and same-author supersession, narrowed human gate, nightly fold/promote stages and scheduler fix; tickets 01 to 11)
- `.scratch/ssr/AGENTS.md` — the spec and ticket area for server-side rendering in `emberloop-web` (Go route table, embedded JS engine spike, per-route prefetch with Boot payload, cache headers, dependency updates first; tickets 01 to 06)
- `.scratch/emberplugins/AGENTS.md` — the spec and ticket area for Emberplugins (runtime-added pages, jobs and servers in `emberloop-web`)
- `.scratch/archive/enderbloom-web/AGENTS.md` — the spec and ticket area for Emberloop (browser UI and Daemon, planned module `emberloop-web`)
- `.scratch/archive/web-pipeline/AGENTS.md` — the spec and ticket area for the web pipeline (deterministic Runs from the browser UI)
- `.scratch/settings-redesign/AGENTS.md` — the design reference (static mock) for the Settings redesign in `emberloop-web`
- `.scratch/acp-bench/AGENTS.md` — the ACP agent/harness benchmark-candidate survey (tables + machine-readable JSON, survey only)
- `.scratch/designer-agents/AGENTS.md` — the spec and ticket area for Designer agents (Ticket types, Designer Workers and design Reviewers, `agent-browser` CLI, `to-tickets` override, grok removal; tickets 01 to 15)
- `GLOSSARY.md` — Emberloop terms (Daemon, Session, Project, Provider, Dashboard, quota percent, Backend, Subagent, Group, Steer, Follow-up, Event, Event log, Idle, Disposition, Chain, Switch, Boot payload, Prefetch table)
- `docs/agent/` — pages agents read as `el://docs/<page>` (update, verify, webui); `el update` refreshes them
- `docs/adr/0001-claude-via-acp-pig-via-sdk.md` — why claude runs over ACP and pig over its Go SDK
- `docs/adr/0002-embedded-quickjs-ssr.md` — why the Daemon uses modernc QuickJS for Svelte SSR
- `docs/adr/0004-automatic-memories-narrowed-human-gate.md` — automatic private Memories with human approval retained for Rules, Profile patches and Project notes
- `docs/adr/0005-designer-tickets-agent-browser-cli.md` — why design Tickets run on a Designer that sees UI through the agent-browser CLI, not an MCP or a native tool
