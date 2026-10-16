# 03: SSR spike

**What to build:** A spike renders one Svelte 5 page from a `vite build --ssr` bundle inside the Daemon on a no-cgo QuickJS engine, hydrates it in a browser, and reports the result. Candidates: QuickJS through wazero, QuickJS through modernc. `v8go` is the cgo fallback to measure only.

**Blocked by:** 01

**Status:** open

- [ ] A Go test renders the Dashboard shell to HTML through the engine
- [ ] A browser hydrates it with no mismatch warning
- [ ] Render time per request is measured; target under 50ms warm
- [ ] Binary size and build change are recorded
- [ ] The result names the engine, or declares the fallback in the spec
- [ ] An ADR records the engine choice
