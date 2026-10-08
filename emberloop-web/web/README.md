# UI

Svelte 5 + Vite single-page app, built to static files. `npm run build` in
`web/` writes `web/dist/`; the Daemon embeds `web/dist` and serves it at
`/`. The checked-in `web/dist/index.html` is the walking-skeleton build:
unstyled, same API contract as `web/src/App.svelte` (session list, create,
transcript over SSE with whole-log replay on open). Later tickets replace
it with the real styled build.
