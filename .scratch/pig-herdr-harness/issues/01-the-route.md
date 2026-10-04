# What is Matt Pocock's implementation route, mechanically?

Type: research
Status: resolved
Blocked by:

## Question

The destination automates "Matt Pocock's whole implementation route". Before anything can be automated we need the route written down precisely: the ordered steps, the HITL/AFK boundary, the spec artifact's real shape, and the worker contract.

## Answer

**Most of the destination already exists as skills. The harness is `implement-spec`. What we are building is a runtime for it, not a new orchestration layer.**

### The route (`ask-matt/SKILL.md`, main flow)

`setup-matt-pocock-skills` once, then:

1. `wayfinder` — only when the effort is foggy or spans sessions. Rejoins at `to-spec`, never straight to implement.
2. `grill-with-docs` (= `grilling` + `domain-modeling`) — directory idea → sharpened decisions, `CONTEXT.md`, ADRs.
3. `research` (parallel detour) and `prototype` (optional detour) — background agent and throwaway artifact respectively.
4. `to-spec` — thread + codebase → spec issue, one checkpoint: confirm test seams.
5. `to-tickets` — spec → tracer-bullet vertical slices with **blocking edges**, published in dependency order.
6. **`implement-spec`** — whole-spec fan-out (the destination's core).
7. `implement` — per-ticket build, `tdd` inside.
8. `code-review` — two-axis parallel Standards/Spec review.
9. `resolving-merge-conflicts` — standalone mid-conflict.

Off-route: `triage` (on-ramp for issues you did not author), `retro`, `handoff`/`claude-handoff`, `to-questionnaire`, and the non-route skills `wait-what`/`zoom-out`/`teach`/`writing-for-agents`.

### HITL boundary

- **HITL, never automatable:** grilling rounds (a self-answering grilling agent has broken), destination and scope judgement, fog graduation, domain-term and ADR verdicts, prototype feel, **ticket granularity and edge approval**, **test-seam agreement**, triage category/state, questionnaire framing, retro acceptance.
- **AFK once a ticket exists:** `implement`, `implement-spec` fan-out and cleanup, `tdd` cycles at *already-confirmed* seams, `code-review`, conflict resolution, handoff emission, wayfinder bookkeeping.
- `to-spec` is AFK except the seam checkpoint. `to-tickets` has one HITL approval gate.

The destination therefore starts **after** `to-tickets` approval. Nothing before it is automatable, and nothing in it needs to be.

### The spec artifact — the DAG is already parseable

`to-tickets` writes `.scratch/<feature-slug>/issues/<NN>-<slug>.md`, one file per ticket, numbered from `01` in dependency order. Each ticket carries:

```
# <NN>: <title>
## What to build
Blocked by: NN, NN
Status: ready-for-agent
- [ ] acceptance checkboxes
```

**`Blocked by:` is a literal line in the file.** That is the task graph a scheduler can read with no new format, no YAML sidecar, and nothing invented. The spec itself is `.scratch/<feature-slug>/spec.md`; wayfinder maps are `.scratch/<effort>/map.md` with children under `issues/`.

This dissolves the open question of how a spec exposes its dependency graph — it already does.

### The worker contract — also already written

`triage/AGENT-BRIEF.md` is the canonical brief: **Category** (bug/enhancement), **Summary** (one line), **Current behavior**, **Desired behavior**, **Key interfaces** (type/function/config deltas), **Acceptance criteria** (independently-verifiable checkboxes), **Out of scope**. Durable over precise — no paths, no line numbers, no structure. Behavioural, not procedural: *what*, not *how*. Tickets carry a lighter variant (Parent / What-to-build / Acceptance / Blocked-by).

Communication stays sparse: context pointers to spec, ticket, notes and commits. No duplication. Tickets are a task graph and a frontier, not a step list.

### The gate, verbatim from `implement`

> Use /tdd where possible, at pre-agreed seams. Run typechecking regularly, single test files regularly, and the full test suite once at the end. Once done, use /code-review to review the work. Commit your work to the current branch.

No `bun run check` string appears anywhere in the suite — that gate is repo-specific, not suite-mandated. The closest generic form, from `resolving-merge-conflicts`, is "run them, typically typecheck, then tests, then format". **So the harness must ask each repo what its verification command is.** That is a per-repo fact the harness does not currently have.

`implement-spec` closes with: all tickets done → `/code-review` → a single fixer subagent → mark ready → clean up worktrees.

### The gap the suite leaves

`implement-spec` spawns "background worktree+branch implementers" and merger subagents, but **never says which agent or which model any of them are.** It assumes one harness. Every other ingredient the destination needs — task graph, brief template, gate, review, merge, cleanup — is already specified. Harness and model assignment is the genuine new work.

### Hygiene constraints that will bite

Grill → spec → tickets is one unbroken window (~150k smart zone). Each `implement` run gets a fresh context (`/clear` between tickets). Prototype detours round-trip through `handoff`. Prefer Continue when the next phase needs verbatim reasoning. Handoffs carry suggested skills, references, and redaction — never duplicated content.