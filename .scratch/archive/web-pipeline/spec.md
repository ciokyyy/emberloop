# Web pipeline: the deterministic implementation Run in the browser UI

## Problem Statement

Today the deterministic implementation pipeline (spec to tickets to workers to reviewers to merge) runs only through the el CLI inside a herdr pane. A user who works in the browser UI cannot start it, watch it, or answer it when it needs them. They must leave the UI, open a terminal and herdr, and read pane output. The Daemon already owns Sessions, Subagents, the Event log and live updates, but it has no notion of a Run, a Ticket, a Reviewer verdict or a merge.

## Solution

The browser UI gains a Runs view. The user points it at a repo, the Daemon lists the specs it finds there, and the user picks one. The UI previews the Tickets with their blockers and checkbox progress. The user starts the Run. The Daemon then runs the same deterministic driver the CLI runs: one Worker Session per Ticket in its own worktree, two persistent Reviewer Sessions per Ticket, a gate that refuses to merge unless both reviewers passed at the current tip, then a fast-forward merge. The Runs view shows every Ticket live. When a Ticket needs the user, the view shows a banner with buttons. A Daemon restart leaves the Run paused until the user resumes it. This is a 1:1 port of the CLI behaviour. Visual design comes later.

## User Stories

1. As a user, I want a Runs view next to chat and settings, so that I can run the pipeline without leaving the browser.
2. As a user, I want to enter a repo directory, so that the Daemon knows where to look for specs.
3. As a user, I want the repo directory to default to the working directory of the selected Session, so that I type less.
4. As a user, I want the Daemon to list the specs it finds under the repo directory, so that I do not have to remember paths.
5. As a user, I want each listed spec to show its title and ticket count, so that I can tell them apart.
6. As a user, I want a clear message when no spec is found, so that I know the directory is wrong and not that the UI is broken.
7. As a user, I want to pick a spec and see its Tickets, so that I can check the plan before I start.
8. As a user, I want each Ticket preview to show number, title, blockers and checkbox progress, so that I see what will run and in what order.
9. As a user, I want a clear error when a Ticket has an unparsable Blocked by line, so that I fix the file before the Run starts.
10. As a user, I want to start a Run with one click, so that the pipeline begins.
11. As a user, I want the Run to start Tickets in number order once their blockers have merged, so that the result matches the CLI.
12. As a user, I want a Worker Session for each Ticket in its own worktree and branch, so that parallel Tickets never touch each other.
13. As a user, I want to open any Worker Session from the Runs view and read its normal transcript, so that I can see what it did.
14. As a user, I want two Reviewer Sessions per Ticket, one for the spec and one for the repository standards, so that both checks happen.
15. As a user, I want Reviewers to keep their context across passes, so that a rework loop does not start from zero.
16. As a user, I want the Reviewer findings sent back to the same Worker, so that the Worker fixes them.
17. As a user, I want the rework loop to repeat until both Reviewers pass, so that nothing weak merges.
18. As a user, I want the Ticket row to show each Reviewer verdict and whether it is at the current tip, so that I can see why a Ticket has not merged.
19. As a user, I want the merge to refuse unless both required Reviewers passed at the current tip, so that a stale pass never counts.
20. As a user, I want the merge to rebase onto the target and fast-forward only, so that history stays linear.
21. As a user, I want a dirty worktree, a missing pass or a rebase conflict to keep the Worker alive instead of merging, so that nothing is lost.
22. As a user, I want the last refusal reason shown on the Ticket row, so that I know what blocked the merge.
23. As a user, I want the Ticket checkboxes ticked in the ticket file as the Worker reports them, so that progress survives outside the UI.
24. As a user, I want the Runs view to update live without refreshing, so that I can watch progress.
25. As a user, I want a banner when a Ticket needs me, so that I notice without watching the table.
26. As a user, I want a Ticket to need me after the same refusal three times or ten minutes of inactivity, so that I am not paged for noise.
27. As a user, I want a Ticket to need me after the third stall nudge, so that a stuck Worker reaches me.
28. As a user, I want to steer a Worker from the Runs view, so that I can redirect it.
29. As a user, I want to retry a Ticket, so that a refused merge is tried again after I fix the cause.
30. As a user, I want to park a Ticket, so that it never starts or finishes and its slot frees.
31. As a user, I want to discard a Ticket, so that the Run leaves it alone.
32. As a user, I want the other Tickets to continue while one needs me, so that one problem does not stop the Run.
33. As a user, I want an idle Worker nudged on a fixed schedule, so that a silent Worker does not stall forever.
34. As a user, I want a Run to survive a Daemon restart as a paused Run, so that I lose no state.
35. As a user, I want to click Resume on a paused Run, so that nothing spends tokens without my say.
36. As a user, I want a resumed Run never to start a second Worker for a Ticket that already has one, so that work is not duplicated.
37. As a user, I want to edit max_workers in settings, so that I choose how many Sessions run at once.
38. As a user, I want a change to max_workers to take effect on the live Daemon without a restart, so that I can adjust mid-Run.
39. As a user, I want the settings field locked with a note when the environment variable sets max_workers, so that I understand why my edit does nothing.
40. As a user, I want Run Sessions to count against the same cap as chat Sessions, so that the machine is never overloaded.
41. As a user, I want Workers and Reviewers to use the harness, model and effort from my settings roles, so that I configure them once.
42. As a user, I want a missing role to fall back to the Session default, so that a Run starts with partial settings.
43. As a user, I want only one active Run per spec, so that two Runs never fight over the same Tickets.
44. As a user, I want Runs for different specs to run side by side within the cap, so that I can work on two features.
45. As a user, I want a finished Run to stay listed with its final Ticket states, so that I can review the outcome.
46. As a user, I want a Run to need no herdr, so that the browser is enough.

## Implementation Decisions

- The Daemon imports the core module and reuses its deterministic driver, ticket parsing and git operations. It does not rewrite them. Core never imports web, so the dependency points one way.
- Web supplies its own driver world, backed by Sessions and git. It imports nothing from herdr. If core hides a piece the world needs, core exports the minimum. Where core assumes a herdr pane, core accepts the pane being absent.
- Specs and Tickets stay files. Ticket files are the source of truth for blockers, status and checkboxes. The UI never edits them in this spec. The driver is the only writer of checkbox progress, as in the CLI.
- Spec discovery is new. The Daemon scans a repo directory, to a small fixed depth, for spec files and for issue directories beside them. It returns a list. Choosing one goes through core's spec resolution and ticket reading, which also produce the preview.
- A Worker and each Reviewer is a Session. A Worker is a Subagent of the Run. Each uses the existing agent type prompts. The Runs view links each to the normal Session view.
- Worktrees live under the repo in the worktree directory, branch per Ticket, as in the CLI. The merge is a rebase onto the target followed by a fast-forward-only merge. The gate refuses unless every required Reviewer passed at the current tip.
- Session cap: Run Sessions, Reviewers included, count against the existing live-Session cap. The cap becomes a setting. The settings endpoint gains max_workers for read and write. A write changes the live cap with no restart. The environment variable still wins, and the read response says so, so the UI can lock the field.
- Roles: each agent type reads its harness, model and effort from the existing settings roles. A missing role falls back to the Session default. No other settings change.
- Needs-you: the CLI notification becomes a Run event and a UI banner. The triggers stay as in core: same refusal three times, ten minutes of inactivity, third stall nudge. Actions per Ticket: steer, retry, park, discard. Each action maps to an existing core behaviour or Session operation.
- Run state: one row per Ticket (state, Worker Session, Reviewer verdicts at tip, last refusal, times), persisted as a file per Run beside the existing group files. It is not stored in the core state database. On Daemon start, saved Runs load as paused. Resume continues the driver and never starts a second Worker for a Ticket that has one.
- Orchestrator: only the deterministic driver. No LLM orchestrator. Ticket creation stays out of scope.
- API contract (names are proposals): list and preview specs for a repo directory; create a Run from a spec; list Runs; read one Run; resume, pause; per-Ticket steer, retry, park, discard; Run updates over the existing server-sent events style.
- UI: a runs view beside chat and settings on the existing view switch, no router. Run list, Ticket table, needs-you banner, picker with preview. Plain markup. Design is deferred.
- One active Run per spec. Several Runs for different specs may run at once within the cap.
- Glossary gains Run, Ticket, Reviewer, Pass and Needs-you.

## Testing Decisions

- A good test drives external behaviour only: HTTP calls in, JSON and events out, or a mounted view against a mocked API. No test reads private driver state.
- Seam 1, the highest: the Daemon HTTP API with a fake Backend and a fake git. It covers discovery, preview, start, ordering by blockers, Worker and Reviewer Sessions, the gate, rework, needs-you, actions, pause and resume, and the cap and settings behaviour.
- Seam 2: the UI test runner mounting the Runs view against a mocked API. It covers the picker, preview, table, banner and buttons.
- No new seam in core. Core keeps its own driver tests.
- Prior art: the Daemon API tests that use the fake Backend, the lifecycle tests for the cap, the spawn tests for Subagents and Groups, and the existing UI component tests.
- Live probes with real agents stay out of the main check.

## Out of Scope

- Visual design of the Runs view.
- Creating or editing specs and Tickets in the UI.
- The LLM orchestrator and its flags.
- Herdr integration and the compress action.
- Automatic resume after a restart.
- Auth, remote access, or anything beyond loopback.
- Sharing run state with the core state database.

## Further Notes

- The CLI path stays supported and unchanged. The web Run and the CLI Run use the same driver, so behaviour must match.
- The spec area follows the repo convention: this file plus one file per Ticket in the issues directory.
