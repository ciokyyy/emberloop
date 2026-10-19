# Workspace fluidity

One spec from the 2026-10-19 grilling Session. It has five Parts: A, the Ladder (peak-aware, credit-aware model routing for DeepSeek Flash through Relace, plus credit-based quota cards); B, a compact, filterable sidebar; C, streaming and history performance; D, fluid navigation with Project-scoped Settings and Memory; E, an offline cache (listed, no Tickets yet).

## Problem Statement

1. DeepSeek bills twice as much in peak hours as off-peak, and a Session stops dead when DeepSeek credit runs out. The User has a Relace account that serves the same Flash model at a price below DeepSeek's peak price, and has no way to say "use DeepSeek off-peak, Relace at peak, Relace when DeepSeek is out of credit".
2. Quota shows percentages for providers that bill in credits. A percentage of credit means nothing; the User wants dollars left.
3. The Chats sidebar mixes active and old Runs and Sessions in one long list. Rows are 48 px with a meta line, so finding one chat means scrolling.
4. A fast model (DeepSeek) makes the UI lag while it streams, on phone and on desktop with animations off. Some history loads transfer a whole log at once and lag.
5. Moving between areas does not feel guided: scope is dropped on Quota, Memory and Settings, areas forget where the User was, and there is no quick way to jump.

## Solution

- **Part A.** A Ladder is a named virtual model. A rung names a provider and a model and says when it applies. The seeded Ladder `deepseek-flash` serves DeepSeek Flash from DeepSeek off-peak and from Relace at peak, and skips any provider that is out of credit. The User picks the Ladder anywhere a model is picked, and a Chain entry may point at one. Each turn records which provider served it and what it cost. Quota cards for providers that bill in credits show dollars left; rate-limit providers keep percent windows.
- **Part B.** The sidebar gets an Active/All control (Active by default), time groups in All, and single-line 32 px rows in the style of DeepSeek's chat list. Phone rows stay 44 px.
- **Part C.** The transcript stays smooth at a high chunk rate; opening a Session and loading child Sessions are paged.
- **Part D.** Each area remembers its position, entering a Project opens its last Session, lists show their last payload instantly, a quick switcher jumps anywhere, a scope chip always shows where the User is, and Settings and Memory work per Project.
- **Part E.** TanStack DB with an IndexedDB collection as an offline read cache. Out of scope until a later spec.

## User Stories

1. As a User, I want DeepSeek Flash Sessions to use Relace during DeepSeek peak hours, so that I pay less.
2. As a User, I want the same Sessions to use DeepSeek off-peak, so that I use the cheapest source.
3. As a User, I want a Session to move to Relace when DeepSeek reports no credit, so that work does not stop.
4. As a User, I want the move to happen on the request that hit the problem, so that no turn fails.
5. As a User, I want the switch at the start of peak to take effect on the next request, so that I am never billed peak.
6. As a User, I want to pick the Ladder like any model, so that Sessions, Runs, compress and Subagents all use it.
7. As a User, I want a Chain entry to point at a Ladder, so that harness failover and provider routing combine.
8. As a User, I want to edit rungs, their providers and models, and their schedules in Settings, so that I set this up without files.
9. As a User, I want the editor to show every rung and mark the one active now, so that I see where requests go.
10. As a User, I want the peak schedule editable with the official hours as the default, so that I can follow DeepSeek if it changes them.
11. As a User, I want each turn to say which provider served it and what it cost at that price, so that I can check the savings.
12. As a User, I want DeepSeek's balance on the quota page in dollars, so that I know when to top up.
13. As a User, I want OpenRouter to show dollars left, not a percentage, so that the number means something.
14. As a User, I want Relace to say "credits unknown" while no balance source exists, so that the card does not lie.
15. As a User, I want OpenCode-Go to keep its percentage windows, so that rate limits stay visible.
16. As a User, I want to set the Relace key in the Providers page, so that I do not edit `auth.json`.
17. As a User, I want an error when every rung fails to reach the normal Chain failure path, so that existing recovery still works.
18. As a User, I want the sidebar to default to Active, so that I see only what needs me or runs now.
19. As a User, I want All to group by Today, Yesterday, Previous 7 days and older, so that I find a chat by when I used it.
20. As a User, I want rows to take one line, so that more chats fit on screen.
21. As a User, I want search to stay at the top of the sidebar, so that I jump to a chat by name.
22. As a phone User, I want 44 px touch rows, so that I do not mis-tap.
23. As a User, I want the transcript to stay smooth while DeepSeek streams, so that I can read as it writes.
24. As a User, I want opening a long Session to show the last turn at once, so that I do not wait on the whole log.
25. As a User, I want opening a child Session to load only its tail, so that a Group card does not stall.
26. As a User, I want each area to remember my last position, so that switching back restores it.
27. As a User, I want entering a Project to open its last Session, so that I land where I stopped.
28. As a User, I want lists to appear instantly from the last payload and refresh after, so that I never see an empty flash.
29. As a User, I want a quick switcher on Ctrl/⌘+K, so that I jump to any Session, Run, Project or page by name.
30. As a User, I want a scope chip or breadcrumb at all times, so that I know which Project I am in.
31. As a User, I want Back and forward to behave the same on every area, so that I can predict them.
32. As a User, I want Project Settings to override model Chains and instructions for that Project only, so that one Project can use cheaper models.
33. As a User, I want Memory inside a Project to show that Project's notes and Rules first, so that I see what applies.
34. As a User, I want Quota to stay global, so that I see all providers in one place.
35. As an operator, I want a failing high-rate stream test before the fix, so that the fix is proven.

## Implementation Decisions

### Part A: Ladder

- A Ladder is a provider-level virtual model, exposed to pig as the provider `ladder` with one model per Ladder (`ladder/<name>`). It resolves each request, inside pig's stream path, to a concrete provider and model, and delegates the stream. It is not a Chain and not a harness.
- The Ladder lives in the pig provider layer so Sessions, Runs, compress and Subagents all get it. Run Chains snapshot the id `ladder/<name>`, which stays stable; the route changes per request.
- Ticket 01 is a probe: confirm the pig SDK lets a provider stream delegate to another provider's model. If it does not, the fallback design is a local OpenAI-compatible loopback endpoint on the Daemon that the `ladder` provider points at. The rest of the spec does not depend on which one wins.
- A rung is `{provider, model, when}`. `when` is `always`, `in <schedule>` or `outside <schedule>`. Rungs are evaluated top to bottom per request. The first rung whose `when` holds and whose provider is not flagged out of credit serves the request.
- A schedule is a named set of UTC weekly windows. The seeded schedule `deepseek-peak` is Monday to Friday, 01:00–04:00 and 06:00–10:00 UTC. The Chinese holiday calendar is ignored; holidays count as peak.
- The seeded Ladder `deepseek-flash`: rung 1 is DeepSeek `deepseek-flash` outside `deepseek-peak`; rung 2 is Relace `deepseek-ai/DeepSeek-V4.1-Flash`, always. Scope is Flash. The rung table is data, so Pro can follow.
- Credit is a provider state, not a rung condition. A 402 flags the provider out of credit; the request retries once on the next eligible rung. A flagged provider's rungs are skipped. If no rung can serve, the error reaches the shared failure classifier as a credit failure, and the existing Chain behavior applies.
- A transient failure (5xx, 429 with its Retry-After) tries the next eligible rung once. Both failing surfaces the error.
- Clearing the flag: DeepSeek polls its balance endpoint about every 10 minutes while flagged, and checks once at Daemon start. Providers with no balance endpoint (Relace today) are retried on a timed re-probe of the same period; a 402 re-flags.
- The route is chosen per request. At the start of peak the next request moves to Relace. The prompt cache is lost at each boundary; that is accepted.
- Usage events carry the provider that served the turn and the cost at that provider's price. The transcript turn shows a small badge such as "via Relace · peak" or "via DeepSeek · off-peak".
- Relace and DeepSeek are managed API-key providers in the Providers page. Relace gets its models from its `/v1/models` catalog; prices and reasoning flags come from it.
- Settings has a Ladder editor: rungs, providers and models, schedules, the active rung marked, balance status. It is also the place that lists both legs of the seeded Ladder.
- Quota cards have one of two kinds. `credits` cards show dollars left; `windows` cards show percentages. A card is never both. OpenRouter, DeepSeek and Relace are `credits`; OpenCode-Go, Claude, ChatGPT and Antigravity stay `windows`. DeepSeek's dollars come from its balance endpoint. Relace shows "credits unknown, healthy until a 402".

### Part B: Sidebar

- The sidebar gets an Active/All control. Active is the default and shows running, idle and needs-you items. All shows everything, grouped by Today, Yesterday, Previous 7 days and older. Active keeps Project sections and shows only the User's own Sessions unless Everyone is picked.
- Rows are single-line on desktop at about 32 px: status glyph, title, unread dot. Meta moves to a tooltip and the selected row's detail. Hover reveals the actions menu. Phone rows are 44 px.
- Filtering, grouping and the Active rule live in the sidebar data module, not in components, so they test without a DOM.

### Part C: Streaming and history

- Profile first. A repro drives a high chunk rate with an interleaved tool call into a large synthetic Session and fails on a work-per-flush budget before any fix.
- The event store updates in place and flushes on a frame boundary instead of copying the whole array per flush.
- Grouping merges the tail on every event kind, not only chunk and tool-call events, so a non-chunk event no longer forces a full regroup.
- Child-Session loads in Group cards and Run pages use the same tail paging as the main Session view.
- Budget at the start: frame time p95 under 16 ms at 200 chunks per second with a tool call interleaved, for a 5,000-event Session, on a throttled CPU. Opening a Session shows its last turn in under 300 ms from the Daemon's paged tail. The first profile may retune both numbers.

### Part D: Navigation

- Each area (Chats, Runs, Quota, Memory, Settings) remembers its last position and restores it on return.
- Entering a Project opens its last Session.
- Lists render their last payload at once and refresh behind it; no empty or spinner flash on a revisit.
- Rows prefetch on hover or touchstart.
- A quick switcher opens on Ctrl/⌘+K and from a visible button on phone. It searches Sessions, Runs, Projects and pages.
- A scope chip or breadcrumb shows the current Project and area in every page header.
- Project-scoped pages: Settings inside a Project overrides model Chains and instructions for that Project only; Memory shows that Project's notes and Rules first. Quota stays global. URLs follow the existing `/p/<id>/...` shape.
- Back and forward follow one rule on every area: they move through the User's last positions, and Back closes an open sheet first (existing behavior).

### Part E: Offline cache (not built here)

- TanStack DB with an IndexedDB collection as a read cache for Session events and lists, with the Daemon staying the source of truth. A later spec defines the sync contract.

### Seams

- The Daemon HTTP API with the offline `fakellm` server (`scripts/`) for Ladder routing, credit flags and failover. Highest seam; no live model calls.
- The web store and sidebar data modules under Vitest for Parts B, C and D logic.
- Design Tickets are judged at 390 px and 1280 px.

## Testing Decisions

- Test external behavior: which provider a request reaches for a clock time, a 402 and a 5xx; the quota card shape; the sidebar rows a given set produces; the work done per flush for a given event stream; the route and scope after a navigation.
- Ladder: fake clock across the peak boundary, fake provider returning 402 and 5xx, balance poll flag set and cleared. Prior art: the quota and usage tests in the Daemon, and the failure classifier tests.
- Sidebar and navigation: Vitest on the pure modules and the route table. Prior art: `router.test.js`, `owners` and `sidebar` tests.
- Streaming: the red-first repro, then green. Live check with `agent-browser` under CPU throttle in the closeout Ticket.
- Design Tickets: rendered check at 390 px and 1280 px.

## Out of Scope

- Part E.
- A Chinese holiday calendar.
- Ladders for Pro or non-DeepSeek models beyond the seeded one (the table is data and allows them later).
- A Relace balance endpoint, until Relace publishes one.
- Auto-switching a harness mid-Session.

## Further Notes

- Facts behind the numbers (2026-10-19). DeepSeek Flash is $0.30 in and $1.20 out per million tokens at peak, half of that off-peak. Relace's DeepSeek V4.1 Flash is $0.20 in and $0.60 out. DeepSeek peak hours: 01:00–04:00 and 06:00–10:00 UTC, Monday to Friday, excluding Chinese public holidays (official pricing page). DeepSeek out of credit returns HTTP 402 "Insufficient Balance". Relace out of credit returns HTTP 402 "Out of credits".
- The Relace provider and its key are already set in the pig agent directory (`models.json`, `auth.json`). Do not copy the key into any repo file.
- The Chain term stays. A Ladder is a model; a Chain is an ordered failure path of `{Backend, model, effort}` entries and already spans harnesses.
- Design Tickets cover desktop and phone.
