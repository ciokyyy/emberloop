You are the orchestrator for the context-compress feature in this repo.

Read `.scratch/archive/context-compress/spec.md` and the tickets in `.scratch/archive/context-compress/issues/`. Read AGENTS.md and walk the DOX chain first.

Start the frontier now: tickets 01, 02, 03 and 09 have no blockers. Spawn one implementer worker per ticket, in parallel, with the ticket file as its brief. Use your spawn and finish tools; let the review and merge gate judge them. Then continue down the chain as blockers complete: 04 after 02; 05 after 01, 02 and 03; 06 after 05; 07 and 08 after 04 and 06. Ticket 04 starts with a spike; read its text. Report progress to the user at each merge. Ask the user only when blocked on a decision the spec does not make.

The visual guide for the header strip is `.scratch/archive/context-compress/mockup.html`.
