# Spec: one spawn tool with agent types, results are submitted, nobody waits

Status: ready-for-agent

## Problem Statement

When I run an orchestrator session, the parent spawns workers and then has to block on `wait_workers` to learn what happened. That tool is capped below the harness tool timeout, so the parent calls it again and again: every call costs tokens and a turn, and the tool definition itself costs tokens on every request. `tell_worker` blocks the same way. Detecting "the worker is done" depends on reading each harness's transcript and guessing from the end of a turn, so a worker that stops to think out loud, gets stuck, or simply ends its turn looks the same as one that finished. There is no reliable, compact result: the parent gets whatever the worker said last, however long.

On top of that, every kind of sub-agent is its own hardcoded tool: `spawn_worker`, `scout`, `code_review`, each with its own lifecycle written into el. Adding a new kind of agent (a security reviewer, a test writer, a docs agent) means writing a new tool. Spawning several agents of different kinds at once means several separate calls, which run in parallel only if the harness happens to batch them.

## Solution

There is one `spawn` tool. It takes a list of agents, each with a **type**, and starts them all in parallel. A type is a definition, not code: it declares the agent's tools, its checkout (its own worktree, none, or its parent's worktree), whether spawning it blocks, what statuses it may submit, and what it may spawn itself. el ships `worker`, `scout`, `reviewer-standards` and `reviewer-spec`, and a repo can add or override types with files. `tell` and `finish` complete the set.

Every spawned agent reports with a `submit` tool, with a status from its type. Non-blocking types submit a one-line summary; blocking types submit a full report. Each harness has a stop gate that refuses to let an agent stop until it has submitted, as hard as that harness allows. For a **blocking** type (scouts, reviewers), `spawn` returns once every agent in the call has submitted, so a parent can run three scouts in parallel and get all three answers in one result; if they outlast the tool budget, `spawn` returns what is in and the rest arrive as notices. For a **non-blocking** type (workers), `spawn` returns right away, and each submit is delivered straight into the parent's herdr pane as a system notice: the harness's steering takes it mid-turn if the parent is busy, or it starts a turn if the parent is idle. The parent never waits or polls: `wait_workers` is gone, and so is "call again".

## User Stories

1. As a parent agent, I want one `spawn` tool for every kind of sub-agent, so that my tool list is small and I learn one interface.
2. As a parent agent, I want to pass several agents, of mixed types, to one `spawn` call and have them all start in parallel, so that fanning out is one call.
3. As a parent agent, I want `spawn` of blocking types (scouts, reviewers) to return every agent's submit together, so that I get all the answers in one result.
4. As a parent agent, I want `spawn` of non-blocking types (workers) to return right away, so that I can end my turn and spend no tokens while they work.
5. As a parent agent, I want a call that mixes blocking and non-blocking types to return once the blocking ones have submitted, listing the non-blocking ones as started, so that a mixed call behaves predictably.
6. As a parent agent, I want each non-blocking agent's result to arrive in my session as a message on its own, so that I never poll.
7. As a parent agent that is busy, I want that result to reach me mid-turn through the harness's steering, so that I can use it without first going idle.
8. As a parent agent, I want a non-blocking agent's notice to be a one-line summary with a status, so that a typed notice never floods my context.
8a. As a parent agent, I want a blocking agent's result to be its full report (a scout's facts, a reviewer's findings), so that a spawn I wait on gives me everything I need.
8b. As a parent agent, I want a blocking spawn that outlasts the tool budget to return what has already submitted and deliver the rest as notices, so that I never call a tool again just to poll.
8c. As an agent with backgrounded children still running, I want to end my turn without submitting, so that I can wait for their notices.
9. As a parent agent, I want a worker's status to tell `done` from `blocked`, and a `blocked` to carry the reason, so that I know whether to merge or unblock it with `tell`.
10. As a parent agent, I want a long result to point at a commit or file instead of being pasted, so that I read details only when I need them.
11. As a parent agent, I want a notice clearly marked as a system notice (`<system-notice> … Resume your work using the result below.`), so that I don't mistake it for user input.
12. As a parent agent, I want `tell` to return as soon as the message is delivered, with the agent's next submit arriving as a notice (or, for a blocking type, being returned), so that follow-ups behave like spawning.
13. As a parent agent, I want `spawn`'s result to remind me to end my turn when it started non-blocking agents, so that I don't look for a wait tool that no longer exists.
14. As a parent agent, I want `finish` to merge or discard any agent whose type has its own worktree, so that finishing doesn't depend on the type's name.
15. As a parent agent, I want the tool description of `spawn` to list the types I may spawn, with their descriptions, so that I can pick one without guessing.
16. As a worker, I want to spawn the reviewer types my type allows, and have them review my worktree, so that my review loop is a normal spawn.
17. As a worker, I want re-spawning a reviewer type I already have to re-prompt that same reviewer, so that it keeps its context and only re-checks open findings.
18. As a worker, I want `finish` to require a `pass` at my branch tip from every reviewer type my type requires, so that the review gate still holds.
19. As a user, I want to add an agent type by dropping a definition file into the repo, so that new kinds of agents need no el code.
20. As a user, I want a repo type with the same name as a built-in one to override it, so that I can tune a built-in type per project.
21. As a user, I want an invalid type definition reported clearly at startup, naming the file and the problem, so that a typo doesn't fail silently.
22. As an agent, I want a `submit` tool whose `status` is limited to my type's statuses, with a one-line `summary` of at most ~300 characters if my type is non-blocking or a free `report` if it is blocking, so that reporting back is one explicit act.
23. As an agent, I want `submit` to reject a wrong status, a missing field, or an over-long or multi-line summary, so that I learn the limit right away.
24. As an agent, I want to stay open after I submit, so that the parent can `tell` me more and I keep my context.
25. As an agent on pig, I want to be kept working when I try to settle without submitting, and have my last chance pin `tool_choice` to `submit`, so that I can't silently stop.
26. As an agent on claude, I want a stop without a submit to be blocked with a reason, so that I can't silently stop.
27. As an agent, I want the gate's reminder to say "every turn ends with a tool call; keep working unless you are genuinely done or blocked; never invent a blocker", so that the reminder doesn't push me into submitting early.
28. As an agent whose turn ended in a provider error, I want the gate not to block me, so that it doesn't keep re-prompting into the same error.
29. As a parent agent, I want an agent that still hasn't submitted after 3 blocks to be released and reported to me as "stopped without submitting" with an excerpt of its last message, so that I always hear something.
30. As a user, I want two agents submitting at the same moment not to type over each other in the parent's pane, so that both notices arrive intact.
31. As a user, I want a notice for a parent pane that no longer exists to be dropped, so that nothing piles up for sessions I've closed.
32. As a user, I want `implement` to always run in a herdr pane, so that its parent can receive notices.
33. As a maintainer, I want one stop-gate decision used by every harness, so that the block/remind/give-up rules live in one place.
34. As a maintainer, I want a new harness to plug in a gate and get the same rules, so that adding a CLI stays one type plus one register call.
35. As a parent agent, I want re-spawning a ticket whose tab is still open to start the implementor in that tab's real root pane, after stopping whatever still runs there, so that the new implementor never lands in a reviewer's pane or gets typed into the old implementor's input.
36. As a parent agent, I want re-spawning a ticket whose finished branch was kept to work, so that a ticket can be picked up again after finish.
37. As an agent, I want panes opened by the `background` tool (dev servers, watchers) placed by the same grid as everything else, so that they sit side by side instead of stacking.

## Implementation Decisions

- **Agent types.** A type is a markdown file with frontmatter, modelled on omp's agent definitions. Frontmatter fields:
  - `name` and `description`
  - `tools`: the MCP tools the agent gets, replacing the role table for spawned agents
  - `checkout`: `worktree` (its own git worktree and branch, as workers have today), `none` (runs in the repo, read-only tools only), or `parent` (runs in the spawning agent's worktree)
  - `blocking`: true or false
  - `statuses`: the values its `submit` accepts
  - `spawns`: the types it may spawn
  - `persistent`: whether re-spawning the same type for the same parent reuses the existing agent
  - `requires_review`: the reviewer types whose `pass` gates `finish`
  - optional `harness` and `model` defaults

  The body is the agent's brief or prompt template.
- **Built-in types** are embedded in el:
  - `worker`: `worktree`, non-blocking, statuses `done`/`blocked`, spawns both reviewer types, requires `pass` from both. It replaces `spawn_worker`, and its brief is today's worker brief.
  - `scout`: `none`, blocking, read-only tools, status `answered`. It replaces the `scout` tool.
  - `reviewer-standards` and `reviewer-spec`: `parent`, blocking, persistent, statuses `pass`/`changes`. They replace `code_review`'s two axes.
- **Repo types** live in `.el/agents/*.md`. They add new types and override built-ins by `name`. They are loaded and validated at MCP server start, and an invalid file is an error naming the file and field.
- **Who may spawn what.** A top-level session's role says which types it may spawn (`implement`: worker and scout; `plan`: scout). A spawned agent may spawn its type's `spawns`. `spawn` rejects anything else.
- **`spawn` tool.** Its argument is a list of `{type, prompt, …}`. For a `worktree` type, the entry also takes the type-specific fields `spawn_worker` takes today (ticket id, base, target, harness, model). Every agent in the list is started in parallel, with launches spaced as today. Blocking agents are awaited together, and their reports are returned in one result in list order. Non-blocking agents are returned as "started" at once. The tool description lists the types the caller may spawn, with their descriptions.
- **Auto-background** (omp's auto-background). A blocking `spawn` or `tell` waits at most the tool budget (`ToolBudget`, 4 minutes today). When the budget runs out, it returns the reports already in, and says the rest are still running, their results will arrive as notices, and the caller should end its turn. From then on those agents' submits are delivered like non-blocking ones (the report, flattened where the delivery path needs it). This replaces the "still running, call the same tool again" answer everywhere.
- **Reviewers through `spawn`.** Spawning a reviewer type from a worker runs today's review machinery: repo checks first, the review packet, follow-ups that cover only the open findings, re-checking a stale pass, and replacing a dead reviewer. It sits behind the type, so behaviour is unchanged and only the entry point moves. Persistent reviewers are re-prompted, not re-spawned. Their `submit` (`pass`/`changes` + report, one line per finding as today) replaces verdict parsing, and the report is kept as the reviewer's findings for briefing a replacement.
- **Review gate.** `finish` on an agent requires a `pass` submit at the current branch tip from every type in its type's `requires_review`.
- **`tell` and `finish`** replace `tell_worker` and `finish_worker`. They work for any agent id. `tell` delivers through the nudge path and returns right away. For a blocking type, `tell` waits for the next submit and returns it. `finish` works only for `worktree` agents and keeps today's merge and discard behaviour.
- **Removed tools.** `spawn_worker`, `scout`, `code_review`, `wait_workers`, `tell_worker` and `finish_worker`.
- **`submit` tool** (every spawned agent). Arguments: `status` from the type's `statuses`, plus `summary` (one line of at most ~300 characters) for a non-blocking type, or `report` (free text, no length limit) for a blocking type. Anything else is rejected with a clear error. The limit exists because non-blocking notices are typed into a pane; blocking reports go back as a tool result and are never typed unless they were auto-backgrounded. A submit is recorded in the agent's state as its last submit and as "submitted this turn".
- **Delivery.** A blocking agent's submit is returned to the spawning call and never typed anywhere. A non-blocking agent's submit is delivered right away into the parent's herdr pane through the existing nudge path (`herdr agent prompt`, falling back to `send-text` + Enter flattened to one line). The harness's steering handles a busy parent; this was checked by hand on claude and pig. A per-parent-pane lock serializes deliveries. When the parent pane no longer exists, the notice is discarded. There is no queue, no batching and no delivery loop.
- **Parent pane.** An agent learns its parent's pane at spawn: the spawning session's `HERDR_PANE_ID` is recorded with the agent and passed to it.
- **Notice format** (from omp's async-result notice): `<system-notice> <type> <id> submitted (<status>). Resume your work using the result below. ── <id> ── <summary></system-notice>`, flattened to one line where the delivery path needs it.
- **Stop gate, one decision.** A single module decides, at the moment an agent tries to stop: was a submit recorded this turn, how many times this turn has already been blocked, and did the turn end in a provider error. It returns one of:
  - **allow:** a submit was recorded, the turn ended in a provider error, or the agent has auto-backgrounded children whose results have not arrived yet (it is waiting for their notices, as omp treats a pending async wake)
  - **block** with the reminder
  - **block and pin `submit`:** 3rd attempt, only where the harness can pin
  - **give up:** after 3 blocks, allow the stop and send the parent `<system-notice> <type> <id> stopped without submitting: <excerpt of last message>`, returned or delivered as that agent's submit would be

  The attempt count resets when a new turn starts, for example after a `tell` follow-up or a delivered notice. On claude, the `Stop` hook input's `stop_hook_active` tells the two apart: false means a fresh turn.
- **Reminder wording** follows omp's yield reminder. It says the last turn had no submit (reminder n of 3) and that every turn ends with a tool call. It then gives the choices, first applicable first: keep working (the default, not an order to stop); submit a final status only if the work is genuinely complete; submit `blocked` only for a real, nameable blocker. It ends with: never invent a blocker, and never end a turn with text only.
- **claude gate.** The claude launch passes a `Stop` hook inline through `--settings` (it still applies with `--setting-sources ""`). The hook runs a el subcommand that reads the hook JSON on stdin, asks the decision, and writes `{"decision":"block","reason":<reminder>}` or nothing. claude cannot pin a tool, so its 3rd block just says it is the last one.
- **pig gate.** The pig agent's extension handles `agent_before_settle`, continuing with the reminder instead of settling. On the 3rd attempt it also handles `before_provider_request` to pin `tool_choice` to `submit`. It asks the same decision.
- **Harness interface.** A harness provides a gate. pig and claude are the only harnesses and both get one. What to do about a harness that cannot have a gate is decided when such a harness is added.
- **Parent guidance.** The `spawn` and `tell` results, the orchestrator prompts and the skills el adapts say to end the turn while non-blocking agents run, because results arrive as messages.
- **implement.** `implement` always runs in a herdr pane, and the headless parent path is removed.
- **Follow-ups from the ticket-tabs run.** These are folded in here, as one ticket:
  - **Respawn pane.** The implementor's pane is found by its grid slot (slot 1, the tab's root), not by taking the first pane herdr lists for the tab. Before relaunching, whatever still runs in that pane is stopped, so the launch line is never typed into a live agent.
  - **Respawn after finish.** When `implement/<id>` already exists (finish keeps the branch), spawn reuses it with `git worktree add <path> <branch>`, without `-b`, provided it is still at or behind the requested base. Otherwise spawn fails with a clear error that names the branch.
  - **`background` panes.** The `background` tool places its pane through the grid into the caller's tab, with the same `scouts` overflow, instead of splitting `--current --direction down`.
  - **Slot 1 of an overflow tab.** A dead reviewer whose slot is 1 in a fresh overflow tab is not rebuilt in place; the fallback to a free slot stays. Two reviewers never reach it.

## Testing Decisions

- A good test drives a public entry point (an MCP tool through the server, a harness command, the stop-gate subcommand, the pig extension's event handlers, type loading) and asserts on what an outsider sees: herdr and git commands issued, tool results, hook output, extension return values, load errors. It never asserts on internal helpers.
- **Types.** The built-in types load. A repo file adds a type, and a repo file with a built-in's name overrides it. A file with a missing or invalid field fails loading, naming the file and field. `tools/list` describes only the types the caller may spawn, and `spawn` rejects any other type.
- **`spawn`, through the `Exec` seam.**
  - Several entries launch in parallel.
  - Blocking entries return their submits together, in list order.
  - Non-blocking entries return "started" at once.
  - A mixed call returns once the blocking ones have submitted.
  - With a shrunk budget, a blocking spawn returns the reports already in plus a "still running, results arrive as notices, end your turn" line, and a later submit is delivered as a notice.
  - A `worktree` type gets a worktree and branch.
  - A `parent` type runs in the spawning agent's worktree.
  - Re-spawning a persistent type re-prompts the same pane.

  Prior art: `TestSpawnWaitFinish`, `TestCodeReviewStartsTwoReviewers`, `TestCodeReviewFollowUpGoesToOpenFindingsOnly`, `TestConcurrentReadOnlyLaunchesSerializeOnlyTheSplit`.
- **Review gate.** `finish` refuses until every required reviewer type has submitted `pass` at the tip, and refuses again after a new commit. Prior art: `TestFinishNeedsBothReviewersToPassAtTheTip`, `TestCodeReviewRechecksAStalePass`, `TestCodeReviewReplacesADeadReviewer`. The existing review tests move to the reviewer types and keep their assertions.
- **Delivery and `tell`, through the `Exec` seam.**
  - A non-blocking submit issues `herdr agent prompt` to the parent pane with the notice, and falls back to `send-text` + Enter on one line.
  - Concurrent submits to one parent never overlap.
  - A missing parent pane drops the notice without error.
  - A blocking submit is never typed into any pane.
  - An invalid status, a missing `summary` or `report`, or an over-long or multi-line summary, is rejected. A multi-line `report` from a blocking type is accepted and returned intact.
  - `tell` to a non-blocking agent returns without any `agent wait`.
  - `tools/list` has none of the removed tools for any role.

  Prior art: `TestTellWorkerTypesIntoAPigPane` and the MCP server tests over stdin/stdout.
- **Gate wiring, through the harness command seam.** The claude command carries `--settings` with a `Stop` hook that runs the stop-gate subcommand. The pig command loads the gate extension. Prior art: `TestClaudeRunsStripped`, `TestPigWorkerGetsNoSkillsAndTheReporter`.
- **Stop-gate decision, new seam.** The subcommand is fed claude `Stop` hook JSON on stdin:
  - It writes nothing when a submit is recorded this turn.
  - It blocks with reminder 1, 2 and 3 on successive stops.
  - On the 4th stop it writes nothing and sends the "stopped without submitting" notice.
  - It writes nothing when the turn ended in a provider error, or when the agent has auto-backgrounded children still running.
  - It starts counting again on a new turn (`stop_hook_active` false).

  The pig extension's `agent_before_settle` and `before_provider_request` handlers, called directly, follow the same sequence, with `tool_choice` pinned only on the 3rd attempt.
- **Removed tests.** The `wait_workers` cap tests, the `tell_worker` settle and wait tests, and the verdict parsing tests go with the behaviour they test.
- **Follow-ups.**
  - Respawning into an open ticket tab whose pane list returns a reviewer first launches into the slot-1 pane, and stops the old process there first.
  - Respawning when `implement/<id>` exists runs `git worktree add` without `-b` on that branch. A branch ahead of the base fails with an error naming it.
  - `background` start places its pane through the grid and never issues `--direction down`.

## Out of Scope

- A model proxy, or any other way to force harnesses that have no hook.
- Patched or modded claude builds.
- Deciding what happens with a harness that cannot have a gate (deferred until one is added).
- Keeping or replaying notices for a parent that is gone.
- Batching several non-blocking submits into one notice.
- Questions from an agent to its parent as a status.
- Changing what the review machinery does: checks, packet, follow-ups, stale-pass and dead-reviewer handling only move behind the reviewer types.
- New built-in types beyond worker, scout and the two reviewers.
- Per-type tab or layout rules beyond what ticket-tabs defines (worktree agents get the ticket tab, others the grid).

## Further Notes

- Prior art: oh-my-pi's subagents. Its single `task` tool with markdown agent definitions (frontmatter `name`, `description`, `tools`, `spawns`, `model`, `blocking`; it ships scout, reviewer, security-reviewer and task) is the model for agent types. Its `yield` tool, its reminder ladder (3 reminders, then `tool_choice` pinned to `yield`, skipped after a provider error, then a `SYSTEM WARNING: Subagent exited without calling yield tool after 3 reminders` fallback) and its `<system-notice>` async-result format are the models for `submit`, the gate and the notice. omp delivers in-process at step boundaries, while el relies on herdr delivery and harness steering.
- Defaults chosen without a grilling round, open to override:
  1. Statuses are defined per type, and the review gate reads reviewer submits.
  2. Reviewers and scouts are blocking. A worker only waits idle on its reviewers when their spawn was auto-backgrounded, and the stop gate allows that.
  3. Types are built in, and repo files in `.el/agents/` override them.
  4. Top-level roles still exist and decide what a session may spawn.
- Decided in the re-review: the summary limit applies only to non-blocking types (blocking types submit a full report); blocking spawns auto-background past the budget instead of asking to be called again; every current harness gets a gate, and harnesses without one are decided when they arrive.
- This changes the same areas as the ticket-tabs spec (worker spawn, orchestration tools, harness launch, reviewers). Start it only after ticket-tabs has merged and been installed.
- It is built by the tool it changes: the run that builds it uses the installed binary, so it keeps the old tools until it is installed.
