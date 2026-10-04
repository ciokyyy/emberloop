# What is a worker's verification gate, per repo?

Type: grilling
Status: open
Blocked by: 01

## Question

The `implement` skill's gate is verbatim: use `/tdd` at pre-agreed seams, typecheck regularly, single test files regularly, full suite once at the end, then `/code-review`, then commit. It names **no command**. The closest generic form in the whole suite is `resolving-merge-conflicts`'s "run them, typically typecheck, then tests, then format".

A worker on a headless VPS cannot typecheck without knowing the repo's toolchain. So the harness needs a per-repo answer to: *what command proves this worker did not break anything?*

Decide:

1. **Where the answer lives.** A repo-local file the harness reads, something in the tracker's config, or sniffed from `package.json` scripts. A named file is boring and works; sniffing is fragile and fails silently on an unusual repo.
2. **Granularity.** One verification command, or per-surface commands for pre-commit and pre-merge. Workers run "regularly" during a ticket and "once at the end" across the suite — those are different budgets.
3. **Who authors it.** Hand-written per repo, or generated once from the repo's existing tooling. A repo whose convention is already written down somewhere in `AGENTS.md` should not need a second copy.
4. **What a failure does.** The `implement-spec` flow already ends in one fixer subagent. Decide how many repair attempts a worker gets before the run escalates, and whether a failed gate blocks the merge or only the worker.
5. **The environment half.** Verification needs the repo's dependencies installed in a fresh worktree, plus whatever toolchain the check shells out to. Decide what the worktree bootstrap does — this is the same class of problem as ticket 07 and may want one shared answer.

Note this ticket and ticket 07 are both "the worktree must be runnable before work starts". They may resolve into one mechanism.