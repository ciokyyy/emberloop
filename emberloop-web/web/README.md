# UI

Svelte 5 + Vite single-page app, built to static files. `npm run build` in
`web/` writes `web/dist/`; the Daemon embeds `web/dist` and serves `/`,
`/index.html` and the bundle assets (JS, CSS, favicon, manifest, service
worker, icons) at `/` with PWA headers (see Installable app). No server
rendering anywhere, so the same build wraps in Tauri later unchanged.

## Design rules

Follow these for every UI change.

 - Shell: `Rail.svelte` is the narrow icon rail (Chat, Specs and Runs, Memory,
   Quota, Settings) with the second-sidebar collapse toggle in its header
   (Ctrl/⌘+B, persisted); a contextual second sidebar fills the next column
   (`RunsSidebar.svelte` owns Runs spec discovery: repo directory, spec list,
   Find specs form, preview pick). The sidebar collapses to the rail on
   desktop (width transition on `--ease-smooth`) and becomes an off-canvas
   drawer on `<md` screens (top-bar menu button, closes on navigate, safe-area
   padding). The main area owns the Run itself (preview table, Start, live
   table/graph). Tokens live in `src/app.css`; never hardcode colors.
- shadcn-svelte everywhere: every interactive control (button, combobox,
  select, dialog, input, badge, ...) comes from `src/lib/ui/`, built on bits-ui.
  Never use a raw `<select>`, `<dialog>` or unstyled `<input>`. Add a missing
  primitive to `src/lib/ui/` first.
- Pickers use `src/lib/ui/combobox.svelte` (Popover + Command with search and a
  TanStack-virtualized list); `src/lib/ui/select.svelte` stays a plain bits-ui
  Select for simple fixed lists.
- Icons come from `@lucide/svelte`.
- Tool output color is the `tk-*` token classes in `src/app.css` (`tk-kw`,
  `tk-str`, `tk-num`, `tk-comment`, `tk-fn`, `tk-key`, `tk-lit`, `tk-path`,
  `tk-flag`, `tk-op`, `tk-hunk`, `tk-dim`, `tk-err`, `tk-warn`, `tk-ok`,
  `tk-bold`, `tk-add-sign`, `tk-del-sign`), painted by `lib/highlight.js`;
  never invent a new output color outside them.
- Motion everywhere: new blocks use `animate-rise`, overlays use
  `tw-animate-css` (`animate-in`/`fade-in-0`/`zoom-in-95`), interactive
  elements transition on the shared `--ease-smooth` curve. Reduced motion
  is honored in `app.css`.
- Models show display names (`modelLabel` in `harnesses.js`); the Daemon owns
  the model list (`GET /api/models`).
- Dev: `bun run dev` runs the Daemon (127.0.0.1:8910) and Vite (5190, proxies `/api`) together. Prod: `bun run prod` builds, then runs the Daemon (8920) and `vite preview` (8921, proxies `/api`). Override with `API_PORT` / `UI_PORT` per script.

## API base URL

Same-origin by default. To point the static build at another Daemon:

- build time: `VITE_API_BASE=http://127.0.0.1:PORT npm run build`
- runtime: set `window.__EL_API_BASE__` before the bundle loads

## Source map

 - `src/Root.svelte` — sign-in gate: probes `GET /api/auth/me`; signed out shows `AuthScreen`, signed in mounts `App` with the User; sign out and any 401 from a non-auth route return to sign-in; an unreachable Daemon offers Try again
 - `src/lib/AuthScreen.svelte`, `src/lib/authForm.js` — sign-in, create-account and first-admin screen on `ui/` primitives; `authForm.js` holds the mode, copy and error-line helpers
 - `src/App.svelte` — layout, rail-1 icon rail (`Rail.svelte`) plus a contextual rail-2 sidebar (collapses to the rail on desktop via the rail-header toggle and Ctrl/⌘+B, persisted as `eb.sidebar-collapsed`; the same content renders in the phone drawer), Session tail loading (`?tail=6`, `?at=`) with load-earlier prepend, SSE stream with reconnect, pending prompt cards that the echoed prompt Event replaces with no second entrance animation (`noAnim` swap), queued-message panel with clear, route-driven chat/settings/runs switch, Session-not-found state, version badge mount, turn-done/failed and sudo-request sounds off the stream, service worker cue plus exact-path navigate handling
 - `App.svelte` sidebar footer: the signed-in User and Sign out (desktop rail 2 and the drawer); the `user` and `onSignOut` props come from `Root.svelte`; unmount calls `stopLive`
- `App.svelte` Session header: `created by <User>` from `meta.owner`, read through the people store. The sidebar feeds `OwnerGroups` from `sessions`, `runs`, `projects` and the account list (refreshed on the `users` topic).
- `src/lib/router.js` — pure route parse/build (`/s/<id>` with `?at=`, `/runs[/<runId>[/<ticketId>]]`, `/settings`, `/quota`, `/memory`, `/` as last-viewed `eb.last` else newest); `App.svelte` owns pushState/popstate around it
- `src/lib/DraftBar.svelte` — harness, model and effort pickers for a new chat draft (effort for claude and pig)
- `src/lib/Transcript.svelte`, `src/lib/transcript.js` — TanStack-virtualized transcript (dynamic measurement, overscan, stick-to-bottom unless scrolled up with time-based glide, jump-to-latest) with a load-earlier sentinel (auto-load on scroll up, scroll-anchored, spinner, stops at `hasMore=false`); pure stick/anchor/seq helpers in `transcript.js`
- `src/lib/api.js` — Daemon client (configurable base URL), incl. Session snapshot paging params (`getSession(id, { tail, before, turns, at })`), settings read/write, spec list, ticket preview, Run start/get and Run SSE stream, per-ticket steer/retry/park/discard, Run pause/resume/rebase/abandon actions, build version read, prompt image ids, Session image upload/list/serve, and the Memory page calls (`getRules`, `getObservations`, `getRulesQueue`, `decideRuleQueue`, `batchRuleQueue`, `getRuleQueueStats`, `getMemoryLedger`, `getMemoryStatus`, `purgeMemory` alongside the profile calls)
- `src/lib/api.js` auth calls (`getAuthState`, `getMe`, `login`, `signup`, `logout`) and the `UNAUTHORIZED` window event that any 401 from a non-auth route fires
- `src/lib/live.js` — the one EventSource channel: `watch(topic, fn)` per topic; `stopLive` closes it and drops the watchers on sign-out
- `src/lib/version.js` — the bundle's own build hash, the Daemon version fetch, the stale predicate, the badge line and the clipboard copy
- `src/lib/VersionBadge.svelte` — fixed bottom-right build stamp with click-to-copy and the update-available reload pill
- `src/lib/events.js` — Event-to-card mapping and transcript grouping (one Group header block per spawn call; consecutive tool calls collapse into one live `toolstrip` block per burst except spawn/submit/sudo, which keep their own cards; idle/working status pills map to nothing)
- `events.js` user prompt cards carry `sender` and `senderId`; `Card.svelte` shows the sender badge only on `sender=user` prompts, so system prompts show none.
- `src/lib/composer.js` — composer mode (prompt / steer / follow-up)
- `src/lib/harnesses.js` — harness catalogue for the new-Session dialog, plus the per-harness card accent (claude terracotta, pig teal)
- `src/lib/subagents.js` — pure Subagent display helpers (name fallback, spawn payloads, latest-tool line, child state, Group counts)
- `src/lib/GroupCard.svelte` — live Group header card: one row per child (name, harness badge, type chip, latest-tool line, status dot) opening the child transcript
- `src/lib/Card.svelte` — transcript card for every Event kind, incl. user image chips, the Group header, the live tool strip and the memory injection divider (`events.js` maps `injection` to a `divider` card carrying the injected Rule IDs, never the raw JSON); the divider offers a Promote button per Rule (`api.js` `promoteSessionRule`) that files the promotion against the Session's worktree for queue approval
- `src/lib/ToolStrip.svelte`, `src/lib/toolstrip.js` — live tool strip: one card per burst of consecutive tool calls (status icon, tool glyph, server chip, target text, elapsed timer, progress hairline, swapping output stage capped at 232px, newest-first history drawer, settled `N tools · Xs` summary; per-tool file/diff/stream/JSON renderers; reduced-motion path); pure summary/target/format helpers in `toolstrip.js`
 - `src/lib/Rail.svelte` — narrow icon rail with the sliding active pill plus the rail-2 collapse toggle in its header
- `src/lib/OwnerGroups.svelte` — sidebar Session list: filter chips Mine, Everyone and Project (persisted `eb.owner-filter` and `eb.owner-project`; the Project picker uses `ui/select.svelte`); one collapsible group per User, own group first and open, others collapsed with a count and an online dot (`eb.owners-collapsed`); the group's left bar takes its owner color (`--owner-1` to `--owner-6` in `app.css`)
- `src/lib/owners.js` — pure owner grouping: `buildOwnerGroups` (groups, then Project sections, then tree roots with Runs and Subagents), `runRow` (a Run as a row), `dotState` (status dot), `ownerTone` (palette slot by account order), `projectIdForPath`
- `src/lib/people.svelte.js` — the account list from `GET /api/users` (`people`, `refreshPeople`, `personFor`); a failed refresh keeps the last good list
- `src/lib/UserAvatar.svelte` — a User's initial on its owner color, with a green dot while the User is online
- `src/lib/ProjectList.svelte` — one User group's Project sections: collapsible headers (persisted `eb.projects-collapsed`, keyed by User and Project), hover rename and remove, `+` starts a draft chat there; a chat filter force-opens every section
- `src/lib/SessionList.svelte` — Session rows of one Project section: root rows (Runs and root Sessions) with Subagents and Run Workers nested under a parent chevron (default collapsed once the Group is all done); Run rows carry the `Run` tag; needs-you rows get the amber treatment, and capped Sessions show a capped status.
- `src/lib/sidebar.js` — Session tree, row label, chat match and the persisted collapse prefs
- `src/lib/Composer.svelte` — floating composer with mode badge and a Stop button (`onStop`, `POST /interrupt`) while working, inline image tray plus the Session gallery (click an image to reference it by stable id; sends `imageIds` with the prompt), quiet voice tick on record start/stop (message sends stay silent)
- `src/lib/NewSessionDialog.svelte` — harness / model / working directory
- `src/lib/RunsSidebar.svelte`, `src/lib/runsDiscovery.svelte.js` — rail-2 sidebar for Runs: repo directory form, spec list with preview pick, needs-you strip, Run list newest-first; the shared `discovery` store (repo, specs, selected, preview, pick nonce, single-flight `ensureDiscoveryContext` cold-link seed) repaints `RunsView`
- `src/lib/RunsView.svelte` — picked spec ticket preview table, Run start plus live Tickets as a dependency graph (default) or table (List/Graph toggle) with Worker links to the Session view and per-Ticket reviewer verdict, tip freshness and refusal columns, plus the needs-you banner deep-linking each alerting ticket to its page; realtime from the Run SSE, no polling; deep-link props (`runId`, `ticketId`, `onNavigate`) load one Run, mounting the ticket page when a ticket is routed; needs-you/merged/complete sounds off snapshot transitions (first snapshot seeds silently); a sidebar spec pick drops a mismatched live Run
- `src/lib/RunsGraph.svelte`, `src/lib/runsGraph.js` — the Runs dependency graph: pure dagre layout (`@dagrejs/dagre`, fixed card footprint so SSE status flips never move nodes; blockers resolve by ticket id or number, unknown counts as unmerged; duplicate ids dedupe to one node) plus SVG edges (dashed into blocked nodes) and HTML cards, drag-pan and wheel-zoom with buttons and reset (one panning mechanism: the viewport clips and drag/keyboard move the canvas, plus keyboard pan/zoom), legend counts, card click deep-links to the ticket page; tones merged green, running blue, needs-you amber, blocked dim, parked/discarded muted, queued neutral; all app tokens from `src/app.css`
- `src/lib/TicketPage.svelte`, `src/lib/ticketPage.js` — ticket page mounted by `RunsView` at `/runs/<runId>/<ticketId>`: state track (queued, running, in review round N, merging, merged; needs-you/parked/discarded/paused as alert pills), needs-you banner plus the ticket action row (Steer inline form, Retry, Park/Unpark, two-click Discard, each answered with the fresh snapshot; hidden on merged/discarded tickets), worker transcript (reused `Transcript.svelte`) with review rounds woven in (two cards per round in gate order, same-tip pass as "pass held", all findings sent to the Worker with blocking marks and recorded Dispositions), right panel with agents list (worker opens in the Session view; reviewers and scouts open read-only in a drawer with no composer), live acceptance checkboxes, gate state and worker follow-ups; realtime from the Run SSE snapshot plus the worker Session stream, no polling; exact single-pending echo drops (`dropPendingEcho`), finished run/ticket closes the Run stream, load-earlier flags always reset, stale fetches never open streams; pure state-track/rounds/gate/agents/echo helpers in `ticketPage.js`
- `src/lib/SettingsPage.svelte` — role harness/model/effort groups plus the max_workers field, locked with a note under the environment override; mounts the Profile, Notifications, Install app and Sounds sections
- `src/lib/ProfileSettings.svelte` (`api.js` profile calls) — the Profile editor: live lines with add, edit and retire, the pending proposal queue with approve and reject, versions with restore; every approval writes a `profile_updated` Event
- `src/lib/MemoryPage.svelte` (`api.js` memory calls, `memory.js` normalizers) — the Memory page on `/memory`: engine status chip, stats (queue size, approval latency, last nightly run and ledger), Rules and observations list plus FTS5 search with a Promote button per Rule, the rule queue with quotes, source Sessions, approve, edit, reject and per-Session plus selected batch approve, the promotion approval queue with diffs, the reused Profile editor with versions, the ledger tail, and the purge section with the typed PURGE confirmation and the removed-counts report; loads once, buttons fetch again
- `src/lib/memory.js` — pure Memory page helpers: row normalizers that accept Go-case and lowercase keys, proposal parsing with the old remember-shape upgrade, per-Session queue grouping, purge counts and summaries, latency labels
- `src/lib/PromotionQueue.svelte` (`api.js` promotion calls) — the promotion approval queue embedded in the Memory page: each item shows its target path and diff preview with approve and reject; approving writes the worktree file and leaves it uncommitted for review
- `SettingsPage.svelte` Accounts section, admin only: the `Allow sign-up` switch saves `signup_enabled` with the rest of the Save bar; other Users never see or send the key
- `src/lib/stt.js` — voice input: `startRecording` records one take (1 s chunks, level meter for the live waveform, Screen Wake Lock held and re-requested on `visibilitychange`; a browser-ended take still yields the audio so far) and `transcribe` posts the whole recording to the Daemon `POST /api/stt` (OpenRouter gpt-transcribe, `EL_STT_MODEL` overrides; `?format=1` runs the mistral-small-24b cleanup pass, `EL_STT_FORMAT_MODEL` overrides; key stays on the Daemon: env or pig). The Composer shows a recorder bar (timer, waveform, cancel), then a draft voice bubble (`VoiceBubble.svelte`) while the transcript lands in the message box for editing before send; send blocks while transcribing. Sent, the recording uploads as `aud_` image id (peaks, duration) and renders as a voice bubble with the text as its transcript (button or hover). `voicePrefs.js` (`el.voice`: `enabled`, `cleanup`) backs the Settings `VoiceSettings.svelte` section; `voiceMeta.js` reads recording peaks from the image list. Needs https or localhost
- `src/lib/images.js` — image attachments: file picker and paste in the Composer, read to base64, sent through `postPrompt(id, text, { mode, images })`
- `Card.svelte` sudo cards (`sudo`, `sudoresult`): Approve/Deny with optional password; `events.js` merges `sudo_resolved` into the request card and parses the TOON `sudo_result:` prompt
- `src/lib/sound.js` — interface sounds: uisfx@0.4.0 cues (pack `minimal` default, all twelve packs vendored as mp3 under `public/sounds/<pack>/`), unlock on first pointerdown/keydown with pre-unlock drop, 400ms throttle with urgency override, keyed dedup across the live stream and service worker messages, `localStorage` prefs (`el.sound`: pack, volume, mute, per-event toggles), pure Run snapshot transition helper; `SoundSettings.svelte` is the settings section (pack picker plus preview, volume, mute, per-event toggles)
- `src/lib/Notifications.svelte`, `src/lib/push.js`, `public/sw.js` — push opt-in on the settings page, subscribe client, service worker; the push handler messages every open tab its cue and shows the OS notification only when no window is visible; notification taps open the exact `/s/<id>` or `/runs/<run>/<ticket>` route (focus + navigate message, else openWindow); the worker's fetch handler is network-first for the shell and never touches `/api/*`, so streams and version stay live
- `src/lib/ui/combobox.svelte` — picker (Popover + Command, virtualized list)
- `src/lib/ui/select.svelte` — plain bits-ui Select for simple fixed lists
- `src/lib/ui/slider.svelte`, `src/lib/ui/switch.svelte` — bits-ui Slider (single value, labelled thumb) and Switch for the Sounds section
- `src/lib/ui/button.svelte` — shadcn-svelte-idiom button variants
- `src/lib/InstallApp.svelte`, `src/lib/install.js` — settings Install app card: the `beforeinstallprompt` button plus the iOS Add-to-Home-Screen hint; pure standalone/iOS/state helpers in `install.js`
- `src/lib/pwa.js` — service worker registration at app boot (before push is ever enabled)
- `public/manifest.webmanifest`, `public/icons/`, `public/apple-touch-icon.png` — install metadata and artwork; icons come from `scripts/generate-icons.py` (from `public/logo.png`, outputs committed)

`npm test` runs vitest: Event-to-card mapping and grouping
(`events.test.js`), tool strip grouping, exclusions, summary and formatting
(`toolstrip.test.js`), strip header/summary/failure/history/stage-cap/reduced-motion/streaming
render (`ToolStrip.test.js`), composer mode resolvers (`composer.test.js`),
mounted Composer badge and send behavior (`Composer.test.js`), harness
model filtering (`harnesses.test.js`), the virtualized picker open/filter/pick,
keyboard, empty state and windowing (`ui/combobox.test.js`), Card rendering per kind
plus the echoed-prompt no-animation swap (`Card.test.js`), Runs spec-discovery
form, list and pick plus the Run list picker (newest first, counts, navigate)
and the needs-you strip (`RunsSidebar.test.js`), picked preview, Start, live graph
default/toggle with card deep-link, live table with Worker links and reviewer
verdict columns, the needs-you banner, spec branch distance, checkpoint rebase,
confirmed abandon and SSE-only Run updates (`RunsView.test.js`), graph layout,
state mapping, edges, cyclic graphs, duplicate-id dedupe and legend counts
(`runsGraph.test.js`), graph cards, legend,
select, zoom, keyboard pan, duplicate ids and wheel-listener cleanup (`RunsGraph.test.js`),
ticket state mapping, rounds and dispositions, gate, exact echo drops and finished helpers
(`ticketPage.test.js`), the mounted ticket page (state track, rounds, finding blocking
marks and Dispositions, read-only reviewer drawer, SSE box ticks, needs-you,
per-ticket steer/retry/park/discard buttons with snapshot apply, stream close on
unmount/key change, finished-run close, reconnecting indicator, brief expand
against a mocked API and Run stream (`TicketPage.test.js`), install state helpers (`install.test.js`), the mounted install card (button on prompt, iOS hint, prompt-on-click against a stubbed event; `InstallApp.test.js`), sound player logic, throttle, unlock, keyed dedup and Run transitions (`sound.test.js`), the Sounds section against a fake player (`SoundSettings.test.js`), version fetch, staleness and copy (`version.test.js`), badge render, reload pill and copy flash (`VersionBadge.test.js`), settings max_workers edit, validation and locked state against a mocked API (`SettingsPage.test.js`), Subagent display helpers (`subagents.test.js`), the live Group card against a mocked API (`GroupCard.test.js`), route parse/build with the Memory route and popstate entries (`router.test.js`), stick-to-bottom, scroll anchoring, seq lookup and load-earlier paging against a mocked API (`transcript.test.js`), Composer stored image references, sidebar navigation to Runs and Memory, rail collapse persistence, the Ctrl+B shortcut and the drawer open/close (`App.test.js`), owner groups, filters and the chat query (`owners.test.js`, `OwnerGroups.test.js`), Project sections and their collapse (`ProjectList.test.js`), Session rows, nesting, the Run tag and needs-you rows (`SessionList.test.js`), the sidebar tree, labels and prefs (`sidebar.test.js`), the rail collapse toggle plus the Memory area (`Rail.test.js`), the Memory page list, search, queue actions, versions, purge and status (`MemoryPage.test.js`, `memory.test.js`), the account list (`people.test.js`), the sender badge on user prompts (`Card.test.js`, `events.test.js`), the promotion approval queue with diffs and decisions (`PromotionQueue.test.js`), the chat promote buttons on injection dividers (`Card.test.js`, `events.test.js`), the Memory page per-rule Promote button (`MemoryPage.test.js`).
Sign-in and accounts: `authForm.test.js` (mode, copy and error lines), `AuthScreen.test.js` (admin, login, signup, the create-account link and server answers), `Root.test.js` (the gate, footer sign out, 401 return, retry), `api.test.js` (auth client and the 401 signal, promotions client), `live.test.js` (`stopLive`), plus the App footer (`App.test.js`) and the Accounts switch (`SettingsPage.test.js`).

## Routes

Real paths, served from `index.html` by the Daemon SPA fallback (missing
assets and `/api/*` still 404): `/s/<id>` Session (optional `?at=<seq>`
scroll target), `/runs`, `/runs/<runId>`, `/runs/<runId>/<ticketId>`,
`/settings`, `/quota`, `/memory`, `/` (last viewed from `eb.last`, else newest). Unknown ids show
Session-not-found, never a random Session. The build uses vite base `'/'`
with root-absolute icon, service worker and asset URLs so nested paths load.

## Installable app

The UI installs as a phone app (PWA): `index.html` links
`manifest.webmanifest` (Emberloop, standalone, `#212121`) and the iOS meta
(`apple-touch-icon`, `mobile-web-app-capable`, `viewport-fit=cover`), and
`main.js` registers `sw.js` on boot. The worker caches the shell
network-first and never caches `/api/*`, so Session streams and the Daemon
version badge stay correct; the Daemon serves the manifest as
`application/manifest+json` and `sw.js` with `Service-Worker-Allowed: /` and
`no-store`. The settings Install app card sits next to Notifications and
shows the `beforeinstallprompt` button, or the Share-then-Add-to-Home-Screen
steps on iOS. Install and push both need a secure context: serve the Daemon
over https (`tailscale serve`, with the Daemon's `-allow-host` for the public
 name) or open it on localhost. The floating composer pads for the home
 indicator (`env(safe-area-inset-bottom)`); the phone drawer (`Sheet`) pads
 the top and bottom insets the same way.
