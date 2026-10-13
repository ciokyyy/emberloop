# Quota tab

## Problem Statement

The Quota page is slow, partly wrong and limited. `GET /api/quota` blocks on sequential upstream fetches (up to 20s each, 25s for the handler), so the page sits on a skeleton. Claude quota is push-only: it shows "never updated" or a stale number unless a terminal pane runs the statusline, and headless ACP Sessions never trigger it. ChatGPT Plus and OpenRouter credits show nowhere. Antigravity is "not configured" on most hosts with no hint why. Windows use short labels (`5h`, `wk`, `mo`) and some cards show dollars where others show percent, so cards do not compare at a glance. The page collapses the second sidebar and does not follow the Settings redesign shell on phones.

## Solution

The Quota page serves cached values at once and refreshes in the background. The Daemon pushes new values to open pages over the live channel, so the page needs no manual reload. Every card shows percent used (0-100) first; dollars and tokens are secondary text. Providers: Claude (read-only host login poller, no prompt), ChatGPT Plus (pig `openai-codex` login, `wham/usage`), OpenCode-Go, Antigravity (Gemini and Claude + GPT-OSS pools; not-configured state names the missing login), OpenRouter (credits). The page sits in the redesigned shell: the Quota rail icon opens a second sidebar with the Quota entry and a Live status list (tone dot and percent per provider); on phones the same list fills the drawer. The design reference is `.scratch/settings-redesign/mock/index.html` (the `/quota` route and its sidebar).

Real values read from the host on 2026-10-13, which the cards must reproduce:

- Claude: 5-hour 36%, weekly 8%, plan Pro (`five_hour`, `seven_day`, each with `utilization` and `resets_at`).
- ChatGPT Plus: 5-hour 4% (`primary_window`, 18000s), weekly 4% (`secondary_window`, 604800s), no extra credits.
- OpenCode-Go: 5-hour 0%, weekly 7%, monthly 53%.
- OpenRouter: 59.16 of 63.00 credits used (94%), 3.84 left; key spend today 0.91, week 2.37, month 5.70.
- Antigravity: not configured on this host (no `~/.el/agy-oauth.json`).

## User Stories

1. As a user, I want the Quota page to show cached values immediately, so that I never wait on upstream APIs.
2. As a user, I want values to refresh in the background after the page opens, so that the numbers are current without a click.
3. As a user, I want open pages to update over the live channel, so that I see a window move without reloading.
4. As a user, I want one slow or failing provider to never delay another card, so that one outage hides nothing.
5. As a user, I want every card to show percent used from 0 to 100 first, so that I compare accounts at a glance.
6. As a user, I want dollars and tokens as secondary text under the bar, so that I still see the amount.
7. As a user, I want window names in words (5-hour, Weekly, Monthly), so that I do not decode `5h` and `wk`.
8. As a user, I want a reset countdown on each window, so that I know when it frees up.
9. As a user, I want bar colors to follow the header strip thresholds (calm below 60, warm below 80, hot from 80), so that warning levels look the same everywhere.
10. As a user, I want Claude quota without running a terminal pane, so that I see it from the browser alone.
11. As a user, I want Claude quota read from the host login with no prompt and no model call, so that checking it never costs usage.
12. As a user, I want the Claude poller never to refresh or write the host login, so that it can never log me out of Claude Code.
13. As a user, I want an expired host login to show an "login expired" state with the last values dimmed and a hint, so that I know why it stopped updating.
14. As a user, I want the Claude card to show the plan (Pro, Max), so that I know which limits apply.
15. As a user, I want a Claude refresh failure (rate limit, network) to keep the last values and show "stale" with their age, so that a blip does not blank the card.
16. As a user, I want the Claude poller to back off on a rate-limit reply, so that polling never makes the limit worse.
17. As a user, I want the ChatGPT Plus card with the 5-hour and weekly windows, so that I see my Codex headroom.
18. As a user, I want the ChatGPT card read from the existing pig login, so that I add no new credential.
19. As a user, I want a ChatGPT login that is missing or expired to show not connected or expired, so that I know to sign in again.
20. As a user, I want an OpenRouter card with credits used as a percent and the dollars left, so that I see when the balance runs low.
21. As a user, I want the OpenRouter card to show spend today, this week and this month, so that I see the burn rate.
22. As a user, I want OpenCode-Go to show 5-hour, weekly and monthly bars, so that I see all three limits.
23. As a user, I want Antigravity split into a Gemini pool and a Claude + GPT-OSS pool, each with a 5-hour and a weekly bar, so that I see each pool.
24. As a user, I want a provider with no login to read "not connected" with the exact missing piece named, so that I do not mistake it for an error.
25. As a user, I want a not-connected card to link to where I connect it, so that I fix it in one click.
26. As a user, I want a per-card refresh button, so that I update one provider now.
27. As a user, I want a refresh-all button, so that I update everything now.
28. As a user, I want a manual refresh to skip the age guard but never hammer an upstream, so that the data is fresh and the account stays safe.
29. As a user, I want the card to show a busy state during refresh and keep its values, so that I know the click registered.
30. As a user, I want an "updated Xs ago" stamp on each card, so that I judge freshness.
31. As a user, I want a Live status list in the second sidebar with a tone dot and percent per provider, so that I glance at all quotas from anywhere on the page.
32. As a user, I want the Quota second sidebar to stay open on desktop, so that Quota navigates like the other areas.
33. As a user, I want the same list in the phone drawer, so that phones follow the same shell.
34. As a phone user, I want one-column cards and 44px buttons, so that every control is easy to hit.
35. As a user, I want the `/quota` route to survive a reload and back/forward, so that the page behaves like the other views.
36. As a user, I want Daemon fetches to share the cache and lock with header panes, so that N viewers cause one upstream request.
37. As a user, I want the header strip in the terminal panes to keep working unchanged, so that the redesign breaks nothing.
38. As an operator, I want a measured response time for `GET /api/quota`, so that the speed claim is verified, not assumed.

## Implementation Decisions

- Quota reads never wait on the network. `GET /api/quota` returns the cache at once with each provider's state and `updated_at`. A background refresher keeps each provider fresh on its own cadence and publishes changes on a live quota topic. A provider fetch never blocks another provider's fetch (per-provider concurrency, not a sequential loop). The existing 90s age guard and per-key lock stay; the shared cache files stay shared with header panes.
- The refresher runs only while at least one Quota page (or the Live status list) is subscribed, plus a cold start read from cache. It stops when no one listens, so no needless upstream requests run while the user works elsewhere.
- Response contract: each provider carries `key`, `label`, `plan`, `state` (`ok`, `stale`, `expired`, `not_configured`, `error`), `updated_at`, `error`, optional `hint`, and `windows[]`. Each window carries `label` (word form), `pct` (integer 0-100, percent used), `resets_at`, and optional `detail` (secondary text such as `$59.16 of $63.00 · $3.84 left`). Antigravity returns two groups (`agy-gemini`, `agy-3p`). `pct` is the only field the bar and the Live status list read.
- Claude: a read-only poller calls the OAuth usage endpoint with the host login's access token. It sets the required `User-Agent: claude-code/<version>` (version read from the installed `claude` binary, with a fallback constant) and the OAuth beta header. It never refreshes, rewrites or deletes the credentials file, never starts `claude`, and never sends a prompt. It maps `five_hour` to the 5-hour window and `seven_day` to Weekly; `utilization` is already percent used. The plan name comes from the credentials file's subscription type. Cache TTL is 60-300s; a rate-limit reply (`Retry-After`) backs off for that long. A 401 or an expired `expiresAt` sets `expired` with the last values kept. The push-based statusline file remains a fallback source when the host login is absent, and the header strip stays unchanged.
- ChatGPT Plus: read the `openai-codex` login from pig's `auth.json` (read-only; never refresh in this feature), call the Codex usage endpoint with the account id header, and map `primary_window` to 5-hour and `secondary_window` to Weekly (`used_percent`, `reset_at`). A missing login is `not_configured`; an expired one is `expired`. Connect (device code) belongs to the Providers spec.
- OpenRouter: read the pig `openrouter` key; credits percent = `total_usage / total_credits * 100`; `detail` carries dollars used, total and left; the key endpoint supplies spend today, week and month as secondary text. A missing key is `not_configured`.
- OpenCode-Go and Antigravity keep their existing fetchers; their windows gain word labels and the `pct` contract. Antigravity `not_configured` carries a `hint` naming the missing login file.
- Provider fetchers sit behind one small interface (fetch to windows and state) so the refresher, the cache and the endpoint stay provider-agnostic. Each fetcher is testable against a local HTTP stub.
- Live: a `quota` live topic carries the provider list whenever a refresh changes it; the page subscribes on mount and patches cards. Manual refresh calls the existing refresh endpoint, which still skips the age guard but enforces a minimum interval per provider (default 10s) so repeated clicks never hammer an upstream.
- UI: the Quota area uses the redesigned shell. The second sidebar lists the Quota entry (a Providers entry arrives with the Providers spec) and a Live status list fed by the same live topic. The page no longer collapses the second sidebar. Cards, states (ok, stale, expired, not connected) and phone layout follow the mock. Colors come only from `src/app.css` tokens. Logic stays in colocated `lib/*.js`, markup in `lib/*.svelte`.
- Percent is the standard for every provider; the Go layer produces it, the UI never computes a percent from dollars.
- Docs: update `emberloop-web/AGENTS.md`, the Quota rows in `emberloop-web/web/README.md`, and the root index.

## Testing Decisions

- A good test checks external behavior only: HTTP response shape, live topic payload, rendered text, route result.
- Go: tests at the `/api/quota` and `/api/quota/refresh` HTTP seam with a temp state dir and local HTTP stubs for each upstream (Claude usage, ChatGPT usage, OpenRouter credits and key, OpenCode-Go, Antigravity). Cover: cached response returns before a slow upstream finishes; one failing provider leaves the others intact; 401 and expired token give `expired` with last values; `Retry-After` backs off; the poller never writes the credentials file; missing login gives `not_configured` with a hint; minimum refresh interval; shared cache and lock with header panes. Prior art: `emberloop/usage_providers_test.go`, `emberloop-web/quota_test.go`.
- Go: a time-bounded test asserting the cached `GET /api/quota` answers under 200ms while an upstream stub blocks.
- JS: vitest on the router (`/quota`), quota helpers (threshold, countdown, stale label, word labels), the live status list, and the page component (cards per state, secondary detail text, refresh buttons, live patching). Prior art: colocated `*.test.js`, `QuotaPage.test.js`.
- No live tests in the check. One gated live probe under `scripts/` may read the real Claude and ChatGPT endpoints, read-only.

## Out of Scope

- The Providers page and all connection flows (API keys, device code, Antigravity paste, Claude login pty). They get their own spec.
- Refreshing or writing any login token.
- Per-request token and cost totals, alerts, notifications on high usage.
- Changes to the terminal header strip.
- Converting ChatGPT message ranges or Claude usage into tickets per window.
- Project-scoped quota. Quota stays global.

## Further Notes

- Claude prior art: every percent tool polls the OAuth usage endpoint read-only; a refresh race with a running Claude Code logs the user out, which is why this feature never refreshes.
- The Claude usage response also carries `seven_day_opus`, `seven_day_sonnet`, `limits[]`, `extra_usage` and a weekly breakdown. This spec maps only 5-hour and Weekly; extra windows may be added later without a contract change.
- Antigravity OAuth client path differs between `usage_providers.go` (`~/.el/agy-oauth.json`) and older notes (`~/.emberloop/agy-oauth.json`). Use the `~/.el` path from the code and name it in the hint.
- Ticket concurrency: the global Settings `max_tickets` caps each Run; a spec's `max_tickets` frontmatter can only lower it. This spec sets none.
