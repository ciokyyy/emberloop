# Workspace overhaul (combined spec)

This spec combines everything agreed in the 2026-10-13 grilling Session into one document. It stands on its own: an agent can implement from it without reading the two source specs. The source specs stay as they are and are not edited:

- `.scratch/quota-tab/spec.md` covers Part 1. Its open tickets moved into this spec's `issues/` (04 to 16); quota-tab keeps its done tickets 01 and 02.
- `.scratch/projects-providers-settings/spec.md` covers Parts 2 to 6.

If a source spec and this document disagree, this document decides.

## Overview and order

| Part | Deliverable | Depends on |
|---|---|---|
| 0 | Already shipped in this Session (adapter retry, disk hygiene) | none, listed for context |
| 1 | Quota tab: non-blocking reads, percent standard, Claude, ChatGPT Plus, OpenRouter, redesigned shell | none |
| 2 | Speed: Runs list summaries, git cache, Session index, non-blocking boot | none |
| 3 | Providers page and connect flows | Part 1 (shell) |
| 4 | Project scope: switcher, Dashboard, `/p/<id>` URLs | Parts 1 and 2 |
| 5 | Draft project pick with jev | Part 4 |
| 6 | Settings redesign: hub, Agents chains with effort | Part 1 (shell) |
| 7 | Run test suite must never stall | none, do first |

Parts 1, 3, 4 and 6 all edit the Rail, `areas.js` and `App.svelte`. Run them one at a time. Part 7 goes first because every agent runs the full web suite.

## Part 0: Shipped in this Session (context, no work)

- `adapter closed` handling (`emberloop-web/acpretry.go`): a prompt failing on a deliberately closed Session (end, pause settle, abandon, restart Shutdown) drops quietly. An unexpected adapter death respawns through `ResumeSession` and resends the same prompt, at most 3 sends with backoff 1s then 3s. Shared by claude, pig-acp and grok.
- Disk hygiene (`emberloop-web/disk.go`): Daemon janitor every 10 minutes (Go cache cap setting `disk_go_cache_cap_gb`, default 8 GiB, 0 disables; stale `go-build*`, `el-web-test-*`, `el-test-*` dirs older than 6h; worktrees of finished Runs older than 24h with a clean status, never forced), a disk guard that refuses new Runs and ticket retries when space is low (new chat Sessions stay allowed), an admin-only `GET /api/disk`, and a UI banner. Test leak fixes in `testsupport.TempDir` and both `TestMain` functions.
- Repo history was rewritten once to remove key-like test fixtures from `redact_test.go`. Old branches based on the previous history must be rebased before they can be pushed.

## Part 1: Quota tab


### Problem Statement

The Quota page is slow, partly wrong and limited. `GET /api/quota` blocks on sequential upstream fetches (up to 20s each, 25s for the handler), so the page sits on a skeleton. Claude quota is push-only: it shows "never updated" or a stale number unless a terminal pane runs the statusline, and headless ACP Sessions never trigger it. ChatGPT Plus and OpenRouter credits show nowhere. Antigravity is "not configured" on most hosts with no hint why. Windows use short labels (`5h`, `wk`, `mo`) and some cards show dollars where others show percent, so cards do not compare at a glance. The page collapses the second sidebar and does not follow the Settings redesign shell on phones.

### Solution

The Quota page serves cached values at once and refreshes in the background. The Daemon pushes new values to open pages over the live channel, so the page needs no manual reload. Every card shows percent used (0-100) first; dollars and tokens are secondary text. Providers: Claude (read-only host login poller, no prompt), ChatGPT Plus (pig `openai-codex` login, `wham/usage`), OpenCode-Go, Antigravity (Gemini and Claude + GPT-OSS pools; not-configured state names the missing login), OpenRouter (credits). The page sits in the redesigned shell: the Quota rail icon opens a second sidebar with the Quota entry and a Live status list (tone dot and percent per provider); on phones the same list fills the drawer. The design reference is `.scratch/settings-redesign/mock/index.html` (the `/quota` route and its sidebar).

Real values read from the host on 2026-10-13, which the cards must reproduce:

- Claude: 5-hour 36%, weekly 8%, plan Pro (`five_hour`, `seven_day`, each with `utilization` and `resets_at`).
- ChatGPT Plus: 5-hour 4% (`primary_window`, 18000s), weekly 4% (`secondary_window`, 604800s), no extra credits.
- OpenCode-Go: 5-hour 0%, weekly 7%, monthly 53%.
- OpenRouter: 59.16 of 63.00 credits used (94%), 3.84 left; key spend today 0.91, week 2.37, month 5.70.
- Antigravity: not configured on this host (no `~/.el/agy-oauth.json`).

### User Stories

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

### Implementation Decisions

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

### Testing Decisions

- A good test checks external behavior only: HTTP response shape, live topic payload, rendered text, route result.
- Go: tests at the `/api/quota` and `/api/quota/refresh` HTTP seam with a temp state dir and local HTTP stubs for each upstream (Claude usage, ChatGPT usage, OpenRouter credits and key, OpenCode-Go, Antigravity). Cover: cached response returns before a slow upstream finishes; one failing provider leaves the others intact; 401 and expired token give `expired` with last values; `Retry-After` backs off; the poller never writes the credentials file; missing login gives `not_configured` with a hint; minimum refresh interval; shared cache and lock with header panes. Prior art: `emberloop/usage_providers_test.go`, `emberloop-web/quota_test.go`.
- Go: a time-bounded test asserting the cached `GET /api/quota` answers under 200ms while an upstream stub blocks.
- JS: vitest on the router (`/quota`), quota helpers (threshold, countdown, stale label, word labels), the live status list, and the page component (cards per state, secondary detail text, refresh buttons, live patching). Prior art: colocated `*.test.js`, `QuotaPage.test.js`.
- No live tests in the check. One gated live probe under `scripts/` may read the real Claude and ChatGPT endpoints, read-only.

### Out of Scope

- The Providers page and all connection flows (API keys, device code, Antigravity paste, Claude login pty). They get their own spec.
- Refreshing or writing any login token.
- Per-request token and cost totals, alerts, notifications on high usage.
- Changes to the terminal header strip.
- Converting ChatGPT message ranges or Claude usage into tickets per window.
- Project-scoped quota. Quota stays global.

### Further Notes

- Claude prior art: every percent tool polls the OAuth usage endpoint read-only; a refresh race with a running Claude Code logs the user out, which is why this feature never refreshes.
- The Claude usage response also carries `seven_day_opus`, `seven_day_sonnet`, `limits[]`, `extra_usage` and a weekly breakdown. This spec maps only 5-hour and Weekly; extra windows may be added later without a contract change.
- Antigravity OAuth client path differs between `usage_providers.go` (`~/.el/agy-oauth.json`) and older notes (`~/.emberloop/agy-oauth.json`). Use the `~/.el` path from the code and name it in the hint.
- Ticket concurrency: the global Settings `max_tickets` caps each Run; a spec's `max_tickets` frontmatter can only lower it. This spec sets none.

## Parts 2 to 6: Speed, Providers, Project scope, Draft project pick, Settings


### Problem Statement

Everything the Quota spec leaves out of the design agreed with the owner on 2026-10-13 is still unbuilt.

- The sidebar lists every Project by itself and the Runs page needs a folder typed in. There is no way to see what runs in which Project, and a Session started outside a Project lands in the User's home directory under "Other".
- Accounts cannot be connected from the browser. ChatGPT, Antigravity and Claude logins and pig API keys need the pig CLI on the host, and the Settings page never writes `auth.json`.
- The Runs page and the first load are slow. `GET /api/runs` runs two `git` calls per Run and one `git show` per Ticket, each Ticket scans every Session, every Run change refetches the whole list from both `App.svelte` and `RunsSidebar.svelte`, and boot waits for all lists even on `/quota`.
- Settings is one long page of lists. The Run model fallbacks have no thinking effort and live apart from the role models, icons are inconsistent, and phones have no hub.

### Solution

Five deliverables, built in this order after the Quota spec has merged (they share the Rail, `areas.js` and `App.svelte` with it):

1. **Speed.** Run list summaries, a short git cache, a Session index and a non-blocking boot.
2. **Providers.** A global `/providers` page where an admin connects accounts and adds API keys.
3. **Project scope.** A Project switcher, a Dashboard at `/`, and `/p/<id>/...` URLs.
4. **Draft project pick.** When the first prompt is sent from "No project", jev suggests a Project and a banner asks to confirm.
5. **Settings redesign.** A hub with one page per concern, and one merged Agents page with a model chain per role.

Design references: `.scratch/settings-redesign/mock/index.html` (served at `https://kobo-scintilla-workplace.tailef9ae.ts.net/settings-mock/`) for the shell, Settings hub, Agents, Providers and phone layout; Part 1 above for Quota; `.scratch/project-suggest/jev-prompt-experiment.md` for the jev measurements.

### User Stories

Speed

1. As a user, I want the Runs page to open without waiting on `git` for every Run, so that it loads in well under a second.
2. As a user, I want the Runs list to carry summaries only, with Ticket bodies loaded when I open a Run, so that the list stays small.
3. As a user, I want git results cached for a short time, so that repeated list loads do not re-run `git`.
4. As a user, I want Ticket-to-Session lookups served from an index, so that cost does not grow with every Session.
5. As a user, I want the sidebar and the Runs page to share one refetch that coalesces bursts of Run events, so that one change causes one request.
6. As a user, I want the first route to render without waiting for the full Session and Run lists, so that `/quota` and `/runs` open fast on a cold Daemon.
7. As an operator, I want before and after timings recorded for `GET /api/runs` and boot, so that the speed claim is measured.

Providers

8. As an admin, I want a `/providers` page listing every pig provider with its connection status (stored key, environment variable, OAuth, or none), so that I see what is connected.
9. As an admin, I want to paste an API key for any API-key provider, so that I add OpenRouter, OpenCode-Go, Gemini direct, DeepSeek or OpenAI without the CLI.
10. As an admin, I want a stored key never sent back to the browser, only a "set" flag, so that keys cannot leak through the UI.
11. As an admin, I want to remove a stored key, so that I revoke a provider.
12. As an admin, I want ChatGPT to connect with a device code and a verification link, so that login works from a browser on another machine.
13. As an admin, I want Antigravity to connect by pasting the callback URL or code, so that the failing localhost page is not a blocker.
14. As an admin, I want Claude to connect through `claude auth login` run by the Daemon in a pty, with the link shown and the code pasted back, so that the host `~/.claude` login is written by Claude Code itself.
15. As an admin, I want the Claude login written only to `~/.claude` and never to pig, so that the two credentials stay separate.
16. As an admin, I want other OAuth providers to show the exact CLI command (for example `pig login <provider>`) with a copy button, so that nothing is a dead end.
17. As an admin, I want cloud-auth providers (Bedrock, Vertex, Cloudflare) listed as "configure via CLI", so that the list is complete.
18. As an admin, I want a configurable redirect base URL for browser OAuth, off by default and probed per provider, so that I can try a tailnet redirect without changing code.
19. As an admin, I want the card on `/quota` to refresh after a successful connect, so that I see the effect at once.
20. As a non-admin user, I want to see connection status read-only with Connect buttons locked, so that I know the state without changing the host login.
21. As a user, I want a "not connected" Quota card to link to Providers, so that I fix it in one click.
22. As a phone user, I want Providers to follow the phone layout of the mock, so that connect panels work on a small screen.

Project scope

23. As a user, I want one Project switcher instead of a sidebar listing every Project, so that the sidebar stays short.
24. As a user, I want an "All projects" Dashboard at `/` showing running and needs-you Sessions and Runs grouped by Project, so that I see what is active everywhere.
25. As a user, I want the Dashboard to hide idle history, so that it shows only what needs attention.
26. As a user, I want `/p/<projectId>` to scope the sidebar, Sessions and Runs to that Project, so that the URL says where I am.
27. As a user, I want `/p/<projectId>/runs` to list that Project's Runs with no folder to type, so that Runs follow the scope.
28. As a user, I want `/s/<id>` to stay canonical and set the scope from the Session's Project, so that existing links keep working.
29. As a user, I want URLs built from the stable Project id, so that renaming a Project breaks no link.
30. As a user, I want `/p/none` as a built-in scope for Sessions in my home directory, so that they stay reachable and visible on the Dashboard.
31. As a user, I want `/quota`, `/providers`, Memory, Users and account Settings to stay global, so that account pages do not depend on the Project.
32. As a user, I want `/p/<projectId>/settings` to show rename, remove and the editable Project gist, so that a settings page exists for later overrides.
33. As a phone user, I want the Project switcher and Dashboard in the phone drawer, so that scope works on phones.

Draft project pick

34. As a user, I want the first prompt sent from "No project" checked against my Projects before the Session starts, so that the Session starts in the right folder.
35. As a user, I want a banner "Run this in <Project>?" when jev is confident, so that one click puts the Session in that Project's path.
36. As a user, I want no banner when jev is unsure or the prompt is general, so that wrong suggestions stay rare.
37. As a user, I want each Project to have a one-line gist derived from its `AGENTS.md` or README, cached, so that jev has context.
38. As a user, I want to edit a Project's gist on its settings page, with my edit winning over the derived one, so that I correct a bad summary.
39. As a user, I want a Session that is already started never moved by this feature, so that the working directory never changes mid-Session.

Settings

40. As a user, I want `/settings` to be a hub with one page per concern, so that no page is a long list.
41. As a user, I want Workspace pages (Agents, Runs, Context and names, Memory, Users and access) and You pages (Profile, Notifications, Sound and voice), so that settings group by who they affect.
42. As a user, I want a scope pill on each page (Global, Admin, You), so that I know who a change affects.
43. As a user, I want one Agents page where each role has one ordered chain, so that role models and Run fallbacks are no longer separate lists.
44. As a user, I want every chain entry to set harness, model and thinking effort together, so that fallbacks carry an effort like the primary does.
45. As a user, I want entry 1 labelled Primary and the rest Fallback, with reorder, remove and add, and at most three entries, so that the chain is easy to read and edit.
46. As a user, I want the effort choices to list only what the chosen model supports, so that I cannot pick an invalid effort.
47. As a user, I want roles grouped as Session agents (Orchestrator, Subagent, Scout) and Run agents (Ticket worker, Spec reviewer, Standards reviewer), so that I find a role by where it runs.
48. As a user, I want the Runs page to label the cap "Maximum tickets per Run (spec)" and explain it, so that the scope of the cap is clear.
49. As a user, I want the Memory page to show the decider as a segmented control and keep the custom Jev endpoint collapsed (empty means OpenRouter), so that the common path is simple.
50. As a user, I want a save bar that appears on the first change with Revert and Save, so that edits are explicit.
51. As a user, I want saved settings from before the redesign to keep working, so that no role silently loses its model.
52. As a phone user, I want the hub list and a "< Settings" link on each sub-page, so that Settings works on phones.

### Implementation Decisions

Speed

- `GET /api/runs` returns summaries (id, spec, status, counts, repo, Project id, updated time). Ticket bodies and acceptance text load from a per-Run endpoint when the Run opens. This changes the list response shape, so `App.svelte`, `RunsSidebar.svelte` and `RunsView.svelte` change with it.
- A short TTL cache (seconds) holds `git` results per Run and per Ticket, invalidated by the Run's own events.
- The Ticket-to-Session lookup becomes an index built when a Session is attached to a Ticket, replacing the per-Ticket scan of all Sessions.
- The sidebar and the Runs page share one coalesced refetch. A burst of Run events triggers one request.
- Boot no longer awaits the Session and Run lists before the first route renders. Each view loads its own data.
- Measure `GET /api/runs`, Run-list render and cold boot before any change and record the numbers in the first ticket. Success is a recorded improvement, not an assumed one.
- Quota non-blocking reads are owned by the Quota spec and not repeated here.

Providers

- A global `/providers` route and a Providers entry in the Quota second sidebar (the mock groups Quota and Providers under one rail icon). Writes are admin-only; other Users see status read-only.
- The list comes from the pig SDK provider list. Each row reports connected via stored key, environment variable or OAuth. API-key rows write through `AuthStorage.Set` into the host pig `auth.json`; remove deletes the credential. Keys are never returned to the browser.
- Connect flows run as short-lived login sessions in the Daemon:
  - ChatGPT: the pig device-code login. The UI shows the user code and the verification link, then the card refreshes.
  - Antigravity: the existing prompt/callback flow in `pig-antigravity-auth/oauth.go`. The UI takes the pasted callback URL or code.
  - Claude: `claude auth login` in a pty. The UI shows the link and sends back the pasted code. The result lives in `~/.claude` only. Claude OAuth never goes to pig in this spec.
  - OpenCode-Go and other API-key providers: a password field.
  - Other OAuth and cloud-auth providers: the CLI command with a copy button, no flow.
- A redirect base URL is a config setting (a settings file key, default empty). When set, it is used only for providers where a probe shows the redirect is accepted. It is never the default. Providers register their own redirect URIs, so a tailnet redirect may be rejected; the probe decides.
- The mock groups rows as subscription logins (Claude, ChatGPT, Gemini via Antigravity), API keys (OpenRouter, OpenCode-Go, Gemini direct, DeepSeek, OpenAI) and CLI-only providers. Providers with no usage endpoint show connection status only.
- Providers stays a separate spec from Quota: `/quota` is read-only.

Project scope

- URL table: `/` Dashboard; `/p/<id>` Project scope; `/p/<id>/runs` Project Runs; `/p/<id>/settings` settings shell; `/p/none` home-directory Sessions; `/s/<id>` canonical Session URL that sets the scope; `/quota`, `/providers`, Memory, Users and account Settings global.
- A Run belongs to a Project through its repo path (deepest containing Project, as `owners.js` and the Daemon already map it). Memory stays one Bank per User.
- The Dashboard lists only running or needs-you Sessions and Runs, grouped by Project. Idle history stays in each Project view.
- The Project switcher has an "All projects" entry (the Dashboard) and filters the sidebar, Sessions and Runs. It replaces the per-Project sidebar sections.
- `/p/<id>/settings` ships as a shell: rename, remove (existing behavior) and the editable gist. No override fields.
- Project gist: derived from `AGENTS.md` or README, cached, stored with the Project; an edited gist wins and is never overwritten by the derivation.
- GLOSSARY.md: add Provider, Dashboard and quota percent next to the existing Project entry.

Draft project pick

- Trigger: the first prompt sent from a draft with no Project, before the Session starts. The Daemon asks jev (OpenRouter `systemone`, model `typesafe/jev-1.13`, client `emberloop-web/el/tools/systemone/systemone.go`, key from `systemone.APIKey()`) one `noul` question per Project plus one "none" question, in one call. No new key setting is needed.
- Wording per Project: name plus its one-line gist. Measured on 25 prompts and 4 Projects: names only 64% top-1; name plus path 64%; name plus gist 92% top-1 with 0% false picks on "none" prompts. Paths add nothing.
- Gate: accept only if top probability is at least 0.5 and the lead over the second is at least 0.2. Measured: 100% correct on 80% coverage. Misses fall to "none", which is the safe direction; below the gate no banner appears. Latency is about 300ms per call, so the banner can appear before the first turn.
- `memory.ValidateQuestions` allows only the five miner questions, so a project-pick question needs its own question type or a direct wire call. Thresholds are valid for `typesafe/jev-1.13` and must be re-measured if the model changes.
- Yes on the banner creates the Session with that Project's path. The Session never changes working directory afterwards.
- Without an OpenRouter key, no suggestion is made and the draft works as today.
- The existing live probe `emberloop-web/jev_projectpick_live_test.go` stays gated behind environment variables and out of the check.

Settings

- `/settings` becomes a hub. Workspace: Agents, Runs, Context and names, Memory, Users and access. You: Profile, Notifications, Sound and voice. Providers and Quota keep their own pages. Each page has its own URL; on phones `/settings` shows the hub list and each page a "< Settings" link.
- Agents: role rows and Run Chains merge into one chain per role. Each entry has harness, model and effort. The mock uses Default, Low, Medium, High and Max; the real page lists only the efforts each model supports, which must be checked per model. Entry 1 is Primary; others are Fallback 1 and 2; maximum three entries; reorder, remove and add.
- The mock renames roles: Worker becomes Subagent and Implementer becomes Ticket worker. The saved settings keys must keep working. A migration from the old role and chain keys to the merged form is part of the Agents ticket and ships with a test on a pre-redesign settings file.
- Runs page: "Maximum tickets per Run (spec)". `max_tickets` stays global in Settings, caps each Run, and a spec value can only lower it. There is no Daemon-wide total cap and no per-Project cap.
- Memory page: decider as a segmented control; custom Jev endpoint collapsed, empty meaning OpenRouter.
- Shell and colors follow the mock inside the real shell: rail, second sidebar, main; tokens only from `src/app.css`. Pills, white primary buttons, blue `#0a84ff` for focus only; 44px touch targets and 16px inputs on phones.
- Docs: update `emberloop-web/AGENTS.md`, `emberloop-web/web/README.md`, `GLOSSARY.md`, `.scratch/settings-redesign/AGENTS.md` and the root index.

### Testing Decisions

- Test external behavior only: HTTP shapes, live payloads, rendered text, route results.
- Speed: Go tests at the `/api/runs` seam asserting the summary shape, the per-Run detail endpoint, git cache hits and invalidation, and the index; a bounded-time test on a fixture with many Runs. JS tests for the coalesced refetch and a boot that renders before the lists arrive. Prior art: `runs_test.go`, `App.test.js`.
- Providers: Go tests with local stubs for each login flow and a temp pig dir. Cover the provider list and status sources, key set and remove (and that no response contains a key), admin-only writes, the device-code, paste and pty flows (a fake `claude` for the pty), and the redirect-base probe. JS tests for the page states and phone layout.
- Project scope: JS router tests for every URL in the table, scope from `/s/<id>`, `/p/none`, and rename-stable links. Go tests for the Dashboard payload (running and needs-you only) and the gist derivation and edit precedence.
- Draft pick: Go tests with a stubbed `systemone` endpoint covering the gate (accept, below top, below margin, none), wording per Project, no key, and a Session that starts in the chosen path. JS tests for the banner.
- Settings: vitest on the hub routes and each page; chain editing with effort per model; the old-to-new settings migration on a fixture; the save bar.
- No live tests in the check; the gated jev probe stays under the test file's environment guard.

### Out of Scope

- Per-Project overrides of roles, Chains, caps or `run_skills`. `/p/<id>/settings` is a shell only.
- Per-Project caps, a Daemon-wide `max_tickets_total`, and per-Project Rules.
- Moving an already-started Session to another Project, and an agent tool that changes the working directory mid-Session (with `AGENTS.md` injection). Revisit after seeing how often the draft pick misses.
- Per-User provider accounts; connecting pig's own Anthropic login as a second Claude row.
- Running two specs in the same Project in parallel (to be grilled in a later session).
- Quota page work (Part 1 of this spec).

### Further Notes

- Order: Speed, then Providers, then Project scope, then Draft pick. The Settings hub can run in parallel with Speed after the Quota spec merges, but not with Project scope, since both rewrite the Rail and `App.svelte`.
- Delivery risk: this spec and the Quota spec both edit the Rail, `areas.js` and `App.svelte`. Run them one after the other, not together.
- Facts to carry: pig has the ChatGPT and Antigravity logins (the earlier claim that it did not was wrong). Host Claude login has `user:profile`; the setup-token does not.
- Known gap: the no-prompt way to refresh an expired host Claude login is not found. Quota shows "login expired" and Providers' Claude Connect covers it.

## Part 7: The Run test suite must never stall

Do this part first: every Worker and Reviewer runs the full web suite, and a hang there blocks every Ticket.

### Problem

`go test .` in `emberloop-web` ran for more than 23 minutes with no progress and was killed by the 25 minute timeout. Load average stayed near zero, so nothing was computing; tests were waiting on each other.

### Findings from the goroutine dump

- Run tests share a semaphore, `runTestSlots` in `emberloop-web/runs_driver_test.go` (capacity 2, env `EL_TEST_RUN_PAR`). A test takes a slot in `initRunRepo` through `holdRunTestSlot` and frees it in a `t.Cleanup`.
- At the time of the dump, two tests (`TestAbandonRunRemovesBranchesAndWorktrees`, `TestRebaseConflictNeedsYouWithFiles`) never freed their slots. Their goroutine was inside `Daemon.stopRunsForTest`, blocked in `sync.Mutex.Lock` on `d.mu` for 23 minutes.
- The same stack shows why: the first `stopRunsForTest` call panicked while holding `d.mu` (it was inside the loop over `d.runs` and Tickets), so `d.mu` was never unlocked. The cleanup is registered twice (`runTestDaemon` and `authedHandler`), and the second call blocked on the held mutex. The `runsStopped` guard does not help, because it only skips a call that has already finished the `LoadOrStore`.
- With both slots lost, every later Run test blocked forever in `holdRunTestSlot` (`chan send`). Every other test only waited in `waitParallel`.
- The panic value was not printed. The Go timeout dump shows `panic({0x164d480?, ...})` without the message.
- Separately, some tests are slow on their own (`TestStopAfterSubmitRefusedKeepsWorking` takes about 30 seconds alone), so the full suite also takes many minutes even when healthy.
- Another change landed on master while this was being investigated: commit `3010137` ("Daemon: fix Run test flakes and the final-merge resume count") touches `runs_driver_test.go`. Re-read the current code before changing it.

### Requirements

- `stopRunsForTest` must never leave `d.mu` held: take it with `defer` or recover from a panic, and report the panic value through `t.Errorf` or `t.Logf` instead of swallowing it. It must still never fail a test by itself because of its own wait.
- Releasing a Run test slot must not depend on any other cleanup. Free the slot in its own `t.Cleanup` registered first, so it runs last, and make it unconditional.
- Name the cause of the original panic. Reproduce it by running the two holding tests with `-run` and `-count` and `-race`, and fix it at the source.
- Add a suite watchdog: when a test waits more than 60 seconds for a Run slot, log which tests hold slots. A hung suite then names its culprits instead of printing nothing for 25 minutes.
- Add a per-test deadline for Run tests (derived from `t.Deadline()` or a context) so one stuck test fails with its own name and goroutine dump, not the whole package.
- Record the healthy full-suite wall time and the ten slowest tests. Set a target (for example under 10 minutes with `EL_TEST_RUN_PAR` left at its default) and fix or split the tests that dominate it.
- The repo check (`.emberloop/check`) must run the web suite with an explicit `-timeout`.

### Testing

- A test that panics inside a Run test body must free its slot and leave `d.mu` unlocked: run it as a subtest under a helper and assert that the next test can take the slot.
- A unit test of the slot watchdog with a fake clock.
- Prior art: `emberloop-web/tempdir_test.go` guard tests.

### Out of Scope

- Rewriting the Run tests to use fewer real `git` subprocesses, unless needed to reach the time target.
