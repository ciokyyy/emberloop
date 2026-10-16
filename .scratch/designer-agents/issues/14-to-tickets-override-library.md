# 14: Emberloop `to-tickets` override

**What to build:** Emberloop ships its own skill library whose `to-tickets` splits design from code and writes the `**Type:**` line. `el skills install` installs it beside the upstream `matt-pocock` library without being overwritten by upstream updates, and `el://matt-pocock/to-tickets` (and the Skill lookup agents use) resolves to the override when both exist. The override keeps the upstream behaviour and adds: a `code` Ticket delivering logic, API, data, tests and a plain-semantic-HTML UI that works; a `design` Ticket, blocked by it, owning layout, styling, visual states and copy, allowed to change markup but not logic; restyle-only work is one `design` Ticket; no-UI work is one `code` Ticket; a slice is complete only when both are done; the `**Type:**` line in its local ticket template; and Type shown for each Ticket in the quiz step. Re-read `el/skills.go` (single `skillsNamespace`, `InstallSkills`), the `el://` skill resolution in the read tool, `skills_test.go` and the upstream skill at `el://matt-pocock/to-tickets`.

**Blocked by:** 02-ticket-type-parse

**Status:** ready-for-agent

- [ ] The override library lives in the repo, embeds or copies into the skills root on `el skills install`, and survives a reinstall of upstream
- [ ] A lookup by the upstream name returns the override text; a skill that exists only upstream still resolves upstream
- [ ] The override's text contains the split rule, the Type line and the quiz Type column, and otherwise follows the upstream skill
- [ ] The override's template writes `**Blocked by:**` as comma-separated Ticket ids like `02-ticket-type-parse` (digits then slug, the form the Run driver parses), never `slug (NN)` or titles
- [ ] Tests cover install order, override precedence and an upstream-only skill; `el/AGENTS.md` records the two-library contract; `.emberloop/check` passes
