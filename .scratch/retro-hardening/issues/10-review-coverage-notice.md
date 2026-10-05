# 10: Reviewers are told when the diff covers less than the tree

**What to build:** The reviewer prompt states how many files the diff covers and how many the working tree changed or added; a mismatch is flagged in the prompt, so a gap is visible to the reviewer instead of discovered by accident.

**Blocked by:** 03 One reviewer per worktree and type (both change the review code)

**Status:** ready-for-agent

- [ ] A prompt for a tree whose diff covers every changed file states matching counts
- [ ] A prompt where the diff covers fewer files than the tree flags the mismatch (test)
- [ ] Spec story 25

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
