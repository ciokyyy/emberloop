# 05: An agent restarts in the same pane with a handoff

**What to build:** The process that owns each agent child (the top-level `el` command for the user's session, the launch executor for spawned panes) becomes a supervisor loop. On a compress signal it waits until every outstanding tool call has its result, stops the child, starts a new child in the same pane with the supplied handoff text as the first message, and records the new session ID. The pane never closes. The agent keeps its agent ID, and the state tracks a `compressed xN` count. The handoff here is plain input to the loop, so this slice is testable with a fixed handoff.

**Blocked by:** 01-prompt-delivery-inline, 02-context-core, 03-session-identity

**Status:** done

- [x] A signal during a running tool call waits; the child stops only once all tool calls have results
- [x] The new child receives the handoff as its first message, delivered directly
- [x] Same pane, same agent ID, new session ID, `compressed xN` incremented
- [x] The pane stays open on the restart and on a failed relaunch
- [x] Works for the top-level session and for spawned panes
- [x] Tests through the exec seam with a fake child and temp transcripts
- [x] Spec stories 10, 16, 18, 23, 24

Spec: `.scratch/archive/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
