# Spec: server-side rendering for emberloop-web

Source: grilling Session 2026-10-15.

## Problem

A browser refresh on `/p/<id>`, `/p/none`, `/p/<id>/runs` or `/p/<id>/settings` returns 404. `ui.go` `isSPARoute` is a hardcoded allowlist that predates the Project routes. Every page also paints empty, then fetches.

## Decisions

1. **Engine.** The Daemon renders Svelte 5 in-process on an embedded JS engine. Preferred: no-cgo QuickJS through wazero or modernc, so the Daemon stays one Go binary. Rejected: Node/Bun sidecar, SvelteKit, Inertia (Node SSR, replaces the router), `sveltego` (pre-alpha, rewrite). The spike (ticket 03) proves or rejects the engine.
2. **Route table.** One Go route table (pattern, scope, prefetch calls) replaces `isSPARoute`. It mirrors `web/src/lib/router.js`. An unknown path returns a real 404 page. Missing assets and `/api/*` still 404 as today.
3. **Auth.** SSR resolves the cookie session in-process and calls Daemon handlers directly, never over HTTP loopback. Signed-out requests render the `AuthScreen` shell.
4. **Server scope.** Server-rendered: rail, sidebar, header, Dashboard, Project pages, Settings pages, and a Session header with its tail snapshot. Placeholders until hydration: the virtualized transcript, SSE, Composer, `localStorage`-backed state. Browser-only code gets guards.
5. **Boot payload.** The prefetch results feed both the render and `window.__EL_BOOT__`. The client reads it and skips its first fetches when present. Without it, the client fetches as today.
6. **Fallback.** An engine panic, a render error, or a render over 200ms serves the plain `index.html` shell plus the Boot payload and logs the error. A valid route never returns 500 from SSR.
7. **Standalone client.** The client-only bundle keeps working with no Boot payload (Tauri, `VITE_API_BASE`). SSR is an additive layer owned by the Daemon. The fallback and the standalone path are the same client path.
8. **Caching.** SSR HTML is user-specific: `Cache-Control: private, no-store`, `Vary: Cookie`. `sw.js` stops caching navigation documents and keeps caching hashed assets.
9. **Dev.** `bun run dev` and `bun run prod` stay client-only on Vite. SSR is verified on the Daemon port and by the e2e suite.
10. **Dependencies first.** Update npm (Vite 8, plugin-svelte 7, explicit `svelte`, the rest), then Go modules to latest, before SSR work. If PiG 0.4.1 breaks the Backend, pin it and note why.
11. **Build.** The web build adds an SSR bundle to the embedded dist. `uiSourceHash` in `vite.config.js` and `hashWebSource` in `emberloop/update.go` stay byte-identical.

## Fallback if the spike fails

Skip server-rendered HTML. Tickets 04 to 06 ship the route table, the 404 page, prefetch with the Boot payload, and the cache headers, all on the plain `index.html` shell.

## Out of scope

Full transcript SSR; a Node runtime anywhere; changing `router.js` URL shapes.

## ADR

Write an ADR for the embedded JS engine after the spike: hard to reverse, surprising, a real trade-off.
