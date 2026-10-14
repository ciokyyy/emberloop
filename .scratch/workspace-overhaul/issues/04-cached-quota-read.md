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
- **After change:** paired measurements from the reusable timing test, five runs: 2.250ms, 1.027ms, 768µs, 644µs, 1.156ms (median 1.027ms). Its synchronous path measured 303.2–303.8ms per run.

## Verification notes

- Re-run paired timings with `cd emberloop-web && go test -run '^TestQuotaEndpointTiming$' -count=5 -v`. The test uses separate temp cache dirs and a local upstream stub delayed 100ms per request; it compares the old synchronous handler path with the current cached GET. The pre-change samples above were measured before editing production code with `cd emberloop-web && go test -run '^TestMeasureQuotaBaseline$' -count=5 -v`.
- The under-200ms blocking-upstream assertion is independently rerunnable with `cd emberloop-web && go test -run '^TestQuotaGetReturnsCachedDataBeforeBlockedRefresh$' -count=1 -v`.
- Tests that need populated upstream values now seed them through synchronous `POST /api/quota/refresh`; GET deliberately returns the cached snapshot without waiting for the fetch.
- Ticket 10 (`10-live-topic-refresher`) owns subscriber-gated fetching and publishing updates to open pages. This ticket starts a guarded background refresh per GET but does not push results; a reload is needed to observe them until Ticket 10 lands.
