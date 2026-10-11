# 11: Declined findings feedback

**What to build:** Each later review prompt lists the Ticket's declined findings with reasons, and the Reviewer prompt says not to re-raise one unless the cited lines changed.

**Blocked by:** minors-and-dispositions (10)

**Status:** ready-for-agent

- [ ] Review prompt includes declined findings with reasons
- [ ] Reviewer prompt carries the no-re-raise rule
- [ ] Run seam test asserts the prompt after a declined finding; `.emberloop/check` passes
