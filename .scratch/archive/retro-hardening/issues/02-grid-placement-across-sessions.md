# 02: Two sessions placing panes never collide

**What to build:** Two sessions that place panes in the same tab at the same moment get distinct grid slots, no pane displaces or closes another, and only one overflow tab per label ever exists. Failing-first with the multi-process helper: the grid's slot record becomes a locked file (one lock per tab) read under the lock, not process memory.

**Blocked by:** 01 Worker store survives simultaneous sessions (the multi-process helper)

**Status:** done

- [x] A multi-process test places panes concurrently in one tab and fails on the unfixed code
- [x] After the fix: distinct slots, no close of a pane another spawn owns, one overflow tab per label
- [x] Single-session grid behaviour and its tests are unchanged
- [x] Spec stories 31, 32, 33

Spec: `.scratch/archive/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
