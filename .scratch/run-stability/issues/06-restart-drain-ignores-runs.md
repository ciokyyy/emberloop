# 06: Restart drain stops waiting on Runs

**What to build:** A Daemon restart no longer waits up to five minutes on Runs. Restart already pauses live Runs at tool boundaries and resumes the same Worker after boot, including after a hard kill. The drain wait ends as soon as no Session other than a paused Run Session is busy.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A restart with only Run Sessions busy completes without the full drain wait
- [ ] A restart with a non-Run Session busy still waits up to the drain limit
- [ ] After boot, each paused Run resumes its same Worker or Reviewer with the continue prompt
- [ ] No turn is interrupted while a tool call is open
- [ ] The boot-loop guard still stops after four boots in five minutes
- [ ] `.emberloop/check` passes
