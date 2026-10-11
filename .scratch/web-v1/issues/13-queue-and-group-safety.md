# 13: Queue carry-over and Group wait

**What to build:** Queued follow-ups and held steers arrive after the Handoff in order. Compress waits while a spawn Group of the Session is outstanding. The boxes were checked in the old spec; verify them against the code before closing.

**Blocked by:** 05

**Status:** done

- [x] Queued follow-ups move to the new handle in order, after the Handoff prompt
- [x] A held steer degrades to a follow-up across the swap and is not lost
- [x] Compress waits while a Group of the Session is outstanding, then runs
- [x] The grouped result still reaches the parent after a compress
- [x] Tests at the Daemon seam cover queue order, held steer and Group wait
