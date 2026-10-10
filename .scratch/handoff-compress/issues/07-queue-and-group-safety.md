# 07: Queue carry-over and Group wait

**What to build:** Queued follow-ups and held steers arrive after the Handoff in order. Compress waits while a spawn Group of the Session is outstanding, so Group and Steer links survive the swap.

**Blocked by:** 01

**Status:** ready-for-agent

- [x] Queued follow-ups move to the new handle in order, after the Handoff prompt
- [x] A held steer degrades to a follow-up across the swap and is not lost
- [x] Compress waits while a Group of the Session is outstanding, then runs
- [x] The grouped result still reaches the parent after a compress
- [x] Tests at the Daemon seam cover queue order, held steer and Group wait
