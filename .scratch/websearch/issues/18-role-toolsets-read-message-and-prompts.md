# 18: Role toolsets, `read` oversize message, and prompt directions

**What to build:** Finish the agent-facing wiring. (1) Confirm the role allowlists: `review` and `scout` are read, find, execute (plus submit); `work`, `plan` and `full` include `execute`; `implement` does not; the reviewer-spec, reviewer-standards and scout agent types list `execute`. (2) `read`'s refusal of an oversized file names `tools.grep` in `execute` or `read` with offset and limit when the role has no `bash`, and `bash` when it does (replacing the current text that tells every role to use bash). (3) Prompt directions, one positive line each: the worker-class role prompts say to search and process content with `execute`; the reviewer and scout agent types say use `tools.grep` for exact lookups, `find` for concepts, `tools.git` for history and the diff, and (reviewers) `tools.check` to verify a worker's claim. The spawn-time "Checks run by wick" block stays. No prohibitions in the wording.

**Blocked by:** 16

**Status:** ready-for-agent

- [ ] A test per role asserts its served tool list (main list) and its `execute`-reachable tools
- [ ] The oversize `read` message is correct for a role with bash and one without
- [ ] The prompt lines exist exactly once per role, in positive wording
- [ ] Agent-type files list `execute`; repo check passes
