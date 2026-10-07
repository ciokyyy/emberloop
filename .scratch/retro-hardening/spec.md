# Spec: retro hardening, review integrity, install safety, and DOX for this repo

Status: ready-for-agent

## Problem Statement

A retrospective on the run-records implement run found failures that every run will hit again:

- `finish` refused a merge because the integration checkout had uncommitted changes, and its error truncated the file list, so the orchestrator could not say why and sat blocked.
- The repo's check (`go vet`, `go test`) does not run gofmt, and one file has been unformatted since an early ticket. Two workers spent turns deciding whether that was theirs to fix.
- A emberloop MCP server started by hand with no role falls back to the full tool set. A worker that believed its `spawn` tool was missing used this to start reviewers outside its own role.
- Nothing records that a ticket is done: statuses stay `ready-for-agent` and checkboxes stay empty, because the orchestrator cannot edit files and workers cannot safely edit shared status lines.
- `edit` answers only "updated successfully", so a range edit that swallowed code went unnoticed until the worker re-read the whole file. Failed edits and vet loops cost whole turns.
- The repo has no `AGENTS.md`. Three workers each re-read the same 18 KB harness file in full, and all six read the whole spec; workers find where things live by reading, not by being told.

A second look at the live sessions' spawn calls found that **reviews were not judging what they claimed to**:

- The review diff is `git diff <base>`, which leaves out files git does not yet track. Workers that created new files (the archive and memory tickets) were reviewed on a diff holding only the tracked hunks. One reviewer's own report said "the supplied diff only contains the one hunk"; another said a new file "is untracked, so it is again absent". Those reviewers passed incomplete work.
- A pass on a dirty tree is stored as "the commit plus uncommitted", the same string for every dirty state. The memory worker rewrote its code after a first pass, called spawn again, and was told "every reviewer passed at <sha>; nothing to review" in 4 seconds, with nothing re-judged.
- A follow-up review is given the diff since the reviewer's last commit. When the worker's changes sat uncommitted or the branch moved, that diff was the whole cumulative change again (one reviewer noted it), so follow-ups were neither narrow nor cheap.
- Blocking spawns stopped waiting at about 4 minutes (213 to 247 seconds in every case) and answered "still running; end your turn". That is the intended hand-off, but it is undocumented and unmeasured, and it explains why some workers looked idle.
- Spawn calls whose entries omitted `type` were rejected by the tool schema in under a second; that is the schema working, and a model error, not a emberloop fault.

A third finding, reported by the user: **when two agents spawn at the same moment, one of the two spawned agents is thrown away.** Reading the code, spawn's shared state is only safe inside one process, and every session runs its own emberloop MCP process:

- Grid placement (which slot a new pane takes, and creating the overflow tab) is serialised by an in-process lock over an in-memory record. Two processes read the same pane list, choose the same slot, and split the same source pane; one pane then sits outside the grid's record, and a later replace or prune can close it. Two processes can also both create the same overflow tab.
- The worker store is read, changed and written back under an in-process lock only, and is written in place rather than renamed. A second process can overwrite an entry, and a reader can see a half-written file and treat a worker as unknown (which changes a review's base and tab).
- The reviewer record is keyed by worktree and type. Two spawns that both find no live reviewer for the same key each start one, and the second record overwrites the first, leaving the first pane running and unowned.
- The pause between launches, added because simultaneous first requests make the provider answer with empty bodies or a 503 (seen in three worker sessions), is kept per process, so simultaneous spawns from different sessions still collide.

The exact symptom has not been reproduced; the first ticket reproduces it before any fix.

A fourth finding came from the second retro, on the finished run-records run: **a pass is tied to content, so every rebase invalidates it, and the orchestrator ran a stale tool server.**

- Worker 05 needed six reviewer rounds; four were follow-ups after a rebase onto other tickets' merges, on its own unchanged work. A clean rebase moves the content around the worker's change but leaves the change itself identical.
- The orchestrator's emberloop MCP process is long-lived. After the gate was fixed and the binaries rebuilt, it kept the old gate and refused two workers twice each for a bug already fixed; both were merged from outside. Replacing a emberloop binary under a running harness is what causes this, and also changes tool behaviour mid-session for every other live agent.

## Delivered already (commit 81d72bd)

The review-integrity fixes below shipped early because they blocked the run: 6 of 9 `finish` calls failed on "passed at X, not at the tip Y". The cause was ordering. The worker brief tells a worker to commit after its review passes, a pass on uncommitted work was recorded as "commit plus uncommitted", and the gate wants a pass at the committed tip, so every worker that committed after review was refused and had to review again. Now reviewers judge a snapshot of the working state (a git tree id: tracked, modified and new files), the diff is taken against it, a pass stays valid when exactly that content is committed, "nothing to review" compares content, and follow-up diffs are tree to tree when the branch was not rebased. Stories 24, 26, 27 and 28 are done; what remains of that section is the file-count notice (25) and the wait bound (29, 30).

## Delivered already (commit 88715ea): lost results after a cancelled call

During the retro-hardening run, workers sat waiting for reviews that had already come. A blocking spawn waits for its agents inside the session's tool server, and when the harness cancels the call (any text typed into the pane, a user keystroke, a delivered message) the harness tells the server, but the server ignored the cancellation. The wait kept going; if the reviewer submitted before the deadline, the result went to a call nobody was reading, and the reviewer's own result path was switched off, so the worker never saw it. The server now ends the named call on a cancellation, a cancelled spawn or tell hands each blocking agent to the notice path (running ones are backgrounded, finished ones are delivered at once), and a reviewer spawned again at a state it already judged repeats its answer instead of being re-prompted. Sessions started before this fix keep the old server until restarted.

## Solution

Small mechanical fixes close each failure, and the repo adopts DOX (agent0ai/dox): a hierarchy of `AGENTS.md` files in which the root holds project-wide rules and an index, each durable folder holds its own contract, an agent reads the chain from the root to its target before editing, and updates the owning docs after meaningful changes. The DOX tree replaces the pointer file the retro proposed.

## User Stories

1. As an orchestrator, I want `finish` to name the exact files that block a merge, so that I can tell the user what to do instead of looping.
2. As a user, I want `finish` to refuse up front, with a clear message, when uncommitted changes in the integration checkout overlap the incoming change, so that a merge never dies halfway on a git error.
3. As a user, I want uncommitted changes that do not overlap the incoming change to leave the merge alone, so that notes and ticket edits do not block work.
4. As an orchestrator, I want git's own error text passed through whole, so that nothing is truncated that would explain a refusal.
5. As a user, I want the repo's check to fail on unformatted Go, so that formatting is a result and not a judgement call.
6. As a worker, I want the whole tree already gofmt-clean, so that a failing format check is always mine to fix.
7. As a user, I want a emberloop MCP server with no role to refuse to start, so that no agent can reach spawn, finish or tell by launching one by hand.
8. As a developer debugging emberloop, I want an explicit way to ask for the full tool set, so that the live scripts and manual debugging keep working.
9. As a worker, I want the spawn tool to appear in my brief by its real name, so that I never conclude it is missing. (Delivered; keep the brief wording.)
10. As a user, I want a ticket marked done by emberloop when its worker merges, so that status never depends on an agent remembering.
11. As a user, I want the ticket's checkboxes ticked at the same moment, so that a merged ticket reads as complete; the reviewers' pass is the check.
12. As a user, I want a worker that is not merged to leave its ticket untouched, so that status tells the truth about the integration branch.
13. As a user, I want the status change committed on the integration branch right after the merge, so that it never sits as uncommitted noise.
14. As a worker, I want `edit` to show me the lines it changed with a little surrounding context, so that I see a wrong range at once and do not re-read the file.
15. As a worker, I want `edit` to report a patch that deleted far more lines than it added, so that a swallowed block is flagged where it happens.
16. As an agent, I want an `AGENTS.md` at the root that states the repo-wide rules and indexes the child docs, so that I know where everything lives from one short read.
17. As an agent, I want a child `AGENTS.md` per durable folder naming its purpose, what it owns, its local contracts and its check, so that I read a few hundred words and not a 20 KB source file to find my bearings.
18. As an agent, I want to read the root-to-target chain before editing, so that I edit with the local rules in front of me.
19. As an agent, I want to update the owning `AGENTS.md` after a change that alters purpose, contracts or structure, so that the docs do not drift from the code.
20. As a reviewer, I want to check that the DOX pass happened, so that doc upkeep is enforced where it is cheapest, in review, not in the implementer's context.
21. As a claude worker, I want the same `AGENTS.md` chain, so that both harnesses get the same guidance.
22. As a user, I want no duplicated rules across `AGENTS.md` files, so that a rule changes in one place.
23. As a user, I want each doc's Verification section to name an existing check and nothing invented, so that no doc promises a gate that does not exist.
24. As a reviewer, I want the change I judge to include files that are new and not yet tracked, so that I never pass work I could not see.
25. As a reviewer, I want to be told when the diff I was given covers fewer files than the working tree holds, so that a gap is visible to me.
26. As a worker, I want "nothing to review" only when my working tree is the same state a reviewer already passed, so that changed code is always reviewed again.
27. As a worker, I want my second spawn of reviewers to give each one just what changed since it last looked, including uncommitted and new files, so that a follow-up is small and exact.
28. As a user, I want a pass recorded against an exact working state, so that a pass on one version of the code never covers another.
29. As a worker or orchestrator, I want spawn to return at once and never wait for the agents it starts, so that a cancelled or interrupted call can never strand a result.
30. As a worker, I want the results of one spawn call to arrive as a single notice once every agent of that call has submitted, so that I am interrupted once, with everything, and never mid-tool-call.
31. As a user, I want two agents that spawn at the same time to both get the agents they asked for, so that no spawned agent is closed, overwritten or orphaned.
32. As an orchestrator, I want two spawns from different sessions to place their panes in distinct grid slots, so that no pane displaces another.
33. As a user, I want only one overflow tab per label to ever exist, so that simultaneous spawns do not open duplicates.
34. As a user, I want the worker store to survive simultaneous writers and readers, so that no entry is lost and no reader sees a partial file.
35. As a worker, I want a second simultaneous spawn of the same reviewer type for my worktree to re-prompt the one already starting instead of starting another, so that there is one reviewer per type.
36. As a user, I want launches from separate sessions spaced apart too, so that simultaneous first requests do not make the provider answer with empty bodies.
37. As a maintainer, I want a concurrency test across real processes, so that this class of bug cannot return unseen.
38. As a worker, I want my reviewers' pass to survive a clean rebase onto other merged tickets, so that I am not sent back to review unchanged work.
39. As an orchestrator, I want a rebase that changes my worker's own change, or conflicts, to invalidate the pass, so that a real change is always reviewed.
40. As a user, I want installing or updating emberloop's binaries to refuse while a harness run has live agents, so that no running session changes behaviour under itself.
41. As a user, I want that refusal to name the live agents, so that I know what to wait for or stop.
42. As a user, I want an explicit force option for the rare case, with a warning that live sessions keep the old binary until they restart, so that I can still choose.
43. As a user, I want a emberloop process to say when it is older than the installed binary, so that a stale orchestrator is visible, not silent.
44. As a user, I want a cap on how many workers run at once, so that a wide frontier does not overload the model provider.

## Implementation Decisions

- `finish` checks the integration checkout's working tree before the merge. A dirty path that the incoming change also touches blocks the merge and is named, with the instruction to commit or stash it. Dirty paths elsewhere, including the orchestrator's allowed note and ticket paths, do not block. Whatever git itself reports is passed through whole.
- The repo's check command gains a formatting check, and the one unformatted file is formatted in the same change, so the check is green when it lands.
- A emberloop MCP server started with no role exits with a message naming the roles. The full role is still available when asked for explicitly, by role flag or environment value; the live scripts and the debugging path use it. The pig and claude harnesses always set a role, so launches are unaffected.
- Ticket completion moves into `finish`. After a successful merge, emberloop sets the ticket's status line to done, ticks its checkboxes, and commits that on the target branch. The worker entry gains the ticket path it was spawned from, which it already receives. A refused or conflicting merge leaves the ticket untouched. The orchestrator prompt stops asking the orchestrator to do this; the orchestrator's allowed-paths ability stays for specs and notes.
- `edit` returns the changed region with a few lines of context on each side, and flags a patch whose deleted line count greatly exceeds its added count. The patch format and tag semantics do not change.
- DOX is adopted with the framework's own contract: read the chain before editing, update the closest owning doc and affected parents and children after meaningful change, shape of child docs (purpose, ownership, local contracts, work guidance, verification, child index), concise and operational style. The root `AGENTS.md` is the framework text plus a project preamble and the child index; the tree is built by scanning the repo, not by hand-seeding.
- Child docs go at each durable boundary: the main Go package, its tools, its MCP server, its built-in agent types and its test data, the herdr report extension, the Antigravity provider extension, the live scripts and the spec and ticket area. The main package's doc carries the file map the retro asked for. Verification in every doc points at the repo's check or says nothing.
- Review enforces the DOX pass: both built-in reviewer types are told to flag a change that alters a documented contract or structure without updating the owning doc, and a stale index. The implementer is not asked to police it.
- Claude reads `CLAUDE.md`, not `AGENTS.md`. A one-line `CLAUDE.md` that imports the root `AGENTS.md` gives both harnesses one source of truth. A probe first confirms which context files each stripped harness loads, because the pig harness disables built-in tools and skills and the claude harness disables setting sources; if either does not load the file, the worker brief carries one line telling it to read the root `AGENTS.md`.
- The root document stays short: always-loaded text costs every turn, so detail lives in children and the root only indexes and states the contract.
- The review packet is built from a snapshot of the working state that holds tracked, modified and new files, taken without disturbing the worker's index. The snapshot's identity replaces "commit plus uncommitted" as what a pass is recorded against. A committed clean tree keeps using its commit, so the merge gate's "passed at the tip" rule is unchanged.
- A reviewer's follow-up diff is the difference between the snapshot it last judged and the current one, so it covers new and uncommitted files and shrinks to the real change. If the previous snapshot is gone (a rebase, a gc), the full diff is given and the prompt says so, as now.
- The reviewer prompt states how many files the diff covers and how many the working tree changed or added; a mismatch is flagged in the prompt, not left for the reviewer to notice.
- "Nothing to review" compares snapshots, not commits.
- Concurrency: every piece of spawn state shared between sessions is guarded by a cross-process lock and written atomically, using the lock helper emberloop already has for agent state and reviewers. That covers grid placement and overflow-tab creation (per tab), the worker store, and the check-then-launch of a reviewer (per worktree and type). The grid's slot record is read under the same lock, from a file, not process memory.
- The spacing between launches uses a shared timestamp under a lock, so it holds across sessions.
- The first ticket is a reproduction: concurrent spawns across real processes against a fake herdr, asserting every requested agent exists, owns a distinct pane, and appears in the stores. The fixes follow its failures.
- A pass is recorded against the patch identity of the worker's own change (its diff from its base, hashed so that it does not depend on where the branch sits), beside the content snapshot. After a clean rebase the identity is the same, so the pass holds; after a conflict, an edit or a different change it differs, so the pass is stale. The gate accepts a pass when either the content or the patch identity matches. This supersedes the rebase handling of follow-up diffs: when only the base moved, the follow-up is the worker's own change against the new base, never the other tickets' work.
- Emberloop binaries are installed through one command. It refuses while any agent of a run is live (worker, reviewer or scout panes known to the stores and the terminal multiplexer), names them, and exits without changing anything. A force option installs anyway and prints that running sessions keep the old binary. Nothing replaces a binary any other way.
- A emberloop MCP server records its own binary's identity at start and, on a call, notes in its reply when the installed binary has changed since. It does not restart itself: restarting mid-run would drop state, and the install gate is what prevents the situation.
- A setting caps concurrent workers (default three). Further workers from one spawn call start as earlier ones finish, and the call reports which are queued.
- Spawn never waits. It starts the agents and returns, saying that one notice with every result will arrive and that the caller should end its turn. This applies to every agent type, blocking or not; the blocking flag now only decides whether the notice carries a full report or a one-line summary. The wait loop, the wait bound and the 'still running' reply are removed.
- Results of one spawn call are a group. Each agent's submit is recorded as it comes; when the last agent of the group has submitted, one notice carrying all the results is delivered to the parent pane. An agent that cannot submit (pane gone) is reported in the group as lost, so the group always completes. A reviewer that is already judged at the exact state (held pass, same content) is reported in the first reply, not started, and is not part of the group.
- Delivery waits for the parent to be idle: the notice is held while the parent pane is working a tool call, because typed text cancels a running call, and delivered when the pane goes idle. A parent that never goes idle for a long time gets the notice anyway, with a warning logged.
- The briefs say once: after spawn, end your turn; the result arrives as a system notice.

## Testing Decisions

- Good tests assert behaviour through the tool or command, not internals.
- `finish`: a dirty overlapping file blocks the merge and is named; a dirty non-overlapping file does not; a successful merge marks the ticket done and ticks its boxes in a commit on the target; a refused merge leaves the ticket as it was. Prior art: the existing finish tests with a fake git.
- Role guard: a server with no role exits with an error naming the roles; an explicit full role serves everything. Prior art: the role tests.
- Check: a deliberately unformatted file fails the check command; the tree passes it.
- `edit`: the result includes the changed region and flags a block deletion. Prior art: the hashline tests.
- DOX: a repo test checks that every `AGENTS.md` is listed in its parent's child index and that every listed child exists, so the tree cannot rot silently. The probe of which context files each harness loads is a recorded finding, not a test.
- Review wording is covered by the existing reviewer-type tests.
- Review packet: a worktree with a new untracked file produces a diff that contains it; an unchanged dirty tree yields "nothing to review"; the same tree after another edit does not; a follow-up diff after an uncommitted edit contains only that edit; a rebase falls back to the full diff with a note. Prior art: the review tests with a fake herdr and a real temp git repo.
- Merge gate: a pass on a clean commit still opens the gate; a pass on a dirty snapshot still does not.
- Spawn: the reply returns at once even when agents are still running; a group of two reviewers delivers exactly one notice, after the second submit, carrying both results; a lost agent is reported in the group; a notice for a busy parent pane is held until the pane is idle. Prior art: the deliver and spawn tests with a fake herdr.
- Concurrency: several helper processes spawn scouts, workers and reviewers at once against a fake herdr that records splits and closes; the test asserts no close of a pane another spawn owns, distinct slots, one overflow tab per label, a complete worker store, and one reviewer per worktree and type. It must fail on the current code. Prior art: the fake herdr in the grid and review tests, and the cross-process file lock tests.
- Rebase: a pass survives a clean rebase onto an unrelated merged change and the gate accepts it; a conflicting or content-changing rebase makes it stale. Prior art: the existing rebase review test.
- Install gate: with a live agent in the stores and the fake multiplexer, install exits non-zero, names the agent and replaces nothing; with none it installs; force installs and prints the warning.
- Staleness: a server started before its binary changed says so in its next reply.
- Worker cap: a spawn call for more workers than the cap starts the cap and reports the rest as queued, then starts them as slots free.

## Out of Scope

- Anything about the `rtk` output wrapper or token-saving shell changes; planned separately.
- A `CODING_STANDARDS.md`: every miss found was mechanical and gets a check.
- Model-side reliability of a particular worker model beyond the brief wording already shipped.
- Automatic generation of docs after each commit; DOX relies on the agent's pass and the reviewers.
- Re-reviewing work already merged under the old diff behaviour. The two affected tickets (archive, memory) are still open and get reviewed correctly when they next spawn reviewers.
- Changing the reviewers' own judgement or prompts beyond the diff coverage notice.

## Further Notes

Source of the findings: the retro run on the run-records implement session, this session's transcripts, and a second pass over the live worker sessions' spawn calls (durations, results and reviewer reports). The brief wording for the spawn tool (story 9) and the spec reviewer's holding pass are already delivered and need no work here. DOX comes from https://github.com/agent0ai/dox.

## Hard limits for implementers

This work is built by the tool it changes, so:

- Edit only files under the main Go package directory and this spec's ticket files. Never touch the installed binaries directory, the home-directory configuration of pig, claude, agents or emberloop, or the live scripts directory.
- Never install a binary or copy one anywhere. Build and test from the package directory with the build and test commands, skipping live tests.
- Never run a live test. Never start emberloop, pig or claude inside the work. Tests use a fake exec and temp directories, never the network or the real home.
- Servers, watchers and tails go to the background tool, never bash.
- Keep vet and the test suite green at every commit.
