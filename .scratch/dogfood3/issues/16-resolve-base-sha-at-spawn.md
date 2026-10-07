# Resolve a worker's base to a SHA at spawn time

Type: build
Status: open
Blocked by: nothing

## What

`Spawn` in `emberloop/worker.go` stores `"base":"HEAD"`. Inside the worker's worktree `HEAD` is the worker's own branch tip, so every later `git diff <base>` (self-review, `review_worker`, finish) is empty once the worker commits.

## Done when

- `Spawn` resolves `base` with `git rev-parse <base>` in the repo before creating the worktree and stores the full SHA in `Worker.Base` and passes the SHA to `herdr worktree create --base`.
- A caller that passes a branch name still works; the stored value is still a SHA.
- Existing store records with a non-SHA base keep working: where `Base` is read, a ref is fine.
- A test with a fake exec proves `HEAD` becomes a SHA in the store and in the herdr arguments.
## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
