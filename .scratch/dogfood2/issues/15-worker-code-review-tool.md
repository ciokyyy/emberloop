# Workers need a code_review tool

Type: build
Status: open
Blocked by: nothing

## What

The `implement` skill, loaded into every worker, ends with "use /code-review to review the work". The `work` role has no tool that can do it, so workers skip it. `ReadOnly` and `Review` in `wick-pig/readonly.go` already run a read-only reviewer in a herdr pane and return its last message; `review_worker` in `wick-pig/orchestrate.go` uses them for the merger.

## Done when

- New tool `code_review` (no arguments) in the `work` role, and therefore `plan` and `full`. It reviews the diff of the current worktree against its base: committed changes since the base plus uncommitted ones. It returns the reviewer's reply as the tool result.
- It reuses `ReadOnly`; it does not copy it. The diff file is written like `Review` does.
- Tool description tells the worker to call it once, after committing, and to fix real findings before finishing.
- `wick-pig/roles.go` and its test list the tool in exactly the right roles. `review` and `scout` roles still serve only read and find.
- A test with the existing `fakeHerdr` proves the tool returns the reviewer's last message.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.

