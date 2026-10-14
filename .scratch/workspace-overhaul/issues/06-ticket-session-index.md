# 06: Ticket-to-Session index

**What to build:** Looking up a Ticket's Sessions does not scan every Session. An index is built when a Session is attached to a Ticket and replaces the per-Ticket scan, so the cost does not grow with the Session count (spec Part 2, Speed).

**Blocked by:** 04-runs-list-summaries (both change the Runs list path)

**Status:** ready-for-agent

- [ ] Attaching a Session to a Ticket records it in an index
- [ ] The Runs list and detail read Ticket Sessions from the index; no per-Ticket scan of all Sessions remains
- [ ] The index survives a Daemon restart (rebuilt from disk or persisted)
- [ ] Go tests cover the index through the Runs HTTP seam; repo check passes
