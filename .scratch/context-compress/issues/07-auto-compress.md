# 07: Sessions compress themselves past the threshold

**What to build:** The supervisor watches the context core and compresses automatically when context passes the threshold, only at a tool boundary or at idle, never during a running tool call. Settings add a switch to disable auto-compress (the manual path still works) and a cooldown of N tool calls after a compress. The header strip shows `compressing…` during the work and a warning such as `no compressModel` when auto-compress cannot run. After an auto-compress between tool calls the new session continues the interrupted step without waiting for the user.

**Blocked by:** 04-header-strip, 06-summarizer-and-manual-compress

**Status:** done

- [x] Crossing the threshold during a tool call waits; compress starts at the boundary
- [x] Compress also starts when the agent is idle and over the threshold
- [x] Disable setting stops auto-compress and leaves manual working
- [x] Cooldown blocks a second auto-compress within N tool calls
- [x] Header state shows `compressing…` and the missing-model warning
- [x] Auto-compress ends with the agent continuing the interrupted step
- [x] Spec stories 9, 10, 15, 17, 20, 25, 26

Spec: `.scratch/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
