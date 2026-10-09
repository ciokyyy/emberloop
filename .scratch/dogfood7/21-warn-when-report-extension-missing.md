# Warn when the herdr-report extension cannot be found

Type: build
Status: open
Blocked by: 20-spawn-time-tiling-and-silent-errors

## What

Without the `herdr-report` extension herdr shows a pig worker as `unknown` for ever, so `wait_workers` never sees it settle and the merger waits on a state that cannot arrive. `reportExtension` (`extension.go`) returns "" with no signal when no candidate exists. This happened in the ticket 19 run: the binary lived in `~/.local/bin`, the lookup checked `~/.local/herdr-report`, and nothing was found.

## Done when

- `spawn_worker` adds a warning line to its result when the harness is pig and `reportExtension` returns "": status will read unknown, judge a worker by its branch commits and an idle pane.
- `wait_workers` output already has the all-unknown sentence from ticket 14; make it name the missing extension and the three places that were searched.
- A test covers the warning with a temp $HOME and no env var.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
