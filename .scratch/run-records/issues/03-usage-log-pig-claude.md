# 03: Per-agent token usage log for pig and claude

**What to build:** Every request a pig or claude agent makes ends up as a line in that agent's usage log: `{agent, at, harness, provider, model, input, output, cacheRead, cacheWrite, reasoning, reportedCost?}`, raw, with the harness-reported cost exactly as reported (pig's per-message `usage.cost`, claude's per-message usage and its `cost-state` totals). The session's own wick process tails the harness transcript and appends as requests happen. A usage-parser interface per harness turns transcript lines into records; agy plugs in later.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Parser interface with pig and claude implementations
- [ ] Fixture tests from real pig and claude transcript lines, including claude's cost-state record
- [ ] Tailing resumes from a stored offset, never double-counts
- [ ] A final pass when the session settles
- [ ] No wick-side price calculation
