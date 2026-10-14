# 12: Quota page and card redesign to the mock

**What to build:** The Quota page and every card match the design reference `.scratch/settings-redesign/mock/index.html` (the `/quota` route, `quotaPage` and `quotaCard`; served at `https://kobo-scintilla-workplace.tailef9ae.ts.net/settings-mock/`). Every card reads at a glance: percent used first with header-strip threshold colors, dollars and tokens as secondary text, word-form window labels, a reset countdown per window, and distinct ok / stale / expired / not-connected states with a one-click link to where a missing login gets connected.

Design details from the mock:

- Page header: gauge icon, title "Quota", an outline "Read-only" scope badge, the description "Percent used per account window, and when it resets. Dollars and tokens are secondary. Updates live.", and a "Refresh all" outline button on the right.
- Cards in a grid, two columns from `sm`, one column below; each card `rounded-2xl border bg-card p-4`, entering with `animate-rise`.
- Card head: provider icon tile, provider name, the plan as an outline badge (Pro, Plus, Go, Credits, Gemini + 3P), a state badge (stale: warning with clock icon; login expired: warning with lock icon; not connected: secondary), a subline "updated 8s ago · <source>" (source is Host login, pig login or API key; stale reads "updated 12m ago · retrying", expired reads "last read 3h ago"), and an icon-only refresh button labelled "Refresh <name>".
- Window rows: label left, percent right in the tone color with tabular numbers, then a `h-1.5` bar on a muted track with `role="progressbar"` and `aria-valuenow`; reset countdown and `detail` under the bar.
- A window with no percent (OpenRouter "Spend this week") renders as a text row above a top border: label left, secondary text right ("$2.37 · today $0.91 · month $5.70"), no bar.
- Expired: an amber callout above dimmed (`opacity-50`) windows: "Host login expired, so quota cannot refresh. Run `claude auth login` or reconnect in Providers." with the link.
- Not connected: the hint text (Antigravity names `~/.el/agy-oauth.json` and both pools) and a "Connect in Providers" outline button with a plug icon linking to `/providers`.
- Thresholds: calm below 60 green-500, warm below 80 amber-500, hot from 80 red-500. The mock's state preview control is not part of the product.

**Blocked by:** 03-suite-time-target-and-check-timeout (Part 7 goes first; builds to the spec response contract with stubbed payloads)

**Status:** ready-for-agent

- [ ] Page header, grid and card layout match the mock on desktop and phone widths
- [ ] Cards render `pct`, `detail`, word labels, countdowns, plan badge, state badge and the source subline from the contract; bars and status read only `pct`, never compute a percent from dollars
- [ ] Text-only windows (no `pct`) render as a bordered text row without a bar
- [ ] Stale keeps last values with age; expired shows the callout and dims last values; not-connected shows the hint and "Connect in Providers"
- [ ] Per-card and refresh-all buttons show a busy state and keep values during refresh
- [ ] Colors come only from `src/app.css` tokens and the threshold classes; logic stays in colocated `lib/*.js` helpers, markup in `lib/*.svelte`
- [ ] Vitest covers helpers (threshold, countdown, stale label, word labels) and the page (cards per state, text-only window, secondary detail text, buttons); repo check and `npm test` pass
