# 06: Workers on `spawn`, delivery to the parent, auto-background

**What to build:** Workers are spawned through `spawn`, and their results, along with any blocking result that outlasts the tool budget, reach the parent as system notices in its pane.

- **`worker` type.** emberloop embeds `worker` (`worktree`, non-blocking, statuses `done`/`blocked`, today's worker brief). `implement` may spawn it. A `worktree` entry takes today's worker fields (ticket, base, target, harness, model), and gets its git worktree, branch and ticket tab exactly as `spawn_worker` did. Non-blocking entries come back as "started" at once, and a mixed call returns once the blocking ones have submitted. `spawn_worker` is removed.
- **Parent pane.** At spawn, the spawning session's `HERDR_PANE_ID` is recorded with the agent as its parent pane.
- **Delivery.** A non-blocking agent's submit is delivered right away into the parent pane through the nudge path: `herdr agent prompt`, falling back to `send-text` + Enter flattened to one line. The message is `<system-notice> <type> <id> submitted (<status>). Resume your work using the result below. ── <id> ── <summary></system-notice>`.
  - Deliveries to one parent pane are serialized with a per-pane lock.
  - A notice for a missing parent pane is dropped without error.
  - An agent stays open after submitting.
- **Auto-background.** A blocking `spawn` waits at most the tool budget. When the budget runs out, it returns the reports already in, plus a line saying the rest are still running, their results will arrive as notices, and the caller should end its turn. From then on those agents' submits are delivered like non-blocking ones, with the report as the notice body. The "still running, call the same tool again" answer is removed. The agent's state records which of its children are backgrounded and still owe a result, for the stop gate in ticket 03.

**Blocked by:** 01 (Agent types, `spawn` and `submit`, with scouts moved over), 07 (Follow-ups from the ticket-tabs run; both change worker spawn)

**Status:** ready-for-agent

- [ ] `tools/list` has no `spawn_worker`. `spawn` of a `worker` entry creates the worktree, branch and ticket tab as before (existing spawn tests move over and stay green).
- [ ] A fake-exec test: a mixed `spawn` (two scouts, one worker) returns the scouts' reports plus "worker started" once both scouts have submitted.
- [ ] A worker's submit issues `herdr agent prompt <parent pane>` with the notice. When herdr refuses, it falls back to `send-text` + `send-keys Enter` on one line. A blocking submit within budget is never typed into any pane.
- [ ] Concurrent submits to one parent pane never overlap their herdr calls. Submits to different parent panes do overlap.
- [ ] A parent pane that herdr reports as not found makes `submit` succeed, and nothing more is sent.
- [ ] With a shrunk budget, a blocking spawn returns the reports already in plus the still-running line, a later submit from the rest is delivered as a notice, and no response anywhere says "call again".
- [ ] The spawning agent's state lists its backgrounded children until each has submitted.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
