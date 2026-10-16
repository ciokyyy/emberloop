# 05: Dedup and supersession

**What to build:** Before insert, compare a new Memory with active Memories of the same bank: same `NormalizeQuote` text is a duplicate; cosine at or above 0.95 with shared tokens is a duplicate (bump `mention_count`, `last_seen_at`, importance +0.05 up to 0.95); cosine at or above 0.88 with two or more shared significant tokens and opposite polarity is `contradicts` and the newer Memory supersedes the older one from the same author in one transaction; cosine at or above 0.88 without a polarity split is `refines`. Without vectors only the text and `Relation` checks run.

**Blocked by:** 01, 04

**Status:** done

- [x] "always return wrapped errors" and "never return wrapped errors" contradict and supersede; they never merge
- [x] A duplicate raises `mention_count` and creates no row
- [x] Contradiction across authors does not supersede
- [x] A superseded Memory is reachable only through `recall` with `include_superseded`
- [x] Null-vector rows use the text-only path
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
