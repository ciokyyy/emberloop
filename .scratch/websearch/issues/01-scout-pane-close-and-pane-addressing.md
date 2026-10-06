# 01: Scout panes close on delivery; tell and finish accept a pane id

**What to build:** A scout's herdr pane closes as soon as its result is delivered to the parent, so idle scout panes stop piling up. `spawn` returns both the agent id and the pane id for every entry. `tell` and `finish` accept either `agent` or `pane` (the herdr pane id), exactly one; the pane id resolves to an agent through its record. An unknown pane, or one matching no live agent, returns a clear error. There is no keep option, no idle reaper and no prune command in this ticket.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] After a scout's result is delivered, its pane is closed and its launch file cleaned up
- [x] A failed spawn still rolls back as before
- [x] `spawn` output lists agent id and pane id for each entry
- [x] `tell` and `finish` take `agent` or `pane`; neither or both is an error
- [x] An unknown pane id gives a clear error and acts on nothing
- [x] Fake-herdr tests cover the close after delivery and pane resolution; repo check passes
