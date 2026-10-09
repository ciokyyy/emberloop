# el settings: choose the decision model

Type: build
Status: open
Blocked by: nothing

## What

`tools.Settings{DecisionModel, Threshold}` with `LoadSettings`/`SaveSettings` exists in `el/tools/settings.go`, but nothing sets it outside pig's old `/decision-model` command. Add `el settings`.

## Done when

- `el settings` prints the current settings.
- `el settings decision-model <id>` and `el settings threshold <n>` save them via `SaveSettings`.
- `el settings models` lists ids from `GET https://openrouter.ai/api/v1/models?output_modalities=decisions`; the HTTP call goes through a function value so tests inject a fake. No network in tests.
- Unknown subcommand prints usage and exits non-zero.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./... ` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

