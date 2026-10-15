# 35: Agents page with one chain per role

**What to build:** One Agents page merges role models and Run Chains into one ordered chain per role. Every entry sets harness, model and thinking effort together; effort choices list only what the chosen model supports. Entry 1 is Primary, others Fallback 1 and 2, at most three, with reorder, remove and add. Roles group as Session agents (Orchestrator, Subagent, Scout) and Run agents (Ticket worker, Spec reviewer, Standards reviewer). Saved settings from before the redesign keep working through a migration (spec Parts 2 to 6, Settings).

**Blocked by:** 34-settings-hub

**Status:** done

- [x] One chain per role with harness, model and effort per entry, at most three entries
- [x] Effort choices per model come from the model's supported efforts
- [x] Old role and chain keys migrate to the merged form; no role loses its model
- [x] Go test migrates a pre-redesign settings fixture; vitest covers chain editing with effort per model
- [x] Repo check and `npm test` pass
