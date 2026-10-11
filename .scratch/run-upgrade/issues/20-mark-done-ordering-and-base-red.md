# 20: Mark done ordering and base red

**What to build:** A Ticket's `done` status and boxes are written only after its merge into the spec branch, the spec's status only after the master merge. Before a Ticket starts, the repo check runs once per spec-branch tip; a red base marks the Ticket Needs-you with the failing tests listed instead of starting a Worker.

**Blocked by:** spec-branch-core (09)

**Status:** ready-for-agent

- [ ] Status and boxes written after merge only
- [ ] Spec status written after master merge only
- [ ] Red base gives Needs-you with failing tests; result cached per tip
- [ ] Run seam tests; `.emberloop/check` passes
