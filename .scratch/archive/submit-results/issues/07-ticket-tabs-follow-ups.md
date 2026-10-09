# 07: Follow-ups from the ticket-tabs run

**What to build:** Three gaps left by the ticket-tabs run, fixed together.

- **Respawn lands in the right pane.** When a ticket is re-spawned while its tab is still open, the implementor goes into the tab's root pane, found by its grid slot (slot 1). Today it goes into whichever pane herdr happens to list first, which can be a reviewer. Whatever still runs in that pane is stopped before the launch, so the launch line is never typed into a live agent.
- **Respawn after finish works.** `finish` keeps the `implement/<id>` branch. When that branch already exists at spawn, el reuses it with `git worktree add <path> <branch>` (no `-b`), provided it is at or behind the requested base. A branch with commits the base doesn't have makes spawn fail with a clear error naming the branch. Today `git worktree add -b` fails because the branch already exists.
- **`background` panes use the grid.** The `background` tool places its pane through the shared grid into the caller's tab, with the `scouts` overflow, instead of `pane split --current --direction down`.

The slot-1 case raised by ticket 03 (a dead reviewer in slot 1 of a fresh overflow tab falls back to a free slot) is left as it is, because two reviewers never reach it.

**Blocked by:** None (can start immediately; ticket-tabs is merged and installed)

**Status:** ready-for-agent

- [ ] A fake-exec test: re-spawning a ticket whose tab is open, with herdr's pane list returning a reviewer pane first, launches into the slot-1 pane, and stops the process running there before `pane run`.
- [ ] Re-spawning when `implement/<id>` exists at or behind the base issues `git worktree add <path> implement/<id>` without `-b`, and the spawn succeeds.
- [ ] Re-spawning when `implement/<id>` has commits the base lacks fails before creating anything, with an error naming the branch.
- [ ] `background` start places its pane through the grid into the caller's tab, issues no `--direction down`, and overflows into `scouts` past 9 panes.
- [ ] Existing spawn, grid and background tests stay green.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
