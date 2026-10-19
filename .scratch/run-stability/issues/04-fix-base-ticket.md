# 04: Fix-base Ticket on a red base

**What to build:** When the Start preflight finds a red base, the User can accept one generated "fix base" Ticket that runs before the spec Tickets. It carries the failing test and its output. The spec Tickets begin only after it merges and the base is green.

**Blocked by:** 03-start-preflight

**Status:** ready-for-agent

- [ ] The refusal offers the fix-base Ticket; declining leaves the Run unstarted
- [ ] Accepting adds exactly one Ticket that blocks every spec Ticket
- [ ] The Ticket's brief names the failing test and includes its output
- [ ] After it merges the base is re-checked and the spec Tickets start
- [ ] If the base is still red after it merges, the Run raises needs-you rather than adding a second fix-base Ticket
- [ ] `.emberloop/check` passes
