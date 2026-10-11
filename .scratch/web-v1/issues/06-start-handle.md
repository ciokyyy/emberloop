# 06: One startHandle for create, resume and compress

**What to build:** Extract one function, `startHandle(session)`, that derives owner, role tool set (web, web-worker, scout), bank bindings and the injection block from the Session record. Create, resume and compress all call it. Compress behaves like create: Profile, fresh recall, then the Handoff. This removes identity state from harness handles and closes the cross-spec conflicts (owner across token rotation, tool gating after compress, injection order).

**Blocked by:** 02, 05

**Status:** ready-for-agent

- [x] Create, resume and compress share one code path for owner, tools and injections
- [x] A Worker, Reviewer or scout has the same tool set before and after a compress (no `remember` appears)
- [x] `owner` is identical across a resume-token rotation
- [x] Injection order after compress is Profile, recall, Handoff; before memory exists the hook is a no-op
- [x] Parent-id links for `spawn`, `steer`, `check` and `cancel` survive the swap
- [x] Daemon-seam tests cover all of the above on the fake Backend
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
