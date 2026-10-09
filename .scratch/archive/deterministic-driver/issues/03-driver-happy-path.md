# 03: `--driver` runs a spec end to end without an LLM orchestrator

**What to build:** `el implement <spec> --driver` runs the whole spec with no orchestrator session. Tickets start in ticket-number order up to `max_workers` once their blockers have merged. The driver calls `finish` for a worker only when every required reviewer type has a pass at the worker's current tip, and treats a branch change after a pass as unreviewed again. It ends with a plain report of merged tickets with their sha. `--llm` selects the existing LLM orchestrator and stays the default; both modes use the same worker, reviewer and `finish` behavior.

**Blocked by:** 02 Ticket graph

**Status:** done

- [x] A spec with a chain and a fan-out merges in dependency order, never exceeding `max_workers` (injected world, fake clock)
- [x] `finish` is never called before both reviewers pass at the current tip, including after a post-pass change
- [x] The driver leaves uncommitted edits in the integration checkout alone
- [x] The final report lists every merged ticket with its sha
- [x] `--driver` and `--llm` both exist; a bare run still uses the LLM orchestrator
- [x] Spec stories 1, 2, 3, 9, 10, 29, 30, 39, 40, 41, 43

Spec: `.scratch/archive/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
