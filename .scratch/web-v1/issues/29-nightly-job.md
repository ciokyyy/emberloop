# 29: Nightly job

**What to build:** At 03:00 local when `session.Idle()` holds, and on demand: snapshot, mine settled Sessions, distill (one cheap-LLM call per Session with new observations, sentinel `FLUSH_OK`, 15 KB input cap), contradiction pairing, Profile patch proposals, anchor refresh, run summary. Each stage writes a ledger row and a failure does not stop later stages. No decay, heat or compression stage.

**Blocked by:** 07, 19, 20, 22, 23

**Status:** ready-for-agent

- [ ] The job runs only when idle and logs a ledger row per stage
- [ ] A failing stage leaves later stages running and is visible on the Memory page
- [ ] Snapshot failure aborts the run before any write
- [ ] A Session with no durable content yields `FLUSH_OK` and no queue item
- [ ] Distill input is redacted and capped
- [ ] Nothing is deleted; tests assert row counts before and after
- [ ] Tests drive the job with the fake `Memory`, fake Decider and a fake clock
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
