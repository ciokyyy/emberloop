# AGENTS.md — el-web

## Purpose

Enderbloom Web: a local Daemon and embedded browser UI driving claude, pig and
grok Sessions behind one `Backend` interface. See
`.scratch/archive/enderbloom-web/spec.md`.

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
- Quota HTTP/live seam: `quota.go` serves the shared cache immediately on GET;
  one `/api/live?topic=quota` subscriber starts the guarded refresher, which
  refreshes configured providers concurrently, while ordinary live watchers keep
  the shared EventSource stable. The first non-forced pass retains the 90s/provider
  age guard; the last quota subscriber leaving cancels it. Changed provider
  lists publish only to quota streams. Header panes share cache files and
  per-key locks; manual POST skips the age guard and enforces a 10s/provider
  minimum. ChatGPT Plus uses pig's read-only `openai-codex` login.
  `el/usage_providers.go` polls Claude's host OAuth read-only with a separate
  90s cache and persisted `Retry-After` backoff; it never refreshes or writes
  credentials, starts a harness or sends a prompt. Statusline data is the
  fallback only when host OAuth is absent; the terminal header remains
  unchanged. OpenRouter reads pig's `openrouter.key`, combines credits and
  current-key spend in the Credits detail, and caches its percent bar plus
  secondary text. Unconfigured sources are skipped. `quota_test.go` covers
  cache reads, live payloads/gating/coalescing, provider isolation, cooldowns,
  shared locks, ChatGPT and Claude behavior, and OpenRouter endpoints against
  local stubs.
- Dashboard payload: `GET /api/dashboard` returns working or attention-needed
  Sessions and running/needs-you Runs grouped by Project id. Unassigned work
  uses the built-in `none` scope; Runs resolve to the deepest Project containing
  their repo path. `dashboard_test.go` covers HTTP grouping, status filtering,
  nested ownership and home-directory Sessions.
- Project metadata: `projects.go` derives a cached one-line gist from the root
  `AGENTS.md` (else README), persists it with the Project, and preserves an
  explicit gist edit across re-derivation; `projects_test.go` covers both
  sources, cache refresh and edit precedence.
- Draft Project pick: `POST /api/projects/suggest` (`projectpick.go`) evaluates
  one `noul` per Project plus `none` before any Session starts. Jev receives the
  prompt and each Project's name and gist, never its path; a suggestion requires
  top >= 0.5 and a lead >= 0.2. Missing keys and failed calls return no pick;
  `projectpick_test.go` checks the HTTP response and local systemone stub.
  The response always carries `suggestedProject` (the Project or `null`).
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
- Memory package tests are mapped in `memory/AGENTS.md`; browser tests are
  mapped in `web/README.md`.

- Browser boot: `App.svelte` routes from the URL before list reads finish.
  `/quota` skips Session and Run list fetches; `/runs` loads summaries through
  `RunsSidebar`. Home loads Sessions asynchronously and resolves last/newest
  only after the list arrives; the chat sidebar shows `Loading Sessions…`.
  Session events while another area is open mark the list stale; returning
  home refreshes it before selecting last/newest. `App.test.js` covers delayed
  lists, route-owned loading states and this return-to-home refresh.

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
  bounded wait never fails a test itself. `runTestDaemon` acquires the Run-test
  slot before registering shutdown; `initRunRepo` also acquires it for tests
  that do not start a Daemon. The unconditional slot cleanup runs last. After
  60 seconds waiting, the watchdog writes holder names to stderr and `t.Log`.
  Each Run test has a 5-minute deadline, shortened to precede `t.Deadline()`;
  expiry streams its name and goroutine dump to stderr, then calls `t.Errorf`
  so only that test fails. Cleanup waits for the watchdog to exit before
  releasing the slot, so it cannot report after the test completes. The helper
  interrupts busy Sessions every poll and stops waiting on a Session still
  busy after 50 polls. Fake turns last about 5ms, so tests wait for a turn's
  end with `waitTurnEnd` (an idle status after the prompt's seq), never
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
- `emberloop-web/memory/AGENTS.md` — the memory engine package and its contracts
