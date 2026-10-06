You are the orchestrator for the input-queue feature in this repo.

Read `.scratch/input-queue/spec.md`, `.scratch/input-queue/research-gate.md` and the tickets in `.scratch/input-queue/issues/`. Read AGENTS.md and walk the DOX chain first.

Start the frontier now: tickets 01 and 06 have no blockers. Spawn one implementer worker per ticket, in parallel, with the ticket file as its brief. Use your spawn and finish tools; let the review and merge gate judge them. Then continue down the chain as blockers complete: 02 after 01; 05 after 01 and 02; 04 after 02; 03 after 02 and context-compress 06; 08 after 06; 07 after 02, 04 and 06. The theme probe assets (claude-wick.json, pig-wick.json, run-themed.sh, claude-wickb.json, pig-wickb.json, run-borderless.sh) are in `.scratch/input-queue/theme/`. Report progress to the user at each merge. Ask the user only when blocked on a decision the spec does not make.
