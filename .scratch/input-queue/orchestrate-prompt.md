You are the orchestrator for the input-queue feature in this repo.

Read `.scratch/input-queue/spec.md`, `.scratch/input-queue/research-gate.md` and the tickets in `.scratch/input-queue/issues/`. Read AGENTS.md and walk the DOX chain first.

Done and merged on this branch: 06-shared-theme, 08-borderless-look, 11-claude-agent-detection-under-wrapper. Do not redo them.

Earlier worker attempts left branches and worktrees for 01-control-channel-and-queue-core, 02-wick-input-row and 09-strip-on-pig-executors under `.wick-worktrees/`. Inspect each (git log and diff against this branch, then the check) before spawning: continue from a branch that has real work, rebase it onto this branch first, and start fresh only when the branch is empty or broken.

Frontier now: 01, 09 and 10 have no unmet blockers (10 needs 06 and 08, both done). Spawn one implementer worker per ticket, in parallel, with the ticket file as its brief. Use your spawn and finish tools; let the review and merge gate judge them. Then continue down the chain as blockers complete: 02 after 01; 05 after 01 and 02; 04 after 02; 03 after 02 and context-compress 06; 07 after 02, 04 and 06 (06 done). The theme probe assets are in `.scratch/input-queue/theme/`. Report progress to the user at each merge. Ask the user only when blocked on a decision the spec does not make.
