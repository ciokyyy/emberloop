# wick-pig settings: choose the decision model

Type: build
Status: open
Blocked by: nothing

## What

`tools.Settings{DecisionModel, Threshold}` with `LoadSettings`/`SaveSettings` exists in `wick-pig/tools/settings.go`, but nothing sets it outside pig's old `/decision-model` command. Add `wick-pig settings`.

## Done when

- `wick-pig settings` prints the current settings.
- `wick-pig settings decision-model <id>` and `wick-pig settings threshold <n>` save them via `SaveSettings`.
- `wick-pig settings models` lists ids from `GET https://openrouter.ai/api/v1/models?output_modalities=decisions`; the HTTP call goes through a function value so tests inject a fake. No network in tests.
- Unknown subcommand prints usage and exits non-zero.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./... ` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

