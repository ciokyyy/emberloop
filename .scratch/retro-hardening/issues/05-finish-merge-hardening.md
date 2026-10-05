# 05: finish says exactly why it refuses, and refuses before it starts

**What to build:** When uncommitted changes in the integration checkout overlap what the merge would change, finish refuses up front, names the files, and says to commit or stash them. Uncommitted changes that do not overlap, including the orchestrator's note and ticket edits, do not block the merge. git's own error text is passed through whole, never truncated.

**Blocked by:** 01 Worker store survives simultaneous sessions (both change the worker code)

**Status:** ready-for-agent

- [ ] A dirty file that the incoming change also touches blocks the merge and is named in the reply
- [ ] A dirty file that does not overlap leaves the merge alone
- [ ] A git failure's full text appears in the reply
- [ ] Spec stories 1, 2, 3, 4

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
