# 20: One coalesced Runs refetch

**What to build:** One Run change causes one request. The sidebar and the Runs page share one refetch, and a burst of Run events triggers a single request (spec Part 2, Speed).

**Blocked by:** 17-runs-list-summaries (uses the summary shape)

**Status:** ready-for-agent

- [ ] `App.svelte` and `RunsSidebar.svelte` no longer each refetch the Runs list; one shared store refetches
- [ ] A burst of Run events within the coalesce window triggers one request
- [ ] Vitest covers the coalescing; repo check and `npm test` pass
