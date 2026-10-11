# 04: Prompt injection

**What to build:** The driver builds each Worker and Reviewer first prompt from the Ticket, the spec, the `AGENTS.md` chain for the changed paths (root chain when unknown) and the role's trimmed skills, listed in Run settings.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Per-role skill list in Run settings; optional sections dropped
- [x] Worker and Reviewer first prompts carry Ticket, spec, AGENTS.md chain and skills
- [x] Existing standards and spec briefs become the first skill entries
- [x] Run seam tests assert prompt content per role; `.emberloop/check` passes
