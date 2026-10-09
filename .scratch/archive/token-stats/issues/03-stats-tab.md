# 03: Stats tab with scopes, top items and raw drill-down

**What to build:** A Stats tab in the TUI with a scope selector (global, repo, spec, ticket, agent; opening from a selected spec or ticket pre-sets it) showing: tokens by piece type, per-tool table, the 50 most expensive single items, breakdowns by role, model, model plus effort, and harness. Any row opens the raw content (full prompt, arguments or result) read from the archive. Estimates are visibly marked.

**Blocked by:** 02, el-tui 03

**Status:** ready-for-agent

- [ ] Every table row drillable to raw content
- [ ] Exact vs estimated visibly distinct
- [ ] Model tests: switching scope and opening a row
