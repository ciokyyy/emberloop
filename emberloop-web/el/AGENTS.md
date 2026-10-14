# AGENTS.md — emberloop-web/el (main Go package)

## Purpose

The el library behind the Emberloop Daemon: the tool roles the single el MCP
server serves, the deterministic Run driver over ticket files, the spec and
ticket views, the session archive and usage tailer, the quota snapshot, and
the operator CLI (`el`) with its update/install path. Builds and tests from
the module root; the repo check covers this package including `tools/` and
`mcp/`.

## Ownership

Owns every `.go` file directly in this directory. The two entry points live
in `../cmd/` (owned by `emberloop-web/AGENTS.md`). `tools/`, `mcp/`,
`statedb/`, and `testdata/` are separate
durable boundaries with their own docs. File map (non-test file owns its
`_test.go` and the behaviour below):

- Sessions and roles: `harness.go` (the harness registry: transcript layout
  `SessionRoot` and model listing `Models` per harness, used by the archiver
  and the usage tailer), `voice.go` + `caveman/SKILL.md` (the register: the
  caveman plugin skill, byte for byte, at ultra; plus the per-turn reminder,
  `el voice-reminder`), `models.go` (harness model listings through the Exec
  seam, with a cache; served by `el settings models`), `resolve.go` (spec
  resolution: `SpecRun`, `ResolveSpec`, `IsSpecFileName`, the per-spec ticket
  cap `MaxTicketsForSpec`, and the small frontmatter reader), `stopgate.go`
  (the shared stop-gate decision `DecideGate` over `GateInput`, applied by
  the web Daemon; the `stop-gate`/`pig-gate` CLI hooks are gone),
  `agentstate.go` (the agent record the usage tailer reads for the current
  harness session id, plus `statePath`), `transcript.go` (transcript lookup
  by session id for the archiver and the tailer), `archive.go` (the session
  archive: blob store, session records, transcript copies, `SystemPrompt`,
  the file lock), `memory.go` (the harness process-tree sampler),
  `usage.go` (transcript tailing into usage rows, served to el-mcp through
  `UsageTailFromEnv`/`RunUsageTail`/`TailUsageFile`), `claude_usage.go` (the
  statusline command `el claude-status` and its usage windows),
  `usage_providers.go` (the opencode and Antigravity usage pollers, one
  fetch per 90 s per account, cached as `usage-<key>.json` in the state
  dir), `quota_api.go` (the Quota-tab snapshot over the same cache:
  per-card state, `RefreshQuota`, no fetch logic of its own),
  `compress_prompt.go` (the Handoff summarizer prompt, its section list
  `HandoffSections` that emberloop-web uses, and the handoff sanity check),
  `context_window.go` (model window table; settings override via
  `context.windows` / `el settings window <model> <n>` beats table; unknown
  models get 200k default), `docs.go` (install copies the checkout's
  `docs/agent/*.md` into the `docs` namespace, read as `el://docs/<page>`;
  the system prompt names it), `skills.go` (the skill library root and
  `el skills install`), `deps.go` (the `el deps` installer), `logs.go`
  (`el logs` over the prod Daemon journal), `restart.go` (`el restart`),
  `version.go`, `update.go` (rebuilds el, el-mcp and el-web with version
  stamps, installs them, rolls back when the new el-web fails its health
  check; `el install` is an alias), `install.go` (the shared build-and-swap
  machinery update uses)
- Runs: `driver.go` (the deterministic driver `RunDriver` over an injected
  `DriverWorld`: ticket graph, start order, reviewer gate at the tip,
  stall nudges and needs-you, park/discard, checkbox ticks, the plain
  report; plus the web driver's file ops `TickDriverTicketFile`,
  `MarkDriverTicketDone`, `MergeCheckedBranch`, the rebase-conflict refusal
  `RebaseConflictRefusal` (both merge paths send it; it tells the Worker to
  redo the rebase in its worktree and resolve it), the worker id `WorkerID`,
  and the reviewer vocabulary), `driver_run.go` (the shared run state
  `DriverRun`, published every pass; `ParkDriverTicketFile` is the park
  command), `ticketview.go` (the Runs preview surface: per-ticket title,
  resolved blockers and checkbox progress where one bad ticket fails only
  its own row)
- Tools served to agents: `tools/` (child doc)
- Transport: `mcp/` (child doc); binaries are `../cmd/el/main.go`
  (operator CLI: settings, update/install, restart, logs, deps, skills,
  version, claude-status, voice-reminder) and
  `../cmd/el-mcp/main.go` (tool server; writes the file named by
  `EL_READY_FILE` once its tool listing is done, for harnesses that cannot
  see readiness)
- Policy: `roles.go` (the web Daemon roles `web`, `web-worker`,
  `run-worker`, `scout` and `full`), `toolresolve.go` (one role resolution
  over settings for the main list and execute; `WebSubmitEnabled` (off)
  keeps `submit` unoffered to web children while the Run submit route
  stays; the served `read` refuses an oversized file with `bash` for roles
  that serve it and `tools.grep` in `execute` or offset/limit otherwise),
  `readhint.go` (that refusal wrapper), `bashhint.go` (the served `bash`
  of a role that also serves `execute` appends a filter-in-`execute` tip to
  a stored oversized output), `execute.go` (the per-role execute binding:
  the role's resolved toolset in the stub; sandboxed for the read-only
  scout role), `mcpclients.go` (the MCP server snapshot behind the same
  resolution, plus the graph-gated snapshot the tool server serves)
- Daemon callbacks: `tools/webproxy.go` and `tools/sudogate.go` send
  `EL_WEB_TOKEN`, the web Daemon's machine token (see
  `emberloop-web/AGENTS.md`), as a bearer on every call; the Run-worker
  `submit` proxy also forwards typed per-finding Dispositions.

- Every operation runs through the `Exec` seam (`exec.go`) so tests use a
  fake exec and temp directories; no test touches the real home, a live
  agent or harness, or the network beyond loopback `httptest` servers.
- An el MCP server with no role refuses to start; the full tool set needs
  an explicit full role.
- The tree stays gofmt-clean; formatting is a check result, never a
  judgement call.

## Work Guidance

- Build and test from the module root (`emberloop-web/`): `go build ./...`,
  `go vet ./...`, `go test -skip TestLive ./...`. Never run a live test and
  never start `el`, `pig`, or `claude` from inside the work.
- Behavioural change needs a test through the tool or command, not
  internals; expected values come from the spec or a worked example.

## Verification

`.emberloop/check` (`cd emberloop-web && go vet ./... && go test -skip TestLive ./... && cd ../installer && go vet ./... && go test ./...`).

## Child DOX Index

- `emberloop-web/el/tools/AGENTS.md` — the agent tools both hosts serve
- `emberloop-web/el/mcp/AGENTS.md` — the MCP server transport over those tools
- `emberloop-web/el/statedb/AGENTS.md` — the SQLite state database both hosts share
- `emberloop-web/el/testdata/AGENTS.md` — captured fixtures tests parse
