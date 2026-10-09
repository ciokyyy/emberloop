# 03: Ticket reviewers in the ticket tab

**What to build:** A ticket's code reviewers open next to its implementor, in the ticket's tab, instead of piling up in the orchestrator's tab. Reviewers are placed through the grid from ticket 01 into the ticket tab from ticket 02. The implementor is pane 1, so the first row is `Impl A B`. Overflow goes to `<ticket-id> (2)`, then `(3)`, and so on, in the same workspace. Reviewer records remember the tab they live in. Persistence, re-prompting on later review rounds and the review gate are unchanged: reviewers stay open until the ticket finishes. When a reviewer's pane has died, its replacement goes into the dead pane's slot and the grid is not re-flowed. Finish and discard close the reviewers and every overflow tab of the ticket along with the ticket tab.

**Blocked by:** 01 (Grid placement for read-only agents in the caller's tab), 02 (Ticket tab in the project workspace on a git worktree)

**Status:** ready-for-agent

- [ ] A code review on a worker splits the worker's ticket tab, never the caller's pane, and the reviewers sit beside the implementor in rows of three.
- [ ] A later review round re-prompts the same reviewer panes. No new panes are created (existing follow-up tests stay green).
- [ ] A dead reviewer is replaced in the same slot of the same tab.
- [ ] With more than 8 reviewers, the overflow lands in a `<ticket-id> (2)` tab in the project workspace.
- [ ] Finish and discard close the ticket tab and all its overflow tabs, and forget the reviewer records.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/ticket-tabs/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
