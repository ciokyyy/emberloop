# 01: Tempdir guard

**What to build:** A test fails when any `emberloop-web` test file calls `t.TempDir()`. The existing call sites use `tempDir(t)`, and `cmd/emberloop-web` uses an exported shared helper.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Scan test fails on a `t.TempDir()` call in a test file and ignores comments
- [x] All current call sites converted, including `cmd/emberloop-web`
- [x] Shared test helper exported; `emberloop-web/AGENTS.md` names the guard
- [x] `.emberloop/check` passes
