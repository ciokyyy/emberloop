# 18: Short git result cache for Runs

**What to build:** Repeated Runs list loads do not re-run `git`. A cache with a TTL of a few seconds holds `git` results per Run and per Ticket, and the Run's own events invalidate it (spec Part 2, Speed).

**Blocked by:** 17-runs-list-summaries (both change the Runs list path)

**Status:** done

- [x] `git` results per Run and per Ticket come from a short TTL cache
- [x] A Run event invalidates that Run's entries
- [x] Go tests cover a cache hit, TTL expiry and invalidation by a Run event
- [x] Repo check passes
