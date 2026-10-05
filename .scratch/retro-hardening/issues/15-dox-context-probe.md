# 15: Know which context files each harness loads

**What to build:** A recorded probe answers which context files the pig harness (built-in tools and skills off) and the claude harness (setting sources off) actually load into a worker's context, and whether either already reads the root AGENTS.md. A one-line CLAUDE.md that imports the root AGENTS.md is added so claude reads the same source. If a harness does not load it, the worker brief carries one line telling the worker to read the root AGENTS.md first. The finding is written into this ticket.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The ticket records, for each harness, which of AGENTS.md and CLAUDE.md load, with evidence
- [ ] A CLAUDE.md importing the root AGENTS.md exists once the root file does
- [ ] The worker brief carries the read-the-root-doc line only where a harness needs it (brief test)
- [ ] Spec story 21

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
