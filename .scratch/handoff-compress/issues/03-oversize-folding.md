# 03: Oversize input folding

**What to build:** A Session whose Event log exceeds the summarizer window still compresses. Tool results are cut to head and tail. Oversize input is folded in budgeted chunks: summarize each chunk, then summarize the summaries. Each fallback records an Event with its reason.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] Tool results are cut to about 2000 chars (head and tail) before summarizing
- [ ] Input over the summarizer window is summarized in chunks, then the chunk summaries are summarized
- [ ] A chunk that still overflows is halved down to a floor
- [ ] At the floor, tool results are cut to 500 chars of head
- [ ] If the input still does not fit, the deterministic Handoff is used
- [ ] Each fold and fallback step appends an Event with its reason
- [ ] Tests with a fake summarizer in oversize mode cover each path
