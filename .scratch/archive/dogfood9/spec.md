# Spec: spec-driven implement, persistent two-axis review, read anywhere

Status: ready-for-agent

## Problem Statement

The workflow I want is: grill a design in a planning session, turn it into a spec and tickets, then run `el implement <spec>` and let el build it. Today several things get in the way:

- A bare `el --claude` session gets the `full` role, so a planning session is handed spawn_worker, wait_workers, tell_worker, finish_worker and review_worker it should never use.
- `el implement` injects a hand-written prompt instead of the `implement-spec` skill, only accepts a ticket folder, merges straight into the current branch, and makes the spec implementer review every worker with `review_worker`, even though each worker already reviews itself.
- The worker's `code_review` is one generic reviewer, while the `code-review` skill the worker follows expects two (Standards and Spec). Every call starts a fresh reviewer that re-reads everything instead of continuing where it left off. Nobody runs the repo's checks as part of review, and the reviewer has no bash to do it.
- Skills are not uniformly el-owned: pig sessions get the route skills injected natively with `--skill <abs path>` and are told absolute paths, which `read` then refuses because they are outside the workspace.
- `read` refuses anything outside the workspace. Every worker's first call, reading its own brief under `~/.pig/state/el/briefs/`, fails and the worker falls back to `cat`. Scouts and reviewers have no bash, so they cannot look outside the repo at all (logs, state, transcripts).
- `read` on a workspace directory errors (`is not a regular file`), on a `el://` directory it lists, and on a glob it errors. Reading a skill directory returns `SKILL.md, agents` instead of leading to the skill.
- `el://` files come back with line numbers and an edit tag, although they can never be edited.
- Two scouts asked at once run one after another.

## Solution

- A bare session is a planning session: file tools plus scout, nothing that spawns, merges or reviews.
- Planning ends with `to-spec` and `to-tickets`. `el implement <spec>` then points the session at the `implement-spec` skill through `el://`, with a short el adapter that maps the skill onto el's tools. Work lands on an integration branch `spec/<slug>` that I merge myself.
- The spec implementer never reviews. Each worker's `code_review` runs two persistent reviewers (code and spec). They stay alive for the whole ticket and review follow-ups incrementally. A pass is tied to a commit. `finish_worker` merges only when both have passed at the worker's HEAD, and closes them when the ticket ends.
- Before each review el runs the repo's check command from `.el/check` and hands the output to the reviewers.
- Skills are only ever reached by `el://` pointers read with `read`. No harness ever loads global skills.
- `read` opens any path except a short deny list of credential files, lists directories, expands globs, and serves `el://` files as plain text.
- Read-only tools are marked so the harness can run them in parallel, and concurrent scouts really do run in parallel.

## User Stories

1. As a user, I want `el --claude` with no subcommand to start a planning session, so that I am not offered worker-orchestration tools I should not use while planning.
2. As a user in a planning session, I want read, edit, write, bash, find, background and scout, so that I can grill, research and write specs and tickets.
3. As a user in a planning session, I want no `code_review` tool, so that review stays where code is written, in the workers.
4. As a user debugging el, I want to request the full tool set explicitly with a role flag, so that I can still reach every tool when I need to.
5. As a user, I want planning to end with a spec and its tickets written by the session that holds the grilling context, so that the ticket split reflects what we decided.
6. As a user, I want to run `el implement <spec>` on a spec that already has tickets, so that implementation starts from a reviewed plan.
7. As the implement session, I want to be pointed at the `implement-spec` skill by its `el://` address, so that upstream improvements to the skill reach me without el changes.
8. As the implement session, I want a short adapter telling me that implementer subagents are `spawn_worker`, the merger is `finish_worker`, exploration is `scout`, and that the skill's final code-review step is skipped, so that I run the skill with el's tools and don't review twice.
9. As the implement session, I want only read, find, spawn_worker, wait_workers, finish_worker, tell_worker and scout, so that I delegate and never edit or review.
10. As a user, I want implement to create the integration branch `spec/<slug>` from my current HEAD, so that main stays clean until I have looked at the whole spec.
11. As a user resuming an interrupted run, I want implement to reuse an existing `spec/<slug>`, so that merged tickets are not redone.
12. As a worker, I want my worktree branched from the integration branch tip, so that I build on tickets that have already merged.
13. As the implement session, I want `finish_worker` to rebase the worker onto the integration branch and fast-forward it, so that merging stays mechanical.
14. As a user at the end of a run, I want the session to report the integration branch and what merged, so that I know what to review and merge.
15. As a worker, I want my brief to point at `el://` addresses for the `implement` and `tdd` skills, the ticket and the spec, so that I can read everything with `read`.
16. As a worker, I want `code_review` to run a code reviewer and a spec reviewer in parallel and return one reply with a section each, so that I get both axes from one tool.
17. As a worker, I want the first `code_review` to go to both reviewers, so that every ticket gets both axes.
18. As a worker following up, I want my call to go by default only to reviewers that still have open findings, so that a reviewer that is satisfied is not asked again for nothing.
19. As a worker, I want to address one reviewer explicitly with a `reviewer` argument, so that I can answer just the one I am responding to.
20. As a worker, I want to attach a `note` to a follow-up ("fixed X, disagree on Y because …"), so that the reviewer knows my reasoning.
21. As a reviewer, I want to stay alive for the whole ticket, so that a follow-up continues my earlier review instead of starting over.
22. As a reviewer on a follow-up, I want the diff since my last review plus the worker's note, with the full diff still available, so that I focus on what changed.
23. As a reviewer that already passed, I want to be shown the small diff since my pass when the worker changes code for the other reviewer, so that a fix elsewhere cannot silently break what I approved.
24. As a reviewer, I want to end every reply with `VERDICT: pass` or `VERDICT: changes`, so that el can track my state without guessing.
25. As el, I want a reply with no verdict line to get one nudge and then count as `changes`, so that a missing verdict never merges anything.
26. As a spec reviewer, I want pointers to the ticket and its parent spec, so that I judge whether the diff covers the ticket and stays inside the spec.
27. As a spec reviewer, I want to report work the ticket does not cover as a finding, so that scope creep is caught.
28. As a code reviewer, I want to judge the diff against the repo's standards and correctness, so that the two reviewers don't overlap.
29. As a reviewer, I want the output of the repo's check command in my review packet, so that I judge real test results rather than the worker's word.
30. As a reviewer, I want no bash and no edit tools, so that I can never change the worker's tree.
31. As a user, I want the check command defined in `.el/check` in the repo, so that it is explicit and not guessed.
32. As a user in a repo without `.el/check`, I want review to proceed and say "no checks configured", so that nothing breaks.
33. As a user, I want reviewer panes to stay visible while the ticket is open, so that I can watch the review conversation.
34. As a worker whose reviewer pane died, I want el to start a replacement reviewer with the full diff and a summary of the earlier findings, and tell me it did, so that review continues.
35. As the implement session, I want `finish_worker` to refuse a worker unless both reviewers passed at its current HEAD, and to say which reviewer is not satisfied and at which commit, so that unreviewed work never merges.
36. As the implement session, when `finish_worker` refuses, I want to `tell_worker` it to address the review and call `code_review` again, so that a worker that stopped early continues with its context and reviewers intact.
37. As a user, I want no cap on review rounds, so that review ends when the reviewers are satisfied, not when a counter runs out.
38. As el, I want `finish_worker` to close a worker's reviewers whether it merges or discards the worker, so that no reviewer outlives its ticket.
39. As a maintainer, I want `review_worker` and its review path deleted, so that there is one review path and no drift.
40. As a user, I want no el session to load global skills from `~/.agents`, `~/.claude` or the harness defaults, so that sessions see only the el library.
41. As a pig session, I want no `--skill` injections, only `el://` addresses, so that pig and claude behave the same.
42. As any session, I want `read` to open any absolute path, so that I can read briefs, state, logs and transcripts without falling back to `cat`.
43. As a user, I want `read` to refuse a short deny list of credential paths (ssh keys, harness auth files, `.env*`), and to say why, so that secrets don't end up in model context or transcripts.
44. As any session, I want `read` on a directory to return an `ls`-style listing one level deep (names, `/` on directories, sizes), so that exploring is one tool.
45. As any session, I want offset and limit to page through a large directory listing, so that huge directories stay readable.
46. As any session, I want `read` on a glob to return the matching paths, so that I can find files by pattern without bash.
47. As any session, I want a workspace directory and a `el://` directory to list the same way, so that read behaves consistently.
48. As any session, I want reading a `el://` skill directory to end with a hint pointing at its `SKILL.md`, so that I go straight to the skill.
49. As any session, I want `el://` files served as plain text with a short header (address and line count) and no line numbers or edit tag, so that I don't spend tokens on anchors I can never use.
50. As any session, I want offset and limit to still page through `el://` files, so that long skills stay readable.
51. As a claude session, I want read, find and scout marked `readOnlyHint`, so that the harness runs my parallel calls in parallel.
52. As el, I want `code_review` not marked read-only, so that the harness never treats a call that starts persistent reviewers as safe to batch.
53. As a session asking two scouts at once, I want both to finish in about the time of one, so that parallel exploration saves time.
54. As el, I want concurrent herdr pane splits serialized while the agents themselves run concurrently, so that parallel scouts and reviewers cannot race on the layout.
55. As a scout, I want to read outside the repo, so that questions about logs, state and transcripts can be answered.

## Implementation Decisions

- **Roles.** A bare session gets `plan` = read, edit, write, bash, find, background, scout (code_review removed). `implement` = read, find, spawn_worker, wait_workers, finish_worker, tell_worker, scout (review_worker removed, scout added). `work` keeps code_review. `review` and `scout` keep read and find. `full` stays reachable only through an explicit role flag on the launcher.
- **Implement launcher.** `el implement <spec>` takes a spec file. It creates or reuses `spec/<slug>` (slug from the spec's directory or file name) from the current HEAD, and records it as the base that workers branch from and `finish_worker` merges into.
- **Implement prompt.** The hand-written loop prompt is replaced by a pointer to `el://matt-pocock/implement-spec/SKILL.md`, the spec path, and a el adapter. The adapter says: implementer subagent = spawn_worker, merger subagent = finish_worker (it rebases and fast-forwards; no merger agent), exploration subagent = scout, the integration branch already exists, skip the final code-review step, and on a refused finish tell_worker the worker to continue its review. The adapter keeps the existing rule that a conflicting rebase goes back to the worker (ticket 25 of dogfood8) and that the implement session never edits.
- **Skills.** Skill text is never injected into any prompt. The pig harness drops the `--skill` loop and keeps `--no-skills`. Every session, on every harness, gets only `el://` addresses. The harness-dependent skill reference and the session's skill list go away. claude keeps its existing isolation (no setting sources, no built-in tools, slash commands disabled).
- **Worker brief.** It points at the ticket, the parent spec, and the `el://` addresses of `implement` and `tdd`. It is readable with `read` because read opens any path.
- **code_review interface.** Arguments: optional `reviewer` (`code` | `spec`) and optional `note`. The first call starts both reviewers. Later calls go to the reviewers named, or by default to every reviewer whose last verdict is `changes`. A reviewer whose pass is older than HEAD also gets the `pass..HEAD` diff to recheck. The reply has one section per reviewer that answered, and states each reviewer's verdict and the commit it applies to.
- **Reviewer state.** El keeps, per worker: each reviewer's pane, session, last reviewed commit, and last verdict with its commit. This lives in el's state next to the worker record.
- **Reviewer delivery.** The first prompt goes through a prompt file, as `ReadOnly` does today. Follow-ups are typed into the live pane the way tell_worker delivers, and the reply is read from the transcript. A pane that is gone is replaced by a fresh reviewer given the full diff and a summary of its predecessor's findings, and the `code_review` reply says so.
- **Review packet.** Per call: the full diff file (base..working tree, as today), the incremental diff since that reviewer's last review, the check output, the note, and pointers to the ticket and spec (spec reviewer) or to the repo's standards docs (code reviewer). Reviewers keep the `review` role: read and find only.
- **Verdicts.** The last line of each reply must be `VERDICT: pass` or `VERDICT: changes`. If it is missing, el nudges once, then records `changes`.
- **Checks.** Before each review, el runs the one shell command in `.el/check` (repo root) in the worker's tree, with a timeout inside the tool budget, and puts exit code and trimmed output into the packet. If the file is missing, the packet says "no checks configured". This repo gets `.el/check` running vet and tests with live tests skipped from `el/`. `.el/check` is on trial: the first dogfood run judges it.
- **finish_worker.** Merging requires both reviewers to have `pass` recorded at the worker's current HEAD. Otherwise it refuses without touching anything and names the unsatisfied reviewer and commit. Discarding (`merge=false`) is always allowed. Both paths close the worker's reviewers. There is no round cap.
- **Deletions.** The `review_worker` tool and `Review` are removed. The shared diff-review path stays and becomes the two-reviewer path.
- **read.**
  - Absolute paths anywhere are allowed. The `EL_READ_ALLOW` mechanism goes away.
  - A deny list in code refuses `~/.ssh`, the harness auth and credential files (pig agent auth, claude config and credentials), and `.env*` anywhere, with an error naming the reason.
  - Directories return a one-level listing with `/` on directories and sizes. offset and limit page through the entries.
  - Globs return the matching paths.
  - Workspace and `el://` directories share the listing format. A `el://` directory holding `SKILL.md` ends with a hint naming it.
  - `el://` files return plain text with a header of address and line count, no line numbers and no tag. offset and limit still apply.
  - edit and write stay confined to the workspace.
- **MCP annotations.** `tools/list` carries `annotations: {readOnlyHint: true}` for read, find and scout. code_review, bash, background and the orchestration tools get none.
- **Parallel scouts.** The server already runs calls concurrently. The herdr pane split inside the read-only launcher is serialized with a lock. Pane run, waiting and reply reading stay concurrent.

## Testing Decisions

- Good tests drive a public entry point and assert observable output: tool results, the exec calls made, files written, the tools a role serves. They never test internal helpers.
- **Fake Exec seam** (prior art: orchestrate_test, readonly_test, worker_test). Covers:
  - two reviewers start on the first call
  - default routing to reviewers with open findings, and the `reviewer=` override
  - the note reaches the reviewer
  - VERDICT parsing, plus nudge-then-`changes`
  - a stale pass gets the `pass..HEAD` diff
  - a dead pane is replaced with a summary
  - `.el/check` output, and "no checks configured", in the packet
  - finish_worker refuses without both passes at HEAD and names why, merges when they hold, and closes reviewers on merge and discard
  - concurrent ReadOnly calls serialize only the split
- **Roles and harness commands** (prior art: harness_test, readonly_test): the plan, implement and work tool sets; a bare launch gets plan; pig commands carry no `--skill`; every harness gets `el://` addresses only.
- **read tool Execute** (prior art: tools/files_test): any absolute path, deny-list refusals and their message, directory listing and paging, glob matches, the shared listing format, the skill-folder hint, plain `el://` files with header and paging.
- **MCP server over stdin/stdout** (prior art: the server's existing tests): `readOnlyHint` present exactly on read, find and scout; two slow fake calls finish in about one call's time.
- **Prompt text** (prior art: extension_test): the implement prompt names the implement-spec `el://` address, maps the subagents to spawn_worker, finish_worker and scout, skips the final review, and carries the refused-finish and conflict instructions. The worker brief names the ticket, the spec, and `el://` addresses for implement and tdd.
- Every test uses fakes and temp dirs. No live harness, no network, no real home.

## Out of Scope

- Auto-detecting check commands or parsing them from AGENTS.md.
- A cap on review rounds or a `needs-human` state.
- Review at the spec level (implement-spec's final code-review step).
- Ticket writing inside the implement session; `to-tickets` belongs to planning.
- Other skill namespaces and skill auto-update.
- Changes to how edit and write confine paths.

## Further Notes

- Evidence from the latest pig runs (dogfood8 workers 22–24): each worker's first `read` of its brief failed with "is outside the workspace" and fell back to `cat`, and skill files were read with `cat` from absolute paths.
- The likely reason scouts ran serially is the missing `readOnlyHint`. The parallel test confirms the fix.
- Dogfood this spec with `.el/check` in place and judge whether the check output earned its place in the review packet.
- The hard limits used by dogfood8 tickets (only edit `el/`, never install binaries, never run live tests or start real harnesses) apply to every ticket of this spec.
