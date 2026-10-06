# 03: One reviewer per worktree and type, even when two spawns race

**What to build:** Two simultaneous spawns of the same reviewer type for a worktree produce one reviewer, not two: the second re-prompts the one already starting. Failing-first with the multi-process helper; the check-then-launch of a reviewer runs under a cross-process lock per worktree and type, so the record can no longer be overwritten and leave an orphan pane.

**Blocked by:** 01 Worker store survives simultaneous sessions (the multi-process helper)

**Status:** done

- [x] A multi-process test spawns the same reviewer type for one worktree twice at once and fails on the unfixed code
- [x] After the fix exactly one reviewer exists per worktree and type and no pane is orphaned
- [x] A different type, or a different worktree, is unaffected
- [x] Spec story 35

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
