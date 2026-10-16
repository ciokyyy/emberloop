---
status: accepted
---

# Use modernc QuickJS for embedded Svelte SSR

The spike rendered the Dashboard Svelte 5 SSR bundle in-process with
`modernc.org/quickjs` v0.25.0, a no-cgo QuickJS implementation. A reused VM
rendered 1,000 warm requests at 94,185 ns/request (0.094ms), below the 50ms
target. `TestDashboardSSRBrowserHydration` exercises the browser bundle with
Chromium and asserts zero hydration/mismatch warnings.

The SSR bundle is 188.15 kB and the test hydrator is 133.56 kB. On Linux amd64,
the Daemon grew from 58,752,478 to 59,264,652 bytes (+512,174 bytes, 0.87%).
`npm run build` creates both files under `web/dist/ssr/`, included by the
embedded filesystem. This ADR chooses the engine for SSR integration; requests
do not invoke it yet.

## Considered options

- QuickJS through wazero (`github.com/fastschema/qjs` v0.0.6): no-cgo design, but the smoke test could not instantiate its wasm module due to an import signature mismatch.
- `v8go`: cgo fallback; not needed after the modernc spike passed.
- Node/Bun sidecar: adds a runtime and process to the one-binary Daemon.

## Consequences

- Keep the Daemon free of cgo and Node runtime dependencies for SSR.
- The SSR bundle and modernc engine increase binary/build size modestly.
- Reuse a warm VM for request rendering; a cold runtime is outside the measured per-request result.
- Request routing, per-user data, fallback handling, and browser boot integration remain subsequent work.
