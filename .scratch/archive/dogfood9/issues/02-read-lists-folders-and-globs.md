# 02: Read lists folders, expands globs, serves el:// as plain text

**What to build:** `read` on a directory returns an `ls`-style listing one level deep (names, `/` on directories, sizes), paged by offset/limit, and a workspace directory and a `el://` directory list in the same format. A `el://` directory holding `SKILL.md` ends with a hint naming it, so reading `el://matt-pocock/grilling` leads straight to the skill. A glob (`*.md`, `dir/*.go`) returns the matching paths, no contents. `el://` files return plain text under a short header (address and line count) with no line numbers and no edit tag, since they can never be edited; offset/limit still page them.

**Blocked by:** 01 (same tool, same code)

**Status:** ready-for-agent

- [ ] A workspace directory read returns the listing instead of "is not a regular file".
- [ ] Workspace and `el://` listings share one format; offset/limit page entries.
- [ ] A `el://` skill directory listing ends with the SKILL.md hint.
- [ ] A glob returns matching paths; a glob with no match says so.
- [ ] A `el://` file has the header, no line numbers, no tag; offset/limit work.
- [ ] Tests over the read tool's Execute; existing el:// index tests updated to the shared format.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
