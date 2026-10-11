# 19: Checkpoint rebase abandon and ui

**What to build:** A User can rebase the spec branch onto master at any time, see the branch and its distance from master in the Run view, and abandon a spec to discard its branch, Ticket branches and worktrees after confirmation.

**Blocked by:** spec-branch-core (09)

**Status:** ready-for-agent

- [ ] Rebase action; conflict becomes Needs-you with files
- [ ] Run view shows branch name and commits behind master
- [ ] Abandon removes branches and worktrees after confirmation
- [ ] Seam and `RunsView.test.js` tests; `.emberloop/check` passes
