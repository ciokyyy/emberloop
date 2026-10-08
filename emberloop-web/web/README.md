# UI

Svelte 5 + Vite single-page app, built to static files. `npm run build` in
`web/` writes `web/dist/`; the Daemon embeds `web/dist` and serves `/`,
`/index.html` and the bundle assets (JS, CSS, favicon) at `/`. No server
rendering anywhere, so the same build wraps in Tauri later unchanged.

Styling is Tailwind v4 with shadcn-svelte-idiom primitives
(`src/lib/ui/`: `cn`, `button` variants via tailwind-variants) and the
same tokens in `src/app.css`: cards, dialog, badges and
`details`-based collapsibles in a calm chat layout (single transcript
column, floating composer, collapsible details pane). System light/dark
via `prefers-color-scheme`. A full registry port (bits-ui Dialog
primitives) is deferred: no behavior delta.

## API base URL

Same-origin by default. To point the static build at another Daemon:

- build time: `VITE_API_BASE=http://127.0.0.1:PORT npm run build`
- runtime: set `window.__EMBERLOOP_API_BASE__` before the bundle loads

## Source map

- `src/App.svelte` — layout, Session loading, SSE stream with reconnect
- `src/lib/api.js` — Daemon client (configurable base URL)
- `src/lib/events.js` — Event-to-card mapping and transcript grouping
- `src/lib/composer.js` — composer mode (prompt / steer / follow-up)
- `src/lib/harnesses.js` — harness catalogue for the new-Session dialog
- `src/lib/Card.svelte` — transcript card for every Event kind
- `src/lib/SessionList.svelte` — Session tree with per-Session status
- `src/lib/Composer.svelte` — floating composer with mode badge
- `src/lib/NewSessionDialog.svelte` — harness / model / working directory
- `src/lib/DetailsPane.svelte` — collapsible Session metadata
- `src/lib/ui/button.svelte` — shadcn-svelte-idiom button variants
- `src/lib/ui/cn.js` — class merging (`clsx` + `tailwind-merge`)

## Tests

`npm test` runs vitest: Event-to-card mapping and grouping
(`events.test.js`), composer mode resolvers (`composer.test.js`),
mounted Composer badge and send behavior (`Composer.test.js`), harness
model filtering (`harnesses.test.js`), and Card rendering per kind
(`Card.test.js`).
