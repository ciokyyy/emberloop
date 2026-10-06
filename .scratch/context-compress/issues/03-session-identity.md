# 03: An agent keeps its identity across a session restart

**What to build:** Agent state records the harness session ID currently in use, separate from the stable wick agent ID. Transcript lookup, the usage tailer's identity and the reply waiting read that current value. Everything already keyed by agent ID (usage rows, submit marks, gate state, error marks, review state, pane mapping) stays as is. A restart can then give the same agent a new harness session ID without losing any state. The new ID is always a fresh UUID.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Agent state holds a current session ID, set at launch and changeable
- [x] Transcript lookup, usage tailing and reply waiting use the current session ID
- [x] Changing the session ID mid-run: usage rows keep accumulating on the agent ID, submit marks and gate state persist
- [x] Existing behaviour for agents that never restart is unchanged
- [x] Tests use the fake exec seam and temp directories
- [x] Spec story 24

Spec: `.scratch/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
