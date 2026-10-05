# Spec: retro hardening, and DOX for this repo

Status: ready-for-agent

## Problem Statement

A retrospective on the run-records implement run found failures that every run will hit again:

- `finish` refused a merge because the integration checkout had uncommitted changes, and its error truncated the file list, so the orchestrator could not say why and sat blocked.
- The repo's check (`go vet`, `go test`) does not run gofmt, and one file has been unformatted since an early ticket. Two workers spent turns deciding whether that was theirs to fix.
- A wick MCP server started by hand with no role falls back to the full tool set. A worker that believed its `spawn` tool was missing used this to start reviewers outside its own role.
- Nothing records that a ticket is done: statuses stay `ready-for-agent` and checkboxes stay empty, because the orchestrator cannot edit files and workers cannot safely edit shared status lines.
- `edit` answers only "updated successfully", so a range edit that swallowed code went unnoticed until the worker re-read the whole file. Failed edits and vet loops cost whole turns.
- The repo has no `AGENTS.md`. Three workers each re-read the same 18 KB harness file in full, and all six read the whole spec; workers find where things live by reading, not by being told.

## Solution

Small mechanical fixes close each failure, and the repo adopts DOX (agent0ai/dox): a hierarchy of `AGENTS.md` files in which the root holds project-wide rules and an index, each durable folder holds its own contract, an agent reads the chain from the root to its target before editing, and updates the owning docs after meaningful changes. The DOX tree replaces the pointer file the retro proposed.

## User Stories

1. As an orchestrator, I want `finish` to name the exact files that block a merge, so that I can tell the user what to do instead of looping.
2. As a user, I want `finish` to refuse up front, with a clear message, when uncommitted changes in the integration checkout overlap the incoming change, so that a merge never dies halfway on a git error.
3. As a user, I want uncommitted changes that do not overlap the incoming change to leave the merge alone, so that notes and ticket edits do not block work.
4. As an orchestrator, I want git's own error text passed through whole, so that nothing is truncated that would explain a refusal.
5. As a user, I want the repo's check to fail on unformatted Go, so that formatting is a result and not a judgement call.
6. As a worker, I want the whole tree already gofmt-clean, so that a failing format check is always mine to fix.
7. As a user, I want a wick MCP server with no role to refuse to start, so that no agent can reach spawn, finish or tell by launching one by hand.
8. As a developer debugging wick, I want an explicit way to ask for the full tool set, so that the live scripts and manual debugging keep working.
9. As a worker, I want the spawn tool to appear in my brief by its real name, so that I never conclude it is missing. (Delivered; keep the brief wording.)
10. As a user, I want a ticket marked done by wick when its worker merges, so that status never depends on an agent remembering.
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

## Implementation Decisions

- `finish` checks the integration checkout's working tree before the merge. A dirty path that the incoming change also touches blocks the merge and is named, with the instruction to commit or stash it. Dirty paths elsewhere, including the orchestrator's allowed note and ticket paths, do not block. Whatever git itself reports is passed through whole.
- The repo's check command gains a formatting check, and the one unformatted file is formatted in the same change, so the check is green when it lands.
- A wick MCP server started with no role exits with a message naming the roles. The full role is still available when asked for explicitly, by role flag or environment value; the live scripts and the debugging path use it. The pig and claude harnesses always set a role, so launches are unaffected.
- Ticket completion moves into `finish`. After a successful merge, wick sets the ticket's status line to done, ticks its checkboxes, and commits that on the target branch. The worker entry gains the ticket path it was spawned from, which it already receives. A refused or conflicting merge leaves the ticket untouched. The orchestrator prompt stops asking the orchestrator to do this; the orchestrator's allowed-paths ability stays for specs and notes.
- `edit` returns the changed region with a few lines of context on each side, and flags a patch whose deleted line count greatly exceeds its added count. The patch format and tag semantics do not change.
- DOX is adopted with the framework's own contract: read the chain before editing, update the closest owning doc and affected parents and children after meaningful change, shape of child docs (purpose, ownership, local contracts, work guidance, verification, child index), concise and operational style. The root `AGENTS.md` is the framework text plus a project preamble and the child index; the tree is built by scanning the repo, not by hand-seeding.
- Child docs go at each durable boundary: the main Go package, its tools, its MCP server, its built-in agent types and its test data, the herdr report extension, the Antigravity provider extension, the live scripts and the spec and ticket area. The main package's doc carries the file map the retro asked for. Verification in every doc points at the repo's check or says nothing.
- Review enforces the DOX pass: both built-in reviewer types are told to flag a change that alters a documented contract or structure without updating the owning doc, and a stale index. The implementer is not asked to police it.
- Claude reads `CLAUDE.md`, not `AGENTS.md`. A one-line `CLAUDE.md` that imports the root `AGENTS.md` gives both harnesses one source of truth. A probe first confirms which context files each stripped harness loads, because the pig harness disables built-in tools and skills and the claude harness disables setting sources; if either does not load the file, the worker brief carries one line telling it to read the root `AGENTS.md`.
- The root document stays short: always-loaded text costs every turn, so detail lives in children and the root only indexes and states the contract.

## Testing Decisions

- Good tests assert behaviour through the tool or command, not internals.
- `finish`: a dirty overlapping file blocks the merge and is named; a dirty non-overlapping file does not; a successful merge marks the ticket done and ticks its boxes in a commit on the target; a refused merge leaves the ticket as it was. Prior art: the existing finish tests with a fake git.
- Role guard: a server with no role exits with an error naming the roles; an explicit full role serves everything. Prior art: the role tests.
- Check: a deliberately unformatted file fails the check command; the tree passes it.
- `edit`: the result includes the changed region and flags a block deletion. Prior art: the hashline tests.
- DOX: a repo test checks that every `AGENTS.md` is listed in its parent's child index and that every listed child exists, so the tree cannot rot silently. The probe of which context files each harness loads is a recorded finding, not a test.
- Review wording is covered by the existing reviewer-type tests.

## Out of Scope

- Anything about the `rtk` output wrapper or token-saving shell changes; planned separately.
- A `CODING_STANDARDS.md`: every miss found was mechanical and gets a check.
- Model-side reliability of a particular worker model beyond the brief wording already shipped.
- Automatic generation of docs after each commit; DOX relies on the agent's pass and the reviewers.

## Further Notes

Source of the findings: the retro run on the run-records implement session and this session's transcripts. The brief wording for the spawn tool (story 9) and the spec reviewer's holding pass are already delivered and need no work here. DOX comes from https://github.com/agent0ai/dox.
