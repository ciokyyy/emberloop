# 09: Babysitter read-only tools

**What to build:** The Babysitter investigates with `read`, `find` and an allowlisted `git` tool limited to status, diff, log, show, merge-base and ls-files. Scope is the Run's worktrees and the spec branch; the master checkout is read-only. It has no shell, no write or edit, and no spawn, cancel or steer tools.

**Blocked by:** 08-babysitter-wake-and-session

**Status:** ready-for-agent

- [ ] The git tool refuses any subcommand outside the allowlist and any argument that writes
- [ ] The Babysitter can inspect a conflicted final-merge worktree and the Event log of a Ticket
- [ ] It cannot write a file, run a shell command, spawn, cancel or steer
- [ ] A path outside the Run's worktrees, the spec branch and the read-only master checkout is refused
- [ ] `.emberloop/check` passes
