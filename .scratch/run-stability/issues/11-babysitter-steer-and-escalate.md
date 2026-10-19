# 11: Steer and escalate in active mode

**What to build:** In `active` mode the Babysitter's steer actions send a concrete instruction to the Worker or a Reviewer through the Ticket steer path, such as "commit your work", "answer finding X in a reply", or "finding X was already raised on this tip". Reviewer verdicts stay the Reviewer's. When two steers fail to change the outcome, or no action fits, it escalates: needs-you with its diagnosis in the reason. It never interrupts, pauses, Switches or ends a Session, never waives a finding and never edits code.

**Blocked by:** 10-babysitter-action-queue

**Status:** ready-for-agent

- [ ] A steer reaches the Worker or Reviewer like any other Steer and is shown on the timeline
- [ ] A Reviewer steer does not change that Reviewer's stored verdict
- [ ] Two failed steers on one Ticket produce a needs-you whose reason carries the diagnosis
- [ ] No Babysitter action interrupts, pauses, Switches or ends a Session, or marks a finding declined
- [ ] There is no Reviewer round cap anywhere in the driver after this ticket
- [ ] `.emberloop/check` passes
