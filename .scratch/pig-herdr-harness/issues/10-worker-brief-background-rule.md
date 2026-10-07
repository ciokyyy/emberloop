# Workers must put servers in background, never bash

Type: build
Status: open
Blocked by: nothing

## What

The rule "never start a dev server, watcher or log tail with bash; use `background`" lives only in `systemPrompt` (`emberloop/harness.go`). Workers get a brief (`writeBrief` in `emberloop/extension.go`) and the merger gets `implementPrompt`. Neither repeats the rule.

## Done when

- The worker brief written by `writeBrief` contains the rule.
- `implementPrompt` mentions it for any command the merger might run.
- A test in `emberloop` asserts both strings contain `background`.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./... ` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

