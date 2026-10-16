# 05: Prefetch table and Boot payload

**What to build:** Each route entry names its prefetch calls (Dashboard payload, Session header and tail snapshot, scoped Run summaries, Project list, Settings data). The Daemon runs them in-process under the request cookie, renders the shell (server-rendered when ticket 03 passed), and embeds `window.__EL_BOOT__`. The client reads it and skips its first fetches. An engine failure serves the plain shell plus the Boot payload.

**Blocked by:** 04

**Status:** open

- [ ] Signed-in refresh paints content with no first-load fetch for the prefetched routes
- [ ] Signed-out request renders the `AuthScreen` shell
- [ ] Render error, panic or over 200ms falls back with a logged error, never 500
- [ ] The client bundle works with no Boot payload
- [ ] Tests cover prefetch per route, auth, and the fallback
- [ ] `GLOSSARY.md` terms match the code
