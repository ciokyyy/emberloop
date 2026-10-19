# 10: Babysitter named actions and the pending queue

**What to build:** The Babysitter acts through named actions: steer the Worker, steer a Reviewer, retry, run a named hygiene step, escalate with a note. An action queues as pending. A driver pass consumes it under the per-Ticket lock and drops it when the tip, round or state differs from what the Babysitter saw. In `shadow` mode an action only writes to the Babysitter log and the Ticket timeline what it would have done, and changes nothing else.

**Blocked by:** 08-babysitter-wake-and-session

**Status:** ready-for-agent

- [ ] Every action goes through the pending queue; none acts directly
- [ ] An action whose tip, round or state changed is dropped and logged as dropped
- [ ] In `shadow` mode an action changes no Session, branch or file and is logged as would-have
- [ ] In `active` mode a queued action is consumed on the next driver pass
- [ ] A Babysitter action never races a nudge or a Worker feedback send for the same Ticket
- [ ] `.emberloop/check` passes
