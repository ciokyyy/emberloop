# Run upgrade

One spec, from the web-v1 retro. It changes the Run driver in `emberloop-web` (Reviewer reports, dispositions, injection, fallback chains, pause and switch, spec branches), adds repo hygiene guards, and improves exploration tools in `emberloop`.

## Problem Statement

A retro of the web-v1 Runs (41 commits, 11 Tickets, about 20 agent sessions) showed these losses.

1. A Reviewer that finds only minor issues answers `changes`, so the Worker loops on nits. One standards Reviewer gave 114 `changes` verdicts on one Ticket and repeated one nit about 90 times on an unchanged tip.
2. A Reviewer reports through free text that the Daemon parses. A malformed reply costs a re-prompt. Reviewers pass with "I did not run the check or the tests".
3. Workers and Reviewers spend calls reading the spec, the Ticket and `AGENTS.md` chains by hand. In worktrees the spec directory is missing and there is no git identity.
4. A Worker or Reviewer that hits a model quota dies silently. Two Tickets stalled with no review, and the driver wrote hand-made worker prompts that left uncommitted work behind.
5. A User cannot pause a Ticket or a Run, switch its Backend or model, and resume.
6. Every Run merges Tickets straight into master. Two specs cannot run at once, and parallel Workers collide on `AGENTS.md`, `web/dist` and lockfiles.
7. Tests use `t.TempDir()` despite the rule, producing `TempDir RemoveAll cleanup: directory not empty` flakes. A Ticket is marked `done` without its branch merged. A red base blocks every Ticket's check.
8. Agents explore with about 100 grep/ls bashes per Ticket. `find` mixes jev and grep but does not use the code graph.

## Solution

Reviewers report through a `submit` tool with a typed schema. A `pass` may carry minor findings; minors do not block. The Worker answers each finding with a Disposition, and a declined finding is not raised again unless its lines change. The driver injects the Ticket, spec, `AGENTS.md` chain and role skills into the first prompt of every Worker and Reviewer, and prepares each worktree. Each role has an ordered Chain of Backend and model pairs; a quota, credit, auth or session-limit failure moves to the next pair with a Handoff. A User can pause a Ticket or the Run, change the Backend and model, and resume by hand (a Switch). Each Run works on its own `spec/<slug>` integration branch, merged into master once, through the repo check. Guards catch `t.TempDir()`, a red base and a premature `done`. `find` gains the code graph as a third source, and `execute` is prompted as the exploration tool.

## User Stories

1. As a User, I want Reviewer minors to reach the Worker without blocking the Ticket, so that nits do not cause review loops.
2. As a Worker, I want each finding listed with severity, file and line, so that I act without parsing prose.
3. As a Worker, I want to mark each finding `fixed`, `declined` with a reason, or `deferred` to a named Ticket, so that the Reviewer sees my decision.
4. As a Reviewer, I want a declined finding to stay closed until the cited lines change, so that I do not repeat myself.
5. As a User, I want a verdict that names the commands it ran, so that a `pass` without evidence is refused.
6. As a User, I want a verdict tied to a tip SHA, so that a stale verdict never counts.
7. As a User, I want a re-review skipped when the tip is unchanged or the diff against base is patch-identical, so that rebases do not burn review turns.
8. As a Reviewer, I want a `submit` tool with a schema, so that my output needs no format re-prompt.
9. As a Reviewer, I want re-reviews to be new messages in my existing Session, so that I keep my context.
10. As a Worker, I want the Ticket, spec, route `AGENTS.md` chain and my role's skills in the first prompt, so that I spend no calls finding them.
11. As a Reviewer, I want the spec and Ticket in my first prompt, so that I judge against the stated criteria.
12. As a User, I want injected skills trimmed per role, so that the first prompt stays small.
13. As a User, I want a git identity and a readable spec directory in every worktree, so that Workers hit no `stat` errors or aborted rebases.
14. As a User, I want an ordered Chain of one to three Backend and model pairs per role, so that a quota failure does not stop the Run.
15. As a User, I want the Chain to advance only on quota, credit, auth and session-limit errors, so that other failures still surface.
16. As a User, I want a Chain advance to resume from the committed branch with a Handoff, so that no work is lost.
17. As a User, I want a limit-capped Session to stop receiving "you have not submitted" nudges, so that the Chain handles the cap.
18. As a User, I want the Ticket to show its active Chain entry, so that I can see a fallback happened.
19. As a User, I want to pause one Ticket, so that its Worker stops with work committed.
20. As a User, I want to pause the whole Run, so that every live Worker and Reviewer stops cleanly.
21. As a User, I want to change the Backend and model of a paused Ticket or Run, so that I can leave an exhausted account.
22. As a User, I want to resume a paused Ticket or Run by hand, so that nothing restarts without my action.
23. As a User, I want a Switch to keep the Ticket branch and carry a Handoff, so that commits are not duplicated and context survives.
24. As a User, I want a `done` submit refused when no commit followed the last `changes` verdict, so that empty resubmits stop.
25. As a User, I want a `done` submit whose text says the work is not done refused, so that a ticked Ticket is complete.
26. As a User, I want a second `blocked` submit with the same question to become Needs-you, so that a Worker does not loop.
27. As a User, I want a missing verdict, for example from a Reviewer that hit a limit, to count as no review, so that a dead seat is never coverage.
28. As a User, I want each Run to have its own `spec/<slug>` branch, so that master is untouched until the spec is done.
29. As a User, I want to run two specs at once, so that independent work does not wait.
30. As a User, I want each Ticket to branch from and merge into its spec branch, so that Tickets in one spec see each other's work.
31. As a User, I want the final merge into master gated by the repo check on the merge result, so that master never goes red.
32. As a User, I want the spec branch rebased onto master on my action or at final merge, so that conflicts surface at a cheap point.
33. As a User, I want a warning when two live specs dirty the same file, so that I know a conflict is coming.
34. As a User, I want `web/dist` and lockfiles rebuilt only at the final merge, so that Tickets never conflict on them.
35. As a User, I want a conflict at final merge to become Needs-you with the files listed, so that I can resolve it.
36. As a User, I want a Run to show its spec branch and its distance from master.
37. As a User, I want to abandon a spec branch and leave no worktrees or branches behind.
38. As a User, I want a Ticket marked `done` only after its merge into the spec branch, and a spec marked done only after the master merge, so that status matches code.
39. As a User, I want a Ticket refused at start when the check is red on its base, so that a red base blocks early.
40. As a developer, I want a test that fails on `t.TempDir()` in `emberloop-web` tests, so that the cleanup flake cannot return.
41. As an agent, I want `find` to return graph hits (symbols, callers) beside jev and grep hits, each tagged with its source, so that one call replaces many greps.
42. As an agent, I want the `execute` and `find` guidance to prefer them over repeated bash grep, so that exploration costs fewer calls.
43. As a developer, I want `emberloop-web/AGENTS.md` pruned of duplication, so that every session loads less.

## Implementation Decisions

- **Reviewer `submit` tool.** The Reviewer Session gets a `submit` tool with `verdict` (`pass` or `changes`), `tip` (full SHA), `ran` (commands with exit codes) and `findings` (`{file, line, severity, problem}`). Severities stay `blocker`, `major`, `minor`. `ParseReview` and the format re-prompt remain only as fallback for a Backend without the tool, and are removed once every Backend has it.
- **Verdict rules.** `changes` requires at least one `blocker` or `major`. A `pass` may carry minors. A `pass` with empty `ran` is refused with a tool error. A verdict whose `tip` is not the current branch tip is recorded stale and does not count.
- **Minors reach the Worker.** The driver sends the Worker every finding of a recorded verdict, minors included, marked blocking or non-blocking.
- **Dispositions.** The Worker's `submit` carries one Disposition per received finding: `fixed`, `declined` with a reason, or `deferred` with a Ticket number. A submit missing a Disposition for a blocker or major is refused. Minor Dispositions are optional and recorded when present.
- **Declined findings.** The driver sends the Reviewer the declined findings with reasons on each later pass. The Reviewer prompt says not to re-raise one unless the cited lines changed. v1 does not check this mechanically.
- **Re-review.** Reviewer Sessions stay persistent per Ticket (as `ensureReviewers` does now); a re-review is a new message to the same Session. If the tip equals the last verdict's tip, or `git patch-id` of the diff against base is unchanged, the driver reuses the verdict and sends nothing. When one seat passed at tip T1 and the other said `changes`, the passed seat keeps its pass at tip T2 without a rerun when the T1..T2 diff touches no file the `changes` verdict cited, changes at most 40 lines (added plus removed), and stays inside the base..T1 diff scope; otherwise it is re-prompted. A carried pass is recorded at T2 noted `carried from T1`. The final merge still needs both seats fresh at the final tip or carried. A fallback or Switch of a Reviewer is the only case that starts a new Reviewer Session, with a Handoff.
- **Missing verdicts.** A Reviewer turn that ended without a recorded verdict at the current tip counts as no verdict; the Ticket is not mergeable until one exists.
- **Injection.** The driver builds each Worker and Reviewer first prompt from: the Ticket file, the spec, the `AGENTS.md` chain for the changed paths (root chain when unknown) and the role skills. The skill list per role lives in Run settings; each entry is skill text with sections marked optional dropped. The existing standards and spec briefs are the first entries.
- **Worktree prerequisites.** Worktree creation sets local `user.name` and `user.email` and makes the spec directory readable (read-only) from the worktree. Only the driver ticks boxes.
- **Chain.** Run settings hold, per role (Worker, spec Reviewer, standards Reviewer), an ordered list of up to three `{backend, model}` entries. The next entry becomes active on a failure classified as quota, credit, auth or session limit, reusing the Compress block classifier. It starts a new Session on the same Ticket branch with a Handoff, then normal injection. The failed Session ends `failed` and keeps its Event log. The Chain position is stored on the Ticket row. Chain exhaustion becomes Needs-you.
- **Capped Sessions.** A Session classified as limit-capped is excluded from the submit gate reminder loop.
- **Pause and Switch.** The Run API gains pause and resume for a Ticket and for the Run, and a change of Backend and model on a paused Ticket or Run. Pause stops the Worker at a safe point: the Worker commits, or the driver makes a WIP commit labelled as such, then the Session ends. Resume starts a new Session with a Handoff on the same branch. Resume is always manual. Switch and Chain advance share one code path.
- **Worker gate refusals.** The gate refuses `done` with no new commit since the last `changes` verdict, and refuses `done` when the text matches a conservative "not done" pattern. A `done` with no commits ahead of the Ticket base, or with uncommitted changes in the worktree, is refused telling the Worker to commit; a `done` on an already-merged Ticket stays an idempotent accept. A second `blocked` submit with the same question hash becomes Needs-you.
- **Spec branches.** The spec branch is `spec/<spec-slug>`, created at Run start from the master tip if absent. Ticket branches keep `implement/<NN-slug>` names and fork from the spec branch. Per-Ticket finish merges into the spec branch under today's gate. When every Ticket is merged or discarded the driver merges the spec branch into master; it runs the repo check on the merge result first. A failing check or conflict makes the Run Needs-you and master does not move.
- **Checkpoint rebase.** A User action rebases the spec branch onto master. The driver also rebases at final merge when master moved. A rebase conflict is Needs-you with files listed.
- **Generated files.** Per-Ticket bundle builds are not run by the driver. The final-merge step builds `web/dist` and lockfiles and commits them on the spec branch before the check.
- **Overlap warning.** At each Ticket merge, the driver compares the spec branch's changed files with other live spec branches' and publishes a warning Event on both Runs when the sets intersect.
- **Persistence and abandon.** The spec branch name and base SHA live on the Run row and survive restart. An abandon action, after confirmation, discards the spec branch, Ticket branches and worktrees.
- **Mark-done ordering.** A Ticket's `done` status and ticked boxes are written only after its merge into the spec branch. The spec's overall status is written only after the master merge.
- **Base-red gate.** Before a Ticket starts, the driver runs the repo check on its base (the spec branch tip) once per tip and caches the result. A red base marks the Ticket Needs-you with the failing tests listed, instead of starting a Worker that cannot go green.
- **Tip check.** Before starting Reviewers on a tip, the driver runs the repo check on that tip once per tip and caches the result. A red tip returns to the Worker with the failing output and spends no Reviewer turn; a green tip is passed to Reviewer prompts as evidence. An unverifiable check fails open and Reviewers still start.
- **Scope check.** The Worker and Reviewer first prompts say to keep the diff to the Ticket's stated scope. The driver diffs the Ticket branch against its base and adds a non-blocking warning to Reviewer prompts listing changed files the ticket file never mentions (neither the path, a parent directory, nor the basename). From round 4 on, the Reviewer prompt says a `major` that only restates a prior-round topic does not block (only `blocker` blocks); the gate still counts majors, so this is prompt-only and reviewers downgrade repeats to `minor`.
- **Single-spec compatibility.** A spec with no other live Run behaves as today from the User's view; the visible change is one extra merge at the end.
- **TempDir guard.** A Go test in `emberloop-web` scans its test files and fails on `t.TempDir()` (comments excluded). The five existing call sites convert to `tempDir(t)`. The `cmd/emberloop-web` copy uses an exported helper in a shared test-support package.
- **`find` graph source.** `find` adds the `codebase-memory` index as a third source beside jev and grep when the index exists for the project, and degrades to the two current sources when it does not. Each hit carries a `source` tag (`jev`, `grep`, `graph`). Ranking merges sources with the existing score fusion; graph hits that match a symbol name rank first.
- **Exploration guidance.** The tool descriptions and the Worker and Reviewer prompts say: use `find` for locating code and `execute` for multi-step exploration (loops over `grep`/`read`, filtering output) before repeated bash grep. A measurement script replays web-v1 exploration patterns and counts calls with and without the guidance.
- **Docs.** The glossary gains **Disposition**, **Chain** and **Switch**. The root `AGENTS.md` child index lists `.scratch/run-upgrade/AGENTS.md`. `emberloop-web/AGENTS.md` is pruned of duplicated seam text (the "at the same seam" list), restated owner/auth rules and code-describing paragraphs, keeping ownership, the seam list and verification.

## Testing Decisions

- A good test drives the Daemon HTTP API with the fake Backend and fake git, and asserts external behavior (Ticket status, recorded verdicts, prompts sent, branch graph), not internals.
- Seams: (1) the Run seam, `runs_driver_test.go`, `runs_reviewers_test.go` and `runs_persist_test.go`, for everything in the driver; the fake Backend gains scripted failures (quota, auth, session limit) and a Reviewer `submit` script, and the fake git models two branch levels and a moving master. (2) The `find` tool tests in `emberloop/tools/find_test.go`, with a stub graph source. (3) One scan test for the TempDir guard.
- Cases: pass with minors does not block and reaches the Worker; Dispositions required and recorded; declined findings sent back; empty `ran` refused; stale tip ignored; patch-id and same-tip short-circuit; re-review stays in the same Session; missing verdict blocks merge; injection per role; worktree identity and spec dir; Chain on each error class and none on others; exhaustion; capped Session gets no nudge; pause, Switch and resume for Ticket and Run; empty `done`, "not done" text and repeated `blocked`; uncommitted-work `done` refused; red tip returns to the Worker with no Reviewer turn and green evidence reaches Reviewers; selective carry skips, cited-file and large-diff reruns; scope warning never blocks; round-4+ prompt note; Ticket merges into spec branch; gated final merge; red check and conflict keep master; two Runs on disjoint specs finish independently; overlap warning; generated files only at final merge; restart resume; abandon; mark-done ordering; base-red gate; `find` source tags and graph absent degrade.
- Prior art: Reviewer rework and staleness tests in `runs_reviewers_test.go`; Compress block tests in `compress_retry_test.go`; dirty and conflict refusals in `runs_driver_test.go`; `find_test.go`.
- Real Backends are proven in live probes only, never the main check.

## Out of Scope

- Automatic resume after a pause.
- Mechanical detection of a re-raised declined finding.
- Cross-spec Ticket dependencies and automatic conflict resolution.
- The terminal harness Run loop in `emberloop/`.
- A graph index builder: `find` only reads an existing `codebase-memory` index.

## Further Notes

- Evidence: reviewer loop `e19e687f`; empty `ran` caveats in tickets 01, 02, 03, 04, 05, 08, 13; quota stalls in 15 and 33; manual prompts `08-33` to `08-57`; `2520366` and `74b3539` (integration divergence); TempDir flake in about 8 sessions.
- Fallback and Switch share one mechanism: new Session, same branch, Handoff.
- This spec runs on the current driver, so Tickets still merge to master until ticket 09 lands.
