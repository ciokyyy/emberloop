# 13: Reviewers and scouts open in their ticket's tab, never in a stray tab

**What to build:** A reviewer opens in its ticket's tab beside the implementor, and a scout opens in the tab of the agent that spawned it. Today a failed or missing worker lookup silently falls back to the caller's tab: reviewers for a ticket landed in the orchestrator's tab, and a stray `scouts` overflow tab appeared. The lookup of a worker by its worktree no longer swallows a failed read of the worker store: it retries, and when it still fails it uses the tab recorded in the review state and the parent pane's own tab, never an unrelated one. A scout's tab is the tab of its parent pane, and the `scouts` fallback tab is created only when the parent pane cannot be found at all.

Evidence from the live check: reviewers whose parent was the worker sat in the ticket tab, while later reviewers started from the orchestrator sat in the orchestrator's tab, although the worker record held the right tab; a `scouts` tab appeared with no scout expected there.

**Blocked by:** None (can start immediately)

**Status:** cancelled (already fixed on spec/input-queue; do not spawn)

- [ ] A failed read of the worker store is retried and logged, never treated as "no worker"
- [ ] A reviewer started from the orchestrator for a ticket opens in that ticket's tab
- [ ] A scout opens in the tab of its parent pane
- [ ] The `scouts` fallback tab is created only when the parent pane is gone
- [ ] Tests with a failing then recovering store read, and with a parent in a different tab than the caller
- [ ] Spec: context-compress story 1 (every agent pane shows the header strip; placement is part of that tab layout)

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
