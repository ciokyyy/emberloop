# 22: Approval queue, subject keys and supersession

**What to build:** Queue of pending Rule candidates, Profile patches and promotions. A candidate reaches the queue only on `/remember` or when its subject key recurs in two or more Sessions on different tasks; generalizing language counts for it, situational language against. The approver sees one proposed generalization with its quotes and can approve, edit or reject; per-Session batch approval. Subject keys are proposed by the Decider and normalized; same key, same author, newer observation supersedes deterministically; a different author is a disagreement and stays in the queue. Rejected items are kept as labels. Queue budget and approval latency are recorded.

**Blocked by:** 19, 20

**Status:** ready-for-agent

- [ ] A single mention never reaches the queue; the same subject key in two Sessions on different tasks does
- [ ] Approve creates a new rule version; edit keeps both the proposal and the final text; reject keeps the item as a label
- [ ] Same key, same author, later date supersedes; a different author appears as a disagreement item
- [ ] Rule text over 300 chars is refused at write, not clipped later
- [ ] Batch approval per Session works
- [ ] API tests cover each path with a fake clock
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
