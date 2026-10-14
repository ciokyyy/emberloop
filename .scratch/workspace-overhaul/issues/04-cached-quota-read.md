# 04: Cached non-blocking quota read with measured baseline

**What to build:** Opening the Quota page never waits on upstream APIs. The Daemon's quota endpoint answers at once from the cache with each provider's state and update time, while any fetching happens in the background. The speed claim is verified, not assumed: this ticket measures the endpoint's response time and records it here.

**Blocked by:** 03-suite-time-target-and-check-timeout (Part 7 goes first)

**Status:** ready-for-agent

- [ ] `GET /api/quota` answers from the cache before a slow upstream stub finishes
- [ ] Time-bounded Go test at the HTTP seam (temp state dir, blocking upstream stub) asserts the cached read answers under 200ms
- [ ] Baseline response time of the current endpoint is measured BEFORE any code change, then the new time after; both recorded in this ticket
- [ ] Existing quota HTTP-seam tests still pass; repo check passes

## Response-time measurements

- **Before change:** cold `GET /api/quota`, 100ms delay on each local upstream request, five runs: 304.8ms, 304.1ms, 303.9ms, 303.5ms, 303.8ms (median 303.9ms). This was measured against the existing synchronous handler before changing production code.
- **After change:** same cold endpoint and 100ms-per-request local upstream delay, five runs: 1.500ms, 1.090ms, 0.927ms, 1.081ms, 1.062ms (median 1.081ms). The three delayed upstream requests finished in the background after each response.
