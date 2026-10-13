# AGENTS.md — el-web

## Purpose

Enderbloom Web: a local Daemon and embedded browser UI driving claude, pig and
grok Sessions behind one `Backend` interface. See
`.scratch/archive/enderbloom-web/spec.md`.

## Ownership

Owns `emberloop-web/`. It may import el core packages through the
`replace example.com/emberloop => ../emberloop` in `go.mod`; the module builds
only beside a sibling `emberloop/` checkout, and core never imports it. No
herdr. Ticket files are preview input; only the Run driver writes them, ticking
boxes and marking Tickets done. Runs start at most max_tickets concurrent
tickets (a spec's max_tickets frontmatter lowers it for that Run); Sessions
and subagents are uncapped.

## Test seams

- Daemon API/lifecycle and invariants: `api_test.go`, `lifecycle_test.go`,
  `handle_test.go`, `invariants_test.go` and `restart_state_test.go`; the
  fake Backend drives the API and lifecycle HTTP/SSE seams.
- Session tools: `steer_test.go`, `steer_child_test.go`, `spawn_test.go` and
  `submitgate_test.go`.
- Users and harness identity: `auth_test.go`, `owner_test.go`,
  `users_list_test.go` and `harnessdirs_test.go`.
- Disk hygiene: `disk.go` (10-minute janitor with the `diskFS` clock/FS/git
  seam, `disk_go_cache_cap_gb` setting, `/api/disk`, start guards) with
  `disk_test.go`; temp-leak guards in `tempdir_test.go` over the
  `testsupport` helper (`TempDir` with chmod+retry and a TestMain-drained
  leak list).
- Run driver: `runs_driver_test.go` uses a fake Backend and fake git;
  `runs_reviewers_test.go`, `runs_reviewer_gates_test.go` (submit gate, tip
  check, selective re-review, scope warning), `runs_persist_test.go`,
  `runs_chain_test.go`, `runs_pause_switch_test.go`,
  `runs_specbranch_test.go`, `runs_worktree_test.go`,
  `runs_toolactive_test.go` (in-flight tool calls date worker activity up to
  a cap), `runs_starttree_test.go` (stale branch and worktree reuse or
  Needs-you), `runs_final_test.go`, `runs_markdone_basered_test.go`,
  `runs_overlap_test.go` and `runs_stopsubmit_test.go` cover Run seams.
  Stop-after-submit ends a Run Worker or Reviewer turn on its accepted
  terminal submit (`runs_stopsubmit.go`, opt out per Run with
  `noStopAfterSubmit`). A merged Ticket's Worker Session ends before its
  worktree is removed (`endMergedWorker`): a live agent process never loses its
  cwd. `runs_restart_resume.go` and `runs_restart_resume_test.go`: a Run found
  running on disk (kill, crash) resumes on the next Daemon like `el restart`
  (`ResumeOnRestart`), and Worker and Reviewer Sessions whose turn a restart
  cut short continue in the same Session and worktree on a continue prompt;
  a restart request stops each paused Run's Worker and Reviewer after its
  current tool call (`armRestartStopsLocked`, `RestartStopped`, queued messages
  in `Carry`), the update watcher requests that restart when Runs run, and 4
  boots in 5 minutes stop automatic resume (`bootLoopTripped`);
  `pigacp.go` waits for the el server's ready file (`EL_READY_FILE`,
  `waitMCPReady`) before a pig start returns, so the first prompt has tools;
  Shutdown marks open turns `restart: turn interrupted` and maps their close
  errors to idle, never failed.
- Backend and Handoff seams: `claude_*_test.go`, `pig_test.go`,
  `handoff_test.go`, `compress_retry_test.go`, `grok_handoff_test.go` and
  `acpretry_test.go`.
- Memory package tests are mapped in `memory/AGENTS.md`; browser tests are
  mapped in `web/README.md`.

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

- In `emberloop-web/`: `go vet ./... && go test ./...` (included in
  `.emberloop/check`).
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

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary
- `emberloop-web/memory/AGENTS.md` — the memory engine package and its contracts
