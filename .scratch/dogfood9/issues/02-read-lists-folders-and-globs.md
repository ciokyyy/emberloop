# 02: Read lists folders, expands globs, serves emberloop:// as plain text

**What to build:** `read` on a directory returns an `ls`-style listing one level deep (names, `/` on directories, sizes), paged by offset/limit, and a workspace directory and a `emberloop://` directory list in the same format. A `emberloop://` directory holding `SKILL.md` ends with a hint naming it, so reading `emberloop://matt-pocock/grilling` leads straight to the skill. A glob (`*.md`, `dir/*.go`) returns the matching paths, no contents. `emberloop://` files return plain text under a short header (address and line count) with no line numbers and no edit tag, since they can never be edited; offset/limit still page them.

**Blocked by:** 01 (same tool, same code)

**Status:** ready-for-agent

- [ ] A workspace directory read returns the listing instead of "is not a regular file".
- [ ] Workspace and `emberloop://` listings share one format; offset/limit page entries.
- [ ] A `emberloop://` skill directory listing ends with the SKILL.md hint.
- [ ] A glob returns matching paths; a glob with no match says so.
- [ ] A `emberloop://` file has the header, no line numbers, no tag; offset/limit work.
- [ ] Tests over the read tool's Execute; existing emberloop:// index tests updated to the shared format.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
