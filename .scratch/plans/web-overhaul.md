# Emberloop web overhaul: plan and handoff

Written 2026-10-09 from one long planning session. Everything here is decided with the user unless marked open. The main checkout holds all finished work uncommitted on top of checkpoint commit `4ddf980` ("wip: checkpoint before merging workers").

## Working rules agreed

- Product name is **Emberloop** everywhere user-visible. The on-disk state subtree `<state-dir>/enderbloom/` keeps its name (internal).
- Workers edit the main checkout directly, never commit, and re-read shared files before editing. Worktree workers branch from a stale HEAD (`e160531`), so their branches cannot be merged mechanically. Reconcile by intent.
- Every change ships with tests. Owning `AGENTS.md` files and `web/README.md` update in the same task (DOX contract).
- The Composer has no "Sends as" badge. The voice-status pill, live-append voice text, `VersionBadge` and the inline image tray must survive every merge.
- Delivery of subagent results is grouped by default. `spawn` now takes `deliver: "each"` for one notice per child.
- The session cap is 5 concurrent agents (`max_workers`).

## Done and merged into the main checkout (uncommitted)

| Work | Result |
|---|---|
| Naming | Enderbloom to Emberloop in UI, docs, tab title, push fallback. systemd unit is `emberloop-web` (docs fixed). "Sends as" badge removed. |
| Live audio | Segmented STT stays (true streaming needs a direct OpenAI key; the Daemon holds only OpenRouter, POST-only). Text appends straight into the textarea, `voice-status` pill, `audioCtx.resume()` fix, silence 700ms, max segment 10s, cleanup pass off by default. |
| Version and update | `/api/version`, `/api/health`, `el-web --version`, bottom-right `VersionBadge` (click copies `ui <hash> · be <commit>`, "update available" pill), one `el update` command (skips `vite build` if web source hash unchanged, rebuilds all four binaries with ldflags, keeps previous set, restores it if new `el-web` fails `/api/health`). `el install` aliases it. |
| Subagent cards | `spawn` `name` field (fallback: first 4 prompt words), harness accent (claude `#d97757`, pig `#4fb08f`), worker/scout chip, live tool line, `GroupCard`, `deliver`, idle/working pills removed, stored session images with gallery and chips. Reconciled with the existing inline-image flow. |
| Runs backend | Run API returns per ticket: `state`, `stateSince`, `boxes`, `body`, `title`, `branch`, `tip`, `gate`, `verdicts`, `agents`, `rounds[]`, `followups`, `needsYou`, `lastRefusal`. Realtime over the Run SSE. Reviewers reply `VERDICT: pass\|changes` plus `file:line severity: problem` lines (no `submit`), one `RESPOND IN FORMAT` retry, then needs-you. Messages to reviewers are refused with 409. Rounds persist in the Run JSON. Contract documented in `emberloop-web/AGENTS.md`. |

Known merge repairs already made: restored `/api/push/` route in `daemon.go`; restored `emberloop-web/AGENTS.md` and `web/README.md` after a stale-branch merge deleted about 100 lines.

## Running when this was written

- `tool-strip`: live tool strip UI from `.scratch/mockup/toolstrip.html`. Consecutive tool calls collapse into one pill. New tool swaps content in place (slide, blur, eased height capped about 232px). History drawer. Settles into a one-line summary after assistant text. Failed tool holds red. Excludes `spawn`, `submit`, sudo cards.
- `session-load-and-routes`: tail paging (`?tail=6`, `?before=&turns=`, `hasMore`), replay compaction (last `tool_call_updated` per tool call id), "load earlier" on scroll up, TanStack virtualization with stick-to-bottom, real URL routes (`/s/<id>`, `/runs`, `/runs/<runId>/<ticketId>`, `/settings`), SPA fallback in `ui.go`, vite `base: '/'`, `sw.js` notification click opens `/s/<id>`, router module with tests.

Update: `session-load-and-routes` finished (Go 254 pass; vitest router, transcript, App and Runs suites pass; `web/dist` rebuilt with absolute `/assets/` paths). Left over: re-run `npx vitest run && npx vite build` once `tool-strip` lands, because `tool-strip`'s `Card.svelte` was mid-edit and broke `vite build` and `Card.test.js`; run `npm run test:e2e` once; a cold deep link to a Run shows the table without spec-picker context. `tool-strip` also finished; with both merged: `go vet` clean, Go 254 pass, vitest 208 pass, `vite build` ok (verified in the main checkout). Remaining: `npm run test:e2e` once.

## Queue, in order

1. **sfx-and-push** (after session routes). `uisfx@0.4.0` (MIT code, CC0 audio), synthesized Web Audio, pack `minimal` by default. Vendor mp3s under `web/public/sounds/<pack>/` (relative URLs do not bundle). Unlock on first pointerdown or keydown, drop cues before unlock, 400ms throttle with urgency override (wick pattern in `~/Projects/wick/apps/frontend/src/lib/sound_player.ts`).
   - Cues: turn done `notification`; failed `error`; sudo request `mention`; Runs needs-you `blocked`; ticket merged or Run complete `complete`; voice start/stop quiet tick; message sent and subagent finished silent.
   - Settings: new section next to Notifications with pack picker plus preview button, master volume slider, mute switch, per-event toggles. Stored in `localStorage` (per device).
   - Push: service worker `push` handler messages open tabs to play the sound and shows the OS notification only when no window is visible. A custom sound inside a push notification is impossible (OS plays its default). New push triggers: sudo request and Runs needs-you, besides turn done and failed. Notification click opens the exact `/s/<id>` or ticket route.
2. **runs-ui**. Dual rail plus ticket page, from `.scratch/mockup/layout.html` (open it; `#ticket` shows the ticket page).
   - Rail 1: narrow icon rail (Chat, Specs and Runs, Settings), sidebar colour. Rail 2: contextual sidebar (chat: projects and sessions; runs: needs-you strip plus spec list; settings: sections). Use the exact app tokens from `web/src/app.css` (note Tailwind CDN needs `borderColor.DEFAULT: '#3a3a3c'`).
   - Runs view: dependency graph (use `@dagrejs/dagre`, SVG edges plus HTML cards, pan and zoom, legend), List/Graph toggle, Pause/Resume. Node colours: merged green, running blue, needs-you amber, blocked dim, parked/discarded muted.
   - Ticket page: state track (queued, running, in review round N, merging, merged), worker transcript as the main column with review rounds woven in (two cards per round; held pass shown as "pass held"; declined-with-reason card), right panel with agents list (worker, reviewers, scouts; reviewers and scouts open read-only in a drawer with no composer), acceptance checkboxes ticking live, gate state, follow-ups.
   - Realtime from the Run SSE, no polling. Dot colour is harness, badge is agent type.
   - Tests: vitest for graph layout, state mapping, rounds rendering, SSE updates.
3. **project-init-and-validate** (user must say "go" first; it refactors the Driver parser that Runs reuses).
   - `.emberloop/settings.json` per project: `specs` (directory), `tickets` (subfolder name), `pipeline` (false silences everything). Project overrides global `~/.emberloop/settings.json`.
   - `el init --specs .scratch --tickets issues` is the validated writer. The agent inspects the project (old specs included) and runs it. No UI form.
   - Tools that need spec or ticket paths read the file fresh on every call and, when missing, say "project not initiated: ask the user, then run `el init`", once per session. No caching, so they react the moment the file exists.
   - Shared ticket parser package for Driver and Daemon. `ValidateSpec` appended to `write` and `edit` tool results under spec and ticket paths (warning only). Turn-end gate: re-validate edited spec files, re-prompt once with errors, then needs-you. Cross-file checks (cycles, unknown blockers, duplicate numbers).
   - Matt skills (`to-spec`, `to-tickets`, `implement`, `wayfinder`) read the settings instead of hardcoding paths.
4. **runs-actions**. Needs-you banner and triggers (same refusal x3, 10 min idle, 3 stall nudges), per-ticket Steer, Retry, Park, Discard routes and buttons, Run list picker (`GET /api/runs` exists, never called), and a live probe under `scripts/` that starts a 2-ticket Run through `/api/runs` against real agents. Steer applies to workers only.
5. **check-green**. Fix the pre-existing failures, remove the old herdr tests (user decision), rebuild `web/dist`, make `.emberloop/check` pass. Known failures: `TestExecuteDescriptionExamplesFollowTheToolList`, `TestImplementDriverRunStartsWorkersThroughTheLiveLoop`, `TestDoxTreeIndexIsComplete` (index versus `.scratch/` layout), a push API test, flaky combobox windowing tests and a TempDir cleanup flake.
6. **Final**: `el update` to ship (never commit unasked).

## Decisions worth remembering

- Reviewers cannot be talked to; only the worker can, and workers keep `submit`.
- Reviewer verdict tip is filled by the Daemon, not the reviewer.
- Running tool strip excludes `spawn`, `submit`, approvals.
- Pig accent teal `#4fb08f` (from the pig logo), claude terracotta `#d97757`. Done status stays Tailwind green.
- Results delivery: UI shows each child finish live; the parent model receives one grouped notice unless `deliver: "each"`.
- Wick has no combined UI+binary update command and no bottom-right badge; only `buildinfo`, `/api/version` and `watchServerBuild` patterns were borrowed.

## Pipeline facts (from real past runs, for the Runs UI)

Driver spawns a worker in worktree `implement/<ticket>`. The worker implements (about 6 min), spawns both persistent reviewers in one call, receives one notice, fixes, rebases on the spec branch, re-prompts reviewers with prior findings and any declined findings plus reason, commits, then `submit(done)` with completed checkbox texts and `Follow-ups:`. The Driver ticks boxes and merges only when both reviewers `pass` at the tip (rebase plus fast-forward). Row states: queued, running, in-review, merging, needs-you, parked, merged, discarded. Detail in `emberloop/AGENTS.md:49-110`.

## Open items

- Say "go" on project-init-and-validate ordering.
- Commit the finished work when ready (nothing committed since `4ddf980`).
- Prod Daemon (`emberloop-web.service`, port 7940) needs the final `el update` to serve any of this; it was running a stale build.
