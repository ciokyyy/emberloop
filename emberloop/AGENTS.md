# AGENTS.md — emberloop (main Go package)

## Purpose

The emberloop harness itself: pig and claude worker sessions, the spawn/grid
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
  for byte, at ultra; plus the per-turn reminder, `el voice-reminder`),
  `models.go`, `resolve.go`, `background.go`, `exec.go`, `launch.go`
  (pane launch text, the child's exit trace), `supervisor.go` (compress restart
  in the same pane: boundary wait, then the wrapper's own child swap through
  its channel when live (queue, row and raw state kept, handoff pinned first),
  else child stop, handoff relaunch, pane-name
  re-assertion, session rotation with the compressed count, forced restart
  past the wait), `compress.go` (manual compress: pig one-off summary,
  handoff and transcript copies, same-pane restart through the supervisor),
  `compress_request.go` (the herdr key's signal: queue mark at the boundary,
  second-press force, top-level fallback, single-flight claim, `request-compress`),
  `autocompress.go` (automatic trigger: threshold decision with cooldown and
  disable switch, concurrent sweep of worker panes and recorded top-level panes, supervisor loop with failure
  backoff and visible failure: summary and restart errors recorded in agent state and tools log, strip shows `compress failed: <reason>`,
  10m cooldown parks pane, later success clears state, always-continue handoff),
  `compress_prompt.go` (the pinned summarizer prompt, its section list
  `HandoffSections` that emberloop-web uses, and the handoff sanity check),
  `compress_render.go` (transcript rendered as text in full),
  `commands.go`, `skills.go`, `install.go` (the live-agent gate, top-level
  exempt, the herdr plugin link and the keybinding line, ending with the deps
  step), `deps.go` (the `el deps` installer), `docs.go` (install copies
  the checkout's `docs/agent/*.md` into the `docs` namespace, read as
  `el://docs/<page>`; the system prompt names it)
- Spawning and layout: `spawn.go`, `spawngroup.go` (one grouped notice per
  spawn call), `worker.go` (brief, worktree, lifecycle),
  `workerqueue.go` (worker-cap queue and drain), `grid.go`, `tile.go`,
  `panename.go` (herdr pane names per agent type; the header strip ticket
  reads the same mapping for its name chip), `background.go`,
  `orchestrate.go` (the spawn/finish/tell orchestration tools)
- Review and merge: `review.go` (packet, pass record), `submit.go`,
  `tell.go`, `deliver.go`, `integration.go`, `run.go`, `history.go`
- Driver: `driver.go` (the deterministic `implement --driver` loop's one seam:
  `RunDriver` takes the spec and an injected world of ticket files, fake ops,
  a fake clock and a notify hook. A world without `Finish` only checks the
  ticket graph and prints the start order for `--driver --check`. A world with
  it runs the happy-path loop: tickets start in number order up to `MaxWorkers`
  once their blockers merged, `finish` runs only after every required reviewer
  type passed at the worker's current tip, a refused `finish` is told to the
  worker unchanged and retried after its next submit with no attempt limit
  (without a submit count each distinct refusal is told once), the same refusal
  three times in a row or ten minutes with no worker activity marks the ticket
  "needs you" (`NeedsYou` in the result, one `Notify` delivery with later
  attempts waiting out the same growing delay, recorded on the report when
  herdr never accepts it) with retries continuing on a doubling backoff while
  every other ticket keeps running (a ticket marked needs-you keeps the mark
  and its alert when later parked or discarded), a worker idle 3m (a pane reporting `working`, read through `Working`, is never idle) with no
  submit gets the fixed stall nudge (repeating every 5m, the third raising the
  needs-you alert with the transcript's spawn error named in it; any submit
  ends it; delivery is best effort and recorded, never fatal). After the alert
  there is no give-up bound: the live run keeps polling and a stuck run waits
  for a human; the spec never gives up on its own. The stall lands in the run
  state's `Stalls` beside the nudge count and the alert. A parked ticket (`Parked`, live:
  the seeded Status park plus the file's parked Status read every pass) is never
  started or finished and frees its slot so only its dependents wait, a ticket the user
  discarded mid-run (`Discarded`, live: no worker record, not queued, run
  history ending in a discard) is left alone the same way and reads discarded,
  the user's `Discard` op (live: the same finish with merge=false) is never
  called by the driver, and the plain report lists merges with sha, unmerged
  tickets with their last refusal, and worker follow-ups verbatim.
  Checkbox progress rides the worker `submit` `completed` list of checkbox line
  texts: during a driver run the driver is the only ticket-file writer, ticks
  each fresh box once through the per-file lock, leaves the edit uncommitted,
  and reports unknown names without failing the merge. A resumed run seeds its
  state from the world (Status done is merged with its sha from `MergedSHA`,
  Status parked sits out, `HasWorker` tickets are already started and hold
  their slot), never starts a second worker for a ticket that has one, writes
  no state file, and exits with the report when nothing runs and nothing can
  start (the rest wait on parked tickets) instead of polling. `RunDriverLive`
  wires the live spawn, verdict, tell, submit-count, submit-read, file-tick,
  transcript, finish, herdr-notification, park, discard, worker-pane and
  follow-up ops, the
  store-or-branch worker check and the history sha lookup.
  `driver_run.go` holds the shared run state: `DriverRun` (one row per ticket
  with state, worker and pane, verdicts at the tip, state entry time, last
  refusal; snapshots copy; a zero pass time falls back to the run's own
  clock; `set` builds rows before taking its lock, so world callbacks
  behind the build may take their own locks), published by the loop every
  pass when the world's `Run` is set and read by the run pane. `ParkDriverTicketFile`
  is the park command (Status parked through the per-file lock, refusing a merged
  ticket and leaving an already-parked one alone).
  `ReadDriverFiles` and `LiveDriverWorld` let the run pane build the same live
  world in its own process. The web driver
  reuses `TickDriverTicketFile`, `MarkDriverTicketDone` and
  `MergeCheckedBranch` (dirty check, rebase onto, checkout check,
  fast-forward-only) without reviewers or herdr. `ticketview.go`
  holds the Runs preview surface (`SpecTitle`, `PreviewTickets`; private
  title and checkbox helpers): per-ticket title, resolved blockers and
  checkbox progress where one bad ticket fails only its own row. The mode flags
  `--driver/--llm/--check` live in `cmd/emberloop/main.go`; on a terminal
  `--driver` opens the pane through `RunDriverPane` (`tui.go`, same binary
  lookup as `tui`), and quitting that pane stops its loop while the workers
  keep running, so a restart resumes; off one it prints the plain report;
  `--llm` and a bare run keep the LLM orchestrator)
- Tools served to agents: `tools/` (child doc); agent-visible shell surface
  is `background.go`, `exec.go`
- Transport: `mcp/` (child doc); binaries are `cmd/emberloop/main.go` (harness
  CLI) and `cmd/emberloop-mcp/main.go` (tool server); `tui.go` execs the installed
  `el-tui` dashboard for the `tui` subcommand
- Agent data: `agentstate.go` (per-agent record: pane, session, compressed
  count, top-level role and cwd for compress restart, mid-compress claim with
  stale steal the header shows as `compressing…`, `CompressFailed` reason for failed auto-compress), `agenttype.go`
  (built-in types plus `.emberloop/agents` overrides), `transcript.go`,
  `usage.go` (tailer parsing; rows land in `statedb/`), `memory.go`,
  `archive.go`, `prune.go` (deletes usage and overflow rows as well as files)
- Context core: `context.go` (snapshot over a transcript: the shared
  formula, outstanding and finished calls, per-tool sizes), `context_parse.go` (the
  per-harness line parsers a new harness registers),
  `context_window.go` (model window table: strips leading `provider/` paths and harness prefixes like
  `antigravity-`, case-insensitive longest prefix match with `deepseek-v4` at 1M and `gemini-` at 1M;
  settings override via `context.windows` / `el settings window <model> <n>` beats table; unknown models get 200k default),
  `context_report.go` (the `el context` output)
- Header strip: `header.go` (pure strip render: snapshot, width and preset
  in, styled string out, Lip Gloss adaptive palette, width tiers, `N pending`
  beside the state pill, `compress failed: <reason>` pill on refusal/error),
  `strip_symbols.go` (the omp-style segment system: the Nerd Fonts v3 icon
  table, the symbol presets, the separator glyphs, the nerd rounded caps),
  `header_tool.go` (the second row's activity label: the tool running now
  or the last one finished, a glyph and verb per tool in `toolStyles`,
  `N pending` when items wait; an idle agent shows no state word. **A new tool emberloop serves needs its
  `toolStyles` entry**: glyph, running verb, done verb. A test lists every
  tool in `tools.All()` plus spawn and submit and fails on a missing entry;
  an unlisted tool still shows, as a bare name), `claude_usage.go` and
  `usage_providers.go` (the usage windows pinned right on the second row:
  claude pushes its 5h and weekly windows through the statusline command
  `el claude-status`; the opencode Go plan and Antigravity are polled
  from their usage endpoints with pig's stored login, one fetch per 90 s
  per account across all panes, cached as `usage-<key>.json` in the state
  dir. A new provider adds a key in `UsageKeyFor` and a fetcher in
  `RefreshUsage`), `quota_api.go` (the Quota-tab snapshot over the same cache:
  per-card state, `RefreshQuota` with the force option behind the shared lock,
  no fetch logic of its own),
  `theme.go` (the shared palette: Catppuccin Mocha and Latte driving the
  strip, the generated claude and pig themes and the OSC pane colors;
  per-run launch flags plus two emberloop-named claude theme files in its config
  dir, never a per-run CLAUDE_CONFIG_DIR (a fresh one has no login or
  onboarding state) and never the user's saved settings),
  `header_wrap.go` (child size, input row height `InputRowHeight`, scroll-region and margin filter, snapshot
  builder reading the queued compress mark, compressing state, and persisted `CompressFailed` reason),
  `header_run.go` (PTY runner: reduced child size taking into account strip and optional input row,
  row/raw input routing with the Ctrl+O toggle, raw dirty tracking and 10 s
  stale clear, forwarding, repaints, pane palette set at start and reset at exit, on signals and across
  restarts, exit restore, the wrapper queue and pump, sets snapshot pending
  count from the in-process queue and raw mode from the rawMode flag via readSnap,
  sole child-PTY writer, the child's own stderr
  pipe beside the PTY; a compress restart swaps only the child in place
  (queue, row draft and history, w.raw kept, handoff pinned
  first through a fresh pump with the old one retired, same per-run theme),
  the read loop following the swap by child
  generation; the borderless soft top edge, with the strip-rule setting restoring the drawn
  line), `header_source.go` (strip source from launch env,
  `wrap`/`exec`/top-level wiring, every wrapped launch serves the control
  channel; `WrapTopLevel` enables the input row for top-level sessions),
  `inputrow.go` (the emberloop input row editor between child screen and strip for top-level
  sessions: takes user typing, Enter steer, Alt+Enter queue, Shift+Enter newline,
  bracketed paste, Up history recall, word motion and delete, a visible cursor,
  Ctrl+C / Esc interrupt forwarding, 10 s stale clear only while messages queue behind the draft),
  `keydecode.go` (the key decoder: raw bytes, legacy or kitty CSI-u, to canonical
  keys; focus reports and unknown sequences consumed; the row and the raw-mode
  toggle and reset checks act on its output, never on raw bytes),
  `rawmode.go` (raw passthrough: Ctrl+O toggles between the row and the child,
  strip shows `raw` and `raw, N queued`, dirty flag set by any forwarded byte
  except bare Enter and reset by Enter, Ctrl+C or the emberloop clear, injections
  held while dirty, 10 s stale with a non-empty queue sends six Ctrl+U then delivers (an empty queue never clears; the strip shows `queued N messages · draft clears in Ns`), per-harness
  clear/newline/interrupt table for claude and pig with a hold-only default),
  `inputqueue.go` (one queue per wrapper: the restart handoff pinned ahead of
  all, then user submissions before pending
  injections in arrival order, steer at tool boundary versus queued message at turn end,
  injections held while the row holds a draft or raw mode is dirty, unix-socket channel named by
  agent id with a restart request (handoff plus new child launch), pump with transcript acceptance and head retry, DeliverPrompt
  in `transcript.go` tries the channel first with the herdr path as
  fallback), `macro.go` (row macro dispatch: prompt templates expand into the
  queue like typed text, unknown slash text stays a message, `/compress` runs
  the compress request through `CompressRun` for any harness); settings carry strip rows, frame, rule,
  symbols and separator, macros, per-harness compress models, the auto-compress switch and cooldown. `exec`,
  top-level runs and `el wrap` put the child on a PTY under the strip;
  EL_HEADER_STRIP=0 runs plainly. Pig sessions with the tools debug log
  keep the strip: stdin and stdout stay on the PTY while stderr keeps its
  own pipe to the log, one stamped line at a time. Generated theme files
  live under the state dir (`themes/`); golden fixtures are `testdata/theme/`.
- Daemon callbacks: `tools/webproxy.go` and `tools/sudogate.go` send `EL_WEB_TOKEN`, the
  web Daemon's machine token (see `emberloop-web/AGENTS.md`), as a bearer on every call;
  `restart.go` reads the same token for `el restart`, and `update.go` for its health probe.
- Policy: `roles.go` (no role refuses to start; `work`, `plan`, `full`,
  `review` and `scout` serve `execute`, `implement` does not; the web
  Daemon's `web` parent, `web-worker` child and `scout` roles take their
  Daemon tools from `tools/webproxy.go`, never the parent's full set; `web`
  also takes `profile` (memory://profile read, patch proposals that wait for
  human approval, only on an explicit User request) and `remember` (Rule
  proposals that wait for human approval, needs a reason, rate-limited per
  Session; workers, scouts and Reviewers never propose either, and put Project
  knowledge in the nearest `AGENTS.md` instead), while every web role takes
  `recall` (the caller's bank plus Project notes); `WebSubmitEnabled` in
  `toolresolve.go` (off)
  keeps `submit` unoffered to web children while the herdr-era submit and Run
  submits stay; the `run-worker` role (a Run ticket Worker) always serves
  `submit` and `spawn`), `toolresolve.go`
  (one role resolution over settings for the main list and execute; the served
  `read` refuses an oversized file with `bash` for roles that serve it and
  `tools.grep` in `execute` or offset/limit otherwise), `readhint.go` (that
  refusal wrapper), `bashhint.go` (the served `bash` of a role that also
  serves `execute` appends a filter-in-`execute` tip to a stored oversized
  output),
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
- A emberloop MCP server with no role refuses to start; the full tool set needs
  an explicit full role.
- The tree stays gofmt-clean; formatting is a check result, never a
  judgement call.

## Work Guidance

- Build and test from this directory: `go build ./...`, `go vet ./...`,
  `go test -skip TestLive ./...`. Never run a live test and never start
  `el`, `pig`, or `claude` from inside the work.
- Behavioural change needs a test through the tool or command, not
  internals; expected values come from the spec or a worked example.

## Verification

`.emberloop/check` (`cd emberloop && go vet ./... && go test -skip TestLive ./... && cd ../emberloop-tui && go vet ./... && go test ./... && cd ../installer && go vet ./... && go test ./...`).

## Child DOX Index

- `emberloop/tools/AGENTS.md` — the agent tools both hosts serve
- `emberloop/mcp/AGENTS.md` — the MCP server transport over those tools
- `emberloop/agenttypes/AGENTS.md` — the built-in worker, reviewer, and
  scout types
- `emberloop/statedb/AGENTS.md` — the SQLite state database both hosts share
- `emberloop/testdata/AGENTS.md` — captured fixtures tests parse
