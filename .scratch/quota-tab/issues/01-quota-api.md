# 01: Quota API with manual refresh

**What to build:** The Daemon serves account quota for Claude, OpenCode-Go and Antigravity. `GET /api/quota` returns every provider's windows, state and update time. `POST /api/quota/refresh` (optional `key`) forces a fresh fetch that skips the 90s age guard, still under the shared per-key lock. Claude refresh only re-reads its file. See the spec for the response shape and rules.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `GET /api/quota` returns Claude, OpenCode-Go and both Antigravity groups with windows `{label, pct, resets_at}`, state and `updated_at`
- [x] A provider without auth reports `not_configured`, not an error
- [x] A failed upstream fetch keeps the stale cache and sets an error text on that provider
- [x] `POST /api/quota/refresh` forces a fetch past the age guard for one `key` or all keys
- [x] Refresh of `claude` re-reads the file and never fetches
- [x] Header pane behaviour and its existing tests are unchanged
- [x] Go tests at the HTTP seam with temp state dir and local HTTP stubs cover each case above
