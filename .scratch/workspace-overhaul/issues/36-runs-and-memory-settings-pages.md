# 36: Runs and Memory settings pages

**What to build:** The Runs settings page labels the cap "Maximum tickets per Run (spec)" and explains that `max_tickets` is global, caps each Run, and a spec value can only lower it. The Memory page shows the decider as a segmented control and keeps the custom Jev endpoint collapsed, empty meaning OpenRouter (spec Parts 2 to 6, Settings).

**Blocked by:** 34-settings-hub

**Status:** done

- [x] Runs page shows the new cap label and explanation
- [x] Memory page shows the decider as a segmented control and the Jev endpoint collapsed
- [x] Vitest covers both pages; repo check and `npm test` pass
