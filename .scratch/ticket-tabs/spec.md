# Spec: one tab per ticket in the project workspace, git worktrees, 3×3 agent grid

Status: ready-for-agent

## Problem Statement

When emberloop spawns a worker, it creates the worktree with `herdr worktree create`. herdr then opens a separate workspace for that worktree, and the sidebar shows it, but clicking it shows an empty terminal. The worker is not running there: in run-tab mode emberloop splits it into the shared run tab, so the worktree workspace's own pane only exists to keep the workspace alive.

Agents are also hard to find and badly laid out:

- The implementor for a ticket and that ticket's reviewers are not together. Reviewers and scouts split the *caller's* pane, so they pile up in the orchestrator's tab.
- Every read-only launch splits `down`, so agents stack in thin horizontal strips instead of sitting side by side.
- Nothing limits how many panes go into one tab.

## Solution

Everything for a project lives in the project's own herdr workspace, which is the workspace the orchestrator runs in. Each ticket gets one tab. The implementor runs in that tab's root pane, and the ticket's reviewers fill the tab as a grid of rows of three: `Impl A B` / `C D E` / `F G H`. A tab holds at most 9 panes, and when it is full the next agent opens an overflow tab. Reviewers stay open until the ticket finishes, so they can be prompted again. Scouts and other read-only agents follow the same grid rules in the orchestrator's tab.

Worktrees are created and removed with plain `git worktree`. herdr is only used for tabs and panes, never for worktrees.

## User Stories

1. As a user watching a run, I want every agent of a project inside that project's herdr workspace, so that the sidebar shows one workspace per project and no empty worktree workspaces.
2. As a user, I want one tab per ticket, labelled with the ticket id, so that I can jump straight to a ticket's agents.
3. As a user, I want the implementor of a ticket in the root pane of that ticket's tab, so that the first pane I see is the one doing the work.
4. As a user, I want a ticket's reviewers in the same tab as its implementor, so that I can watch the review next to the code being reviewed.
5. As a user, I want agents in a tab laid out side by side in rows of three, so that I don't get thin stacked strips.
6. As a user, I want the grid to fill row by row (`Impl A B`, then `C D E`, then `F G H`), so that the layout is predictable.
7. As a user, I want every pane in a tab to be the same size, so that no agent is squeezed into 1/8 of the tab.
8. As a user, I want at most 9 panes in a tab, implementor included, so that every pane stays readable.
9. As a user, I want the 10th agent of a ticket to open a new tab labelled `<ticket-id> (2)` in the same workspace, so that overflow is visible and clearly belongs to the ticket.
10. As an orchestrator, I want a respawned or retried implementor to reuse its ticket's existing tab and replace the implementor pane, so that a ticket never ends up with two tabs.
11. As an orchestrator, I want a ticket's reviewers to stay open until the ticket finishes, so that a later review round re-prompts the same reviewer and it keeps its context.
12. As an orchestrator, I want a reviewer whose pane died to be replaced in the same slot of the grid, so that the layout doesn't shift under the user.
13. As a user, I want the remaining panes evened out after a replacement, without the grid being rebuilt, so that nothing I'm reading jumps around.
14. As an orchestrator, I want scouts and other read-only agents that are not tied to a ticket to keep opening in my own tab, side by side in the same 3×3 grid, so that they stay close to the session that asked for them.
15. As an orchestrator, I want my own pane to count toward my tab's 9, with overflow scouts going to a `scouts` tab, so that my tab stays readable.
16. As an orchestrator, I want reviewers for different tickets to launch in parallel, so that a multi-ticket run isn't slowed by one global split lock.
17. As an orchestrator, I want splits inside one tab to stay serialized, so that two concurrent launches never split a half-built layout.
18. As a user, I want worktrees created with plain `git worktree add` on branch `implement/<id>` at `.emberloop-worktrees/<id>` from the resolved base SHA, so that worktrees behave like normal git worktrees.
19. As a user, I want finishing or discarding a ticket to close its tab and any overflow tabs (reviewers go with them) and remove the worktree with `git worktree remove --force`, so that nothing is left in herdr or on disk.
20. As a user, I want the `implement/<id>` branch kept after finish until it is merged, as it is today, so that the merge flow doesn't change.
21. As a user, I want a failed spawn to undo itself (remove the git worktree and branch, and close the tab if emberloop created it), so that the ticket can be spawned again.
22. As a maintainer, I want the shared run-tab mode (`RunTab`/`Anchor`) removed, so that there is one layout to maintain.
23. As a maintainer, I want every `herdr worktree create/remove` call removed, so that herdr is used for tabs and panes only.

## Implementation Decisions

- **Project workspace.** The project workspace is the orchestrator's own workspace, read from `HERDR_WORKSPACE_ID`. Spawning fails with a clear error when it is unset.
- **Worktree lifecycle moves to git.** Spawn runs `git worktree add -b implement/<id> <path> <base SHA>`, keeping the path and branch scheme. Finish and discard run `git worktree remove --force`. Branch handling at finish is unchanged.
- **Ticket tab.** Spawn finds the tab labelled `<ticket-id>` in the project workspace, or creates one with `herdr tab create --workspace <ws> --cwd <worktree> --label <ticket-id>`, and runs the implementor in its root pane. On respawn the tab is reused and the implementor pane is replaced. The worker record keeps the project workspace, the ticket tab, the implementor pane and any overflow tabs.
- **Grid helper (new, shared).** One deep module places a new pane in a tab and owns all layout rules: row-major order with 3 columns, at most 9 panes, and overflow into a new tab whose label it is given. Because herdr can only split a pane, not a whole region, the grid is built as three columns that are each split into rows: panes 2 and 3 split `right`, and pane k for k ≥ 4 splits `down` from the pane directly above it (k−3). Visually that fills row by row. Sizes are then evened out with the existing tiling logic. Callers ask "put a pane in tab T (overflow label L)" and get back a pane id and tab id.
- **Slot reuse.** When a dead reviewer is replaced, the grid helper places the replacement in the dead pane's slot instead of appending it, then evens the sizes. The rest of the grid is not rebuilt or re-flowed.
- **Reviewers.** The read-only launcher stops splitting `--current` for ticket reviewers. A reviewer goes into its ticket's tab through the grid helper, with overflow label `<ticket-id> (2)`, `(3)`, …. Reviewer records remember their tab. The existing persistence, re-prompting and replace-dead behaviour stay as they are. Reviewers close only when the ticket is finished or discarded.
- **Scouts and read-only agents not tied to a ticket.** These still go into the caller's tab, but through the grid helper (side by side, at most 9 counting the caller), with overflow label `scouts`.
- **Split lock is per tab.** The process-wide split mutex becomes one lock per tab id. Splits in different tabs run at the same time, while splits in one tab are serialized. Overflow tab creation happens under the full tab's lock.
- **Removed.** The run-tab mode (`RunTab`, `Anchor` and the worker split into the run tab), every `herdr worktree create/remove` call, and keeping the worktree workspace's root pane alive.
- **No migration.** Workers created under the old layout are not supported. In-flight workers are finished before switching.
- **Tabs are owned by emberloop.** The user does not close them by hand, so nothing tries to recover a missing tab.

## Testing Decisions

- A good test drives the public entry points (spawn, code review, read-only launch, finish/discard) through the injected `Exec` seam and asserts on the herdr and git commands issued and on the resulting layout. It never asserts on internal helpers. The `Exec` seam is the only seam: no new seams are introduced.
- **Spawn:** issues `git worktree add` with the resolved SHA and never `herdr worktree`; creates the ticket tab in `HERDR_WORKSPACE_ID` with the ticket label; runs the implementor in the tab's root pane; a respawn reuses the tab; a failure after the worktree exists removes it. Prior art: `TestSpawnWaitFinish`, `TestSpawnResolvesBaseToASha`, `TestSpawnRemovesTheWorktreeWhenTheSplitFails` in the worker tests.
- **Grid:** a fake tab that applies splits as a tree (extend the existing `fakeTab` split-tree fake from the tiling tests) shows that panes 1–9 land in rows of three with equal sizes, that the 10th opens an overflow tab with the right label, and that a replacement lands in the dead pane's slot. Prior art: `TestTilePanesEvensATab`, `TestTilePanesEvensAMixedTree`.
- **Reviewers:** reviewers spawn in the ticket's tab rather than the caller's, stay open across review rounds, and close on finish or discard. Prior art: `TestCodeReviewStartsTwoReviewers`, `TestCodeReviewReplacesADeadReviewer`, `TestDiscardNeedsNoReviewAndClosesReviewers`.
- **Lock:** concurrent launches into two different tabs overlap, and concurrent launches into one tab serialize their splits. Prior art: `TestConcurrentReadOnlyLaunchesSerializeOnlyTheSplit`.
- **Finish/discard:** close the ticket tab and overflow tabs and run `git worktree remove --force`, with no `herdr worktree remove`. Prior art: the finish tests in the worker tests.
- **Deleted:** `TestSpawnSplitsTheRunTabForTheWorker` and `TestSpawnDefaultsWorkerTabToTheRunTab`, along with run-tab mode.

## Out of Scope

- Recovering from tabs the user closes by hand.
- Migrating or cleaning up workers created under the old `herdr worktree` layout.
- Re-flowing the grid when a pane closes, or rebuilding the split tree.
- Changing branch naming, base resolution, rebase or merge behaviour at finish.
- Changing reviewer prompts, verdict parsing or the review gate.
- Any layout other than a 3×3 grid (configurable sizes, per-user layouts).

## Further Notes

- `--ratio` on `herdr pane split` is the share the *original* pane keeps, not the share the new pane gets. This was measured earlier and is recorded with the tiling code. The grid helper should split at whatever ratio leaves the tiling the least work, but correctness comes from tiling, not from the split ratio.
- This also fixes the reported symptom: no empty worktree workspace appears in the sidebar any more, because there is no worktree workspace.
