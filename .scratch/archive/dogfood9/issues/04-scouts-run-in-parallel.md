# 04: Scouts run in parallel

**What to build:** Two scouts asked at once finish in about the time of one. The MCP server's `tools/list` carries `annotations: {readOnlyHint: true}` for read, find and scout (and for nothing else: code_review, bash, background and the orchestration tools get none), so a claude session batches them. The server already runs calls concurrently; inside the read-only launcher only the herdr pane split is serialized with a lock, while pane run, waiting and reply reading stay concurrent.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `tools/list` has readOnlyHint on exactly read, find, scout.
- [ ] A server test over stdin/stdout: two slow fake tool calls finish in about one call's time.
- [ ] A fake-exec test: concurrent read-only launches never overlap their split calls, but do overlap their waits.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
