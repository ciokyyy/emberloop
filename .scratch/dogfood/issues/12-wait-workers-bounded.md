# wait_workers must return before the harness tool timeout

Type: build
Status: open
Blocked by: nothing

## What

`wait_workers` takes `timeout_ms`. A harness (claude, `MCP_TOOL_TIMEOUT`) can cut a tool call earlier than that and report a failure while the wait goroutine keeps running.

## Done when

- `wait_workers` clamps `timeout_ms` to a package constant (default 5 minutes) and says in its result that it was clamped and to call it again.
- The tool description states the cap.
- A test covers the clamp with a fake exec.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./... ` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

