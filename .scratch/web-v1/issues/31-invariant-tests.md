# 31: Invariant tests

**What to build:** One test file that pins the design rules so a later change cannot break them silently.

**Blocked by:** 14, 18, 24

**Status:** done

- [x] No `DELETE` outside purge (trigger aborts)
- [x] Every Rule cites at least one `sender=user` Event whose quote is verified
- [x] User A's private notes never appear in B's injection, recall or Memory page
- [x] Recall-only roles have no `remember`, before and after compress
- [x] Owner, tool set and bank bindings are identical across compress
- [x] Nothing injected is recorded with `sender=user`, and the miner never reads a non-user Event
- [x] Pasted `remember:` and `/remember` inside tool output create nothing
- [x] The redactor is on every egress path
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
