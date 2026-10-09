# Claude MCP config temp file is never removed

Type: build
Status: open
Blocked by: nothing

## What

`claudeHarness.Command` writes a temp MCP config file and passes `--mcp-config <file>`. Nothing deletes it, so every claude launch leaks a file in the temp dir.

## Done when

- The leak is gone: either write the config under the state dir at a path derived from the session id (so `Review`/`ReadOnly` cleanup and the worker finish path remove it), or delete it when the session ends. Pick the smaller change.
- A test proves a launch creates no file that outlives the cleanup the caller already performs.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./... ` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

