# 06: A worker that forgot to review or submit gets nudged, then escalated

**What to build:** A worker idle for three minutes with no submit gets a fixed nudge to spawn its reviewers and submit. The nudge repeats every five minutes and the third one raises the "needs you" alert. The driver reads the real reviewer spawn error out of the worker's transcript and keeps it in the run state for the ticket.

**Blocked by:** 04 Refusal loop

**Status:** done

- [x] Three minutes idle with no submit sends the fixed nudge (fake clock); activity resets the timer
- [x] The nudge repeats every five minutes and the third raises the alert
- [x] A reviewer spawn error present in the worker's transcript is shown in the run state for that ticket
- [x] A worker that submits is not nudged
- [x] Spec stories 20, 21, 22

Spec: `.scratch/archive/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
