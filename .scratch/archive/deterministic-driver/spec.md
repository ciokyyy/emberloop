# Spec: a deterministic driver replaces the LLM orchestrator for `el implement`

Status: ready-for-agent

## Problem Statement

`el implement <spec>` starts an LLM orchestrator session (pig or Claude Code) whose whole job is a fixed loop: read the tickets, spawn a worker for each unblocked ticket, call `finish` when a worker is reviewed, send the refusal text back with `tell` when `finish` refuses, and report at the end. The prompt that drives it already states these rules.

An analysis of the archived sessions (22 orchestrator sessions, 137 `finish` calls) shows the LLM adds cost and failure modes, not judgment:

- 62 of 121 `finish` results were refusals. 44 of them were "not reviewed to a pass": the orchestrator called `finish` before both reviewers had passed at the worker's tip. A loop that calls `finish` only after the reviews pass never makes this call.
- The other refusals were mechanical: 5 uncommitted changes in the worker's worktree, 3 uncommitted changes in the integration checkout overlapping the incoming change, and 8 rebase conflicts. Each was resolved by sending the refusal text to the worker with `tell`, and the worker fixed it.
- All 3 `blocked` workers were blocked by reviewer spawn or verdict failures, not by a real decision.
- Orchestrators edited files they should not touch, parked finished workers, and sometimes needed the user to find out why nothing was moving.
- The user sits in front of an agent session to watch a run, when what they need is a view of ticket state.

## Solution

A deterministic Go driver runs the loop with no model. `el implement <spec> --driver` shows a live pane with one row per ticket (queued, running, in review, merging, needs you, parked, merged) and the agent rows below. The driver spawns workers in ticket order up to `max_workers`, calls `finish` only after both reviewers have passed at the worker's tip, and sends any refusal to the worker verbatim and tries again. It never gives up on its own. When a ticket makes no progress it tells the user in the pane and through a herdr notification, and keeps retrying with backoff.

The LLM orchestrator stays the default until the driver has merged a few real specs cleanly. Both modes exist side by side until then. After that the default flips and the LLM path stays behind `--llm`.

## User Stories

1. As a user, I want `el implement <spec> --driver` to run a whole spec without an LLM orchestrator, so that a run costs only worker and reviewer tokens.
2. As a user, I want the driver to start tickets in ticket-number order up to `max_workers`, so that the order I wrote is the order work starts.
3. As a user, I want a ticket to start only after every ticket it is blocked by has merged, so that workers branch from a tip that contains their dependencies.
4. As a ticket author, I want the `Blocked by` line to list ticket ids first, so that the driver can read the dependencies without guessing.
5. As a ticket author, I want free text after the ids on the `Blocked by` line to be ignored, so that I can still explain the reason to humans.
6. As a user, I want the driver to fail loudly with the file and line when a `Blocked by` line does not parse, so that a silent bad dependency never starts work too early.
7. As a user, I want a `Blocked by` id that names no ticket in the spec to be reported before any worker starts, so that a typo does not stall a run halfway.
8. As a user, I want a dependency cycle between tickets to be reported before any worker starts, so that I do not wait on a run that cannot finish.
9. As a user, I want the driver to call `finish` only after both reviewers have passed at the worker's current tip, so that no run produces a "not reviewed to a pass" refusal.
10. As a user, I want a worker that changes its branch after a pass to be treated as unreviewed again, so that a merge always matches what was reviewed.
11. As a user, I want a refused `finish` to send its exact text to the worker through `tell`, so that the worker sees git's own words.
12. As a user, I want the driver to call `finish` again after the worker's next submit, so that a fixed conflict merges without me.
13. As a user, I want no limit on how many times a ticket is retried, so that a hard ticket is not abandoned while it makes progress.
14. As a user, I want the same refusal three times in a row to mark the ticket "needs you" and send a herdr notification, so that I learn about a loop that is not converging.
15. As a user, I want the driver to keep retrying a "needs you" ticket with a growing delay, so that the alert does not stop a ticket that was only slow.
16. As a user, I want to press `p` on a ticket in the pane to park it, so that I can stop a ticket that will never converge without stopping the run.
17. As a user, I want a parked ticket to keep its worktree and branch, so that I can look at or resume the work myself.
18. As a user, I want tickets that depend on a parked ticket to wait, and every other ticket to keep running, so that one bad ticket does not stop the run.
19. As a user, I want a worker with no activity for 10 minutes to be marked "needs you", so that a hung worker is not invisible.
20. As a user, I want the driver to send a fixed nudge to a worker that has been idle for 3 minutes with no submit, so that a worker that forgot to review or submit continues.
21. As a user, I want the nudge to repeat every 5 minutes, and the third one to raise the alert, so that a stalled worker is escalated in a bounded time.
22. As a user, I want the driver to log the real reviewer spawn error found in a stalled worker's transcript, so that I see the cause and not just the stall.
23. As a user, I want a reviewer that fails to spawn to be retried by the worker's own spawn call, so that reviewer reliability is fixed where the reviewers are started.
24. As a user, I want a `discard` command that removes a worker and its worktree, so that I decide when to throw work away.
25. As a user, I want the driver never to discard a worker itself, so that no run destroys work without me.
26. As a user, I want worker follow-ups copied verbatim into the final report, so that nothing a worker noticed is lost.
27. As a user, I want a worker's `submit` to list the ticket checkboxes it completed, so that tickets show real progress.
28. As a user, I want the driver to be the only writer to ticket files during a run, so that two workers never race on a shared status line.
29. As a user, I want `finish` to keep marking a ticket done when it merges, so that ticket status matches the integration branch.
30. As a user, I want the driver to leave my uncommitted edits in the integration checkout alone, so that a run never loses my notes.
31. As a user, I want to start `el implement <spec> --driver` again after a crash or reboot and have it carry on, so that a run survives the machine.
32. As a user, I want a resumed run to find merged tickets, running workers and parked tickets from the tickets, worker records and git, so that the driver needs no private state file.
33. As a user, I want a resumed run not to spawn a second worker for a ticket that already has one, so that a restart does not duplicate work.
34. As a user, I want a pane with one row per ticket and its state, so that I see the whole run at a glance.
35. As a user, I want each ticket row to show its worker, its reviewers' verdicts at the current tip, and how long it has been in this state, so that I can spot a stuck ticket without opening panes.
36. As a user, I want the agent rows (harness, model, tokens, cost) below the ticket table, so that cost and load are visible in the same place.
37. As a user, I want to jump from a ticket row to its worker's herdr pane, so that I can read or steer the worker.
38. As a user, I want the pane to show the last refusal text for a ticket in "needs you", so that I can act without searching logs.
39. As a user, I want the final report to list every merged ticket with its sha, every ticket left unmerged with its last refusal, and the follow-ups, so that a run ends with a clear summary.
40. As a user, I want `el implement <spec>` to keep running the LLM orchestrator by default while the driver is unproven, so that nothing I rely on changes under me.
41. As a user, I want `--driver` and `--llm` to choose the mode explicitly, so that I can compare the two on the same spec.
42. As a user, I want the default to flip to the driver only after a few real specs merge cleanly through it, so that I decide when it is proven.
43. As a user, I want both modes to share the same worker, reviewer and `finish` behavior, so that a comparison tests the loop and nothing else.

## Implementation Decisions

- **Scope.** Only the autonomous `el implement <spec>` run. The interactive session where the user talks to an agent and spawns scouts or reviewers by hand is unchanged.
- **The driver is a loop over existing operations.** It uses the same spawn-a-ticket-worker, `tell` and `finish` operations the LLM orchestrator calls through tools. It adds no new way to start a worker or merge a ticket. The two modes differ only in what decides the next call.
- **Dependency reading.** The driver reads each ticket's `Blocked by` line. The ticket ids come first; everything after is free text for humans. A line that does not parse fails the run before any worker starts, naming the file and line. Unknown ids and cycles also fail before any worker starts.
- **Scheduling.** Tickets whose blockers have all merged are ready. When more tickets are ready than `max_workers` allows, the lowest ticket number goes first. A `priority` field is out of scope for the first version.
- **Finish gate.** The driver calls `finish` for a worker only when every required reviewer type has a pass at the worker's current tip, using the same review record `finish` itself checks. A change to the branch after a pass makes the worker unreviewed again.
- **Refusal handling.** A refusal from `finish` is sent to the worker unchanged through `tell`. The worker's next submit triggers another `finish`. There is no attempt limit.
- **Needs-you alert.** A ticket is "needs you" after the same refusal text three times in a row, or after 10 minutes with no activity from its worker. The driver marks the row, sends a herdr notification, and keeps retrying with a growing delay. Parking is the user's action, taken with `p` in the pane. A parked ticket keeps its worktree and branch and blocks only the tickets that depend on it.
- **Stalled worker.** The reviewers are spawned by the worker, so a failed reviewer spawn leaves the worker idle without a submit. After 3 minutes idle with no submit the driver sends a fixed nudge to spawn its reviewers and submit. The nudge repeats every 5 minutes and the third one raises the needs-you alert. The driver reads the real spawn error out of the worker's transcript and shows it on the row.
- **Reviewer reliability.** The worker still spawns its own reviewers. The reliability work is in the spawn path, not in the driver. The first fix is already in place: the late-prompt wait accepts both `idle` and `done`, because herdr reports an idle but unfocused pane as `done`. Other known failure families, such as a first-run dialog or a pane bumped to another tab, are fixed in the spawn path as they are found.
- **Ticket progress.** A worker's `submit` gains a field listing the ticket checkboxes it completed. The driver is the only writer to ticket files, applies the checkboxes in the integration checkout, and leaves the edit uncommitted like the orchestrator's own note edits. `finish` still marks the ticket done on merge. Generic `read`, `write` and `edit` stay unchanged. The pane and the report treat this as best effort: a run is correct without it.
- **Discards.** The driver never discards. A `discard` command removes a worker and its worktree on the user's request.
- **Follow-ups.** Worker follow-ups are collected verbatim into the final report.
- **Pane.** A new mode of the existing TUI program. The ticket table is on top: one row per ticket with state, worker, reviewer verdicts at the current tip, time in state, and, for "needs you", the last refusal text. The existing agents view sits below it. A row can jump to the worker's herdr pane. The driver loop and the pane share one in-memory run state; the pane reads it and sends actions (`p` park) back.
- **Restart.** The tickets, the worker records and git are the only state. Starting the driver again derives each ticket's state from them: merged, a worker running, parked, or ready. It never starts a second worker for a ticket that has one.
- **Modes.** `--driver` selects the driver and `--llm` the LLM orchestrator. The LLM orchestrator stays the default while the driver is unproven. Flipping the default is a separate, later decision by the user. The `orchestrator` role in settings and `orchestrator_paths` stay until the LLM mode is removed.
- **Shared behavior.** Both modes use the same worker, reviewer and `finish` behavior, including the same integration branch. A comparison of the two is a comparison of the loop.

## Testing Decisions

- **One seam.** The driver's loop is tested through a single entry point that takes the spec and an injected world: the ticket files, a fake of the operations the driver calls (start a worker, `tell`, `finish`, read reviews, notify), and a fake clock. Tests assert on what the driver calls and what the run state shows, never on internals. This follows the existing injected-exec pattern (`spawnExec`, `deliverExec`) used elsewhere in the harness.
- **What a good test checks.** The calls the driver makes and their order for a described world: which tickets start, when `finish` runs, what `tell` receives, when the alert fires. A test does not check how the loop is structured.
- **Scenarios.**
  - Ticket order and `max_workers` limit with blockers.
  - `finish` is never called before both reviewers pass at the tip, including after a post-pass change.
  - A refusal text reaches the worker unchanged, and `finish` runs again after its submit.
  - The same refusal three times raises the alert and retries continue.
  - The 3 and 5 minute nudges and the third-nudge alert, on the fake clock.
  - Parking blocks only dependents.
  - A bad or cyclic `Blocked by` fails before any worker starts.
  - Restart finds merged, running and parked tickets and starts no duplicate worker.
  - The final report lists merges with sha, unmerged tickets with their last refusal, and follow-ups.
- **Pane.** Tested through the existing pattern for the TUI program: the model is built from run state and the rendered rows and key handling are asserted. The pane is not tested against a live herdr.
- **Live check.** One scripted probe, kept beside the other live probes and never part of the automated check, runs the driver against a throwaway spec with a real herdr and reports the state of each ticket. It would have caught the idle/done wait bug, which no unit test could.
- **Prior art.** The injected exec in the spawn and delivery tests, and the TUI model tests for the agents view.

## Out of Scope

- The interactive session and ad hoc spawning of scouts or reviewers.
- Removing the LLM orchestrator, flipping the default, or deleting the `orchestrator` role. These wait until the driver is proven.
- A ticket `priority` field and any scheduling rule other than ticket-number order.
- Automatic discard, automatic parking and any retry limit.
- Having the driver spawn reviewers, or review work itself.
- Parsing tickets or specs inside the generic `read`, `write` and `edit` tools.
- Splitting a spec into tickets. Tickets already exist when the run starts.
- Resolving merge conflicts in the driver. The worker does that after `tell`.

## Further Notes

- The facts behind this spec come from the retrospective on the archived sessions: refusal counts by cause, the three `blocked` workers, and the orchestrator's tool use. The `finish` and `tell` loop it describes is already written as rules in the current orchestrator prompt.
- The seam above is proposed, not yet confirmed. The user should check that one entry point with an injected world is the right level before tickets are cut.
- Open question for tickets: the exact field name and shape for completed checkboxes in `submit`, and how a worker learns the checkbox identifiers (by line text or by index).
