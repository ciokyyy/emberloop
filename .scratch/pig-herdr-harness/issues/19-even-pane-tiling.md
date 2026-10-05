# Tile worker and reviewer panes evenly

Type: build
Status: open
Blocked by: nothing

## What

herdr has no equalize action. `pane split` takes `--ratio` and `pane resize` takes `--direction` and `--amount`, so repeated splits of the last pane give 1/2, 1/4, 1/8. With N panes in one tab each should get 1/N of the space.

## Done when

- First, a throwaway test in a scratch herdr pane settles what `--ratio` means (share kept by the original pane, or share given to the new one) and the answer is written in a code comment.
- A helper `tilePanes(run, tab)` lists the panes with `herdr pane layout` and `herdr pane list`, then resizes them until each share is within 2% of 1/N.
- `spawn_worker` and the reviewer split in `readonly.go` call it after creating a pane.
- A test with a fake exec checks the resize calls for N = 2, 3 and 4.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
