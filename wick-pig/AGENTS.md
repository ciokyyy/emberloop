# AGENTS.md — wick-pig (main Go package)

## Purpose

The wick harness itself: pig and claude worker sessions, the spawn/grid
multiplexer plumbing, persistent reviewers, the vet-and-test gate, and the
`finish` merge. Builds and tests from this directory; the repo check runs
here and covers this module including `tools/`, `mcp/`, and `cmd/`.

## Ownership

Owns every `.go` file directly in this directory plus `cmd/` (the two
entry points). `tools/`, `mcp/`, `agenttypes/`, `statedb/`, and `testdata/`
are separate durable boundaries with their own docs. File map (non-test file
owns its `_test.go` and the behaviour below):

- Sessions and harnesses: `harness.go` (pig/claude commands, system prompt),
  `voice.go` + `caveman/SKILL.md` (the register: the caveman plugin skill, byte
  for byte, at ultra; plus the per-turn reminder, `wick-pig voice-reminder`),
  `models.go`, `resolve.go`, `background.go`, `exec.go`, `launch.go`
  (pane launch text, the child's exit trace), `supervisor.go` (compress restart
  in the same pane: boundary wait, child stop, handoff relaunch, pane-name
  re-assertion, session rotation with the compressed count, forced restart
  past the wait), `compress.go` (manual compress: pig one-off summary,
  handoff and transcript copies, same-pane restart through the supervisor),
  `compress_request.go` (the herdr key's signal: queue mark at the boundary,
  second-press force, top-level fallback, single-flight claim, `request-compress`),
  `autocompress.go` (automatic trigger: threshold decision with cooldown and
  disable switch, concurrent worker sweep and supervisor loop with failure
  backoff, always-continue handoff),
  `compress_prompt.go` (the pinned summarizer prompt and handoff sanity
  check), `compress_render.go` (transcript rendered as text in full),
  `commands.go`, `skills.go`, `install.go` (the live-agent gate, top-level
  exempt, the herdr plugin link and the keybinding line, ending with the deps
  step), `deps.go` (the `wick-pig deps` installer)
- Spawning and layout: `spawn.go`, `spawngroup.go` (one grouped notice per
  spawn call), `worker.go` (brief, worktree, lifecycle),
  `workerqueue.go` (worker-cap queue and drain), `grid.go`, `tile.go`,
  `panename.go` (herdr pane names per agent type; the header strip ticket
  reads the same mapping for its name chip), `background.go`,
  `orchestrate.go` (the spawn/finish/tell orchestration tools)
- Review and merge: `review.go` (packet, pass record), `submit.go`,
  `tell.go`, `deliver.go`, `integration.go`, `run.go`, `history.go`
- Tools served to agents: `tools/` (child doc); agent-visible shell surface
  is `background.go`, `exec.go`
- Transport: `mcp/` (child doc); binaries are `cmd/wick-pig/main.go` (harness
  CLI) and `cmd/wick-mcp/main.go` (tool server); `tui.go` execs the installed
  `wick-tui` dashboard for the `tui` subcommand
- Agent data: `agentstate.go` (per-agent record: pane, session, compressed
  count, top-level role and cwd for compress restart, mid-compress claim with
  stale steal the header shows as `compressing…`), `agenttype.go`
  (built-in types plus `.wick/agents` overrides), `transcript.go`,
  `usage.go` (tailer parsing; rows land in `statedb/`), `memory.go`,
  `archive.go`, `prune.go` (deletes usage and overflow rows as well as files)
- Context core: `context.go` (snapshot over a transcript: the shared
  formula, outstanding and finished calls, per-tool sizes), `context_parse.go` (the
  per-harness line parsers a new harness registers), `context_window.go`
  (model window table), `context_report.go` (the `wick-pig context` output)
- Header strip: `header.go` (pure strip render: snapshot and width in,
  styled string out, Lip Gloss adaptive palette, width tiers),
  `header_wrap.go` (child size, scroll-region and margin filter, snapshot
  builder reading the queued compress mark and the compressing state),
  `header_run.go` (PTY runner: reduced child size, forwarding, resizes,
  repaints, exit restore; the borderless soft top edge, with the strip-rule
  setting restoring the drawn line), `header_source.go` (strip source from
  launch env, `wrap`/`exec`/top-level wiring); settings carry strip rows,
  frame, rule, per-harness compress models, the auto-compress switch and
  cooldown. `exec`,
  top-level runs and `wick-pig wrap` put the child on a PTY under the strip;
  WICK_HEADER_STRIP=0 runs plainly. Pig sessions with the tools debug log
  stay unwrapped.
- Policy: `roles.go` (no role refuses to start; `work`, `plan`, `full`,
  `review` and `scout` serve `execute`, `implement` does not), `toolresolve.go`
  (one role resolution over settings for the main list and execute; the served
  `read` refuses an oversized file with `bash` for roles that serve it and
  `tools.grep` in `execute` or offset/limit otherwise), `readhint.go` (that
  refusal wrapper),
  `execute.go` (the per-role execute binding: the role's resolved toolset in
  the stub, the tools-behind-execute description, sandboxed for read-only roles),
  `mcpclients.go` (the MCP server snapshot behind the same resolution, plus
  the graph-gated snapshot the tool server serves),
  `graph.go` (the code graph lifecycle: async worktree index on creation,
  incremental re-index before reviewer and scout spawns, project delete on
  worktree drop; best-effort, silent when the graph is off),
  `readonly.go`, `stopgate.go`, `extension.go` (pig extension wiring, brief lines)
- Fixtures: `testdata/` (child doc)

## Local Contracts

- Every operation runs through the `Exec` seam (`exec.go`) so tests use a
  fake exec and temp directories; no test touches the real home, a live
  agent or harness, or the network beyond loopback `httptest` servers.
- Liveness is a process past the pane's shell in herdr's process-info, never
  mere pane existence and never herdr's flapping agent detection; automated
  waits honor a boot grace after launch, while tell and finish by pane fail
  fast. A dead reviewer is replaced in its slot and open groups record the
  replacement, so they resolve with its verdict. The launcher records a bad
  child exit (code, signal, stderr tail) in the agent's state, and the lost
  notice carries it.
- A wick MCP server with no role refuses to start; the full tool set needs
  an explicit full role.
- The tree stays gofmt-clean; formatting is a check result, never a
  judgement call.

## Work Guidance

- Build and test from this directory: `go build ./...`, `go vet ./...`,
  `go test -skip TestLive ./...`. Never run a live test and never start
  `wick-pig`, `pig`, or `claude` from inside the work.
- Behavioural change needs a test through the tool or command, not
  internals; expected values come from the spec or a worked example.

## Verification

`.wick/check` (`cd wick-pig && go vet ./... && go test -skip TestLive ./... && cd ../wick-tui && go vet ./... && go test ./...`).

## Child DOX Index

- `wick-pig/tools/AGENTS.md` — the agent tools both hosts serve
- `wick-pig/mcp/AGENTS.md` — the MCP server transport over those tools
- `wick-pig/agenttypes/AGENTS.md` — the built-in worker, reviewer, and
  scout types
- `wick-pig/statedb/AGENTS.md` — the SQLite state database both hosts share
- `wick-pig/testdata/AGENTS.md` — captured fixtures tests parse
