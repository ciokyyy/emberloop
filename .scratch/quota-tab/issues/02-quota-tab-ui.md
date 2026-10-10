# 02: Quota tab UI

**What to build:** A Quota icon in the Rail opens `/quota`: one card per provider with percent bars per window, reset countdowns, "not configured" and "updated Xm ago" labels, and per-card plus refresh-all buttons. The page fetches once on open and never polls. Docs and indexes updated. See the spec.

**Blocked by:** 01-quota-api

**Status:** ready-for-agent

- [ ] Rail shows a Quota entry; `/quota` loads on reload and works with back/forward
- [ ] Cards show bars colored by header thresholds, countdowns, Antigravity `gemini` and `3p` groups
- [x] Not-configured, error (with last values kept) and Claude stale states render
- [ ] Per-card refresh and refresh-all call `POST /api/quota/refresh` and show a busy state
- [x] The page fetches once per visit and sets no timers; only the buttons update it afterward
- [x] vitest covers router, helpers and page behaviour
- [ ] `emberloop-web/AGENTS.md` and root `AGENTS.md` index updated; repo check passes
