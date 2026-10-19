# 01: Final merge in its own worktree

**What to build:** The final merge of a spec branch into master runs in a dedicated worktree, so the User's master checkout never blocks a finished Run. The rebase onto master, the generated build and the final check happen there. The result lands on master only when master's checkout can take it without touching uncommitted work; otherwise it stays on a named branch and the Run raises one needs-you with the exact one-line command to land it.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A Run whose master checkout has untracked and modified files finishes its final merge without refusing
- [ ] Uncommitted work in master is never modified or discarded
- [ ] When master cannot take the result, the Run raises one needs-you naming the branch and the landing command
- [ ] A real merge result that lands is identical to today's result (same history shape, same generated build, same final check)
- [ ] A resumed Daemon restart in the middle of the final merge resumes without a stale worktree left behind
- [ ] `.emberloop/check` passes
