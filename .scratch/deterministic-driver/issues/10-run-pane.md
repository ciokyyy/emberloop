# 10: A pane shows every ticket's state at a glance

**What to build:** `emberloop implement <spec> --driver` opens a new mode of the TUI: a ticket table on top (state, worker, reviewer verdicts at the current tip, time in state, and the last refusal text for a ticket in "needs you") and the existing agents view below. A row jumps to the worker's herdr pane, and the `p` key parks the ticket through the park command. The driver loop and the pane share one in-memory run state.

**Blocked by:** 03 Driver happy path, 07 Park, discard, follow-ups

**Status:** done

- [x] The model renders one row per ticket in each state: queued, running, in review, merging, needs you, parked, merged
- [x] Each row shows the worker, the verdicts at the tip, and time in state
- [x] A "needs you" row shows the last refusal text
- [x] Jump-to-pane focuses the worker's herdr pane through the existing herdr seam
- [x] `p` parks the selected ticket
- [x] Tested through the TUI model pattern, not against a live herdr
- [x] Spec stories 16, 34, 35, 36, 37, 38

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
