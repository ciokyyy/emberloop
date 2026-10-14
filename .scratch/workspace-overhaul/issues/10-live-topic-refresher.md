# 10: Live quota topic with subscriber-gated background refresher

**What to build:** Open Quota pages stay current with no reload and no polling. The Daemon keeps each provider fresh on its own cadence and publishes the changed provider list on a live `quota` topic. The refresher runs only while at least one Quota page (or the Live status list) is subscribed, plus a cold start read from cache — it stops when nobody listens, so no upstream requests run while the user works elsewhere. Daemon fetches share the cache and lock with header panes, so N viewers cause one upstream request.

**Blocked by:** 05-concurrent-fetch-isolation, 06-percent-contract

**Status:** done

- [x] A `quota` live topic carries the provider list whenever a refresh changes it (verified at the live seam: subscribe, force a stub change, observe the payload)
- [x] Refresher runs only with at least one subscriber; cold start serves the cache at once; no upstream calls happen with zero subscribers
- [x] Shared cache files and per-key lock with header panes stay; header pane behaviour and its tests are unchanged
- [x] Existing 90s age guard stays for page-load reads
- [x] Go tests for the topic payload, the subscriber gating and the shared lock; repo check passes
