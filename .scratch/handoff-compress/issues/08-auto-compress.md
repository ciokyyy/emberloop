# 08: Auto-compress on the Handoff path

**What to build:** Auto-compress fires at the existing percent threshold on the Handoff path, at a tool boundary or when idle, with the cooldown.

**Blocked by:** 04, 05, 06, 07

**Status:** ready-for-agent

- [ ] With `auto_compress` on, a Session at or past `compress_threshold_percent` compresses by Handoff
- [ ] Auto fires only at a tool boundary or when idle, never mid-token
- [ ] `compress_cooldown` prevents a compress loop
- [ ] With the compress model unset, auto shows a warning and does not run
- [ ] A blocked Session does not auto-compress again until Retry or dismiss
- [ ] Tests at the Daemon seam cover threshold, boundary timing, cooldown and unset model
