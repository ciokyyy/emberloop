# 05: A ticket that is not converging tells the user and keeps retrying

**What to build:** The same refusal text three times in a row, or ten minutes with no activity from the worker, marks the ticket "needs you" and sends a herdr notification. The driver keeps retrying with a growing delay. Nothing stops the other tickets.

**Blocked by:** 04 Refusal loop

**Status:** done

- [x] Three identical refusals in a row raise the alert; two do not
- [x] Ten minutes without worker activity raises the alert (fake clock)
- [x] The alert sends one herdr notification and marks the ticket in the run state
- [x] Retries continue after the alert with a growing delay
- [x] Other tickets keep running while one is in "needs you"
- [x] Spec stories 14, 15, 19

Spec: `.scratch/archive/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
