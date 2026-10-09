# UI

Svelte 5 + Vite single-page app, built to static files. `npm run build` in
`web/` writes `web/dist/`; the Daemon embeds `web/dist` and serves `/`,
`/index.html` and the bundle assets (JS, CSS, favicon) at `/`. No server
rendering anywhere, so the same build wraps in Tauri later unchanged.

## Design rules

Follow these for every UI change.

- Look: ChatGPT neutral surfaces plus Apple softness (system font, large
  radii, blurred translucent chrome, pill buttons). Tokens live in
  `src/app.css`; never hardcode colors.
- shadcn-svelte everywhere: every interactive control (button, combobox,
  select, dialog, input, badge, ...) comes from `src/lib/ui/`, built on bits-ui.
  Never use a raw `<select>`, `<dialog>` or unstyled `<input>`. Add a missing
  primitive to `src/lib/ui/` first.
- Pickers use `src/lib/ui/combobox.svelte` (Popover + Command with search and a
  TanStack-virtualized list); `src/lib/ui/select.svelte` stays a plain bits-ui
  Select for simple fixed lists.
- Icons come from `@lucide/svelte`.
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

- `src/App.svelte` — layout, Session loading, SSE stream with reconnect, queued-message panel with clear, chat/settings/runs view switch
- `src/lib/api.js` — Daemon client (configurable base URL), incl. settings read/write, spec list, ticket preview, Run start/get and Run SSE stream
- `src/lib/events.js` — Event-to-card mapping and transcript grouping
- `src/lib/composer.js` — composer mode (prompt / steer / follow-up)
- `src/lib/harnesses.js` — harness catalogue for the new-Session dialog
- `src/lib/Card.svelte` — transcript card for every Event kind
- `src/lib/SessionList.svelte` — Session tree with per-Session status
- `src/lib/Composer.svelte` — floating composer with mode badge and a Stop button (`onStop`, `POST /interrupt`) while working
- `src/lib/NewSessionDialog.svelte` — harness / model / working directory
- `src/lib/RunsView.svelte` — repo picker, spec list, ticket preview table, Run start plus live Ticket table with Worker links to the Session view and per-Ticket reviewer verdict, tip freshness and refusal columns
- `src/lib/SettingsPage.svelte` — role harness/model/effort groups plus the max_workers field, locked with a note under the environment override
- `src/lib/stt.js` — voice input: mic records live segments (fresh MediaRecorder each, silence/timer close) posted one per call to the Daemon `POST /api/stt` (OpenRouter gpt-transcribe, `EL_STT_MODEL` overrides; `?format=1` runs the gpt-4o-mini cleanup pass, `EL_STT_FORMAT_MODEL` overrides; key stays on the Daemon: env or pig). Composer appends each part to the draft in order with a dim pending preview and a cleanup toggle. Needs https or localhost
- `src/lib/images.js` — image attachments: file picker and paste in the Composer, read to base64, sent through `postPrompt(id, text, { mode, images })`
- `Card.svelte` sudo cards (`sudo`, `sudoresult`): Approve/Deny with optional password; `events.js` merges `sudo_resolved` into the request card and parses the TOON `sudo_result:` prompt
- `src/lib/Notifications.svelte`, `src/lib/push.js`, `public/sw.js` — push opt-in on the settings page, subscribe client, service worker
- `src/lib/ui/combobox.svelte` — picker (Popover + Command, virtualized list)
- `src/lib/ui/select.svelte` — plain bits-ui Select for simple fixed lists
- `src/lib/ui/button.svelte` — shadcn-svelte-idiom button variants

## Tests

`npm test` runs vitest: Event-to-card mapping and grouping
(`events.test.js`), composer mode resolvers (`composer.test.js`),
mounted Composer badge and send behavior (`Composer.test.js`), harness
model filtering (`harnesses.test.js`), the virtualized picker open/filter/pick,
keyboard, empty state and windowing (`ui/combobox.test.js`), Card rendering per kind
(`Card.test.js`), Runs picker, preview, Start and live table with Worker
links plus the reviewer verdict columns against a mocked API (`RunsView.test.js`),
