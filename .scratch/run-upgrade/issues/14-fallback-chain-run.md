# 14: Fallback chain run

**What to build:** When a Worker or Reviewer fails with quota, credit, auth or session limit, the Run starts the next Chain entry as a new Session on the same Ticket branch with a Handoff and normal injection. The failed Session ends `failed` and keeps its Event log. Exhausting the Chain makes the Ticket Needs-you. Other failures behave as today.

**Blocked by:** prompt-injection (04), failure-classifier-and-chain-settings (05)

**Status:** done

- [x] Advance on each of the four classes and not on others
- [x] New Session on same branch, Handoff then injection
- [x] Chain position stored on the Ticket row and survives restart
- [x] Exhaustion becomes Needs-you
- [x] Run seam tests with scripted failures; `.emberloop/check` passes
