# 05: Reviewer types and the review gate on submits

**What to build:** A worker reviews its own work by spawning reviewer types, not by calling `code_review`.
- **The types.** wick embeds `reviewer-standards` and `reviewer-spec`: `checkout: parent` (they run in the spawning worker's worktree), blocking, persistent, statuses `pass`/`changes`, read-only tools plus what reviewers have today to run checks. The `worker` type gets `spawns: [reviewer-standards, reviewer-spec]` and `requires_review: [reviewer-standards, reviewer-spec]`.
- **Same review machinery, new entry point.** Spawning a reviewer type from a worker runs today's review machinery behind the type: repo checks first, the review packet, follow-ups that cover only the open findings, re-checking a stale pass, and replacing a dead reviewer. Reviewers sit in the ticket tab's grid as ticket-tabs placed them.
- **Persistent reviewers.** Re-spawning a reviewer type the worker already has re-prompts that same reviewer instead of starting a new one.
- **Verdicts become submits.** A reviewer's `submit` (`pass`/`changes` + `report`, one line per finding as today) replaces verdict parsing. The report is kept as the reviewer's findings, for follow-ups and for briefing a replacement. The review gate reads submits: `finish` requires a `pass` at the current branch tip from every type in the worker type's `requires_review`.
- **Removed:** `code_review`, and the verdict parsing and missing-verdict nudge, which the stop gate in ticket 03 replaces.
- **Worker brief.** It says to spawn both reviewer types in one `spawn` call before settling.

**Blocked by:** 06 (Workers on `spawn`, delivery to the parent, auto-background), 03 (Stop gate and the claude `Stop` hook)

**Status:** ready-for-agent

- [ ] The built-in reviewer types load with the fields above, and the `worker` type lists them in `spawns` and `requires_review`.
- [ ] A fake-exec test: a worker's `spawn` of both reviewer types runs the repo checks once, then starts both reviewers in parallel in the worker's worktree, and returns both reports together. A multi-line findings report comes back intact.
- [ ] A second `spawn` of the same reviewer types re-prompts the same panes with only the open findings (moves `TestCodeReviewFollowUpGoesToOpenFindingsOnly` and keeps its assertions).
- [ ] A dead reviewer is replaced, and a stale pass is re-checked (moves `TestCodeReviewReplacesADeadReviewer` and `TestCodeReviewRechecksAStalePass`).
- [ ] `finish` refuses until both reviewer types have submitted `pass` at the tip, and refuses again after a new commit (moves `TestFinishNeedsBothReviewersToPassAtTheTip`).
- [ ] A repo type that overrides a reviewer type, or a worker type with a different `requires_review`, changes what the gate requires.
- [ ] `tools/list` has no `code_review` for any role. The verdict parsing tests are deleted, and the rest of the suite stays green.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
