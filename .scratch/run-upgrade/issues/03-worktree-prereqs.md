# 03: Worktree prereqs

**What to build:** Every Ticket worktree has a local git identity and a read-only readable spec directory, so Workers hit no `stat` errors or aborted rebases.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Worktree creation sets local `user.name` and `user.email`
- [x] Spec directory readable from the worktree and read-only; only the driver ticks boxes
- [x] Run seam test asserts both on the fake git
- [x] `.emberloop/check` passes
