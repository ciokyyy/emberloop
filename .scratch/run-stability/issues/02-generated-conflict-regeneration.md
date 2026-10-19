# 02: Regenerate generated-file conflicts at final merge

**What to build:** A rebase conflict at final merge whose files are all generated build output (the embedded web bundle set) resolves by regenerating the build and continuing, so the Run needs no person. A conflict that touches any other file still raises needs-you naming those files.

**Blocked by:** 01-final-merge-worktree

**Status:** ready-for-agent

- [ ] A rebase that conflicts only on generated bundle files finishes by regenerating and continuing
- [ ] A rebase that conflicts on any source file raises needs-you naming exactly those files
- [ ] A mixed conflict (generated plus source) raises needs-you naming only the source files
- [ ] The regenerated build is the one the final check runs against
- [ ] `.emberloop/check` passes
