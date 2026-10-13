# 08: OpenRouter credits card

**What to build:** The Quota page shows an OpenRouter card with credits used as a percent and the dollars left, plus recent spend as secondary text, so the user sees the burn rate and when the balance runs low. A missing key reads as not connected, never as an error.

**Blocked by:** 05-percent-contract

**Status:** ready-for-agent

- [ ] Reads the pig OpenRouter key; credits percent is total usage over total credits times 100; `detail` carries dollars used, total and left (reproduces the observed shape: 94% used, `$59.16 of $63.00 · $3.84 left`)
- [ ] Key spend today, this week and this month appears as secondary text (0.91 / 2.37 / 5.70)
- [ ] Missing key yields `not_configured`
- [ ] Go tests at the HTTP seam with local stubs (credits plus key endpoints) and temp state dir; repo check passes
