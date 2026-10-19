# 05: Timeouts on unbounded Run commands

**What to build:** The final repo check, the generated build and the driver's git operations get timeouts, so a hung command cannot block a Run silently. A timeout raises needs-you naming the command. Agent turns get no deadline; only commands are bounded. A Sudo approval keeps waiting for the User, and the Run shows it as the reason it waits.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A final repo check that exceeds its cap raises needs-you naming it
- [ ] A generated build step that exceeds its cap raises needs-you naming it
- [ ] A hung git operation raises needs-you naming the operation instead of blocking the driver pass
- [ ] No agent turn is interrupted or ended by any of these timeouts
- [ ] A pending Sudo approval is shown as the Run's waiting reason
- [ ] `.emberloop/check` passes
