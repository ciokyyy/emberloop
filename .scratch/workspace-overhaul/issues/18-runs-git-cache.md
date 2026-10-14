# 18: Short git result cache for Runs

**What to build:** Repeated Runs list loads do not re-run `git`. A cache with a TTL of a few seconds holds `git` results per Run and per Ticket, and the Run's own events invalidate it (spec Part 2, Speed).

**Blocked by:** 17-runs-list-summaries (both change the Runs list path)

**Status:** ready-for-agent

- [ ] `git` results per Run and per Ticket come from a short TTL cache
- [ ] A Run event invalidates that Run's entries
- [ ] Go tests cover a cache hit, TTL expiry and invalidation by a Run event
- [ ] Repo check passes
