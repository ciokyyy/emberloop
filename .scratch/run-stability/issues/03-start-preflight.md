# 03: Start preflight

**What to build:** Before a Run's first Ticket starts, the Daemon checks that the base commit has a repo check script, that no stale implement branch from another Run blocks the Start, and that the base check is green. A stale branch owned by an abandoned or failed Run is removed; an unowned one is named in the refusal. A missing check script or a red base refuses Start with the cause named, instead of parking every Ticket. The base check result stays cached and is reused by Ticket checks as today.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A base commit with no repo check script refuses Start and says so
- [ ] A stale implement branch owned by an abandoned or failed Run is removed and Start proceeds
- [ ] An unowned stale implement branch refuses Start and is named
- [ ] A red base refuses Start and names the failing test
- [ ] A green base starts as before and the Ticket checks reuse the cached base result
- [ ] `.emberloop/check` passes
