# 02: Go modules to latest

**What to build:** Every module in `emberloop-web/go.mod` (and `installer/go.mod`) is at its latest version. A module that breaks a Backend stays pinned with a comment naming the reason.

**Blocked by:** None (can start immediately)

**Status:** open

- [ ] `go list -u -m all` shows no update except documented pins
- [ ] `.emberloop/check` passes
- [ ] Any pin names its reason in `go.mod`
