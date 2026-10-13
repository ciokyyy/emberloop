# 11: Quota card redesign with secondary detail and states

**What to build:** Every quota card reads at a glance and matches the design reference: percent used first with header-strip threshold colors (calm below 60, warm below 80, hot from 80), dollars and tokens as secondary text under the bar, word-form window labels, a reset countdown per window, the plan badge, an "updated Xs ago" stamp, and distinct ok / stale / expired / not-connected states with a one-click link to where a missing login gets connected.

**Blocked by:** None (can start immediately; builds to the spec response contract with stubbed payloads)

**Status:** ready-for-agent

- [ ] Cards render `pct`, `detail`, word labels, countdowns, plan badge and age stamp from the contract; bars and status read only `pct`, never compute a percent from dollars
- [ ] Stale keeps last values with age; expired dims last values with the hint and a reconnect link; not-connected names the missing piece with a connect link
- [ ] Per-card and refresh-all buttons show a busy state and keep values during refresh
- [ ] Colors come only from the app tokens; logic stays in colocated lib helpers, markup in the page component
- [ ] Vitest covers helpers (threshold, countdown, stale label, word labels) and the page (cards per state, secondary detail text, buttons); repo check passes
