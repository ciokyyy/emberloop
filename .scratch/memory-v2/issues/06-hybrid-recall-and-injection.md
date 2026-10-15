# 06: Hybrid recall and injection block

**What to build:** Candidates are FTS5 top 40 unioned with vector top 40. Score `0.5*cosine + 0.3*fts + 0.2*importance` (weights from `memory_recall_weights`, renormalized; missing vector renormalizes over the rest). Kind decay `0.7 + 0.3*decay`: preference and instruction never decay, decision half-life 180 days, fact 90 days, pinned never. Veracity multiplier 1.0 stated, 0.8 inferred. Gates: cosine under 0.30 with no FTS hit is dropped; forgotten, superseded and parked are dropped. Merge with Rules and notes by reciprocal rank fusion (k=60). Injection block: Profile, up to 5 Rules, up to 5 Memories, at most 8 items, 700 tokens, inside the fence; Create and compress inject all, resume injects Profile only. The `injection` Event lists Rule and Memory IDs. The `recall` tool also searches Memories and Episodes (labelled `episode`).

**Blocked by:** 01, 04

**Status:** todo

- [ ] A paraphrase of a stored Memory is found with no shared keyword
- [ ] Forgotten, parked and superseded Memories never appear in the block or in recall
- [ ] Weights renormalize without a vector; FTS-only fallback works
- [ ] Preference never decays; a 400-day-old fact scores lower than a fresh one
- [ ] BM25 scores from separate tables are never compared
- [ ] Episodes appear only through `recall`, never in the injected block
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
