# 09: Oversize input folding

**What to build:** A Session whose Event log exceeds the summarizer window still compresses. Tool results are cut to head and tail; oversize input is folded in budgeted chunks; each fallback records an Event with its reason.

**Blocked by:** 08

**Status:** done

- [x] Tool results are cut to about 2000 chars (head and tail) before summarizing
- [x] Input over the window is summarized in chunks, then the chunk summaries are summarized
- [x] A chunk that still overflows is halved down to a floor; at the floor tool results are cut to 500 chars of head
- [x] If the input still does not fit, the deterministic Handoff is used
- [x] Each fold and fallback step appends an Event with its reason
- [x] Tests with a fake summarizer in oversize mode cover each path
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
