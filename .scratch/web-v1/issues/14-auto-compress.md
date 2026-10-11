# 14: Auto-compress on the Handoff path

**What to build:** Auto-compress fires at the existing percent threshold on the Handoff path, at a tool boundary or when `session.Idle()` holds, with the cooldown.

**Blocked by:** 07, 10, 11, 12, 13

**Status:** done

- [x] With `auto_compress` on, a Session at or past `compress_threshold_percent` compresses by Handoff
- [x] Auto fires only at a tool boundary or when idle, never mid-token
- [x] `compress_cooldown` prevents a compress loop
- [x] With the compress model unset, auto shows a warning and does not run
- [x] A blocked Session does not auto-compress again until Retry or dismiss
- [x] Tests at the Daemon seam cover threshold, boundary timing, cooldown and unset model
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
