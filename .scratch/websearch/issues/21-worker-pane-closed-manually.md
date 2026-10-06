# 21: A worker pane closed by hand frees its slot and notifies the parent

**What to build:** Today a worker whose pane the user closes by hand keeps its entry in `workers.json`. Dead entries fill the worker cap, so new workers queue forever and the parent never learns the worker is gone. Fix it in the worker spawn and cap path. (1) The cap check counts only workers whose pane is alive. Ask herdr for the pane; only an explicit `pane_not_found` (as in `deliver.go`) means dead, and any other herdr error counts the worker as alive. (2) A worker with a dead pane and no submit is marked `cancelled`: its `workers.json` entry is removed, its worktree and branch are kept, and a ticket-history row records the cancellation so the TUI shows it. (3) The parent gets the existing lost notice, worded "pane closed manually", through `sweepDeadChildren`, once. (4) Spawning a worker for a ticket whose worktree and branch already exist from a cancelled worker reuses them instead of failing with "already exists". (5) `drainWorkerQueue` runs after the prune, so queued workers start as after a normal finish. Queue entries for tickets already done or already running are dropped.

**Blocked by:** None (can start immediately).

**Status:** done

- [x] Dead-pane worker is not counted against the cap; a live worker and a herdr error both still count
- [x] Dead-pane worker with no submit is removed from `workers.json`, marked cancelled in history, worktree and branch kept
- [x] Parent receives one "pane closed manually" notice per cancelled worker, never repeated
- [x] Respawn of a cancelled ticket reuses its worktree and branch
- [x] Queued workers start after the prune; stale queue entries are dropped
- [x] Fake herdr covers all of the above; repo check passes
