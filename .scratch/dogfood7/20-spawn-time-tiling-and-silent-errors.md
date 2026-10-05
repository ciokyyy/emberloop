# Make spawn-time tiling do something and stop hiding its errors

Type: build
Status: open
Blocked by: nothing

## What

Ticket 19 added `tilePanes` (`wick-pig/tile.go`). Review found three gaps:

1. `Spawn` in `worker.go` calls it right after `worktree create`, when the new tab holds one pane, so it always exits at the `len(ids)<2` guard. Spawning several workers never tiles anything.
2. Both callers discard the error, so a failed tile is silent. The "no divider can move" and "never evened out" returns are untested.
3. `fakeTab` in `tile_test.go` models only a single-axis cascade, so splits of the first pane and mixed down/right layouts are untested.

## Done when

- Decide where workers should tile (for example, put every worker of one run into one shared tab as splits) and make `spawn_worker` do it, so N workers in a run end up with 1/N of the tab each.
- A tiling failure is returned in the tool result as a short warning line. It does not fail the spawn.
- Tests cover the two untested return paths, and `fakeTab` gains a mixed down/right tree with a split of the first pane.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
