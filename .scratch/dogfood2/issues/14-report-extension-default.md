# Workers must register with herdr without EMBERLOOP_HERDR_REPORT

Type: build
Status: open
Blocked by: nothing

## What

`reportExtension()` in `emberloop/extension.go` reads only `EMBERLOOP_HERDR_REPORT`. When it is unset, pig workers launch without `-e herdr-report`, herdr never sees them as agents, their status stays `unknown`, and `wait_workers` never sees one settle.

## Done when

- When the variable is unset, `reportExtension()` returns the first existing directory of: `<dir of the running executable>/../herdr-report`, `<dir of the running executable>/herdr-report`, `$HOME/.pig/agent/extensions/herdr-report`. Env var still wins when set. Empty string only when none exists.
- The lookup takes the executable path and home as parameters (or a function value) so tests use temp dirs.
- `wait_workers` output, when every pending worker reports `unknown`, adds one sentence saying the workers are not registered with herdr and why that usually means the report extension is missing.
- Tests cover env wins, each fallback, and none-found.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

