# 05: Queue and draft survive a compress restart

**What to build:** When the supervisor restarts the child for compress, the wrapper stays up. The queue, the row draft and history are kept. The handoff is delivered first, ahead of the queue and the user's pending messages. Delivery of the handoff uses the same channel and counts as accepted when the new session shows the turn.

**Blocked by:** 01-control-channel-and-queue-core, 02-wick-input-row

**Status:** ready-for-agent

- [ ] After a restart the row draft and history are unchanged
- [ ] Items queued before the restart are still pending after it
- [ ] The handoff goes before every pending item
- [ ] A restart in raw mode keeps the mode and the dirty state
- [ ] Tests through the supervisor seam with fake pane runners
- [ ] Spec stories 20, 21

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
