# 14: Quick switcher index and routes

**Type:** code

**What to build:** A switcher index over Sessions, Runs, Projects and pages with fuzzy name match, recent first. Opens on Ctrl/⌘+K and from an exposed button; selecting a row routes with the right scope. Logic and tests; the look is in the design Ticket.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A query returns Sessions, Runs, Projects and pages ranked with recent first
- [ ] Selecting a result routes to the right `/p/<id>` or `/s/<id>` URL
- [ ] Ctrl/⌘+K opens it and Escape or Back closes it
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
