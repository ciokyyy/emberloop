# UI

Svelte 5 + Vite 8 (with `@sveltejs/vite-plugin-svelte` 7) single-page app, built to static files plus an SSR bundle. `npm run build` in
`web/` writes `web/dist/`; the Daemon embeds `web/dist` and serves `/`,
`/index.html` and the bundle assets (JS, CSS, favicon, manifest, service
worker, icons) at `/` with PWA headers (see Installable app). `dist/ssr/ssr.js`
is the bundled Dashboard SSR entry; `dist/ssr/hydrate.iife.js` hydrates it in
the browser test. Neither is invoked by request handling. The browser-only build
remains standalone for Tauri.

## Design rules

Follow these for every UI change.

- Shell: `Rail.svelte` is the narrow icon rail (Chat, Specs and Runs, Memory, Quota and Providers, Settings) with the second-sidebar collapse toggle in its header (Ctrl/⌘+B, persisted); a contextual second sidebar fills the next column. `runsStore.js` shares one summary list and coalesces `runs` topic bursts for both `App.svelte` and `RunsSidebar.svelte`; the sidebar owns Runs spec discovery and renders the summary-only list from `GET /api/runs`, while deep links seed the picker from summaries. `QuotaSidebar.svelte` owns the Quota and Providers entries and Live status list. Quota and Providers are global (no Project switcher) and keep their sidebar available on desktop, respecting an explicit user collapse. The Runs sidebar collapses to the rail on desktop (width transition on `--ease-smooth`) and becomes an off-canvas drawer on `<md` screens (top-bar menu button, closes on navigate, safe-area padding); Quota and Providers render the same sidebar in the phone drawer. The Runs main area owns the preview, Start and selected Run detail (`GET /api/runs/<id>`) with its live Ticket table/graph. Tokens live in `src/app.css`; never hardcode colors.
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
- Phones (`<md`): every `Button`/`Input`/`Combobox` is a 44px target
  (`max-md:` sizes in `src/lib/ui/`), text inputs are 16px (no iOS focus
  zoom), switches and checkboxes carry a 44px hit area, popovers never
  exceed `100vw - 1rem`. The drawer is one `Sheet`: a header (User, Sign out,
  Close), one scroll region for the contextual list, a pinned `DrawerNav`
  tab row (areas from `src/lib/areas.js`) and an inline build stamp. It owns
  one transient history entry while open, so system Back closes it; a
  navigation from it replaces that entry. Runs open the ticket table, not
  the graph, on phones.
- Models show display names (`modelLabel` in `harnesses.js`); the Daemon owns
  the model list (`GET /api/models`).
- Dev: `bun run dev` runs the Daemon (127.0.0.1:8910) and Vite (5190, proxies `/api`) together. Prod: `bun run prod` builds, then runs the Daemon (8920) and `vite preview` (8921, proxies `/api`). Override with `API_PORT` / `UI_PORT` per script.

## API base URL

Same-origin by default. To point the static build at another Daemon:

- build time: `VITE_API_BASE=http://127.0.0.1:PORT npm run build`
- runtime: set `window.__EL_API_BASE__` before the bundle loads

## Source map

- `src/ssr.js`, `src/ssr-hydrate.js` — Dashboard SSR entry and browser hydration probe for the embedded QuickJS spike; production requests do not invoke this entry yet.
 - `src/Root.svelte` — sign-in gate: probes `GET /api/auth/me`; signed out shows `AuthScreen`, signed in mounts `App` with the User; sign out and any 401 from a non-auth route return to sign-in; an unreachable Daemon offers Try again
 - `src/lib/AuthScreen.svelte`, `src/lib/authForm.js` — sign-in, create-account and first-admin screen on `ui/` primitives; `authForm.js` holds the mode, copy and error-line helpers
- `src/App.svelte` — app shell and route owner: the global rail and contextual sidebar, Project switcher, active-work Dashboard, Project-scoped Sessions and Runs, and global Settings, Quota, Providers and Memory; Settings has a Workspace/You hub on phones and one route per concern, with a shared SettingsNav in the sidebar and drawer; Session tail/SSE loading, queued and optimistic prompts, Git panel, sounds, service-worker navigation and sign-out; projectless draft prompts ask Jev before creating a Session and hold the first message for the Project-pick banner; stream chunks coalesce at ~80ms while structural events flush immediately.
- `App.svelte` boot — resolves the URL route before list reads. `/` fetches the active-only Dashboard payload without loading Session or Run lists; `/p/<id>` and `/p/none` load scoped Sessions, and `/s/<id>` stays canonical while deriving the sidebar scope from Session metadata. `/p/<id>/runs` loads scoped summaries and discovers/starts Runs in the Project path without a folder field. `/p/<id>/settings` provides Project rename, removal and gist editing with no override fields. `/settings` shows the hub on phones and replaces to `/settings/agents` on desktop; `/settings/{agents,runs,context,memory,access,profile,notifications,sound-voice}` are independent pages. Quota, Providers, Memory and account Settings stay global. Session events while another area is open mark the list stale; returning to a Project scope refreshes before selecting a Session.
   Same 40-Run × 8-Ticket Vitest fixture, 5 trials after 2 warmups: cold boot
   p50 to the Runs sidebar shell is 41.5ms and to all rows is 195.9ms. Ticket
   17's post-summary p50 was 170.3/369.7ms; its pre-implementation baseline
   was 516.2/610.7ms.
 - `App.svelte` sidebar footer: the signed-in User and Sign out (desktop rail 2 and the drawer); the `user` and `onSignOut` props come from `Root.svelte`; unmount calls `stopLive`
- `App.svelte` Session header: `created by <User>` from `meta.owner`, read through the people store. The sidebar feeds `OwnerGroups` from `sessions`, shared-store Run summaries (owner, Project id and aggregate needs-you count), `projects` and the account list (refreshed on the `users` topic).
- `src/lib/router.js` — pure route parse/build for `/`, `/p/<id>`, `/p/none`, `/p/<id>/runs[/<runId>[/<ticketId>]]`, `/p/<id>/settings`, canonical `/s/<id>?at=`, legacy `/runs` details, `/settings` and its concern routes, `/quota`, `/providers` and `/memory`; App owns pushState/popstate.
- `src/lib/ProjectSwitcher.svelte` — the stable-id scope picker: All projects maps to `/`, No project to `/p/none`, and renamed Project labels remain display-only; the same picker is used in the phone drawer.
- `src/lib/DashboardPage.svelte` — grouped active Sessions and Runs from `GET /api/dashboard`; refreshes on Session/Run live invalidations and builds Session links canonically and Run links with the group's stable Project id.
- `src/lib/ProjectSettingsPage.svelte` — Project name and one-line gist editors with explicit saves, Project removal, path display and a return to the scoped Sessions view; no per-Project overrides.
- `src/lib/QuotaPage.svelte`, `src/lib/quota.js` — quota cards render the
  Daemon's percent-used windows, word labels, plan/detail and provider state;
  Claude uses a read-only host OAuth poller with statusline fallback, and the UI
  never derives percentages from money.
 - `src/lib/ProvidersPage.svelte` — global pig provider status grouped as subscription logins, API keys and CLI-only; OAuth rows without a browser flow show the exact `pig login <provider>` command and copy button, while Bedrock, Vertex and Cloudflare cloud-auth rows say `configure via CLI`. Stored key values never enter the response or page, only a `keySet` flag. Admins can set/remove supported API keys, connect ChatGPT with a short-lived device-code session showing its verification link and user code, connect Claude through a Daemon PTY running `claude auth login` (Claude Code writes only the host `~/.claude` login), and connect Antigravity by opening its login link and pasting a callback URL or code; invalid Antigravity input stays in the open panel for correction. Successful ChatGPT, Antigravity, Claude and quota-backed API-key connections refresh and publish their matching quota data; other Users see read-only status.
- `src/lib/DraftBar.svelte` — harness, model and effort pickers for a new chat draft (effort for claude and pig); `App.svelte` owns the first-message Project suggestion banner and Yes/No decision.
 - `src/lib/Transcript.svelte`, `src/lib/transcript.js` — TanStack-virtualized transcript (throttled dynamic measurement at ~5Hz or >32px deltas, overscan, `content-visibility:auto` rows, instant `scroll-behavior` on the scroller so the time-based glide loop runs alone, stick-to-bottom unless scrolled up, jump-to-latest) with a load-earlier sentinel (auto-load on scroll up, scroll-anchored, spinner, stops at `hasMore=false`); pure stick/anchor/seq helpers in `transcript.js`
- `src/lib/api.js` — Daemon client (configurable base URL), including Session/Run/Project/Settings calls, Memory operations (`getMemories`, `getNewMemoryCount`, `forgetMemory`, `parkMemory`, `pinMemory`, `promoteMemoryRule`, `editMemory`, Legacy observation promotion and Episode listing), existing Rules/queue/Profile/purge calls, provider status/auth and quota APIs. `MemoryPage.svelte` renders the Memories, Legacy observations and read-only Episodes tabs; its per-User browser visit marker advances only after the initial reads succeed, while an independent count endpoint supplies the full new-since-last-visit total.
- `src/lib/api.js` auth calls (`getAuthState`, `getMe`, `login`, `signup`, `logout`) and the `UNAUTHORIZED` window event that any 401 from a non-auth route fires
- `src/lib/live.js` — one stable EventSource for ordinary topic invalidations plus an opt-in `topic=quota` stream while quota has watchers; quota events carry the provider list, and `stopLive` closes both streams on sign-out
- `src/lib/version.js` — the bundle's own build hash, the Daemon version fetch, the stale predicate, the badge line and the clipboard copy
- `src/lib/VersionBadge.svelte` — fixed bottom-right build stamp with click-to-copy and the update-available reload pill (`sm` and up); on phones a top update pill, and the `inline` stamp in the drawer
- `src/lib/DiskBanner.svelte` — one-line low-disk warning below the Session header, fed by `getDisk` on mount and the `disk` live topic (amber tokens, `animate-rise`); renders nothing when the disk is fine
 - `src/lib/events.js` — Event-to-card mapping and transcript grouping (one Group header block per spawn call; consecutive tool calls collapse into one live `toolstrip` block per burst except spawn/submit/sudo, which keep their own cards; idle/working status pills map to nothing). `groupTranscriptCached`/`latestUsageCached` keep the settled prefix (same identities, tail-only replace) across chunk bursts so settled cards never re-render; any prepend or turn edge falls back to one full regroup
- `events.js` user prompt cards carry `sender` and `senderId`; `Card.svelte` shows the sender badge only on `sender=user` prompts, so system prompts show none.
- `src/lib/composer.js` — composer mode (prompt / steer / follow-up)
- `src/lib/harnesses.js` — harness catalogue for the new-Session dialog, plus the per-harness card accent (claude terracotta, pig teal)
- `src/lib/subagents.js` — pure Subagent display helpers (name fallback, spawn payloads, latest-tool line, child state, Group counts)
- `src/lib/GroupCard.svelte` — live Group header card: one row per child (name, harness badge, type chip, latest-tool line, status dot) opening the child transcript
- `src/lib/Card.svelte` — transcript card for every Event kind, incl. user image chips, the Group header, the live tool strip and the memory injection divider (`events.js` maps `injection` to a `divider` card carrying the injected Rule IDs, never the raw JSON); the divider offers a Promote button per Rule (`api.js` `promoteSessionRule`) that files the promotion against the Session's worktree for queue approval. A live thinking block (`live` from the cached grouping) appends text only (`appendText.js`) with the `details` height animation off, so the mount-only rise animation never re-triggers; every card keeps `animate-rise` on mount
- `src/lib/ToolStrip.svelte`, `src/lib/toolstrip.js` — live tool strip: one card per burst of consecutive tool calls (status icon, tool glyph, server chip, target text, elapsed timer, progress hairline, swapping output stage capped at 232px, newest-first history drawer, settled `N tools · Xs` summary; per-tool file/diff/stream/JSON renderers; reduced-motion path). While live the stage renders the last 200 output lines with per-line entrances off and memoized per-line highlights (`memoSegs`), and the stage height moves only on tool switch, >48px jumps or 300ms; settled and history rows keep every line with the mount animation; shell/output highlighters decode one JSON-escape layer first (`decodeJsonEscapes` in `lib/highlight.js`: quoted `"..."` lines parse as JSON, unquoted fragments decode only on `\uXXXX`, so real backslashes pass through); pure summary/target/format helpers in `toolstrip.js`
- `src/lib/Markdown.svelte`, `src/lib/markdown.js`, `src/lib/streamFade.js`, `src/lib/pace.js`, `src/lib/appendText.js` — streamed agent text: paced reveal with a bounded backlog (`MAX_BACKLOG`, so fast models catch up), renders throttled to ~5Hz over the incremental head/tail renderer (settled blocks parse once, exact full render at settle), `stream-in` fade for slow streams only (skipped past `SKIP_FADE_BACKLOG`), append-only thinking text; each fenced block gets a delegated copy button (shadcn ghost icon styling, 44px touch hit area, lucide Copy/Check shapes, `Copied` feedback, `execCommand` fallback for plain-http tailnet; the `<pre>` anchor class comes from `markdown.js`); reduced motion shows everything at once
 - `src/lib/areas.js` — the five app areas (id, label, short label, path, icon) shared by `Rail.svelte` and `DrawerNav.svelte`
- `src/lib/DrawerNav.svelte` — phone drawer tab row switching the app area (44px-plus targets, active pill)
- `src/lib/Rail.svelte` — narrow icon rail with the sliding active pill plus the rail-2 collapse toggle in its header
- `src/lib/OwnerGroups.svelte` — sidebar Session list: filter chips Mine, Everyone and Project (persisted `eb.owner-filter` and `eb.owner-project`; the Project picker uses `ui/select.svelte`); one collapsible group per User, own group first and open, others collapsed with a count and an online dot (`eb.owners-collapsed`); the group's left bar takes its owner color (`--owner-1` to `--owner-6` in `app.css`)
- `src/lib/owners.js` — pure owner grouping: `buildOwnerGroups` (groups, then Project sections, then tree roots with Runs and Subagents), `runRow` (maps Run summary counts and Project id into a sidebar row), `dotState` (status dot), `ownerTone` (palette slot by account order), `projectIdForPath`
- `src/lib/people.svelte.js` — the account list from `GET /api/users` (`people`, `refreshPeople`, `personFor`); a failed refresh keeps the last good list
- `src/lib/UserAvatar.svelte` — a User's initial on its owner color, with a green dot while the User is online
- `src/lib/ProjectList.svelte` — one User group's Project sections: collapsible headers (persisted `eb.projects-collapsed`, keyed by User and Project), hover rename and remove, `+` starts a draft chat there; a chat filter force-opens every section
- `src/lib/SessionList.svelte` — Session rows of one Project section: root rows (Runs and root Sessions) with Subagents and Run Workers nested under a parent chevron (default collapsed once the Group is all done); Run rows carry the `Run` tag; needs-you rows take precedence in the status dot and get the amber treatment, while capped Sessions show a capped status.
- `src/lib/sidebar.js` — Session tree, row label, chat match and the persisted collapse prefs
- `src/lib/Composer.svelte` — floating composer with mode badge and a Stop button (`onStop`, `POST /interrupt`) while working, inline image tray plus the Session gallery (click an image to reference it by stable id; sends `imageIds` with the prompt), quiet voice tick on record start/stop (message sends stay silent)
- `src/lib/NewSessionDialog.svelte` — harness / model / working directory
- `src/lib/RunsSidebar.svelte`, `src/lib/runsDiscovery.svelte.js`, `src/lib/runsStore.js` — rail-2 Runs sidebar with spec list, preview pick, needs-you strip and newest-first Run list; global Runs retain the repo directory form, while `/p/<id>/runs` binds discovery to the Project path and hides the folder field; `projectsLoaded` gates the unavailable-path message until project loading finishes. The shared `runsStore.js` supplies one summary list to App and RunsSidebar, coalescing Run-topic bursts into one request; the `discovery` store (repo, specs, selected, preview, pick nonce, single-flight `ensureDiscoveryContext` cold-link seed) repaints `RunsView`.
- `src/lib/RunsView.svelte` — picked spec ticket preview table, Run start plus live Tickets as a dependency graph (default) or table (List/Graph toggle); Project Runs always start in their Project path. The view includes each role's active Chain entry and Worker links; per-Ticket and Run Pause/Resume, Backend/model Switch pickers on paused items, per-Ticket reviewer verdict, tip freshness and refusal columns; ticket needs-you deep-links and final-gate needs-you lists changed files with Resume; realtime from Run SSE, no polling; deep-link props (`runId`, `ticketId`, `onNavigate`) load one Run and mount its ticket page; needs-you/merged/complete sounds off snapshot transitions (first snapshot seeds silently); a sidebar spec pick drops a mismatched live Run.
- `src/lib/RunsGraph.svelte`, `src/lib/runsGraph.js` — the Runs dependency graph: pure dagre layout (`@dagrejs/dagre`, fixed card footprint with all active Chain entries so SSE status flips never move nodes; blockers resolve by ticket id or number, unknown counts as unmerged; duplicate ids dedupe to one node) plus SVG edges (dashed into blocked nodes) and HTML cards, drag-pan and wheel-zoom with buttons and reset (one panning mechanism: the viewport clips and drag/keyboard move the canvas, plus keyboard pan/zoom), legend counts including paused Tickets, card click deep-links to the ticket page; tones merged green, running blue, paused/needs-you amber, blocked dim, parked/discarded muted, queued neutral; all app tokens from `src/app.css`
- `src/lib/TicketPage.svelte`, `src/lib/ticketPage.js` — ticket page mounted by `RunsView` at `/runs/<runId>/<ticketId>`: state track (queued, running, in review round N, merging, merged; needs-you/parked/discarded/paused as alert pills), needs-you banner plus the ticket action row (Steer inline form, Retry, Park/Unpark, two-click Discard, each answered with the fresh snapshot; hidden on merged/discarded tickets), worker transcript (reused `Transcript.svelte`) with review rounds woven in (two cards per round in gate order, same-tip pass as "pass held", all findings sent to the Worker with blocking marks and recorded Dispositions), right panel with agents list (worker opens in the Session view; reviewers and scouts open read-only in a drawer with no composer), live acceptance checkboxes, gate state and worker follow-ups; realtime from the Run SSE snapshot plus the worker Session stream, no polling; exact single-pending echo drops (`dropPendingEcho`), finished run/ticket closes the Run stream, load-earlier flags always reset, stale fetches never open streams; pure state-track/rounds/gate/agents/echo helpers in `ticketPage.js`
- `src/lib/RunSwitchPicker.svelte` — Backend and model picker for a paused Run or Ticket; loads model options by Backend, states that the selection applies to the Worker and Reviewer Chains, and invokes the matching Switch action
- `src/lib/SettingsPage.svelte` — route-selected Agents, Runs, Context & names, Memory and Users & access configuration; Agents groups Session and Run roles into one ordered chain each, with up to three harness/model/effort entries and model-supported effort choices from `/api/models`; legacy role and Run Chain settings migrate into the merged form. Runs labels the global cap “Maximum tickets per Run (spec)” and explains that `max_tickets` caps each Run while a spec value can only lower it. Memory uses a segmented decider and collapsed custom Jev endpoint; an empty endpoint means OpenRouter in Jev mode, while Ensemble keeps its memory-model default unless a custom endpoint is set. Existing Profile, Notifications and device-local Sound & voice components stay on their own pages; data-backed settings share a first-change Revert/Save bar. `SettingsHub.svelte`, `SettingsNav.svelte` and `settings.js` provide the phone hub, grouped navigation, scope labels and route metadata.
- `src/lib/ContextMeter.svelte` — composer-footer context ring: model name plus used/window/percent from `latestUsage(events)`; always renders once a Session is open (read-only with no Compress button on scout, ended and unresumable Sessions), row never wraps (`whitespace-nowrap`, window hidden below `sm`)
- `src/lib/ProfileSettings.svelte` (`api.js` profile calls) — the Profile editor: live lines with add, edit and retire, the pending proposal queue with approve and reject, versions with restore; every approval writes a `profile_updated` Event
- `src/lib/MemoryPage.svelte` (`api.js` memory calls, `memory.js` normalizers) — the Memory page on `/memory`: engine status chip, stats (queue size, approval latency, last nightly run and ledger), Rules and observations list plus FTS5 search with a Promote button per Rule, the rule queue with quotes, source Sessions, approve, edit, reject and per-Session plus selected batch approve, the promotion approval queue with diffs, the reused Profile editor with versions, the ledger tail, and the purge section with the typed PURGE confirmation and the removed-counts report; loads once, buttons fetch again
- `src/lib/QuotaPage.svelte`, `src/lib/QuotaSidebar.svelte`, `src/lib/quotaStore.js`
  and `src/lib/quota.js` — the read-only `/quota` page reads
  `GET /api/quota`'s `{providers: [...]}` cache response. Each provider has
  `key`, `label`, `plan`, `state` (`ok`, `stale`, `expired`, `not_configured`,
  `error`), `updated_at`, `error`, optional `hint` and `windows[]`; each window
  has a word-form `label`, integer 0–100 percent-used `pct`, `resets_at` and
  optional `detail`. Cards cover Claude, ChatGPT Plus, OpenCode-Go, OpenRouter
  and Antigravity's Gemini (`agy-gemini`) and Claude + GPT-OSS (`agy-3p`) pools.
  Cards show percent used first; bars and Live status rows use only `pct`. The
  UI never computes a percent from dollars. OpenRouter's Credits detail includes
  credits used, total and left plus current-key daily, weekly and monthly spend.
  Claude's usage and plan come from a read-only host OAuth poller with statusline
  fallback; the terminal header remains statusline-backed.
  The page, desktop sidebar and phone drawer share `quotaStore`: its first
  subscriber opens one `topic=quota` stream, which patches cards and status rows
  from full provider lists; the last subscriber leaving closes it. The Daemon
  starts the guarded, concurrent refresher only while a quota-topic subscriber
  exists, checks every 15s against the 90s per-provider cache age guard, and
  stops when the last subscriber leaves. The page reads the cache once and uses
  no browser quota timer or polling. `POST /api/quota/refresh` keeps cached
  values visible, marks affected cards busy, skips the age guard and enforces a
  10s per-provider minimum. Age and reset labels recompute on cache reads, live
  provider lists or explicit refresh; the sidebar shows each provider's highest
  `pct` with a neutral marker when absent. `quota.js` owns threshold, age and
  label helpers. Not-connected cards link to their provider row in `/providers`;
  both Antigravity pool cards target Google Antigravity, and `ProvidersPage.svelte`
  focuses the requested row after its async list loads.
- `src/lib/memory.js` — pure Memory page helpers: row normalizers that accept Go-case and lowercase keys, proposal parsing with the old remember-shape upgrade, per-Session queue grouping, purge counts and summaries, latency labels
- `src/lib/PromotionQueue.svelte` (`api.js` promotion calls) — the promotion approval queue embedded in the Memory page: each item shows its target path and diff preview with approve and reject; approving writes the worktree file and leaves it uncommitted for review
- `SettingsPage.svelte` Accounts section, admin only: the `Allow sign-up` switch saves `signup_enabled` with the rest of the Save bar; other Users never see or send the key
- `src/lib/stt.js` — voice input: `startRecording` records one take (1 s chunks, level meter for the live waveform, Screen Wake Lock held and re-requested on `visibilitychange`; a browser-ended take still yields the audio so far) and `transcribe` posts the whole recording to the Daemon `POST /api/stt` (OpenRouter gpt-transcribe, `EL_STT_MODEL` overrides; `?format=1` runs the mistral-small-24b cleanup pass, `EL_STT_FORMAT_MODEL` overrides; key stays on the Daemon: env or pig). The Composer shows a recorder bar (timer, waveform, cancel), then a draft voice bubble (`VoiceBubble.svelte`) while the transcript lands in the message box for editing before send; send blocks while transcribing. Sent, the recording uploads as `aud_` image id (peaks, duration) and renders as a voice bubble with the text as its transcript (button or hover). `voicePrefs.js` (`el.voice`: `enabled`, `cleanup`) backs the Settings `VoiceSettings.svelte` section; `voiceMeta.js` reads recording peaks from the image list. Needs https or localhost
- `src/lib/images.js` — image attachments: file picker and paste in the Composer, read to base64, sent through `postPrompt(id, text, { mode, images })`
- `Card.svelte` sudo cards (`sudo`, `sudoresult`): Approve/Deny with optional password; Profile patch cards offer Approve/Reject through the same queue decision API as action buttons on push notifications. `events.js` updates both request cards from resolution Events and parses the TOON `sudo_result:` prompt.
- `src/lib/Notifications.svelte`, `src/lib/push.js`, `public/sw.js` — push opt-in on the settings page, subscribe client, service worker; the worker messages open tabs for sound and shows OS notifications only when no window is visible. Profile patch pushes expose Approve/Reject actions through the authenticated queue API. Settings includes a per-User opt-in daily Memory digest preference; the nightly stage sends at most one digest each local day when enabled. `Notifications.test.js` covers the off default and opt-in, `sw.test.js` covers both notification decisions, and `push_triggers_test.go` covers API persistence and gated delivery.
- `src/lib/ui/combobox.svelte` — picker (Popover + Command, virtualized list)
- `src/lib/ui/select.svelte` — plain bits-ui Select for simple fixed lists
- `src/lib/ui/slider.svelte`, `src/lib/ui/switch.svelte` — bits-ui Slider (single value, labelled thumb) and Switch for the Sounds section
- `src/lib/ui/button.svelte` — shadcn-svelte-idiom button variants
- `src/lib/InstallApp.svelte`, `src/lib/install.js` — settings Install app card: the `beforeinstallprompt` button plus the iOS Add-to-Home-Screen hint; pure standalone/iOS/state helpers in `install.js`
- `src/lib/pwa.js` — service worker registration at app boot (before push is ever enabled)
- `public/manifest.webmanifest`, `public/icons/`, `public/apple-touch-icon.png` — install metadata and artwork; icons come from `scripts/generate-icons.py` (from `public/logo.png`, outputs committed)
`npm test` runs vitest: Event-to-card mapping and grouping
(`events.test.js`), JSON-escape decode plus backslash passthrough
(`highlight.test.js`), fenced-code copy button and http fallback
(`Markdown.test.js`), streaming bench with before/after flush costs, parse counts, incremental-markdown
exactness, pace bounds, fade skip, highlight memo and append-only text (`streamBench.test.js`), tool strip grouping, exclusions, summary and formatting
(`toolstrip.test.js`), strip header/summary/failure/history/stage-cap/reduced-motion/streaming/live-gating/tail-window
render (`ToolStrip.test.js`), composer mode resolvers (`composer.test.js`),
mounted Composer badge and send behavior (`Composer.test.js`), harness
model filtering (`harnesses.test.js`), the virtualized picker open/filter/pick,
keyboard, empty state and windowing (`ui/combobox.test.js`), Card rendering per kind
plus the echoed-prompt no-animation swap and the live thinking gate (`Card.test.js`), Runs spec discovery plus shared-summary coalescing, summary-only list ordering, counts, deep-link seeding and needs-you navigation (`RunsSidebar.test.js`, `runsStore.test.js`), RunsView preview/start, detail-on-open loading, graph/list, active Chain entries, per-Ticket/Run pause, Switch scope across Worker/Reviewer Chains, resume and paused Backend/model pickers, branch actions and SSE (`RunsView.test.js`); graph state/layout/Chain summaries and
rendering (`runsGraph.test.js`, `RunsGraph.test.js`); Ticket state, review rounds,
Dispositions, gate, actions and SSE (`ticketPage.test.js`, `TicketPage.test.js`);
Settings `SettingsPage.test.js` covers `max_tickets` validation, locked fields and save-bar behavior; Runs labels/persistence; Agents chains, role grouping, effort capabilities and model fallback; compression thresholds; legacy Memory decider and Jev settings; and the new admin-only Memory extractor model/effort, embedding model, automatic-extraction switch, recall weights and egress notice. `settings.js`'s `findSettings` registry and `settings.test.js` cover searching each new control. Quota, Providers, routing, phone navigation, Profile, notifications, sound and voice behaviors are covered in their matching `*.test.js` files.
Providers loading/error states, status groups, API-key forms and quota refresh, ChatGPT device-code link/code/cancel and successful quota refresh, Claude PTY connect/code/success and quota-store patching, CLI-only OAuth command copy, cloud-auth CLI guidance, Antigravity callback success and invalid-input retry, provider-row deep links, admin read-only controls and phone layout are covered by `ProvidersPage.test.js`; `/providers` navigation and sidebar selection are covered by `router.test.js`, `QuotaSidebar.test.js` and `App.test.js`. Quota tests cover all not-connected provider-row links and in-app navigation (`QuotaPage.test.js`), plus the shared live topic and status list.
The Quota tests also verify that one mocked live channel patches the page, desktop status list and phone drawer.
Sign-in and accounts: `authForm.test.js` (mode, copy and error lines), `AuthScreen.test.js` (admin, login, signup, the create-account link and server answers), `Root.test.js` (the gate, footer sign out, 401 return, retry), `api.test.js` (auth, Project suggestion and promotions clients plus the 401 signal), `live.test.js` (stable shared stream, quota stream, payloads and `stopLive`), plus the App's shared, coalesced Run summaries for the sidebar, footer, session-not-found overlay gate and Git Escape (`App.test.js`) and the Accounts switch (`SettingsPage.test.js`).
`App.test.js` covers Dashboard boot without list reads, Project scope and `/p/none`, the Jev draft Project-pick banner (suggestion, Yes, No, stale-request delivery, retry after delivery failure, image-only and no-pick fallback), scope derivation from `/s/<id>`, Project Runs filtering and folderless starts, Project settings rename/remove/gist editing, global routes, phone-drawer navigation, delayed Session/Run loading, the 40-Run cold-boot timing and direct-Session loading races. `ProjectSettingsPage.test.js` covers settings fields, removal and loading/not-found states. `router.test.js`, `DashboardPage.test.js`, `ProjectSwitcher.test.js` and `OwnerGroups.test.js` cover route round trips, stable ids, active payload rendering and scoped sidebar sections.
Settings hub coverage: `SettingsHub.test.js` checks the Workspace/You groups and route links; `SettingsRoutes.test.js` checks the phone hub, each concern route and scope pill, and the desktop `/settings` redirect; `SettingsPage.test.js` checks the Revert/Save bar.

## Routes

Real paths, served from `index.html` by the Daemon SPA fallback (missing assets and `/api/*` still 404): `/` is the active-work Dashboard; `/p/<projectId>` scopes Sessions; `/p/none` scopes home-directory Sessions; `/p/<projectId>/runs[/<runId>[/<ticketId>]]` scopes Runs and uses the Project path for spec discovery and Run starts; `/p/<projectId>/settings` supports Project rename, removal and gist editing without override fields; `/s/<id>` is canonical (optional `?at=<seq>` scroll target) and sets the scope from Session metadata. `/settings` shows the Settings hub on phones and routes to Agents on desktop; `/settings/{agents,runs,context,memory,access,profile,notifications,sound-voice}` are global concern pages with per-page Global, Admin or You scope. `/quota`, `/providers` and `/memory` are global. Unknown Session ids show Session-not-found rather than opening another Session; an unknown Project shows Project not found. The build uses vite base `'/'` with root-absolute icon, service worker and asset URLs so nested paths load.

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
