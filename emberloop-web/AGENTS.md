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
- Deciders (`decider.go`): `Decider` answers `durable`, `kind`, `scope_hint`, `contradicts` and `relevant` (at most 6 per call, choice options 2 to 8) with probabilities and never writes; `HeuristicDecider` is the miner pass as pass/fail (durable carries its confidence), `LLMDecider` asks a model through a caller-supplied one-off seam with redacted, size-capped state and strict JSON parsing, `EnsembleDecider` proposes with the first and confirms with the second against `confirmLow`/`confirmHigh` (defaults 0.35/0.7; drop, observe, eligible), `UnavailableDecider` never answers (the unconfigured ensemble second), and `ScriptedDecider` is the test fake. `JevDecider` (`jev.go`) POSTs the shared `model`/`state`/`questions` body to the configured endpoint URL verbatim (hosted `/api/v1/decisions`, TypeSafe `/v1/systemone`, or local laya-serve `/v1/systemone`), so Laya plugs in by URL with no adapter: unknown reply fields are ignored. Noul, choice and score answers parse into the result (score normalises to `score/(levels-1)`); limits are enforced before anything is sent, the state is redacted with `ForJev` and size-capped in the same function as the send, an empty URL never sends, and every transport, status or shape failure is `ErrDeciderUnavailable`. Laya verified against the upstream README and laya 0.4.2 `serve.py`: `/v1/systemone` only, schema-identical contracted keys, extras ignored, `model` honours checkpoint names (`jev-*` routes), `confidence` is not Jev's formula so callers gate on probability. Invalid or out-of-range output is `ErrDeciderUnavailable`, never a yes; malformed JSON fails the call while one bad answer silences only itself.
- Eval (`eval.go`, ticket 30): pure offline metrics. `CoversRule` is the dedup `Relation` as coverage (contradictions count); `RecallAt5`/`RepeatRate` share covered-over-total (empty is 0); `TopCovered` caps at 5; `HeuristicScore` is the live confidence or 0. Labels are JSONL (`ParseDeciderLabels`: id and text required, kind validated, duplicates rejected); decided queue rows map via `QueueItemLabels` (approved durable, rejected not, pending unjudged and skipped, textless skipped). `ParseDeciderScores` validates 0-to-1 probabilities; `ScoreDecider`/`ScoreEvalDeciders` report precision and recall at the approve threshold for `heuristic` (live), `llm`, `jev`, `laya` (missing unscored). `ScoreEvalCases` reports FTS recall, embedder recall and the gap on the same hit lists. Cases live in `eval_test.go`.
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
In `emberloop-web/`: `go vet ./memory/ && go test ./memory/`. The repo check runs `go test ./...` there. Trigger, FTS integrity (`integrity-check`), purge file-bytes and concurrency tests live in `bank_test.go`, `purge_test.go` and `snapshot_test.go`; grounding, redaction goldens and the egress-guard scan in `ground_test.go`, `redact_test.go` and `egress_test.go`. Heuristic goldens, cursor reset/torn-line and dedup relations with the bump live in `miner_test.go`, `cursor_test.go` and `dedup_test.go`. Decider validation, heuristic answers, LLM parsing with redaction, ensemble thresholds and the scripted fake live in `decider_test.go`. Jev answer parsing (noul, choice, score), error mapping, client-side limits, redaction and caps, and Laya-shape compat live in `jev_test.go`. Eval coverage, labels, scores and precision live in `eval_test.go`. Anchor extraction tests live in `anchors_test.go`; temp-git-repo verification (clean, changed with flag, deleted to dormant, foreign branch, missing commit, preferences, bare files, directory semantics, precise modules, outside/cancelled stale, `ApplyVerification`) lives in `verify_test.go` and `verify_review_test.go`. Project-notes branch-awareness across two worktrees, deleted-anchor skip, malformed-file notices, rank-fusion merge and frontmatter forms live in `notes_test.go`.
  follow-up routes, Run ticket steer and retry, spawn requests), else
  `system` (machine token callers, the Daemon, grouped results, sudo results,
  the Handoff, gate reminders, Run driver prompts). Every injection appends
  an `injection` Event (`injection.go`: Profile version and Rule IDs with
  versions, `sender=system`, never a user message), rendered as a Memory
  injected divider (`events.js`, covered in `events.test.js`). The miner reads
  Sessions only through `FilterMinerEvents` (`sender=user` prompts via
  `MinerReadable`), so injections, Handoffs and Daemon-made prompts are never
  mined; a scan in `injection_test.go` fails a miner that reads the log raw.
  Queued messages merge only
  within one sender, so no prompt Event joins two Users' texts.
  `owner_test.go` covers owners, inheritance, hiding, sender typing and the
  home directory at the same seam; `injection_test.go` covers the injection
  Event on every injection and the miner filter.
- Users list and presence (`handleUsers` in `auth.go`, `presence` in `live.go`): `GET /api/users` lists every account in creation order as `{id, username, admin, online}` for a signed-in User; the machine token answers 401. `online` holds while the User has an open `GET /api/live` channel: the first join and the last leave publish the `users` topic. `users_list_test.go` covers the list, presence and the gate at the same seam.
- Miner (`miner.go`): `MineSession` mines one ended or settle-window-idle (`MinerSettleWindow`, 10 minutes, through `session.Idle`) Session's new `events.jsonl` bytes into observations. It reads through a per-Session cursor file (`miner.cursor.json`: offset, size, first-4 KB hash; head change or shrink resets, torn last line waits) only up to the last newline, classifies only `FilterMinerEvents` prompts (`MinerReadable`) with the memory heuristic, stores each hit with its quote, Event ID, subject key and preceding tool call or diff hunk after redaction and grounding, and routes to the Event sender's bank (a steer from B in A's Session lands in B's). The key comes from the confirming Decider when it proposes one (`minerSubjectKey`), else the heuristic fallback, always keyed from the redacted quote so secrets never reach the key, and recorded with its source in `decider_json`. Duplicates bump `evidence_count`; refines and contradicts insert with the relation and related row in `decider_json`. After the scan every touched key runs the recurrence check (`promoteSubject`, counted in `MineResult.Promoted`); replays re-touch their stored key so a held cursor retries the promotion. A transient bank error aborts without advancing the cursor, so the bytes mine again. No Daemon caller yet: the nightly job (ticket 29) drives `MineSettledSessions`; until then mining runs on demand. `miner_test.go` drives it with canned `events.jsonl` files, including auto-promotion and the scripted key.
- Deciders (`decider.go`, `memory/decider.go`): the heuristic pass proposes every mined candidate and a confirming Decider judges it (`decideMine`): `heuristic` (default, no network, keeps every hit), `llm` (a one-off pig call with no tools and JSON-schema output through the same `pigOneShot` the summarizer uses, input redacted with `ForDecider` in the same function; without a model it stores unconfirmed, never eligible), `jev` (the endpoint client in `memory/jev.go` through `jevMineDecider`, nil while the endpoint URL is empty; off it stores unconfirmed, never eligible), `ensemble` (heuristic proposes, jev confirms when its endpoint is on else llm, `UnavailableDecider` when neither is set). A confirm keeps at `confirmLow` and is queue-eligible at `confirmHigh` in every confirming mode; `confirmed` in `decider_json` tells a judged verdict from the heuristic fallback. The same Decider proposes the subject key (`minerSubjectKey` through `SubjectKeyProposer`: test seam, llm, else heuristic, with jev falling through for no key support; redacted, normalized, empty never promotes). Every output lands in `decider_json` with model and version; invalid or out-of-range output is unavailable, never a yes. No confirmer answering means no eligibility: an error or a missing answer leaves the heuristic keep standing, but never queue-eligible; only a confirmed answer or the heuristic mode offers eligibility. The miner checks a hit Event with the exact `ObservationByEvent` lookup (Session ID and Event ID) before confirming, so a replayed Event never costs an LLM call; a dropped hit still advances the cursor, so a confirm-no never mines again. Settings are host-global with admin-only writes (`decider_mode`, `memory_model`, `decider_confirm_low/high`, `jev_base_url/api_key/model`, defaults heuristic, empty, 0.35, 0.7, empty, empty, `jev-latest`; the key never reaches the view, only `api_key_set`); reads ride the shared settings view like the compress knobs and the Settings page shows the section to the admin only (`SettingsPage.svelte`, `SettingsPage.test.js`). `decider_note` names a missing model, the jev off state and egress, and crossed thresholds. The Daemon seams are `decider` (a scripted Decider) and `deciderOneShot` (a scripted completion). Contradiction pairing (ticket 22) and recall-time relevance (ticket 24) ask their questions where they run. `decider_test.go` covers settings (decider and jev), scripted verdicts, ensemble thresholds, replay-before-confirm, failed-confirm and no-model ineligibility, jev confirm/off/error/ensemble-preference/redaction at the Daemon seam; `jev_live_test.go` (`EL_LIVE_JEV=1`) calls the real endpoint.
- `handle_test.go` covers `startHandle` at the Daemon seam on the fake Backend: owner, role and injections across create, resume and compress; the Profile, recall, Handoff order and the no-op before memory; the tool set and parent links across a compress; and the pig loadout per role.
- `invariants_test.go` pins the web-v1 Design rules at the Daemon seam on the fake Backend: no DELETE outside purge, grounded Rules, private banks, recall-only roles before and after compress, identical owner/toolset/bank across compress, system-only injections with the miner filter, pasted and tool-output remember creating nothing, and the redactor on every egress.
- Owner harness dirs (`harnessdirs.go`): `Daemon.ownerDirsFor(owner)` gives
  `<stateDir>/enderbloom/users/<userId>/config/{claude,pig}` on every harness
  start (create, resume, Handoff, spawn, Run, title) and seeds them from the
  host (`hostClaudeDir`, `hostPigDir`). The claude dir links only the host
  `.credentials.json`. The pig dir links only `pigSharedEntries` (auth, models, models-store,
  sessions, extensions, bin, npm, git, trust, keybindings) and a `settings.json`
  without `skills` and `prompts`, so no host instruction file, skill or prompt
  reaches an owner. `SessionMeta.Dirs` carries the dirs to each Backend, never
  persisted; the fake records each start's owner and env on its `handleStart`
  (`Starts(sessionID)`, `Kind`/`Owner`/`Env`). Owner
  IDs must be plain names: no path separators, `.` or `..`. Known gaps: the
  WorkDir chain still loads ancestor AGENTS.md/CLAUDE.md for every owner; claude
  Sessions created before this change keep their transcripts in the host dir and
  do not resume (no migration). `harnessdirs_test.go` covers the seam and the
  seeding rules.
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
  ExtraTools plus the Daemon tools its role serves (`sessionTools`: web gets
  spawn, steer, check, cancel, profile and remember; web-worker spawn;
  run-worker spawn and submit; every role with a wired backend gets recall,
  scouts included), models from the SDK catalog filtered to providers
  with usable auth
  (`CheckProviderAuth` over `auth.json`, env keys, OAuth; `?current=` keeps
  one unlisted model visible), one SDK stack per live Session
  released on idle or DELETE and reloaded through `Runtime.Resume` (`buildHandle`
  builds one stack for create and Handoff alike).
  Steer and Follow-up use the SDK native queues and report whether they queued
  (`QueuedMessages`/`ClearQueued` expose them; false falls back to the Daemon
  pending list); the Daemon merges them with its own pending
  list. Compress is Handoff restart (`StartHandle`): a fresh SDK Session with
  the record's model and thinking level, native queues carried across as
  follow-ups (held steers degrade), old
  handle released after the Handoff turn starts; the in-place compact path is
  gone. Backends register per harness; the fake stays default.
- Pig SDK Sessions read the owner's pig dir through `ownerResources`: no ambient
  skills or prompt templates (pig reads `~/.agents/skills` from HOME). Context
  files from the WorkDir chain still load. Agent Sessions append the role's
  memory/DOX extra (`ownerResourcesExtra` with `rolePromptFor`, prompts.go) to
  the system prompt; `pigOneShot` builds Services only, so
  the summarizer and pig titles load no instruction files.
- `pigacp.go` is the default pig Backend (`EL_PIG_SDK=1` or a missing
  `pig`/`el-mcp`/`bunx` falls back to the SDK Backend): it runs the pi-acp
  adapter (`bunx pi-acp@0.0.34`, `npx -y` fallback) through the generalized
  `ClaudeBackend` ACP path, so pig Sessions behave like claude ones. Each
  Session gets its own pig agent dir under
  `<stateDir>/enderbloom/pig-acp/<id>/agent` (symlinks to the owner's pig dir,
  `harnessdirs.go`; never `herdr-report`; plus a generated
  `settings.json` with `compaction.enabled=false` and `quietStartup`, and an
  `mcp.json` (direct exposure): the single `el-mcp` server under the Session's
  role (`--role web`, `--role web-worker` for spawned group children,
  `--role scout` for scouts); the user's own MCP servers are not merged. A
  generated `bin/pi` shim (first on PATH, hides `--version`) re-executes the
  Daemon binary as `pig-shim` (`pigshim.go`), which runs `pig --no-builtin-tools --no-skills --no-prompt-templates`
  and owns a per-handle unix socket (`EL_PIG_STEER_SOCK`, unique per handle so a
  swap never shares one) that injects native `steer`,
  `follow_up` and `clear_queue` rpc commands and reports the queue, so
  `PigACPBackend` is a `steerBackend` (false falls back to the Daemon pending
  list) and a `handoffBackend`: compress swaps by
  Handoff (`StartHandle`, new ACP session with MCP ready before the Handoff,
  record model and thinking level re-applied, native queues carried as
  follow-ups with held steers degraded, token rotates), the Session GET
  reports `compressible:true` and the UI shows Compress.
- `handle.go` derives the identity of every harness handle start:
  `startHandle(meta, op)` returns the owner, the el role (`elRoleFor`: web,
  web-worker, run-worker or scout, which sets the tool set) and the memory
  bundle for one start (`handleCreate`, `handleResume`, `handleCompress`).
  Create (`startSession`), resume (`resumeReleased`) and compress (`runHandoff`)
  all call it; the fake Backend records each start it gets. Backends hold no
  identity maps: they read the role from the record they are handed, through
  `elRoleFor`. `Daemon.profileBlockFor` (profile.go) leads the block with the
  owner's Profile (Reviewers the filtered one) ahead of recall
  (`recall_inject.go`), and records its version for the injection Event. Its
  texts go in front of the next turn's prompt (`takeInjection` in
  `runTurnWith`), once per start, in order: Profile, recall, then the
  Handoff. The Event log keeps the prompt alone. Texts, Profile version and
  Rule refs travel as one `injection` value so the injection Event
  (`injection.go`) always describes exactly what the model saw. `session.injectOp`
  marks which start set the bundle (create builds recall, resume carries the
  Profile only, compress already carries recall); `memoryInjection` stays empty
  in prod for tests to simulate texts.
- Profile (`profile.go`, `memory/profile.go`): per-User lines with stable IDs
  in `profile_versions`; ops add, edit, retire by line ID, every version kept,
  retire hides; restore copies an old version as new (the version-safe revert).
  `GET/PUT /api/profile`, `GET /api/profile/versions`,
  `POST /api/profile/restore`, `GET /api/profile/queue` and
  `POST /api/profile/queue/{id}/decide` serve the settings view (approve with
  an optional edit, reject keeps the label); PUT applies directly because the
  signed-in User is the approver, while agent proposals always queue. Each
  approval appends a version and a `profile_updated` Event to every Session of
  the owner. `GET` and `POST /api/sessions/{id}/profile[/propose]` serve the
  agent `profile` tool (memory://profile read, patch proposals that wait for
  approval with a reason; only top-level web Sessions propose; workers,
  scouts, Reviewers and Run workers are refused). The gate is the Session's
  latest sender=user message naming the Profile: a keyword match, not a
  semantic judgement, so an older passing mention does not open it but
  unworded intent is refused. The pig `profile` ExtraTool (`pig.go`) and the
  el-mcp `profile` proxy (`emberloop/tools/webproxy.go`, `web` role only)
  share that gate and return the Daemon's refusal text. Tags are workflow,
  style or other; workflow and style go to Reviewers (`SessionMeta.reviewer`,
  set on Run Reviewer start, with a ParentID-shape fallback for old Sessions
  locked by TestIsReviewerSessionShapes), never barred lines (heuristic
  expertise and review-bar phrases stay `other`). Over-2 KB renders are
  refused with a condense message. `profile_test.go` covers cap, versions,
  retire, filter, queue, gate recency and Events at the Daemon seam;
  `ProfileSettings.svelte` with `ProfileSettings.test.js` covers the settings
  view.
- Remember and recall (`remember.go`): agent memory tools plus the structural
  `/remember` action. `POST /api/sessions/{id}/remember` (`text`, `reason`,
  optional `quote` defaulting to the text) grounds the quote in a sender=user
  prompt of the Session through the shared `memory.CheckGrounding` and enqueues
  a Rule candidate in the bank of the grounded Event's sender (usually the
  owner; a quote from User B's steer inside A's Session lands in B's bank, so
  a steer never lands in another User's bank); it never writes a Rule, needs a
  reason, and past the per-Session limit (default 5, `EL_REMEMBER_LIMIT`
  overrides) answers 429. The limit check and the enqueue hold `rememberMu`,
  so concurrent calls cannot exceed the limit together. Only top-level web
  Sessions propose; workers, scouts, Reviewers and Run workers get 403.
  `GET /api/sessions/{id}/recall?query=` searches the owner's bank (live Rules
  plus observations) and the worktree's Project notes fused by reciprocal rank
  fusion; every role may search and no other User's bank is ever read. A
  `/remember <text>` first line of a typed sender=user message on any of the
  three send paths (prompt, steer and follow-up all route through `sendMessage`)
  validates before logging, then stores the message as its grounding Event and
  creates a Rule directly in the sender's bank; leftover lines still route as
  the turn, while pasted `remember:` text and system messages route normally
  and create nothing. The pig `remember`/`recall` ExtraTools (`pig.go`) and the
  el-mcp proxies (`emberloop/tools/webproxy.go`: `remember` on `web` only,
  `recall` on every web role) share those gates. `remember_test.go` covers queue
  payloads, limits (including concurrency), roles, cross-User banks, the command
  on every path with its invalid and remainder cases, and recall isolation at
  the Daemon seam; `Composer.test.js` covers the button.
- Rule queue approval (`rules.go`, `memory/queue.go`): pending Rule candidates
  with one proposed generalization and its quotes. `GET /api/rules/queue`
  lists, `POST /api/rules/queue/{id}/decide` approves (optional edit to text,
  kind, subject key) or rejects, `POST /api/rules/queue/batch` decides many by
  IDs or by Session, `POST /api/rules/queue/promote` enqueues a recurring
  subject (the miner also promotes every touched key after each scan),
  `GET /api/rules/queue/stats` reports budget and latency. Approve runs one
  atomic bank transaction (`ApproveRule`: create, supersede, decide together;
  refused approvals change nothing), keeps the proposal in the queue row and
  refuses over-300 text; reject keeps the label. Only kind=rule IDs decide
  here. Supersession compares observer identity (source observations' single
  distinct author, approver fallback) and date: same key and observer
  supersedes only when the candidate is at least as new, else the new Rule
  lands stillborn; a different observer disagrees with both Rules live and the
  promotion names the conflict. Every mutation writes a `queue_*` ledger row
  with the counts, budget and latency. A single mention never promotes; the
  same key in two Sessions does unless situational. `rules_test.go` covers
  each path on the fake clock at the Daemon seam.
- Auto-recall injection (`recall_inject.go`): at create and compress the Daemon
  builds Profile plus recall from the task prompt (the Handoff text for
  compress) through `recallBlockFor`: bank `Recall` (20 candidates, `Redact`ed
  query) plus `ApplyVerification` (dormant marked fail-closed, parked and
  over-300 skipped), Project notes via `RecallNotes`, fused by `FuseRecall`
  (k=60), an optional `relevant` Decider (`recallDecider`, 1500 ms budget,
  FTS5 order on timeout or error, `ForDecider` redaction), cut to 5 hits of at
  most 300 chars (`truncateRunes` for notes) and fenced as untrusted notes
  with `FlagNote` verify flags, all under `recallTimeout` (10 s). `Query`
  gives identifiers, paths and rare terms; empty query injects the Profile
  only. `recallOrNotice` is the single error-to-notice home for the four
  sites. Create builds synchronously in `routeLiveOpt` (404 when removed,
  410 when ended mid-recall) and `spawnFrom` (missing children skipped) and
  compress in `runHandoff`; `runTurnWith` keeps the fallback for Run Workers,
  pinned by `TestAutoRecallRunWorkerFallback`. Resume carries the Profile
  only. Engine failure posts a `notice` Event and the turn still runs.
  `recall_inject_test.go` covers order, fence, Events, empty query, caps,
  flags, dormancy, notes merge, identifiers, Decider fallback, resume,
  Reviewer filter, engine notice, compress and the fallback.
- Role prompts (`prompts.go`): the memory/DOX extra per role, the one place
  the wording lives. Web names the `remember` tool (User statements only,
  needs a reason, queue approval, project facts go in AGENTS.md) and the
  explicit-Profile rule; workers, scouts and Reviewers name no memory-write
  tool and get `recall` only. Every role carries the AGENTS.md chain rule and
  the untrusted-notes rule. `rolePromptFor` dispatches on the Session record
  (Reviewers first, though their el role is web). claude (`sessionMetaFor`,
  `claudeSystemPromptFor`), grok (`meta` hook with extra) and pig
  (`ownerResourcesExtra`) inject it into the system prompt on every create,
  resume and Handoff, exactly once per role: Run tasks (`workerTaskPrompt`,
  `reviewerPrompt`) carry no memory text of their own, only the ticket job
  plus (seeded worktrees) one line telling the Worker to commit the seeded
  root. `prompts_test.go` pins the exact texts and asserts no `remember` in
  worker, scout, reviewer or task prompts; `prompts_wiring_test.go` covers
  the per-harness injection and the task non-duplication. Title and
  summarizer calls use the role-free shape.
- Projects (`projects.go`): `POST /api/projects` records a working directory;
  it writes no files. `ensureAgentsMD` (template `agentsMDTemplate`, existing
  files untouched) seeds a root `AGENTS.md` for Run-launched worktrees only:
  `runStartWorker` (runs_exec.go, via `ensureWorktreeDOX`) calls it on the
  fresh worktree before the Worker starts, so the Worker reads a DOX chain
  even when the repo root lacks one; the main checkout stays clean and the
  task tells a seeded Worker to commit the seed (left untracked it would trip
  the dirty-worktree refusal). Seeding through `el init` belongs to
  `.scratch/project-init/issues/02`, which owns that command. `dox_test.go`
  covers the helper, the worktree seeding and the seeding failure;
  `dox_e2e_test.go` merges a seeded Worker end to end in a real rootless repo.
- `handoff.go` is Handoff compress for a `handoffBackend` (fake, pig SDK,
  pig-over-ACP and claude Backends; grok reuses the same swap):
  `POST .../compress` answers 409 with the reason from `compressNote` while the
  compress model is unset or unauthed. Else `runHandoff` writes the body
  (`writeHandoff`, `summarize.go`) through a `retryingSummarizer`
  (`compress_retry.go`), starts a fresh handle (`StartHandle`, retried the same
  way), rotates the resume token, stores `handoff-N.md` in the Session directory,
  appends a `compressed` Event (number, tokens before, model, fallback step,
  repair `retries`, `transientRetries`, `startRetries`, error when the summarizer
  call failed) and the Handoff prompt Event (`handoff:true`,
  `sender:"system"`). It runs the Handoff as the first turn through `runTurn` and
  releases the old handle once that turn works (or ends). Each fold and fallback
  step of `writeHandoff` appends a `compress_step` Event (text: the reason) before
  the `compressed` Event. Session ID, Event log and metadata stay. A compress
  waits while a Group of the Session is outstanding (`groupOutstandingLocked`: a
  Group spawned in this process with results not all sent, or a member with a
  turn running or queued): it answers 202 `{"waiting":"group"}` and
  `runDeferredCompress` runs it at the next turn end or Group delivery. A claude
  Steer that arrives while a Handoff swap runs (`swapping`) degrades to a
  Follow-up, so the Handoff leads. Tests: `handoff_test.go`,
  `handoff_queue_test.go`, `pig_handoff_test.go`, `claude_handoff_test.go` (claude swap, token rotation, restart resume, injections, failure), `grok_handoff_test.go` (effort, login sync, failure).
- `compress_retry.go` owns the compress failure policy:
  - `classifyCompressError` sorts a failure by its text: block (401 or 402
    status, payment required, no or insufficient credits, quota, billing limit,
    unauthorized, authentication, API key), transient (429 or 5xx status, rate
    limit, overloaded, timed out, connection error, fetch failed, unexpected
    EOF, unavailable) or other. A status counts only where the SDK puts one: at
    the start, after a colon or an open paren, or after HTTP or status.
  - Block wins over transient. `retryTransient` retries a transient failure
    after 2s, 8s and 30s (`compressBackoff`; sleep seam `Daemon.compressSleep`).
    `retryingSummarizer` runs every summarizer call under that policy and counts
    the retries.
  - A one-off that hits its deadline (`oneShotDeadline`) is other: no retry.
  - A block sets `SessionMeta.CompressBlock` (persisted), drops a compress that
    waits on a Group, appends a `compress_blocked` Event (number, model, retry
    counts, error, `action:"retry"`) and leaves the old handle and resume token
    alone.
  - While blocked: `routeLiveOpt` queues every message in the Daemon pending
    list, `finishTurn` neither drains nor releases, `maybeAutoCompress` and
    `runDeferredCompress` start nothing, and `requestCompress` answers 409 for a
    compress that would wait on a Group. Only an explicit compress (Retry)
    starts one.
  - A Retry that succeeds clears the block before the Handoff turn, so held
    messages follow the Handoff.
  - `POST /api/sessions/{id}/compress/dismiss` (`dismissCompress`) resumes a
    released Session, claims it, clears the block and drains the held messages
    on the old handle. It answers 409 while the Session works or no block is
    set, and 404 for an unknown Session.
  - Tests: `compress_retry_test.go`.
- `compress.go` owns the compress settings and the auto trigger: `auto_compress`, `compress_threshold_percent` (default 80), `compress_cooldown` (default 5 turns), `compress_instructions` and `compress_model` in the settings file, read fresh on every turn. `runTurnWith` calls `maybeAutoCompress` after the turn settles (Handoff turns never auto); it starts a Handoff through the shared `requestCompress` path when the newest usage Event sits at or past the threshold percent. It fires only at a tool boundary (`openTask` false) or when the one `session.Idle` definition holds (quiet 0, so a settled turn needs no quiet wait), never mid-token; a turn, queued input or resume/release in flight skips to the next turn end, and an outstanding Group defers through the same `compressWait`/`runDeferredCompress` wait as manual. Cooldown counts non-Handoff prompts since the last `compressed` Event (`promptsSinceCompress`/`cooldownPassed`); the first compress is never cooled. Blocked, scout and ended Sessions never auto; an unset or unauthed model shows the existing `compressNote`/Settings warning and runs nothing. Tests: `auto_compress_test.go` (threshold, turn-end and tool-boundary timing, cooldown, unset model, blocked).
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
  under the headings (`lenient`); no text, or an other call error, gives the
  deterministic Handoff (`deterministic`); a transient call error retries first,
  and a credit or auth error blocks (`compress_retry.go`). A Handoff call the
  window refuses folds the input again at half its size, up to 2 times.
  `assembleHandoff` appends the last prompt the Handoff carries, verbatim, and
  `el.ContinueLine` when a tool call never finished (`openTask`). The prompt set
  is `handoffPrompt`, shared by the summary tail and the deterministic Handoff
  (`buildHandoff`): every prompt Event but a Handoff, whatever its sender (a
  Worker or Subagent task is a system prompt). A prompt left empty by
  `dropMemoryBlocks` is skipped. Summarizer input labels system prompts `SYSTEM:`.
  `dropMemoryBlocks` removes fenced memory blocks (an opening line starting with
  three backticks and `memory`, closed by a bare three-backtick line) from all
  text except the user's, in the summarizer input and output. Compress gate:
  `compress_model` (the spec's `compressModel`) must name an authed pig model
  (`pigModelAuthed`, the new-Session picker's test). Every compress runs the
  Handoff path and uses the summarizer. Tests:
  `summarize_test.go`, `summarize_fold_test.go`.
- `summarize_fold.go` folds oversize Handoff input. Tool results are cut to
  head and tail at 2000 characters. Input over the budget is packed into runs;
  each run is summarized under a `PART i OF n:` label, and the run summaries
  are folded the same way until they fit. A run the summarizer refuses is
  halved between its Events while it is over 8000 characters; one Event does
  not split, so it goes to the floor at once. At the floor the tool results are
  cut to 500 characters of head and the run is sent once more. Still refused:
  `errNoFit`, and the deterministic Handoff.
- Grok Sessions (`grokacp.go`) load no user-level files by themselves: each
  Session has its own HOME and GROK_HOME, `config.toml` turns memory and
  `[compat.claude]` scans off, and the only host file copied in is `auth.json`.
  The Daemon's global context file for a grok Session is the owner's pig dir.
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
  (`_meta.systemPromptOverride`: el rules, the role's memory/DOX extra
  (prompts.go, via the `meta` hook), project context, then a compact
  catalog rendered from the el-mcp `tools/list` of the Session's role) lets
  the model call them without `search_tool`, which the profile keeps only
  when the catalog could not be built. The register reminder is appended to
  each prompt (ACP has no hook). Start and load wait for the agent's
  `_x.ai/mcp_initialized` notification. Steer is not native: the Daemon
  turns a busy Steer into a Follow-up. Effort is the `reasoning_effort`
  config option (`max` maps to `xhigh`). Grok Sessions get no titles; compress is
  a Handoff swap like claude with the reasoning effort re-applied. The old agent's login
  syncs before the new start and on the old handle's release, so a rotated refresh token
  survives. `grokacp_live_test.go` (`EL_LIVE_GROK=1`) drives a real agent.
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
  Compress is a Handoff restart (`StartHandle`: a fresh ACP session beside the old one, role tools from `startHandle`, model, effort and hook preserved, token rotates; a failure keeps the old handle).
  The adapter process runs under the owner's `CLAUDE_CONFIG_DIR` (`claudeEnv`).
  Project context (`contextfiles.go`, table-tested in `contextfiles_test.go`)
  injects the Session WorkDir's AGENTS.md/CLAUDE.md chain (outermost-first,
  owner's pig dir global file first (`dirs.pig`), 64 KB cap with truncation note) into the
  system prompt on create, resume and Handoff (`sessionMetaFor`,
  `claudeSystemPromptFor`), scouts included, ahead of the context sits the
  role's memory/DOX extra (prompts.go); title calls use the role-free shape;
  `settingSources`
  stays empty.
- Handoff restarts and title calls run under the owner's dirs (`ownerDirsFor`).
  The summarizer call uses the host's pig auth and loads no instruction files.
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
  steers and follow-ups in the native queues when pig takes them (a miss falls
  back to the Daemon pending list), everything else in the
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
- Image attachments: `POST /api/sessions/{id}/prompt` takes `images: [{mime,data}]` (base64; png, jpeg, gif, webp; 8 per message, 5 MB each; text may be empty). Only `images.go` `imageBackend` Backends take them (pig via `SendContent`, claude via ACP image blocks); others fail the turn with an error Event. The prompt Event stores `images` as a count, never the bytes. A pig message with images stays in the Daemon pending list as a Follow-up (native queues carry text only, so images never enter them); other harnesses do the same. `steer` and `followup` routes stay text-only.
- `sudo.go` gates sudo: the shared `bash` tool (claude through el-mcp, which gets `EL_WEB_PARENT` and `EL_WEB_DAEMON_URL`; pig through `PigBackend.SudoGate`) posts any command that uses sudo to `POST /api/sessions/{id}/sudo` instead of running it, and the model is told the request went to the user. The UI card (`sudo_request` Event, updated by `sudo_resolved`) offers Approve or Deny via `POST /api/sessions/{id}/sudo/{rid}` with an optional password. Approved commands run in the background (`sudo -n`, or `sudo -S` with the password, held in memory only); the output returns to the Session as a TOON `sudo_result:` Follow-up, shown as a result card. Requests live in memory: a Daemon restart expires pending ones.
- `read` returns images: `pigtools.go` turns the shared tool's images into `ai.ImageContent` (a model without image input gets pig's placeholder); claude gets MCP image blocks from el-mcp.
- `push.go` is Web Push (stdlib, RFC 8291 + VAPID): `GET /api/push/key`, `POST|DELETE /api/push/subscribe`; the VAPID key and subscriptions persist in `enderbloom/push/`. A root chat turn going working to idle or failed notifies every subscription; gone ones (404, 410) drop. Browsers allow push only in a secure context, so a plain http tailnet IP cannot subscribe: use `tailscale serve` (https) with `-allow-host`. `web/public/sw.js` shows the notification.
- Event log is `enderbloom/sessions/{id}/` under the state directory:
  `meta.json` (now carrying `owner`, the resume token, `compressBlock`, `parentId`,
  `groupId`,
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
  (ParentID the Run). The fresh worktree is seeded a root `AGENTS.md` first
  (`ensureAgentsMD` via `ensureWorktreeDOX`, projects.go) when its checkout
  lacks one; the Worker's first prompt is `workerTaskPrompt` and each
  Reviewer's is `reviewerPrompt` (prompts.go: task text only, the system
  prompt carries the role extra, no `remember` in any of them). A submitted
  done (`POST /api/sessions/{id}/submit`) merges by rebase plus
  fast-forward-only through the shared core checks; a non-done submit waits
  for done instead of merging; a dirty worktree or
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
to a built Daemon binary (`TestLivePigACP`). The Jev live probe is
`EL_LIVE_JEV=1` with `JEV_API_KEY` set (or `JEV_BASE_URL` at a local server;
`JEV_MODEL` overrides the default) (`TestLiveJevDecider`).

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary
- `emberloop-web/memory/AGENTS.md` — the memory engine package: per-User SQLite banks (observations, Rules, queue, Profile versions, ledger), FTS5 recall, append-only Rules, human purge, snapshots and project notes

## E2E suite

`web/e2e/` holds the browser suite on [tester-army/e2e](https://github.com/tester-army/e2e):
real Chromium against a throwaway Daemon (`serve.sh`: fake backend, fresh state
dir, fixture repos in `/tmp/el-e2e`). No model, no API key. Run it with
`npm run test:e2e` from `web/` (it builds the UI first). It sits on top of the
Vitest component tests and `go test`; it is not part of the main check.

The sign-in suite starts from the empty state of `serve.sh`. `tests/auth.setup.e2e.ts` signs up the admin and saves the `admin` session; a test that needs a User declares `{ session: 'admin' }`. `support/auth.ts` holds the throwaway admin and the sign-in helpers.
