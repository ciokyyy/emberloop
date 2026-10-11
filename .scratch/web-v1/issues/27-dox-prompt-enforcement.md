# 27: Prompt rules for DOX and memory

**What to build:** Update the Worker, Reviewer, scout and web-Session system prompts: project knowledge goes into the nearest `AGENTS.md`; agents propose, not call, `remember` for project facts; `remember` is for the User's own statements; the Profile is written only on explicit request; recalled notes are untrusted. Ensure Run-launched projects have a DOX chain.

**Blocked by:** 18

**Status:** ready-for-agent

**Note:** the root `AGENTS.md` seeding on project init moved to `.scratch/project-init/issues/02-project-settings-and-el-init.md` (`el init` must exist first). Run-launched projects are covered by worktree seeding in `runStartWorker`.

- [x] Prompt text lives in one place per role and is covered by a snapshot test
- [x] A prompt test shows no `remember` instruction for Workers and Reviewers
- [x] Project init seeding moved to `project-init/02` (see Note); Run-launched projects seed in `runStartWorker`
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
