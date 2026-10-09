# 04: Needs-you banner, stall nudges and Ticket actions

**What to build:** When a Ticket needs the user, the Runs view shows a banner naming the Ticket and the reason. Triggers match core: the same refusal three times, ten minutes of inactivity, or the third stall nudge. An idle Worker is nudged on the core schedule. Per Ticket the user can steer the Worker, retry, park or discard. Park means the Ticket never starts or finishes and frees its slot. Discard means the Run leaves it alone. Other Tickets keep running while one needs the user. The CLI notification maps to a Run event, not a desktop notice.

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] The three triggers each raise Needs-you with a readable reason
- [ ] The banner appears live and clears when the cause is resolved
- [ ] Steer sends a prompt to the Ticket's Worker Session
- [ ] Retry runs the merge attempt again; park and discard behave as in core
- [ ] A parked Ticket frees its slot; a discarded Ticket is left alone
- [ ] Idle Workers get the core nudge on the core schedule, and a submit ends the nudges
- [ ] Other Tickets keep running while one needs the user
- [ ] API tests cover each trigger and action; a view test covers the banner and buttons
