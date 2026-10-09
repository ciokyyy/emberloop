# 01: Every session has an agent id; implement writes a run record

**What to build:** Every session el launches, top-level included, carries an agent id in its role environment, so its `el-mcp` knows who it serves. `el implement` writes a run record keyed by spec slug: spec path, repo, integration branch, orchestrator agent id, pane and tab, harness, model, effort, start time; updated with an end time when the session exits normally. Readers decide "running" from whether the orchestrator pane is alive in herdr.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `EL_AGENT_ID` set for top-level, reviewer, scout and worker sessions (harness command tests)
- [x] Run record written at implement launch; re-running implement on the same spec updates it rather than duplicating
- [x] A small exported read function lists run records (used by the TUI later)
- [x] Tests with temporary state dir and fake `Exec`
