# The merger sends a conflicting worker back to resolve it

Type: build
Status: open
Blocked by: nothing

## What

`finish_worker` runs `git rebase <base>` in the worker's worktree. On a conflict it returns `kept:true`, leaves the worktree and pane in place, and never aborts the rebase (`Finish`, `worker.go`). `implementPrompt` (`extension.go`) then tells the merger to stop and report. The merger has no edit tools by design, so it cannot resolve the conflict, but the worker that wrote the code can.

Change step 4 of `implementPrompt`:

    if finish_worker returns kept:true:
        tell_worker(worker, "the rebase onto base conflicted: <detail>. Finish the rebase,
            resolve the conflicts keeping the intent of both sides, run the gate, commit.")
        wait_workers once for that worker
        finish_worker again
        if it returns kept:true a second time: stop and report it, do not retry further

Keep the rule that the merger never edits or resolves conflicts itself.

## Done when

- `implementPrompt` carries the retry-once rule above, and says one retry only.
- A test asserts the prompt names `tell_worker` for a kept worker and limits the retry to one.
- Check, and cover with a test over the fake exec, that `finish_worker` called a second time on a worker whose first call returned `kept:true` still finds the record and merges when the worker has committed. Fix `Finish` if it does not.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
