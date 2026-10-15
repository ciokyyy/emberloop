---
status: accepted
---

# Use modernc QuickJS for embedded Svelte SSR

The spike rendered the Dashboard Svelte 5 SSR bundle in-process with
`modernc.org/quickjs` v0.25.0, a no-cgo QuickJS implementation. A reused VM
rendered 1,000 warm requests at 208,749 ns/request (0.21ms), comfortably below
the 50ms target. Chromium headless hydrated the generated Dashboard shell
without a Svelte hydration/mismatch warning.

The SSR bundle is 188.15 kB. On Linux amd64, the built Daemon grew from 58,752,478 to 59,137,668 bytes (+385,190 bytes, 0.65%). `npm run build` creates `web/dist/ssr/ssr.js`, included by the existing embedded filesystem. This ADR chooses the engine for SSR integration; the spike does not yet invoke it during request handling.

## Considered options

- QuickJS through wazero (`github.com/fastschema/qjs` v0.0.6): no-cgo design, but the smoke test could not instantiate its wasm module due to an import signature mismatch.
- `v8go`: cgo fallback; not needed after the modernc spike passed.
- Node/Bun sidecar: adds a runtime and process to the one-binary Daemon.

## Consequences

- Keep the Daemon free of cgo and Node runtime dependencies for SSR.
- The SSR bundle and modernc engine increase binary/build size modestly.
- Reuse a warm VM for request rendering; a cold runtime is outside the measured per-request result.
- Request routing, per-user data, fallback handling, and browser boot integration remain subsequent work.
