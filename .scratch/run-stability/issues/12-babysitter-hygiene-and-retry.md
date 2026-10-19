# 12: Hygiene steps and retry

**What to build:** The Babysitter can call a fixed, driver-side set of named hygiene steps inside Run-owned worktrees, such as removing a stale Run branch or cleaning untracked build artifacts, and can retry a needs-you state, which bumps the retry generation. A step that would touch the master checkout or a branch the Run does not own is refused.

**Blocked by:** 10-babysitter-action-queue

**Status:** ready-for-agent

- [ ] Each hygiene step is a named driver function; an unknown name is refused
- [ ] A hygiene step never touches the master checkout or an unowned branch
- [ ] Retry of a needs-you behaves like the User's retry and starts the Ticket again at once
- [ ] In `shadow` mode these actions only log what they would have done
- [ ] `.emberloop/check` passes
