# 03: Cached non-blocking quota read with measured baseline

**What to build:** Opening the Quota page never waits on upstream APIs. The Daemon's quota endpoint answers at once from the cache with each provider's state and update time, while any fetching happens in the background. The speed claim is verified, not assumed: this ticket measures the endpoint's response time and records it here.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `GET /api/quota` answers from the cache before a slow upstream stub finishes
- [ ] Time-bounded Go test at the HTTP seam (temp state dir, blocking upstream stub) asserts the cached read answers under 200ms
- [ ] Baseline response time of the current endpoint is measured BEFORE any code change, then the new time after; both recorded in this ticket
- [ ] Existing quota HTTP-seam tests still pass; repo check passes
