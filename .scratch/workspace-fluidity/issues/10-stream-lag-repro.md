# 10: Red-first repro of streaming lag

**Type:** code

**What to build:** A test that feeds a large synthetic Session (5,000 events) a high chunk rate (200 per second) with an interleaved tool call, runs the real store, grouping and flush path, and asserts a work-per-flush budget. It fails on the current code. Report the first profile (where the time goes: array copy, regroup, markdown, measure) and retune the budget numbers if the data says so.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The test fails on current code with the measured numbers in the message
- [ ] The profile findings are written in the ticket result
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
