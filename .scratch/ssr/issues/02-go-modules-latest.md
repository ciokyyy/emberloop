# 02: Go modules to latest

**What to build:** Every module in `emberloop-web/go.mod` (and `installer/go.mod`) is at its latest version. A module that breaks a Backend stays pinned with a comment naming the reason.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `go list -u -m all` shows no update except documented pins
- [x] `.emberloop/check` passes
- [x] Any pin names its reason in `go.mod`
