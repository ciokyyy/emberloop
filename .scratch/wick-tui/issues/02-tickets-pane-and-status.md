# 02: Tickets pane with derived status

**What to build:** Selecting a spec shows its tickets on the right, each with a status glyph: not started, working, retrying, idle, submitted done or blocked, in review with code and spec verdicts, merged, discarded, failed (pane gone). Status is worked out in one function inside `Snapshot` from worker entries, herdr state, submits, reviewer records and history. Clicking selects a ticket, keyboard too.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Table test covering every status from fixture signals
- [ ] Glyphs and colours defined once
- [ ] Click and keyboard selection in the tickets pane
