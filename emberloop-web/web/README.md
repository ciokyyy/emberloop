# UI

Svelte 5 + Vite single-page app, built to static files. `npm run build` in
`web/` writes `web/dist/`; the Daemon embeds `web/dist` and serves `/`,
`/index.html` and the bundle assets (JS, CSS, favicon, manifest, service
worker, icons) at `/` with PWA headers (see Installable app). No server
rendering anywhere, so the same build wraps in Tauri later unchanged.

## Design rules

Follow these for every UI change.

 - Shell: `Rail.svelte` is the narrow icon rail (Chat, Specs and Runs,
   Settings) with the second-sidebar collapse toggle in its header
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

 - `src/App.svelte` — layout, rail-1 icon rail (`Rail.svelte`) plus a contextual rail-2 sidebar (collapses to the rail on desktop via the rail-header toggle and Ctrl/⌘+B, persisted as `eb.sidebar-collapsed`; the same content renders in the phone drawer), Session tail loading (`?tail=6`, `?at=`) with load-earlier prepend, SSE stream with reconnect, pending prompt cards that the echoed prompt Event replaces with no second entrance animation (`noAnim` swap), queued-message panel with clear, route-driven chat/settings/runs switch, Session-not-found state, version badge mount, turn-done/failed and sudo-request sounds off the stream, service worker cue plus exact-path navigate handling
- `src/lib/router.js` — pure route parse/build (`/s/<id>` with `?at=`, `/runs[/<runId>[/<ticketId>]]`, `/settings`, `/` as last-viewed `eb.last` else newest); `App.svelte` owns pushState/popstate around it
- `src/lib/DraftBar.svelte` — harness, model and effort pickers for a new chat draft (effort for claude and pig)
- `src/lib/Transcript.svelte`, `src/lib/transcript.js` — TanStack-virtualized transcript (dynamic measurement, overscan, stick-to-bottom unless scrolled up with time-based glide, jump-to-latest) with a load-earlier sentinel (auto-load on scroll up, scroll-anchored, spinner, stops at `hasMore=false`); pure stick/anchor/seq helpers in `transcript.js`
- `src/lib/api.js` — Daemon client (configurable base URL), incl. Session snapshot paging params (`getSession(id, { tail, before, turns, at })`), settings read/write, spec list, ticket preview, Run start/get and Run SSE stream, per-ticket steer/retry/park/discard, build version read, prompt image ids, and Session image upload/list/serve
- `src/lib/version.js` — the bundle's own build hash, the Daemon version fetch, the stale predicate, the badge line and the clipboard copy
- `src/lib/VersionBadge.svelte` — fixed bottom-right build stamp with click-to-copy and the update-available reload pill
- `src/lib/events.js` — Event-to-card mapping and transcript grouping (one Group header block per spawn call; consecutive tool calls collapse into one live `toolstrip` block per burst except spawn/submit/sudo, which keep their own cards; idle/working status pills map to nothing)
- `src/lib/composer.js` — composer mode (prompt / steer / follow-up)
- `src/lib/harnesses.js` — harness catalogue for the new-Session dialog, plus the per-harness card accent (claude terracotta, pig teal)
- `src/lib/subagents.js` — pure Subagent display helpers (name fallback, spawn payloads, latest-tool line, child state, Group counts)
- `src/lib/GroupCard.svelte` — live Group header card: one row per child (name, harness badge, type chip, latest-tool line, status dot) opening the child transcript
- `src/lib/Card.svelte` — transcript card for every Event kind, incl. user image chips, the Group header and the live tool strip
- `src/lib/ToolStrip.svelte`, `src/lib/toolstrip.js` — live tool strip: one card per burst of consecutive tool calls (status icon, tool glyph, server chip, target text, elapsed timer, progress hairline, swapping output stage capped at 232px, newest-first history drawer, settled `N tools · Xs` summary; per-tool file/diff/stream/JSON renderers; reduced-motion path); pure summary/target/format helpers in `toolstrip.js`
 - `src/lib/Rail.svelte` — narrow icon rail with the sliding active pill plus the rail-2 collapse toggle in its header
 - `src/lib/ProjectList.svelte` — project groups with collapsible headers (persisted `eb.collapsed`), hover rename/remove, and the chat filter (`query` prop, matching projects force-open)
 - `src/lib/SessionList.svelte` — Session tree tiered by status (Needs you / Working / Recent with collapsible counts, quiet rows except needs-you; single-tier lists render headerless) with Subagents nested under a parent chevron (default collapsed once the Group is all done); pure tier/tree/filter/collapse helpers in `src/lib/sidebar.js`
- `src/lib/Composer.svelte` — floating composer with mode badge and a Stop button (`onStop`, `POST /interrupt`) while working, inline image tray plus the Session gallery (click an image to reference it by stable id; sends `imageIds` with the prompt), quiet voice tick on record start/stop (message sends stay silent)
- `src/lib/NewSessionDialog.svelte` — harness / model / working directory
- `src/lib/RunsSidebar.svelte`, `src/lib/runsDiscovery.svelte.js` — rail-2 sidebar for Runs: repo directory form, spec list with preview pick, needs-you strip, Run list newest-first; the shared `discovery` store (repo, specs, selected, preview, pick nonce, single-flight `ensureDiscoveryContext` cold-link seed) repaints `RunsView`
- `src/lib/RunsView.svelte` — picked spec ticket preview table, Run start plus live Tickets as a dependency graph (default) or table (List/Graph toggle) with Worker links to the Session view and per-Ticket reviewer verdict, tip freshness and refusal columns, plus the needs-you banner deep-linking each alerting ticket to its page; realtime from the Run SSE, no polling; deep-link props (`runId`, `ticketId`, `onNavigate`) load one Run, mounting the ticket page when a ticket is routed; needs-you/merged/complete sounds off snapshot transitions (first snapshot seeds silently); a sidebar spec pick drops a mismatched live Run
- `src/lib/RunsGraph.svelte`, `src/lib/runsGraph.js` — the Runs dependency graph: pure dagre layout (`@dagrejs/dagre`, fixed card footprint so SSE status flips never move nodes; blockers resolve by ticket id or number, unknown counts as unmerged; duplicate ids dedupe to one node) plus SVG edges (dashed into blocked nodes) and HTML cards, drag-pan and wheel-zoom with buttons and reset (one panning mechanism: the viewport clips and drag/keyboard move the canvas, plus keyboard pan/zoom), legend counts, card click deep-links to the ticket page; tones merged green, running blue, needs-you amber, blocked dim, parked/discarded muted, queued neutral; all app tokens from `src/app.css`
- `src/lib/TicketPage.svelte`, `src/lib/ticketPage.js` — ticket page mounted by `RunsView` at `/runs/<runId>/<ticketId>`: state track (queued, running, in review round N, merging, merged; needs-you/parked/discarded/paused as alert pills), needs-you banner plus the ticket action row (Steer inline form, Retry, Park/Unpark, two-click Discard, each answered with the fresh snapshot; hidden on merged/discarded tickets), worker transcript (reused `Transcript.svelte`) with review rounds woven in (two cards per round in gate order, same-tip pass as "pass held", declined-with-reason card), right panel with agents list (worker opens in the Session view; reviewers and scouts open read-only in a drawer with no composer), live acceptance checkboxes, gate state and worker follow-ups; realtime from the Run SSE snapshot plus the worker Session stream, no polling; exact single-pending echo drops (`dropPendingEcho`), finished run/ticket closes the Run stream, load-earlier flags always reset, stale fetches never open streams; pure state-track/rounds/gate/agents/echo helpers in `ticketPage.js`
- `src/lib/SettingsPage.svelte` — role harness/model/effort groups plus the max_workers field, locked with a note under the environment override; mounts the Notifications, Install app and Sounds sections
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
default/toggle with card deep-link, live table with Worker links plus the
reviewer verdict columns, the needs-you banner with ticket deep-links, the cold deep-link sidebar seeding via
`ensureDiscoveryContext` (single-flight, no repo clobber), stale-navigation
races, stream close on unmount and SSE-only live updates against a mocked API
and Run stream (`RunsView.test.js`), graph layout, state mapping, edges, cyclic
graphs, duplicate-id dedupe and legend counts (`runsGraph.test.js`), graph cards, legend,
select, zoom, keyboard pan, duplicate ids and wheel-listener cleanup (`RunsGraph.test.js`),
ticket state mapping, rounds, gate, exact echo drops and finished helpers
(`ticketPage.test.js`), the mounted ticket page (state track, rounds,
read-only reviewer drawer, SSE box ticks, needs-you, per-ticket steer/retry/park/discard
buttons with snapshot apply, stream close on unmount/key change, finished-run close,
reconnecting indicator, brief expand)
 against a mocked API and Run stream (`TicketPage.test.js`), install state helpers (`install.test.js`), the mounted install card (button on prompt, iOS hint, prompt-on-click against a stubbed event; `InstallApp.test.js`), sound player logic, throttle, unlock, keyed dedup and Run transitions (`sound.test.js`), the Sounds section against a fake player (`SoundSettings.test.js`), version fetch, staleness and copy (`version.test.js`), badge render, reload pill and copy flash (`VersionBadge.test.js`), settings max_workers edit, validation and locked state against a mocked API (`SettingsPage.test.js`), Subagent display helpers (`subagents.test.js`), the live Group card against a mocked API (`GroupCard.test.js`), route parse/build and popstate entries (`router.test.js`), stick-to-bottom, scroll anchoring, seq lookup and load-earlier paging against a mocked API (`transcript.test.js`), Composer stored image references, sidebar navigation to Runs, rail collapse persistence, the Ctrl+B shortcut and the drawer open/close (`App.test.js`), attention tiers, section collapse and nested subagents (`SessionList.test.js`), the chat filter (`ProjectList.test.js`), the rail collapse toggle (`Rail.test.js`), sidebar tier/tree/filter/collapse helpers (`sidebar.test.js`).

## Routes

Real paths, served from `index.html` by the Daemon SPA fallback (missing
assets and `/api/*` still 404): `/s/<id>` Session (optional `?at=<seq>`
scroll target), `/runs`, `/runs/<runId>`, `/runs/<runId>/<ticketId>`,
`/settings`, `/` (last viewed from `eb.last`, else newest). Unknown ids show
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
