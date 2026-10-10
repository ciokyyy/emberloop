# Quota tab

## Problem Statement

Account quota for Claude, OpenCode-Go and Antigravity shows only in the terminal header strip of an agent pane. The browser UI shows no quota. A user who works in the Emberloop UI cannot see how much of each window is left, or when it resets, without opening a pane.

## Solution

A "Quota" tab in the browser UI: a Rail icon opens the `/quota` page. The page shows one card per provider (Claude, OpenCode-Go, Antigravity) with each window as a percent bar and a reset countdown. The page loads once when opened and never polls. Each card, and the page, has a manual refresh control. The Daemon serves the data from the existing quota code in `emberloop/`.

## User Stories

1. As a user, I want a Quota icon in the Rail, so that I open quota in one click.
2. As a user, I want the `/quota` route to survive a page reload and back/forward, so that the tab behaves like the other views.
3. As a user, I want one card per provider, so that I compare accounts at a glance.
4. As a user, I want each window (5h, wk, mo) as a percent bar, so that I see how much is used.
5. As a user, I want bar colors to follow the header strip thresholds, so that warning levels look the same everywhere.
6. As a user, I want a reset countdown on each window, so that I know when it frees up.
7. As a user, I want Antigravity split into `gemini` and `3p` groups, so that I see each pool.
8. As a user, I want a provider with no auth to read "not configured", so that I do not mistake it for an error.
9. As a user, I want Claude to show an "updated Xm ago" label, so that I know the value is push-based and may be stale.
10. As a user, I want the page to load quota once when I open it, so that I see current numbers on arrival.
11. As a user, I want no background polling, so that no needless upstream requests run while I work elsewhere.
12. As a user, I want a refresh button on each card, so that I update one provider now.
13. As a user, I want a refresh-all button on the page, so that I update everything now.
14. As a user, I want a manual refresh to skip the 90s age guard, so that it fetches really fresh data.
15. As a user, I want a manual refresh of Claude to re-read its file only, so that the button never fails on a push-only source.
16. As a user, I want a failed fetch to keep the last values and show the error on that card, so that one outage hides nothing.
17. As a user, I want the card to show a busy state during refresh, so that I know the click registered.
18. As a user, I want Daemon fetches to share the cache and lock with header panes, so that N viewers cause one upstream request.

## Implementation Decisions

- `emberloop-web` already imports the `emberloop` module. Reuse `ReadClaudeUsage`, `ReadUsage`, `RefreshUsage` and the key mapping. No new fetch logic, no copy.
- New Daemon endpoint `GET /api/quota`: returns all providers. Each provider carries: key(s), label, state (`ok`, `not_configured`, `error`), windows `{label, pct, resets_at}`, `updated_at`, and an error text when set. Antigravity returns two groups (`agy-gemini`, `agy-3p`).
- `GET /api/quota` refreshes OpenCode-Go and Antigravity through `RefreshUsage` with the normal 90s age guard, then reads the cache. It never blocks the response on a slow upstream beyond the existing 25s timeout; a failure falls back to the stale cache.
- New endpoint `POST /api/quota/refresh` with optional `key`: forces a fetch that skips the age guard. The per-key flock stays. No `key` means all keys. For `claude`, it only re-reads the file. Response has the same shape as `GET /api/quota`.
- Forcing past the age guard needs a small addition in `emberloop/` (a force option or sibling function to `RefreshUsage`). Header behaviour stays unchanged.
- Daemon uses the same state dir as header panes, so cache files are shared.
- UI: new Rail entry, route `/quota` in the pure router, one page component, no contextual second sidebar. Logic in a colocated `lib/*.js`, markup in `lib/*.svelte`, fetch in `api.js`.
- No polling and no timers. The page fetches `GET /api/quota` once on mount (each visit to `/quota`). Further updates happen only through the manual refresh buttons.
- Bar thresholds and pie-free percent bar reuse the header's threshold values, exposed from the Go response or mirrored in one JS helper with a test.
- Countdown is computed client-side from `resets_at`; a past reset reads 0%, as in `ReadUsage`.

## Testing Decisions

- A good test checks external behaviour only: HTTP response, rendered text, route result.
- Go: tests at the HTTP seam on `/api/quota` and `/api/quota/refresh` with a temp state dir and local HTTP stubs for the OpenCode and Antigravity endpoints. Prior art: `emberloop/usage_providers_test.go` and the fake-Backend tests in `emberloop-web`.
- Go: cover not-configured, stale fallback on failure, forced refresh skipping the age guard, Claude re-read only.
- JS: vitest on `router.js` (`/quota`), the quota helpers (threshold, countdown, stale label) and the page component (cards, not-configured, error, refresh buttons, fetch once on mount and no timers). Prior art: colocated `*.test.js`.
- No live tests.

## Out of Scope

- A new Claude quota fetch (no endpoint added; push-only stays).
- Per-request token and cost totals.
- SSE or polling for quota.
- Alerts or notifications on high usage.
- Changes to the terminal header strip.

## Further Notes

- Agy OAuth client path differs between `usage_providers.go` (`~/.emberloop/agy-oauth.json`) and `pig-antigravity-auth/client.go` (`~/.el/agy-oauth.json`). Do not fix here; "not configured" covers a missing client.
