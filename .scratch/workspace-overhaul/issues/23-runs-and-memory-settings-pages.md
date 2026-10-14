# 23: Runs and Memory settings pages

**What to build:** The Runs settings page labels the cap "Maximum tickets per Run (spec)" and explains that `max_tickets` is global, caps each Run, and a spec value can only lower it. The Memory page shows the decider as a segmented control and keeps the custom Jev endpoint collapsed, empty meaning OpenRouter (spec Parts 2 to 6, Settings).

**Blocked by:** 21-settings-hub

**Status:** ready-for-agent

- [ ] Runs page shows the new cap label and explanation
- [ ] Memory page shows the decider as a segmented control and the Jev endpoint collapsed
- [ ] Vitest covers both pages; repo check and `npm test` pass
