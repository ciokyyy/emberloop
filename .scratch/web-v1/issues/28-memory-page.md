# 28: Memory page

**What to build:** Rail entry and page: list and search Rules and observations, queue with batch approval, Profile editor with versions, purge, stats (queue size, approval latency, last nightly run and ledger), and engine status chip.

**Blocked by:** 17, 18, 22

**Status:** done

- [x] A User sees only their own bank; admins get no extra read access to other Users' notes
- [x] Queue shows each proposal with quotes and source Sessions; approve, edit, reject and batch approve work
- [x] Profile versions are listed; any version can be restored as a new version
- [x] Purge needs a typed confirmation and reports what it removed
- [x] Unavailable chip shows when the engine fails
- [x] vitest covers list, search, queue actions, versions, purge and status
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
