# 02: Credit flag, per-request failover and balance poll

**Type:** code

**What to build:** A 402 from a rung's provider flags that provider out of credit and retries the request once on the next eligible rung. A 5xx or a 429 with Retry-After tries the next eligible rung once. No eligible rung surfaces the error as a credit failure to the shared classifier. The flag clears from the DeepSeek balance endpoint (poll about every 10 minutes while flagged, one check at Daemon start) and by a timed re-probe for providers with no balance endpoint. A flagged provider's rungs are skipped.

**Blocked by:** 01-ladder-probe-and-resolve

**Status:** ready-for-agent

- [ ] A 402 from the DeepSeek fake moves that request to Relace and flags DeepSeek
- [ ] A flagged provider receives no requests until the poll or re-probe clears the flag
- [ ] Both rungs failing surfaces one credit failure and the Chain advances as before
- [ ] A 5xx retries once on the other rung, then surfaces
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
