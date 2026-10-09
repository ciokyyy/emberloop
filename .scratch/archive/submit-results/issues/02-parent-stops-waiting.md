# 02: `tell`, `finish`, and the parent stops waiting

**What to build:** The parent has no way to wait for an agent, and doesn't need one: it spawns, ends its turn, and is told.
- **`tell` and `finish` replace `tell_worker` and `finish_worker`, and work for any agent id.**
  - `tell` delivers through the nudge path. For a non-blocking agent it returns right away, and the agent's answer arrives later as a submit notice. For a blocking agent it waits for the next submit and returns its report, auto-backgrounding past the tool budget as ticket 06 does for `spawn`.
  - `finish` works only for `worktree` agents and keeps today's merge and discard behaviour.
- **Removed:** `wait_workers` (with its cap and its tests), `tell_worker`, and `finish_worker`.
- **End-your-turn guidance.** When `spawn` or `tell` started or messaged non-blocking agents, its result ends with a line telling the parent to end its turn, because results arrive as messages. The orchestrator prompts, the implement prompt and the skills el adapts that mention waiting are updated to say the same.
- **`implement` always runs in a herdr pane.** The headless parent path is removed, and launching `implement` outside herdr fails with a clear error.

**Blocked by:** 06 (Workers on `spawn`, delivery to the parent, auto-background)

**Status:** ready-for-agent

- [ ] `tools/list` has `tell` and `finish`, and no `wait_workers`, `tell_worker`, `finish_worker` or other wait tool, for any role.
- [ ] A fake-exec test: `tell` to a non-blocking agent issues the delivery and returns without calling `herdr agent wait`. `tell` to a blocking agent returns that agent's next submit.
- [ ] `finish` on a `worktree` agent merges or discards as `finish_worker` did (existing finish tests move over and stay green). `finish` on any other type is a clear error.
- [ ] The `spawn` and `tell` results carry the end-your-turn line when non-blocking agents are involved, and only then.
- [ ] No prompt or brief el renders tells the parent to call `wait_workers` or to wait for an agent.
- [ ] `implement` without `HERDR_PANE_ID` fails with a clear error before spawning anything, and the headless parent path is gone.
- [ ] The `wait_workers` cap tests and the `tell_worker` settle and wait tests are deleted, and the rest of the suite stays green.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
