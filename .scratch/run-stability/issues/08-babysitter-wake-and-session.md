# 08: Babysitter wakes on trouble and runs as a Session

**What to build:** A Run starts a Babysitter Session lazily on the first trouble signal and wakes it by Follow-up afterwards. Signals: a stall nudge, a repeated Reviewer finding on an unchanged tip, review round 4 or later, a needs-you, a refused final merge, a Reviewer silent for 10 minutes, a tool call open for 30 minutes. Wakes are throttled to one per Ticket per five minutes with identical signals deduplicated. The first prompt carries the Run summary, the trigger, and pointers to the Ticket file, the Babysitter log, the Event log and the Run worktrees. The Session ends with its Run. The driver never waits on it; an error, hang or silence leaves the Run on today's nudges and alerts, and three consecutive failed wakes make it silent in that Run until re-enabled.

**Blocked by:** 07-babysitter-role-and-mode

**Status:** ready-for-agent

- [ ] A healthy Run starts no Babysitter Session
- [ ] Each listed signal starts or wakes the Session; an identical signal inside the throttle window does not
- [ ] With the mode `off` no Session starts for any signal
- [ ] The Session ends when the Run ends and leaves no process behind
- [ ] A Babysitter that errors or hangs does not delay a driver pass, a nudge or a needs-you
- [ ] Three consecutive failed wakes silence it for that Run with one timeline note
- [ ] `.emberloop/check` passes
