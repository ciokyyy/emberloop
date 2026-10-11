# 07: Find graph index

**What to build:** `find` reads the `codebase-memory` index as a third source beside jev and grep when the index exists, tags each hit with its `source`, ranks symbol-name graph hits first, and degrades to the current two sources when the index is absent.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Hits carry `source` of `jev`, `grep` or `graph`
- [x] Graph hits for an exact symbol rank first
- [x] Absent or failing index leaves current behavior unchanged
- [x] `emberloop/tools/find_test.go` covers each with a stub graph source; `emberloop/tools/AGENTS.md` updated; `.emberloop/check` passes
