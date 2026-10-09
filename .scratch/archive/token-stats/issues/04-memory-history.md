# 04: Memory history

**What to build:** Memory samples are indexed and the Stats tab and `stats --json` show, per harness, model and role: peak, average and p95 RSS per session; per session, memory over time split by process; and the worst sessions by peak, each openable to that session.

**Blocked by:** 01, run-records 06

**Status:** ready-for-agent

- [ ] Aggregates tested over known samples
- [ ] Per-process split lets harness growth be told apart from child processes
