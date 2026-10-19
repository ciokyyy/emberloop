# 07: Babysitter role and mode in settings

**What to build:** Agents settings gains a `babysitter` role with a model and effort Chain, host-global and admin-only, like Memory settings. Beside it is a mode, `off`, `shadow` or `active`, default `shadow`. In `off` no Babysitter Session ever starts. The role and mode persist and are readable by the Run driver per call.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The Babysitter role appears in Agents settings with the same Chain editor as the other roles
- [ ] A non-admin cannot change the role or the mode
- [ ] The mode defaults to `shadow` and persists across a Daemon restart
- [ ] The driver reads the current mode and role on each wake, so a change applies without a restart
- [ ] The role falls back with a visible warning when unset, like other Chains
- [ ] `.emberloop/check` passes
