# 05: Follow-ups from the submit-results run

**What to build:** Loose ends found in the final review of the submit-results spec, folded in here as one small ticket:

- **`persistent` does nothing.** Agent-type frontmatter `persistent` is parsed but never read: re-spawning a reviewer type always reuses the live reviewer. Re-spawning a type with `persistent: true` reuses the existing agent for the same parent; `persistent: false` starts a fresh one. Built-in reviewer types keep `persistent: true`, so their behaviour is unchanged.
- **Scripts call removed tools.** `scripts/tell-worker-midturn.sh` and `scripts/tell-worker-live.sh` still drive `spawn_worker`, `wait_workers` and `tell_worker`. Port them to `spawn`/`tell` (with the notice arriving in the parent pane instead of a wait), or delete them if the live tests now cover the same ground.
- **Stale comments.** Comments still name removed tools (`spawn_worker`, `code_review`) in the tools registry, the MCP server, spawn, and an extension test. Reword them to the current tools.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A repo type with `persistent: false` gets a fresh agent on re-spawn; a persistent one is re-prompted (test through `spawn` with fake herdr)
- [x] No script references `spawn_worker`, `wait_workers`, `tell_worker` or `finish_worker`; kept scripts run against the current tools
- [x] No production comment names a removed tool
- [x] Existing tests still pass
