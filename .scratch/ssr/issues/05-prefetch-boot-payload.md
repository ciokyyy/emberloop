# 05: Prefetch table and Boot payload

**What to build:** Each route entry names its prefetch calls (Dashboard payload, Session header and tail snapshot, scoped Run summaries, Project list, Settings data). The Daemon runs them in-process under the request cookie, renders the shell (server-rendered when ticket 03 passed), and embeds `window.__EL_BOOT__`. The client reads it and skips its first fetches. An engine failure serves the plain shell plus the Boot payload.

**Blocked by:** 04

**Status:** done

- [x] Signed-in refresh paints content with no first-load fetch for the prefetched routes
- [x] Signed-out request renders the `AuthScreen` shell
- [x] Render error, panic or over 200ms falls back with a logged error, never 500
- [x] The client bundle works with no Boot payload
- [x] Tests cover prefetch per route, auth, and the fallback
- [x] `GLOSSARY.md` terms match the code
