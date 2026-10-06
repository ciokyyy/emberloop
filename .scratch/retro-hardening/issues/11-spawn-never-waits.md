# 11: Spawn never waits: one grouped notice per call

**What to build:** Spawn returns at once for every agent type and never waits for the agents it starts, so an interrupted or cancelled call can never strand a result. The results of one spawn call are a group: when the last agent of the group has submitted, one notice carrying every result goes to the parent pane. An agent that cannot submit is reported in the group as lost, so a group always completes. A reviewer already judged at the exact state is reported in the first reply and is not part of the group. A notice is held while the parent pane is working a tool call and delivered when it goes idle (typed text cancels a running call), and delivered anyway, with a warning, if the parent never goes idle for a long time. The wait loop, its bound and the 'still running' reply are removed, and the worker and orchestrator briefs say once: after spawn, end your turn, the result arrives as a system notice.

**Blocked by:** 04 Launches are spaced across sessions, and concurrent workers are capped (both change spawn)

**Status:** ready-for-agent

- [ ] Spawn returns at once with agents still running, for blocking and non-blocking types
- [ ] Two reviewers started by one call deliver exactly one notice, after the second submit, carrying both results
- [ ] An agent whose pane vanishes is reported as lost in the group notice
- [ ] A notice for a busy parent pane is held until the pane is idle (fake herdr)
- [ ] A cancelled call strands nothing: the result still arrives as the group notice
- [ ] Briefs and the spawn tool description state that results arrive as a notice
- [ ] Spec stories 29 and 30

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
