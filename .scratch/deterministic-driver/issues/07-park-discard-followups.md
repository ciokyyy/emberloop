# 07: The user can park or discard a ticket, and follow-ups reach the report

**What to build:** A `park` command stops the driver working on a ticket while keeping its worktree and branch; only tickets that depend on it wait and every other ticket keeps running. A `discard` command removes a worker and its worktree on request, and the driver never discards on its own. Worker follow-ups are copied verbatim into the final report, and tickets left unmerged are listed with their last refusal.

**Blocked by:** 04 Refusal loop

**Status:** done

- [x] Parking keeps the worktree and branch and blocks only dependents
- [x] `discard` removes the worker and its worktree and is never issued by the driver itself
- [x] Follow-ups appear verbatim in the report
- [x] Unmerged tickets appear in the report with their last refusal text
- [x] Spec stories 16, 17, 18, 24, 25, 26, 39

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
