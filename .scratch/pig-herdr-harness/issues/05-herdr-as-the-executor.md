# How does `implement-spec` fan out through herdr instead of background agents?

Type: grilling
Status: open
Blocked by: 01, 04

## Question

`implement-spec` today spawns background subagents inside one host process, each on its own worktree and branch, then merges them, reviews, runs one fixer, and cleans up. The destination replaces that spawning mechanism with herdr, running on a VPS, with workers that may be `pig` or `claude`.

Decide:

1. **Who owns the loop.** `implement-spec` stays as a skill and its fan-out is rewritten to shell out to `herdr`, or a pig extension takes over the loop and the skill becomes a thin brief. The second is probably simpler — an extension has the tools and a TUI, a skill has prose.
2. **Fan-out shape against the DAG.** Ticket files carry `Blocked by:` in dependency order. Workers for independent tickets start together; a worker whose blockers are unmerged waits. Decide whether the scheduler merges in dependency order as branches land, or runs whole waves and merges at the end.
3. **Worktree lifecycle.** `herdr worktree create --workspace ID --cwd PATH --branch NAME --base REF --path PATH` requires the workspace to sit inside a git work tree, and moving a pane re-IDs it. Decide the branch naming, the base ref, and what happens to a worktree when its worker dies.
4. **Identity and readiness.** Workers self-report as `pig`/`claude` via `pane report-agent`, so `herdr agent wait --until working` is the readiness gate that replaces `agent start`'s. Decide the wait policy — and never pass a wait without `--timeout`, since herdr has no default.
5. **The unmergeable case.** Two workers touching the same file. `resolving-merge-conflicts` is intent-preserving and never uses `--abort`. Decide whether a conflict escalates to a human or goes to a merger subagent, and which.
6. **What happens when herdr is not running.** The VPS is headless and always-on in theory. Decide the failure mode, not just the happy path.

Constraint carried in from ticket 03: herdr's `agent start --kind` has no `pig`, so workers launch with `herdr pane run <pane> <agent>` and readiness is polled, not gated by `agent start`.