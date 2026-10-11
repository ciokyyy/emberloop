# AGENTS.md — el-web

## Purpose

Enderbloom Web: local Daemon (Go) plus embedded browser UI driving
claude, pig and grok Sessions behind one `Backend` interface. See
`.scratch/archive/enderbloom-web/spec.md`.

## Ownership

Owns `emberloop-web/`. May import el core packages through the
`replace example.com/emberloop => ../emberloop` in `go.mod`, so the module
builds only beside a sibling `emberloop/` checkout; core never imports it.
No herdr. Ticket files are preview input plus the driver's only write target:
the driver ticks boxes and marks done, nothing else writes them.

## Local Contracts

- UI design rules (shadcn-svelte primitives everywhere, ChatGPT + Apple look,
  smooth motion everywhere) are in `web/README.md` "Design rules": follow them
  for every UI change.

- `spike/` holds live-only probe programs, never imported by product code.
- Terms follow root `GLOSSARY.md`.
- Users and auth (`users.go`, `auth.go`): accounts live in `<stateDir>/enderbloom/users/users.json`
  (argon2id hashes; login tokens stored as SHA-256 with a 30-day expiry; `users.lock` serializes
  writers across processes). `/api/auth/signup|login|logout`, `/api/auth/me` and `/api/auth/state` answer without a
  cookie. Every other `/api` path, SSE included, answers 401 unless it carries the `el_session`
  cookie or `Authorization: Bearer` with the machine token. The machine token is new per start,
  written to `<stateDir>/enderbloom/daemon.token` (0600). The Daemon's agents get it as
  `EL_WEB_TOKEN` (claude, pig ACP and grok el-mcp servers; the pig SDK submit and sudo gate hold it
  in-process). It is not a User: `me` answers 401. The first signup is the admin. `signup_enabled`
  (settings, default on) is written by the admin only; off refuses signup. `SetUserPassword` resets
  a password and signs that User out; `cmd/emberloop-web` exposes it as `user set-password`.
- `GET /api/auth/state` gives the sign-in screen `{empty, signup_enabled}` and nothing else: no names, no counts. While signed out the sign-in UI (`web/src/Root.svelte`) is the whole app. An empty Daemon creates the admin account; otherwise it signs in, with a create-account link when signup is open. Signed in, the sidebar footer shows the User and Sign out, and the Accounts settings switch is admin only. A 401 from any other `/api` route returns to sign-in.
- Owners and senders (`owner.go`, `event.go`): a Session and a Run carry
  `owner`, the creating User's ID. The create routes (`POST /api/sessions`,
  `POST /api/runs`) need a User: the machine token answers 403. Subagents,
  Workers and Reviewers take their parent's or Run's owner, never the
  requester's. Sessions and Runs with no owner (data from before owners) are
  hidden: lists skip them, every route answers 404, and pushes stay silent.
  No migration. Any User may prompt, steer, interrupt, cancel and end any
  Session. A create with no `projectId` and no `workDir` opens in
  `<stateDir>/enderbloom/users/<name>/home`, made on first use; an explicit
  `workDir` still wins. Every prompt Event carries `sender`: `user` with the
  User's ID in `user` when a signed-in User sent it (prompt, steer and
  follow-up routes, Run ticket steer and retry, spawn requests), else
  `system` (machine token callers, the Daemon, grouped results, sudo results,
  the Handoff, gate reminders, Run driver prompts). Queued messages merge only
  within one sender, so no prompt Event joins two Users' texts.
  `owner_test.go` covers owners, inheritance, hiding, sender typing and the
  home directory at the same seam.
- Real Backends prove out in live probes, never the main check.
- Tests never call `t.TempDir()`: use `tempDir(t)` (`tempdir_test.go`). Daemon
  goroutines write after a test returns and fail `t.TempDir()` cleanup with
  "directory not empty". `main_test.go` shrinks the pig stall watchdog
  (`pigTailStallTimeout`, `pigStallTick`): the SDK sometimes leaves a finished
  stream open and the real 20s watchdog added ~30s per hit.
- API is the test seam: `api_test.go` drives the Daemon's HTTP and SSE API
  with the fake Backend (create, prompt, replay, guard, restart, UI);
  `lifecycle_test.go` covers the cap, idle release, restart resume and
  read-only paths the same way; `steer_test.go` covers steer, follow-up,
  queue list/clear and the claude Steer injection and its Follow-up
  fallback at the same seam; `spawn_test.go` covers spawn, nesting, grouped delivery and
  scout read-only at the same seam; `submitgate_test.go` covers the child submit gate
  with the flag on (submit untouched, reminded-then-submits, give-up notice,
  kill and interrupt skips) plus settle delivery with it off (last message,
  submit text, failed status) at the same seam; `steer_child_test.go` covers a parent
  steering its own child on both tool paths, non-child and scout refusals at
  the same seam; `runs_test.go` covers spec discovery and
  ticket preview at the same seam; `runs_driver_test.go` runs whole Runs
  with a fake Backend and fake git (order, cap, box ticks, dirty and
  conflict refusals, roles); `runs_reviewers_test.go` covers both Reviewer
  Sessions per Ticket, the gate, rework, staleness, roles and the cap at the
  same seam; `runs_persist_test.go` covers per-pass Run persistence, paused
  reload with nothing spawned, pause, resume without a second Worker, a failed Run (driver error) resuming from saved state with its Worker and error cleared (the UI shows Resume on paused and failed Runs; Restart Run starts from zero), and
  finished Runs staying listed, at the same seam; `RunsView.test.js` covers
  the live Ticket table with Worker links, the verdict columns, and the
  paused and failed states with their Resume button; `quota_test.go` covers
  `GET /api/quota` and `POST /api/quota/refresh` with local HTTP stubs for
  the opencode and Antigravity endpoints (cards, not-configured, stale
  fallback, forced refresh, Claude re-read, unknown-key 400); `pig_test.go`
  does the same with the pig Backend on the SDK test-faux provider, and
  `pig_live_test.go` (env-gated) with a real pig.
- `auth_test.go` covers signup, login, logout, `me`, `state`, the 401 gate on every `/api` route,
  the machine token, `signup_enabled` and `SetUserPassword` at the same seam;
  `cmd/emberloop-web/user_test.go` covers the `user set-password` command.
- `quota.go` serves the Quota tab data off el's shared usage cache:
  `GET /api/quota` (guarded refresh, then read) and
  `POST /api/quota/refresh` with optional `key` (forced past the age guard,
  Claude re-reads its file only). Same `{providers:[...]}` shape both ways;
  per-card `ok` / `not_configured` / `error` with stale windows kept.
- `pig.go` is the pig Backend: embedded SDK pinned in `go.mod`, stripped to
  ExtraTools plus the per-Session spawn tool (scouts: ExtraTools without
  spawn), models from the SDK catalog filtered to providers with usable auth
  (`CheckProviderAuth` over `auth.json`, env keys, OAuth; `?current=` keeps
  one unlisted model visible), one SDK stack per live Session
  released on idle or DELETE and reloaded through `Runtime.Resume`.
  Steer and Follow-up use the SDK native queues (`QueuedMessages`/
  `ClearQueued` expose them); the Daemon merges them with its own pending
  list. Backends register per harness; the fake stays default.
- `pigacp.go` is the default pig Backend (`EL_PIG_SDK=1` or a missing
  `pig`/`el-mcp`/`bunx` falls back to the SDK Backend): it runs the pi-acp
  adapter (`bunx pi-acp@0.0.34`, `npx -y` fallback) through the generalized
  `ClaudeBackend` ACP path, so pig Sessions behave like claude ones. Each
  Session gets its own pig agent dir under
  `<stateDir>/enderbloom/pig-acp/<id>/agent` (symlinks to the user's pig
  auth/models/sessions/extensions, never `herdr-report`, plus a generated
  `settings.json` with `compaction.enabled=false` and `quietStartup`, and an
  `mcp.json` (direct exposure): the single `el-mcp` server under the Session's
  role (`--role web`, `--role web-worker` for spawned group children,
  `--role scout` for scouts); the user's own MCP servers are not merged. A
  generated `bin/pi` shim (first on PATH, hides `--version`) re-executes the
  Daemon binary as `pig-shim` (`pigshim.go`), which runs `pig --no-builtin-tools`
  and owns a unix socket (`EL_PIG_STEER_SOCK`) that injects native `steer`,
  `follow_up` and `clear_queue` rpc commands and reports the queue, so
  `PigACPBackend` is a `steerBackend`. Compression is off for ACP pig: the
  Backend is neither a `compactBackend` nor a `handoffBackend`, the Session
  GET reports `compressible:false` and the UI hides the Compress button.
- `handoff.go` is Handoff compress for a `handoffBackend` (the fake Backend):
  `POST .../compress` answers 409 with the reason from `compressNote` while the
  compress model is unset or unauthed. Else `runHandoff` writes the body
  (`writeHandoff`, `summarize.go`), starts a fresh handle (`StartHandle`),
  rotates the resume token, stores `handoff-N.md` in the Session directory,
  appends a `compressed` Event (number, tokens before, model, fallback step,
  retries, error when the summarizer call failed) and the Handoff prompt Event
  (`handoff:true`, `sender:"system"`). It runs the Handoff as the first turn
  through `runTurn` and releases the old handle once that turn works (or ends).
  Each fold and fallback step of `writeHandoff` appends a `compress_step` Event
  (text: the reason) before the `compressed` Event.
  Session ID, Event log and metadata stay. A compress waits while a Group of the Session is outstanding (`groupOutstandingLocked`: a Group spawned in this process with results not all sent, or a member with a turn running or queued): it answers 202 `{"waiting":"group"}` and `runDeferredCompress` runs it at the next turn end or Group delivery. A claude Steer that arrives while a Handoff swap runs (`swapping`) degrades to a Follow-up, so the Handoff leads. Tests: `handoff_test.go`, `handoff_queue_test.go`.
- `summarize.go` is the Handoff summarizer: a one-off pig completion
  (`pigOneShot`) behind the `handoffSummarizer` seam (`pigSummarizer` in
  production, a scripted fake in tests). No tools, no Session. The system prompt
  is `el.CompressPrompt` plus `compress_instructions`; the user message is the
  folded transcript (`summaryBlocks`: prompts, assistant text, tool calls at last
  state, errors). `Window(model)` is the input budget of one call: the catalog
  context window less the reply cap, at 4 characters per token (128000 tokens
  when the catalog lists none). A call the provider refuses for its window
  (`ai.IsContextOverflow`) fails with `errSummaryOverflow`.
  Chain (`writeHandoff`): input over the budget is folded first
  (`summarize_fold.go`). A reply passing `el.CheckHandoff` is used (`summary`);
  empty or wrong-format replies get up to 2 repair retries that restate
  `el.HandoffSections()` (`repair`); then the last non-empty reply is wrapped
  under the headings (`lenient`); no text, or a call error, gives the
  deterministic Handoff (`deterministic`). A Handoff call the window refuses
  folds the input again at half its size, up to 2 times. `assembleHandoff`
  appends the last user prompt verbatim and `el.ContinueLine` when a tool call
  never finished (`openTask`). `dropMemoryBlocks` removes fenced memory blocks
  (an opening line starting with three backticks and `memory`, closed by a bare
  three-backtick line) from all text except the user's, in the summarizer input
  and output. Compress gate: `compress_model` (the spec's `compressModel`) must
  name an authed pig model (`pigModelAuthed`, the new-Session picker's test).
  In-place SDK compaction (`compactBackend`) is not gated and does not use the
  summarizer. Tests: `summarize_test.go`, `summarize_fold_test.go`.
- `summarize_fold.go` folds oversize Handoff input. Tool results are cut to
  head and tail at 2000 characters. Input over the budget is packed into runs;
  each run is summarized under a `PART i OF n:` label, and the run summaries
  are folded the same way until they fit. A run the summarizer refuses is
  halved between its Events while it is over 8000 characters; one Event does
  not split, so it goes to the floor at once. At the floor the tool results are
  cut to 500 characters of head and the run is sent once more. Still refused:
  `errNoFit`, and the deterministic Handoff.
- `grokacp.go` is the grok Backend: `grok agent --no-leader --agent-profile
  <file> stdio` on the user's Grok subscription login, through the plain
  `ClaudeBackend` ACP path (`metaHook`, `promptWrap`, `waitMCP`, `mcpEnv`).
  Registered only when `grok` and `el-mcp` resolve; an unregistered harness
  answers 503, never the fake. Each Session gets `<stateDir>/enderbloom/grok-acp/<id>/`
  with a temp `home/` and `grok/` (the `GROK_HOME`): a copy of the user's
  `auth.json` (written back on release when the agent refreshed it and the
  user's file did not change), a `config.toml` that turns off subagents,
  memory, media and web tools and every `[compat.claude]` scan, and a
  profile whose `tools` allowlist is `use_tool` only (`agent stdio` ignores
  the `--tools` flag; Grok has no setting to expose MCP tools directly).
  The el MCP tools therefore go through `use_tool`; the system prompt
  (`_meta.systemPromptOverride`: el rules, project context, then a compact
  catalog rendered from the el-mcp `tools/list` of the Session's role) lets
  the model call them without `search_tool`, which the profile keeps only
  when the catalog could not be built. The register reminder is appended to
  each prompt (ACP has no hook). Start and load wait for the agent's
  `_x.ai/mcp_initialized` notification. Steer is not native: the Daemon
  turns a busy Steer into a Follow-up. Effort is the `reasoning_effort`
  config option (`max` maps to `xhigh`). Grok Sessions get no titles and
  no compression. `grokacp_live_test.go` (`EL_LIVE_GROK=1`) drives a real
  agent.
- `restart.go` (`Shutdown`) and `cmd/emberloop-web/restart.go` restart the
  Daemon in place: every `-restart-check` (default 1m, 0 off) it compares its
  binary on disk with the one it started from and, after the change held for
  two ticks and `Daemon.Idle(0)` holds, closes live agent processes and `exec`s
  itself with the same args, keeping pid and address.
- `idle.go` holds the one idle definition. `session.Idle(now, quiet, open)` is
  per Session: no turn running, starting or queued, no resume or release in
  flight, no outstanding Group led (`groupOutstandingLocked`, the Compress wait
  rule), and at least `quiet` since its last Event. `Daemon.Idle(quiet)` is Daemon-wide:
  also no Session starting and no Run running. Quiet time and the runs driver
  read `d.clock` (test seam, nil reads `time.Now`). Restart passes 0: it waits
  for work it would cut, not for quiet time. Tests: `idle_test.go`, `restart_test.go`.
- `claude.go` holds the claude ACP Backend (pinned adapter, shared
  new/load stripping set, el system prompt replacing the Claude Code
  preset and the caveman ultra register (system prompt plus a per-turn
  `UserPromptSubmit` reminder hook via inline `settings`), `_session/steering`, allow-once auto-approve) with stub-transport
  tests in `claude_*_test.go` and a `-tags live` probe for the real adapter.
  Project context (`contextfiles.go`, table-tested in `contextfiles_test.go`)
  injects the Session WorkDir's AGENTS.md/CLAUDE.md chain (outermost-first,
  global pig agent-dir file first, 64 KB cap with truncation note) into the
  system prompt on create and resume, scouts included; `settingSources`
  stays empty.
- `titles.go` names root Sessions for the list: the first prompt starts one
  background, one-off title call (`Title` on a Backend: claude a throwaway
  ACP session with a string `systemPrompt`, no MCP, no tools; pig a bare
  `Provider.Stream` completion via `pigOneShot`), stored as `title` in `meta.json`.
  Claude Sessions title with haiku while `session_titles` (default on) allows it;
  pig Sessions title with the pig model of the `pig-title` role (harness
  must be pig) and stay untitled without one. Scouts and Subagents are
  skipped, failures leave the `harness · id` label, and
  `EL_SESSION_TITLES=0` turns all naming off (it also locks the claude
  switch in the settings page). `main_test.go` keeps it off for
  the package; the title tests switch it on.
- `GET /api/models?harness=` offers models per harness (claude: opus,
  sonnet, haiku; pig: only authed providers, plus `?current=`). Creating a
  claude Session with another model is 400; creating one without the claude
  Backend wired, or when the adapter fails to start, is 500 with no Session
  left behind.
- Lifecycle (`lifecycle.go`, `daemon.go`): the Daemon caps live Sessions at
  the global `max_workers` setting (env, settings file, default 3; over-cap
  creates and resumes are 429), releases a settled (idle or failed)
  Session's agent process, and resumes it on the next send: claude
  through ACP `session/load`,
  pig through its saved session file, fake from the log. One resume runs at
  a time and sends fence an in-flight release: a send in either window is
  409 and retryable, so a stale release never kills a resumed process.
  Sessions that cannot resume stay readable with a why
  (`resumable`/`resumeNote` on session GET, read-only banner in the UI);
  a new send there is 409. The settings endpoint (`settings.go`) reads and
  writes `max_workers` next to roles, reporting `max_workers_from_env` when
  `EL_MAX_WORKERS` wins; a write takes effect on the live cap with
  no restart, and Run Sessions share the one cap with chat Sessions. The
  settings page edits the field and locks it with a note under the override.
- Steering is `POST /api/sessions/{id}/steer|followup` with `{"text"}`.
  `POST .../prompt` accepts an optional `mode` (prompt, steer, followup).
  Live idle Sessions start a turn at once; mid-turn messages queue: pig
  steers and follow-ups in the SDK native queues, everything else in the
  Daemon pending list. A mid-turn claude Steer is injected through the adapter's
  `_session/steering`; when that is unavailable or no turn runs it degrades
  to a Follow-up. Messages that queue behind a turn in the Daemon pending list
  are delivered together as one blank-line separated message (one prompt
  Event; `followup` unless they all share a mode). Prompt Events carry the delivery in `mode`
  (empty for plain prompts, else steer or followup).
- `GET /api/sessions/{id}/queue` lists not-yet-delivered messages as
  `{"queue":[{"mode","text"}]}` (pig native queues first); `DELETE` on the
  same path clears them before delivery and answers `{"cleared":N}`.
  Steer, follow-up and prompt-on-working answers are 202 with
  `{"seq","queued"}` (queued Daemon messages add `"position"`); unknown
  Sessions are 404, empty text is 400, ended Sessions are 410 on send.
- Voice input: the browser records in live segments (fresh MediaRecorder per segment, closed on silence pause or timer), `stt.go` (`POST /api/stt`, raw audio body, `{"text"}` back) forwards each to OpenRouter `openai/gpt-transcribe` (`EL_STT_MODEL` overrides) with the key `systemone.APIKey` finds (`OPENROUTER_API_KEY`, else the one pig holds via `pig auth login`, else the openrouter `api_key` in `~/.pig/agent/auth.json` when pig is not on PATH); the key never reaches the browser. With `?format=1` each transcript goes through the cleanup model `openai/gpt-4o-mini` (`EL_STT_FORMAT_MODEL` overrides; fail open to raw text), which fixes punctuation/casing/filler with the transcript pinned as untrusted data. The Composer appends each part to the draft in order (pending dimmed, failed parts skipped, typed text kept). No key is 503, an upstream failure is 502 with its message, `GET /api/stt` reports `{enabled,model,formatModel}`. Audio is capped at 25 MB per segment. The mic needs a secure context, so a plain http tailnet IP has no mic: use `tailscale serve`.
- Interrupt: `POST /api/sessions/{id}/interrupt` drops every queued message, then asks an `interruptBackend` to stop the running turn (ACP `session/cancel` for claude and pig ACP, `Session.RequestAbort` for the SDK Backend). The turn ends idle and the Session stays usable; an idle Session answers `{"interrupted":false}`. The Composer shows a Stop button while the Session works.
- Image attachments: `POST /api/sessions/{id}/prompt` takes `images: [{mime,data}]` (base64; png, jpeg, gif, webp; 8 per message, 5 MB each; text may be empty). Only `images.go` `imageBackend` Backends take them (pig via `SendContent`, claude via ACP image blocks); others fail the turn with an error Event. The prompt Event stores `images` as a count, never the bytes. A pig message with images takes the native queue too (`imageSteerBackend`: the shim forwards pig RPC image blocks); other harnesses keep a mid-turn image message in the Daemon pending list as a Follow-up. `steer` and `followup` routes stay text-only.
- `sudo.go` gates sudo: the shared `bash` tool (claude through el-mcp, which gets `EL_WEB_PARENT` and `EL_WEB_DAEMON_URL`; pig through `PigBackend.SudoGate`) posts any command that uses sudo to `POST /api/sessions/{id}/sudo` instead of running it, and the model is told the request went to the user. The UI card (`sudo_request` Event, updated by `sudo_resolved`) offers Approve or Deny via `POST /api/sessions/{id}/sudo/{rid}` with an optional password. Approved commands run in the background (`sudo -n`, or `sudo -S` with the password, held in memory only); the output returns to the Session as a TOON `sudo_result:` Follow-up, shown as a result card. Requests live in memory: a Daemon restart expires pending ones.
- `read` returns images: `pigtools.go` turns the shared tool's images into `ai.ImageContent` (a model without image input gets pig's placeholder); claude gets MCP image blocks from el-mcp.
- `push.go` is Web Push (stdlib, RFC 8291 + VAPID): `GET /api/push/key`, `POST|DELETE /api/push/subscribe`; the VAPID key and subscriptions persist in `enderbloom/push/`. A root chat turn going working to idle or failed notifies every subscription; gone ones (404, 410) drop. Browsers allow push only in a secure context, so a plain http tailnet IP cannot subscribe: use `tailscale serve` (https) with `-allow-host`. `web/public/sw.js` shows the notification.
- Event log is `enderbloom/sessions/{id}/` under the state directory:
  `meta.json` (now carrying `owner`, the resume token plus `parentId`, `groupId`,
  `scout`) plus append-only `events.jsonl`; Groups persist as
  `enderbloom/groups/{id}.json`; the Daemon reloads all three on start.
- Subagents are `POST /api/sessions/{id}/spawn` with
  `{"agents":[{"prompt","type"}]}` (type worker or scout, default worker).
  Each child inherits the parent's harness, model and working directory,
  records its parent and Group, and runs its prompt at once; over-cap the
  whole call is 429 with no half Group. The session list nests children
  under `parentId` and each opens its own transcript. The last member to
  settle delivers one grouped result to the parent as a Steer (queued when
  the parent works); each member's line carries its submit text when one
  exists, else its last assistant message (whole final reply), with a
  `[failed]` mark on failed children. The parent transcript also keeps one
  `subagent_spawned` Event per child and one `subagent_finished` Event per
  Group. Children settle on their turn alone: the shared stop gate
  (`el.DecideGate` in `submitgate.go`) runs only while `el.WebSubmitEnabled`
  holds (off: no submit tool, no reminders, no give-up notice); interrupts,
  kills and ended Sessions skip it either way. Delivery is exactly-once per Group: a parent that cannot resume
  (cap, ended) still gets its `subagent_finished` marker, but the grouped
  Steer is dropped, and a Group mid-flight across a restart is never
  redelivered; member transcripts stay readable either way. Scouts are
  read-only: sends and nested spawns are 403, the UI hides the composer
  with a banner, pig scout loadouts carry none of spawn, steer, check or
  cancel, and claude scout Sessions run the single el server under `--role
  scout` (no Daemon tools at all).
- `spawn`, `steer`, `check` and `cancel` are Daemon-hosted
  (`spawn.go` for the routes, `emberloop/tools/webproxy.go` for the thin HTTP
  proxies the single `el-mcp` server serves; `submit` stays registered there
  but unoffered while `el.WebSubmitEnabled` is false): pig takes the four as
  per-Session ExtraTools bound to its own id, claude through the base server
  under the Session's role. Top-level parents run `--role web` (spawn, steer,
  check, cancel), spawned group children `--role web-worker`
  (spawn; nesting kept, but no steer, check or cancel), Run ticket Workers
  `--role run-worker` (`SessionMeta.RunWorker`; spawn and submit on every
  harness: a done submit starts the ticket's reviewers; SDK pig gets a submit
  ExtraTool via `PigBackend.DaemonURL`), scouts
  `--role scout` (read-only). A proxy registers only when its role allows
  it and `EL_WEB_PARENT`/`EL_WEB_DAEMON_URL` bind the server to the Session,
  and never reaches an execute script. `steer` (`SteerChild`: only the
  recorded parent, scouts refused) routes a working child as steer and an
  idle or released one as follow-up (resuming first) through the same resume
  plus routeLive path the UI sends use. `check` (`CheckChildren`/`CheckChild`:
  only the recorded parent, scouts readable) peeks without messaging: one
  summary line per child, or one child plus its last twenty Events; grouped
  results still arrive on their own, so check never polls. `cancel`
  (`CancelChild`: only the recorded parent, scouts stoppable) reuses the
  interrupt route, and kill ends through the same end DELETE serves.
  The submit route (`POST /api/sessions/{id}/submit`, kept for Runs and the
  flag-on gate) still records a result the grouped text prefers when one
  exists. The existing
  MCP server never calls back; all Group state stays in the Daemon.
- `runs.go` serves the Runs picker off core spec resolution and ticket
  reading (`GET /api/specs` lists specs with title and ticket count,
  `GET /api/specs/preview` returns number, title, blockers and checkbox
  progress per ticket with a row error for a bad Blocked by line). The scan
  stays four levels deep, matches only files named `spec.md`, and skips
  `.git` and `node_modules`. The UI mounts a Runs view beside chat and
  settings with no router: repo picker defaulting to the Session working
  directory, spec list, ticket table. No herdr import.
- `runs_exec.go` runs one deterministic Run per spec through the core driver
  (`POST /api/runs` starts with spec, repo, harness, model; a second active
  Start on the same spec is 409; `GET /api/runs` lists, `GET /api/runs/{id}`
  shows the live Ticket table, `GET /api/runs/{id}/events` streams it over
  SSE). Each Ticket starts in number order once blockers merged, up to the
  live-Session cap, in its own worktree and branch as a Subagent of the Run
  (ParentID the Run). A submitted done (`POST /api/sessions/{id}/submit`)
  merges by rebase plus fast-forward-only through the shared core checks; a
  non-done submit waits for done instead of merging; a dirty worktree or
  rebase conflict keeps the Worker alive with the refusal on its row and
  tells the Worker. With no target branch (detached repo) the rebase falls
  back to the repo HEAD as it stands at finish, never the recorded base.
  Start refuses a spec outside the repo. Boxes tick from the submit completed
  list through the per-file lock, the driver the only writer. Roles from
  settings choose harness, model and effort, falling back to the Start
  request. A submitted done also spawns one persistent Reviewer Session per
  required type (`reviewer-spec`, `reviewer-standards`), once, as a Subagent
  of the Run in the Worker's worktree; later dones re-prompt the same
  Sessions, so they keep context across Passes. Each prompt queues its tip as
  owed and each verdict answers the oldest queued tip, so a mid-review Worker
  commit reads stale, never fresh; a verdict already stale at birth re-prompts
  its reviewer once at the current tip unless that re-prompt is already out.
  The finish merges only when both types passed at the current tip; a fresh
  refusal lands on the row and reaches the Worker unchanged, retried after the
  next submit, while stale findings stay behind (at only, never the old
  report) and a Ticket with no verdict on the current tip stays under review
  with no new Tell. The done submit runs the ensure sync (bounded to two
  starts plus two steers) so prompts queue in submit order. Reviewer Sessions
  start through the shared live-Session cap like every other Session. The row
  carries `reviewerSessions`, `verdicts` (verdict, at, fresh) and `tip` with
  the last refusal. Runs persist as `enderbloom/runs/{id}.json` with one row
  per Ticket (state, tip, last refusal, times) written after every driver
  pass, and reload paused with nothing spawned. `POST /api/runs/{id}/pause`
  freezes the driver (no new Workers or Reviewers, live Sessions untouched)
  and `POST /api/runs/{id}/resume` continues it; started Tickets keep their
  Worker Sessions and merged ones seed from their shas, so a Resume never
  starts a second Worker. The UI shows the live table with Worker links to
  the Session view plus Pause and Resume buttons on running and paused Runs.
- The UI is a static build: `web/src/` is the Svelte source, `web/dist/`
  is the embedded build served at `/` with its bundle assets and correct
  content types. Styling is Tailwind with shadcn-svelte-idiom components
  in `web/src/lib/ui/`; component logic lives in `web/src/lib/*.js`.

## Verification

In `emberloop-web/`: `go vet ./... && go test ./...`. Wired into the repo check.
In `emberloop-web/web/`: `npm test` (vitest) and `npm run build`. The JS
suite is not in the repo check; the check covers Go only.
The live probe is env-gated, never in the check: `EL_LIVE_PIG=1` with
`PIG_LIVE_MODEL` set to a healthy provider/model runs `TestLivePigBackend`.
The pig ACP live probe is `EL_LIVE_PIG_ACP=1` with `PIG_ACP_SPAWN_BIN` set
to a built Daemon binary (`TestLivePigACP`).

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary

## E2E suite

`web/e2e/` holds the browser suite on [tester-army/e2e](https://github.com/tester-army/e2e):
real Chromium against a throwaway Daemon (`serve.sh`: fake backend, fresh state
dir, fixture repos in `/tmp/el-e2e`). No model, no API key. Run it with
`npm run test:e2e` from `web/` (it builds the UI first). It sits on top of the
Vitest component tests and `go test`; it is not part of the main check.

The sign-in suite starts from the empty state of `serve.sh`. `tests/auth.setup.e2e.ts` signs up the admin and saves the `admin` session; a test that needs a User declares `{ session: 'admin' }`. `support/auth.ts` holds the throwaway admin and the sign-in helpers.
