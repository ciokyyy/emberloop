# 08: A dead child no longer silences its parent's stop gate

**What to build:** A child that died without submitting (pane gone, or its last turn ended in a provider error) stops counting as a pending child, and its parent is told once that it was lost. A turn that ends in a provider error sends the parent a "stopped with error" notice instead of a silent allow. Before the stop gate allows a stop because children are pending, it runs the dead-child sweep, so a worker that stops with a dead reviewer is nudged to submit (blocked up to three times, then the existing give-up notice) instead of stopping text-only with nobody told. Block count, pin and give-up rules are unchanged.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A child with no submit and no live pane is not pending and yields exactly one lost notice
- [ ] A live child that owes a result still lets its parent stop, as today
- [ ] A turn ending in a provider error notifies the parent with the agent id and the error
- [ ] The stop gate runs the sweep before allowing on pending children; a worker with only dead children is blocked and nudged to submit
- [ ] Fake-herdr and agent-state tests cover dead, live and errored cases; repo check passes
