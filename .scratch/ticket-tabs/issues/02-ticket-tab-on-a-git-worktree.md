# 02: Ticket tab in the project workspace on a git worktree

**What to build:** Spawning a worker no longer creates a herdr worktree workspace, so the sidebar stops showing an empty terminal for each worker. Spawn creates the worktree with `git worktree add -b implement/<id> <worktree path> <resolved base SHA>`, keeping today's path and branch scheme. It then finds the tab labelled `<ticket-id>` in the project workspace (`HERDR_WORKSPACE_ID`), or creates it with `herdr tab create --workspace <ws> --cwd <worktree> --label <ticket-id>`, and runs the implementor in that tab's root pane. A respawn or retry reuses the tab and replaces the implementor pane. If anything fails after the worktree exists, the git worktree and branch are removed and the tab is closed if this spawn created it. Finish and discard close the ticket tab and run `git worktree remove --force`. Branch handling at finish is unchanged. Run-tab mode (`RunTab`, `Anchor`, splitting the worker into a shared run tab) and every `herdr worktree create/remove` call are deleted. Workers created under the old layout are not supported.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Spawn issues `git worktree add` with the resolved SHA, the `implement/<id>` branch and the usual worktree path, and never issues `herdr worktree`.
- [ ] Spawn creates a tab in the workspace from `HERDR_WORKSPACE_ID`, labelled with the ticket id, with the worktree as cwd, and runs the implementor in that tab's root pane. The worker record stores the workspace, tab and pane.
- [ ] Spawn fails with a clear error when `HERDR_WORKSPACE_ID` is unset, before creating anything.
- [ ] Respawning a ticket reuses its existing tab and replaces the implementor pane. No second tab is created.
- [ ] A failure after the worktree exists removes the git worktree and branch, and closes the tab only if this spawn created it. The ticket can then be spawned again.
- [ ] Finish and discard close the ticket tab and run `git worktree remove --force`, with no `herdr worktree remove`. Existing rebase and merge tests stay green.
- [ ] `RunTab`/`Anchor` are gone from the spawn input, the tools and the orchestrator, and their tests are deleted.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/ticket-tabs/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
