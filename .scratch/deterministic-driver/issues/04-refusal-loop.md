# 04: A refused finish goes back to the worker and the driver tries again

**What to build:** When `finish` refuses, the driver sends the refusal text to the worker unchanged through `tell`, waits for the worker's next submit, and calls `finish` again. There is no attempt limit. The cases from real runs are the scenarios: uncommitted changes in the worker's worktree, uncommitted changes in the integration checkout overlapping the incoming change, and a rebase conflict that was aborted.

**Blocked by:** 03 Driver happy path

**Status:** done

- [x] The refusal text reaches the worker byte for byte
- [x] `finish` runs again after the worker's next submit and a fixed conflict then merges
- [x] Each real refusal kind (dirty worktree, overlapping dirty checkout, rebase conflict) loops until the world lets it merge
- [x] No attempt limit exists: a fifth refusal is handled like the first
- [x] Spec stories 11, 12, 13

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
