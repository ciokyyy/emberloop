# 06: Submit gate refusals

**What to build:** The Worker submit gate refuses `done` with no new commit since the last `changes` verdict, refuses `done` whose text says the work is not done, and turns a second `blocked` submit with the same question into Needs-you.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `done` with no new commit after `changes` is refused with a reason
- [x] Conservative not-done text pattern is refused
- [x] Second `blocked` with the same question hash becomes Needs-you
- [x] Run seam tests cover each; `.emberloop/check` passes
