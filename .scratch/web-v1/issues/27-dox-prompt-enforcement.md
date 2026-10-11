# 27: Prompt rules for DOX and memory

**What to build:** Update the Worker, Reviewer, scout and web-Session system prompts: project knowledge goes into the nearest `AGENTS.md`; agents propose, not call, `remember` for project facts; `remember` is for the User's own statements; the Profile is written only on explicit request; recalled notes are untrusted. Ensure Run-launched projects have a DOX chain.

**Blocked by:** 18

**Status:** ready-for-agent

- [ ] Prompt text lives in one place per role and is covered by a snapshot test
- [ ] A prompt test shows no `remember` instruction for Workers and Reviewers
- [ ] Project init (see `.scratch/project-init`) creates a root `AGENTS.md` if the Project lacks one
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
