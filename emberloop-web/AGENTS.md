# AGENTS.md — el-web

## Purpose

Emberloop Web is the local Daemon and embedded browser UI for claude, pig and
grok Sessions and deterministic implement/review Runs. It owns the Daemon,
web app and embedded el library; see the Child DOX Index below.

## Ownership

Owns `emberloop-web/` as one Go module: the Daemon and browser UI, `el/`
(the el library with its own doc), and `cmd/` with the three entry points
(`el`, `el-mcp`, `el-web`). The el library never imports the Daemon package;
the Daemon-side packages import el. No herdr. Ticket files are preview input;
only the Run driver writes them, ticking boxes and marking Tickets done.
Runs start at most max_tickets concurrent tickets (a spec's max_tickets
frontmatter lowers it for that Run); Sessions and subagents are uncapped.

## Test seams

- Daemon API/lifecycle and invariants: `api_test.go`, `lifecycle_test.go`,
  `handle_test.go`, `invariants_test.go` and `restart_state_test.go`; the
  fake Backend drives the API and lifecycle HTTP/SSE seams.
- Session tools: `steer_test.go`, `steer_child_test.go`, `spawn_test.go` and
  `submitgate_test.go`.
- Quota HTTP/live seam: `GET /api/quota` returns `{"providers":[...]}` from
  the shared cache only. Each provider has `key`, `label`, `plan`, `state`
  (`ok`, `stale`, `expired`, `not_configured`, `error`), `updated_at`, `error`,
  optional `hint`, and `windows[]`; each window has a word-form `label`, an
  integer percent-used `pct` (0–100), `resets_at`, and optional secondary
  `detail`. Cards show the percent used first; `pct` alone drives quota bars
  and Live status rows. Cards cover
  Claude, ChatGPT Plus, OpenCode-Go, OpenRouter, and Antigravity's Gemini
  (`agy-gemini`) and Claude + GPT-OSS (`agy-3p`) pools.
  One `/api/live?topic=quota` subscriber starts the shared refresher: its first
  pass reads the cache and fetches configured providers concurrently when the
  90s per-provider age guard allows; it checks every 15s and the last subscriber
  leaving cancels it. Ordinary live watchers keep the shared EventSource
  stable. Changed full provider lists publish only to quota streams. Header
  panes share cache files and per-key locks. `POST /api/quota/refresh` accepts
  an optional provider `key` in JSON or the query string; an empty key refreshes
  all providers. Manual refresh skips the age guard and enforces a 10s minimum
  per upstream provider.
  ChatGPT Plus reads pig's `openai-codex` login without refreshing it.
  `el/usage_providers.go` polls Claude's host OAuth read-only with a separate
  90s cache and persisted `Retry-After` backoff; it never refreshes or writes
  credentials, starts a harness or sends a prompt. Statusline data is the
  fallback only when host OAuth is absent; the terminal header remains
  statusline-backed. OpenRouter reads pig's `openrouter.key`; its Credits detail
  combines dollars used, total and left with current-key spend. OpenCode-Go and
  both Antigravity pools use the shared usage fetchers. Missing configuration
  remains a `not_configured` card with an actionable hint and skips upstream
  fetches. `quota_test.go` covers response shape, cached reads, live payloads,
  subscriber gating/coalescing, provider isolation, cooldowns, shared locks,
  ChatGPT and Claude behavior, and OpenRouter endpoints against local stubs.
- Dashboard payload: `GET /api/dashboard` returns only working/needs-you
  Sessions and running/needs-you Runs grouped by Project id. Unassigned work
  uses the built-in `none` scope; Runs resolve to the deepest Project containing
  their repo path. `dashboard_test.go` covers HTTP grouping, status filtering,
  nested ownership and home-directory Sessions.
- Provider API and auth: `providers.go` serves `GET /api/providers` from the pig SDK catalog plus synthetic Claude and Antigravity rows. Responses include status/source metadata, CLI guidance and stored-key flags, never secret values. OAuth rows without a browser flow expose the exact `pig login <provider>` command; Bedrock, Vertex and Cloudflare cloud-auth rows say `configure via CLI`. Admin-only `PUT`/`DELETE /api/providers/<id>/key` write API keys to pig `auth.json`.
   `oauth_redirect_base_url` defaults to empty and is admin-write-only; `provider_redirect.go` applies it to callback-server OAuth only after a provider-specific probe accepts it. Probe errors, non-OAuth providers and ChatGPT device-code login fail closed; no current Providers-page flow consumes a configurable redirect.
   ChatGPT uses one device-code login per Daemon, expiring after 15 minutes: `POST /api/providers/openai-codex/device-code` starts pig's SDK flow; `GET` and `DELETE /api/providers/openai-codex/device-code/<id>` poll its code/link/status or cancel it. Success stores OAuth credentials in pig `auth.json` and forces a ChatGPT quota refresh. Claude uses one admin-only `claude auth login` PTY at a time and accepts code retries while it remains running; Claude Code alone writes its login in the host config directory, never pig. Admin-only Antigravity `/connect` endpoints host the extension callback flow as a short-lived login; the callback accepts a pasted URL or bare code, status stays secret-free, and success stores the finished credential in pig `auth.json`.
   Successful connects refresh their matching quota cards: ChatGPT, both Antigravity pools, Claude and quota-backed API-key providers. The UI applies each returned provider list to `quotaStore`; not-connected Quota cards deep-link to the corresponding Providers row, with both Antigravity pools targeting Google Antigravity. Shutdown cancels ChatGPT and Claude login flows; terminal status is retained for five minutes. `provider_device_code_test.go` covers ChatGPT success, expiry, cancel and authorization against local stubs; `claude_login_test.go` covers PTY success, invalid code, retry, process exit, authorization and pig-auth separation; `providers_test.go` covers catalog/status sources, CLI guidance, secret redaction, API-key writes, Antigravity URL/bare-code/retry login and authorization with local HTTP stubs; `settings_test.go` covers the redirect-base default, persistence, validation and provider-probe gate.
- Project metadata: `projects.go` derives a cached one-line gist from the root
  `AGENTS.md` (else README), persists it with the Project, and preserves an
  explicit gist edit across re-derivation; `projects_test.go` covers both
  sources, cache refresh and edit precedence.
- Draft Project pick: `POST /api/projects/suggest` (`projectpick.go`) evaluates
  one `noul` per Project plus `none` before any Session starts. Jev receives the
  prompt and each Project's name and gist, never its path; a suggestion requires
  top >= 0.5 and a lead >= 0.2. Missing keys and failed calls return no pick.
  `App.svelte` asks only for the first prompt of a projectless draft: a
  suggestion shows “Run this in <Project>?”; Yes creates the Session with that
  Project id and sends the held prompt, while No or no suggestion keeps the
  home-directory start. If navigation closes the draft during Jev evaluation,
  the captured prompt still starts without a pick; a failed delivery after a
  confirmed start remains available to retry in that Session. A Session's
  working directory never changes after creation. The response always carries
  `suggestedProject` (the Project or `null`); `projectpick_test.go` covers the
  endpoint, local systemone stub, backend cwd and suggested Project path.
- Users and harness identity: `auth_test.go`, `owner_test.go`,
  `users_list_test.go` and `harnessdirs_test.go`.
- Disk hygiene: `disk.go` (10-minute janitor with the `diskFS` clock/FS/git
  seam, `disk_go_cache_cap_gb` setting, `/api/disk`, start guards) with
  `disk_test.go`; temp-leak guards in `tempdir_test.go` over the
  `testsupport` helper (`TempDir` with chmod+retry and a TestMain-drained
  leak list; `el/` keeps its own guard in `el/tempdir_guard_test.go`).
- Run collection API: `GET /api/runs` returns git-free summaries (`id`, `spec`,
  `status`, `counts`, `repo`, `projectId`, `updatedAt`, plus `owner` for the
  account sidebar); `GET /api/runs/<id>` returns the detailed Ticket snapshot.
  `TicketSessions` persists each Ticket's Worker, Reviewer and child Session
  index in the Run file; startup backfills it from Run fields and Session
  `ParentID`. Run detail, pause and abandon paths use that index, not a
  per-Ticket scan of `d.sessions`; list summaries do no Ticket Session lookup.
  Detail snapshots cache git-derived Run and Ticket results for three seconds;
  each Run event invalidates that Run's entries (`runs_git_cache.go`).
  `runs_test.go` covers both HTTP shapes and a bounded 40-Run, 8-Ticket list;
  `runs_session_index_test.go` covers indexed HTTP results and restart recovery.
  `runs_git_cache_test.go` covers cache hits, expiry and Run-event invalidation.
- Run driver: `runs_driver_test.go` uses a fake Backend and fake git;
  `runs_reviewers_test.go`, `runs_reviewer_gates_test.go` (submit gate, tip
  check, selective re-review, scope warning), `runs_persist_test.go`,
  `runs_chain_test.go`, `runs_pause_switch_test.go`,
  `runs_specbranch_test.go`, `runs_worktree_test.go`,
  `runs_toolactive_test.go` (in-flight tool calls date worker activity up to
  a cap), `runs_starttree_test.go` (stale branch and worktree reuse or
  Needs-you), `runs_final_test.go`, `runs_markdone_basered_test.go`,
  `runs_overlap_test.go`, `runs_stopsubmit_test.go` and
  `runs_merge_conflict_test.go` (a done submit's spec-branch rebase conflict
  refuses with `el.RebaseConflictRefusal`) cover Run seams.
  Base and tip checks (`runRepoCheckAtTip`, `runs_checkspeed_test.go`) run
  `.emberloop/check` in one reused, cleaned worktree per repo
  (`.el-worktrees/.check`, one check at a time), so Go caches hit. A base
  check is shared per spec tip (`baseChecking`), reuses a green tip check of
  the same commit, and runs in the background while other Tickets have live
  Workers, so the driver never stalls their reviews and merges on it.
  Stop-after-submit ends a Run Worker or Reviewer turn on its accepted
  terminal submit (`runs_stopsubmit.go`, opt out per Run with
  `noStopAfterSubmit`). A merged Ticket's Worker Session ends before its
  worktree is removed (`endMergedWorker`): a live agent process never loses its
  cwd. `runs_restart_resume.go` and `runs_restart_resume_test.go`: a Run found
  running on disk (kill, crash) resumes on the next Daemon like `el restart`
  (`ResumeOnRestart`), and Worker and Reviewer Sessions whose turn a restart
  cut short continue in the same Session and worktree on a continue prompt;
  a restart request stops each paused Run's Worker and Reviewer after its
  current tool call; if a Backend turn has not started, it rolls the stop
  marker back and retries at the first working status or tool boundary
  (`armRestartStopsLocked`, `restartStopAttempt`, `RestartStopped`, queued
  messages in `Carry`), the update watcher requests that restart when Runs run,
  and 4 boots in 5 minutes stop automatic resume (`bootLoopTripped`);
  `pigacp.go` waits for the el server's ready file (`EL_READY_FILE`,
  `waitMCPReady`) before a pig start returns, so the first prompt has tools;
  Shutdown marks open turns `restart: turn interrupted` and maps their close
  errors to idle, never failed.
- Backend and Handoff seams: `claude_*_test.go`, `pig_test.go`,
  `handoff_test.go`, `compress_retry_test.go`, `grok_handoff_test.go` and
  `acpretry_test.go`.
- Memory vectors and recall: `memory/embedder.go` supplies redacted OpenRouter embeddings, a bounded per-Daemon worker, model-filtered cosine ranking and FTS fallback; `memory/hybrid.go` ranks Memories and searches Episodes separately (Episodes are never injected). The idle-gated nightly job in `nightly.go` uses ledger summaries as its local-03:00 due marker, catches up settled Sessions, retries after one daily busy-skip row, and runs reembedding, Episode fold, consolidation, Memory candidate promotion, Profile proposals, 48-hour Profile reminders, and an opt-in daily “learned N things” digest. Reminder and digest delivery serialize per Daemon; sent ledger rows are written only after at least one push endpoint accepts delivery. The digest is skipped when no Memories were learned. Authenticated `GET`/`PUT /api/push/digest` reads and writes the per-User preference in `stateDir/enderbloom/push/digest-prefs.json`; missing entries default off. Profile proposals enter the User's pending queue, raise action-enabled push notifications and broadcast inline Session cards; both approval paths share the queue decision API, stamp `approved_by`, and rejected identical patches are suppressed only while the Profile context is unchanged. Maintenance scans cover all rows; fold preserves Episodes and marks each folded row, and candidate Rules remain pending and require human approval. Admin-only host-global Memory extraction, embedding, recall weights and automatic-extraction settings are served through `/api/settings`; defaults track OpenRouter key presence and saved embedding-model changes reconfigure live. `memory/AGENTS.md` maps package contracts and tests; browser tests are mapped in `web/README.md`.
- Memory dedup: `memory/dedup_memory.go` compares active same-bank Memories before insertion, prioritizes normalized-text duplicates, applies compatible-model vector thresholds when available, bumps duplicates and transactionally supersedes same-author contradictions. Embedding remains asynchronous after storage.
- Memory deciders and turn-end ingestion: `decider.go` uses OpenRouter systemone for a blank Jev URL in Jev mode when a key is available; ensemble keeps its memory-model default unless a custom URL is set. Turn-end ingestion (`ingest.go`, `mine_live.go`) debounces settled turns, stores Episodes, extracts off-turn and deduplicates Memories. Session recall exposes superseded rows only with `include_superseded=true`; package and tool contracts/tests are mapped in `memory/AGENTS.md` and `el/tools/AGENTS.md`.
- Memories replay evaluation reports the Rules-only and recall-with-Memories arms on identical correction cases (`replay.go`); the CLI prints both recall@5 values. `replay_test.go` covers the deterministic replay arms.
- Memory status summarizes ingest, age-qualified forget, promotion, injection, contradiction and model-cost ledger rows into daily production proxies; embedding success rows carry provider-reported cost when available. The Memory page warns above $0.05/day. Forget and injection actions emit their ledger rows; fixture arithmetic is in `memory_metrics_test.go`.
- Browser project scope: `/` renders the active-work Dashboard; stable-id
  Project routes scope Sessions and Runs, with `/p/none` for home Sessions.
  `/p/<id>/runs` filters Run summaries and discovers/starts specs at the Project
  path without a folder field. `/p/<id>/settings` supports rename, removal
  (chats and files stay) and gist editing, with no override fields. `/s/<id>`
  stays canonical and derives scope from Session metadata. Account pages remain
  global; the switcher and Dashboard are available in the phone drawer.
  Browser Settings: `/settings` is the hub on phones and redirects to `/settings/agents` on desktop. Each concern has a stable `/settings/<section>` route, grouped Workspace and You in `SettingsNav.svelte`; each page shows its Global, Admin or You scope and a phone return link. Agents has one ordered chain per Session and Run role (Orchestrator, Subagent, Scout, Ticket worker, Spec reviewer, Standards reviewer), at most three `{harness, model, effort}` entries; effort choices come from each model's `/api/models` metadata. The Daemon migrates legacy role settings and Run Chain aliases to canonical chains; configured legacy role keys mirror primaries for `el settings role` compatibility. Daemon-backed pages share a first-change Revert/Save bar. Runs labels the global cap “Maximum tickets per Run (spec)” and explains that `max_tickets` caps each Run while a spec value can only lower it. Memory combines its legacy decider controls with admin-only host-global extractor model/effort, embedding model, automatic extraction and validated recall weights; settings are searchable by control name, and redacted user turns egress to the extractor/embedder. Profile, notification and device preference components retain their existing behavior. `model_efforts_test.go`, `settings_test.go`, `SettingsPage.test.js`, `SettingsHub.test.js`, `SettingsRoutes.test.js` and `router.test.js` cover model capabilities, editing, migration, grouping, routes, phone navigation and the save bar.
- Browser boot: the Dashboard and global pages do not wait for Session or Run
  lists. Project scope loads Sessions asynchronously; `/runs` and
  `/p/<id>/runs` load summaries in `RunsSidebar`. `runsStore` shares one summary
  list between the sidebar and app and coalesces bursts of Run events into one
  refetch. Session events while another area is open mark the list stale;
  returning to a Project scope refreshes before selecting its last/newest
  Session. `router.test.js` and `App.test.js` cover URL round trips, scope,
  global routes, Dashboard and drawer navigation.

## Adapter retry

- An `adapter closed` prompt failure on a Session the Daemon closed
  deliberately (end, pause settle, abandon, restart Shutdown) drops quietly:
  no error Event, no failed status, no retry (`acpretry.go`).
- An unexpected adapter death respawns through the Session's ResumeSession
  (pig-acp and grok through their outer one) and resends the same prompt,
  at most 3 sends with 1s/3s backoff (`acpRespawnBackoff`); a reconnecting
  status Event marks each retry, and exhaustion fails once naming the
  attempts. `claudeState` keeps the Session record for the respawn.

## Verification

- In `emberloop-web/`: `go vet ./... && go test
  -timeout 15m -skip TestLive ./...` (included in `.emberloop/check`).
- In `emberloop-web/web/`: `npm test` and `npm run build`; these are not part of
  the repo check.
- In `emberloop-web/web/`: `npm run test:e2e`; it is not part of the repo check.
- Live probes are env-gated and never part of the check: `EL_LIVE_PIG=1` with
  `PIG_LIVE_MODEL` runs `TestLivePigBackend`; `EL_LIVE_PIG_ACP=1` with
  `PIG_ACP_SPAWN_BIN` set to a built Daemon runs `TestLivePigACP`; and
  `EL_LIVE_JEV=1` with `JEV_API_KEY` (or `JEV_BASE_URL` for a local server;
  `JEV_MODEL` overrides the default) runs `TestLiveJevDecider`.

## Test speed

- Tests use `t.Parallel()` unless they touch process env or shared globals
  (settings PUT/file tests, titles, quota, wire adapters, the submit gate
  suite, injection hooks). `useSettings` returns a path each Daemon takes
  with `SetSettingsPath`; `initRunRepo` copies a per-ticket-set template
  instead of init/add/commit; git and faux env live in `TestMain`.
- Run tests start Daemons with `runTestDaemon` (100ms driver polls);
  pause/switch/resume and reviewer rework sequences (findings, rereview,
  selective carry) keep `NewDaemon` (500ms) and stay serial: they race a
  fast driver into concluded runs. Same-tip hold, patch-identical reuse,
  retry-after-refusal, failed-run resume and fake-clock idle tests stay
  serial: their counts and waits flake under parallel git load.
- `stopRunsForTest` is registered per Daemon (`runsStopped`; `runTestDaemon`
  and `authedHandler` both register it). Every critical section defers its
  unlock; recovered cleanup panics are reported with `t.Errorf`, and its
  bounded wait never fails a test itself. The Run-test semaphore release is a
  separate unconditional cleanup registered before other cleanup, so it runs
  last. `runTestDaemon` acquires a slot before registering shutdown;
  `initRunRepo` also acquires one for tests that do not start a Daemon. After
  60 seconds waiting, the watchdog writes slot-holder test names to stderr and
  `t.Log`. Each Run test has a 5-minute deadline, shortened to precede
  `t.Deadline()`; expiry streams its name and goroutine dump to stderr, then
  calls `t.Errorf` so only that test fails. Cleanup waits for the watchdog to
  exit before releasing the slot, so it cannot report after the test completes.
  The helper interrupts busy Sessions every poll and stops waiting on a Session
  still busy after 50 polls. Fake turns last about 5ms, so tests wait for a
  turn's end with `waitTurnEnd` (an idle status after the prompt's seq), never
  working-then-idle polls. A test that mutates state after that idle Event also
  waits for `busy=false` and `active==0`; the Event precedes submit-gate and
  post-turn work. A test that jumps the fake clock first waits for the Worker's
  first turn to end.
- A Session exists before its first prompt lands: tests read a transcript only
  after `firstPromptWhere` sees a prompt. A test that forces a Run's status
  after a pause first waits for the driver to exit (`driving` false) or park
  in `pauseGate` (`gateParked`); a driver between gates rewrites the status.
- `ensureReviewers` runs one call per Ticket at a time (`ensuring` lock): the
  submit route and the driver's finish race on one done submit, and an
  unserialized second call re-prompts a seat whose verdict has not landed.

## Child DOX Index

- `emberloop-web/el/AGENTS.md` — the el library, its file map, and its Go workflow; indexes tools, MCP server, state database, and test data
- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary
- `emberloop-web/memory/AGENTS.md` — memory engine, tests, embeddings, hybrid Memory and Episode recall scoring, and injection contracts
