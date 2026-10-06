# 10: A dead agent in a reused pane is detected, and its crash leaves a trace

**What to build:** Wick must never treat a pane holding a bare shell as a live agent. Today the reuse check only asks herdr whether the pane exists, so a reviewer whose process died (for example a pig session killed mid-stream) still counts as alive. Wick then sends the follow-up as typed text and the shell runs it as a command ("fish: Unknown command: Follow-up"). The review never completes and the orchestrator waits on a verdict that cannot arrive. After this ticket, liveness means an agent process runs in the pane, judged from herdr's per-pane agent fields; a pane with no agent counts as dead. A dead reviewer with no verdict, or with a verdict that no longer covers the state, gets a fresh reviewer in its slot instead of a follow-up nudge.

The same incident showed that a crash leaves no trace. The launcher process is the parent of the agent and sees its exit, but discards the exit code. It now records the child's exit status, the signal if any, and the last lines of its stderr in the agent's state, and the orchestrator's lost-agent notice includes them. This also supports the later restart supervisor.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Liveness is true only when herdr reports an agent in the pane; a bare shell, an unknown agent and a missing pane are all dead
- [x] A persistent reviewer whose pane holds no agent gets a fresh reviewer, for both a missing verdict and a stale one; no follow-up text is ever typed into a shell
- [x] The launcher records exit code, terminating signal and a short stderr tail into the agent's state when the child exits
- [x] The lost-agent notice to the parent includes that exit information
- [x] The group waiting on a dead reviewer is not left hanging: it resolves with the fresh reviewer's verdict
- [x] Tests through the exec seam with a fake herdr: pane exists but no agent, follow-up path not taken, fresh reviewer launched; fake child exit with a signal and stderr is recorded
- [x] Incident: the 09 spec reviewer pig session died mid-review at the same second as another pig reviewer; record any finding on the cause here

Spec: `.scratch/context-compress/spec.md` gives the context (compress reuses panes and relies on the supervisor seeing the child's exit).
