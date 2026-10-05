# Workers must put servers in background, never bash

Type: build
Status: open
Blocked by: nothing

## What

The rule "never start a dev server, watcher or log tail with bash; use `background`" lives only in `systemPrompt` (`wick-pig/harness.go`). Workers get a brief (`writeBrief` in `wick-pig/extension.go`) and the merger gets `implementPrompt`. Neither repeats the rule.

## Done when

- The worker brief written by `writeBrief` contains the rule.
- `implementPrompt` mentions it for any command the merger might run.
- A test in `wick-pig` asserts both strings contain `background`.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./... ` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

