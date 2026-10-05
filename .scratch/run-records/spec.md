# Spec: run records, ticket history, token and memory capture, transcript archive

Status: ready-for-agent

## Problem Statement

Nothing records that a spec is being implemented: the orchestrator session leaves no trace, and the only hints are a `spec/<slug>` branch and worker entries. When a ticket merges, its worker entry is deleted, so its history and cost vanish. No token usage is read anywhere, even though pig and claude transcripts carry per-request usage, and `find` calls to the jev decision model aren't tied to any agent. The harness transcripts that hold the prompts, tool calls and results can be cleaned up by the harness, and the system prompt and tool definitions I pay for on every request aren't in them at all. I also can't see how much RAM my harnesses use, now or over time.

## Solution

`wick-pig implement` writes a run record per spec. Finished tickets move into history instead of being deleted. Every session gets an agent id, and wick captures, per agent: the raw per-request token breakdown with the harness-reported cost exactly as reported, `find`/jev calls with whatever usage the endpoint returns, a running archive copy of the harness transcript, the system prompt, tool definitions and flags (stored once by content hash), and process-tree memory samples every 5 seconds. All of it lands in wick's state directory as files, so the TUI and stats can read it and nothing is lost to harness cleanup.

## User Stories

1. As a user, I want each `wick-pig implement` to record the spec, repo, branch, orchestrator pane and tab, harness, model, effort and start time, so that I can see which specs are running.
2. As a user, I want a run marked running while its orchestrator pane is alive and ended otherwise, so that status matches reality.
3. As a user, I want the orchestrator recorded as an agent of its run, so that its tokens count toward the spec.
4. As a user, I want a finished ticket kept in history with its outcome (merged, discarded, kept), merge commit, both reviewer verdicts and final usage, so that merged work and its cost stay visible.
5. As a user, I want every session, top-level and spawned, to carry an agent id, so that every token and every `find` call belongs to someone.
6. As a user, I want the token usage of every request stored raw (model, provider, input, output, cache read, cache write, reasoning), so that costs can be recalculated later however we decide.
7. As a user, I want the cost the harness reports stored exactly as reported, so that I see what pig or claude says it cost without wick second-guessing it.
8. As a user, I want each `find` call logged against the calling agent with its decision model, call count and any usage or cost returned, so that jev spend is visible per agent.
9. As a user, I want wick to ask the decision endpoint for usage, and fall back to logging call counts when it gives none, so that we capture whatever exists.
10. As a user, I want a copy of each agent's transcript kept up to date in wick's archive while it runs, so that stats survive harness cleanup.
11. As a user, I want each session's system prompt, tool definitions and harness flags recorded, so that the costs nobody sees in the transcript are counted.
12. As a user, I want identical prompts and tool sets stored once, so that the archive doesn't grow per session.
13. As a user, I want the RAM of each harness process tree (harness, `wick-mcp`, anything they start) sampled every 5 seconds per process, so that I can tell harness bloat from a heavy test run.
14. As a user, I want memory samples written even when the TUI is closed, so that history is complete.
15. As a user, I want a command to prune archives by age or run, so that I can reclaim disk deliberately; nothing is deleted automatically.
16. As the TUI, I want all of this readable as plain files, so that it can show live state by polling.
17. As a user, I want usage parsed for pig, claude and agy alike, so that every harness is covered.

## Implementation Decisions

- New stores, all siblings of the existing worker store in wick's state directory: run records keyed by spec slug; ticket history per run; per-agent usage log (append-only JSONL, one line per request or per `find` call); archive per run per agent (transcript copy, memory samples); content-addressed blob store for system prompts, tool definitions and flags.
- Run record written by `implement` at launch, updated at exit when possible; liveness is decided by readers from herdr pane presence, not trusted from the record.
- `Finish` writes the ticket history entry before deleting the worker entry; reviewer verdicts are copied in before reviewers are forgotten.
- Agent id assigned in the session builder for every role; passed through the existing role environment so `wick-mcp` sees it for top-level sessions too.
- One usage-parser interface per harness: transcript line → zero or more normalized usage records `{agent, at, harness, provider, model, input, output, cacheRead, cacheWrite, reasoning, reportedCost?}`. pig and claude parsers live here; the agy parser comes from the harnesses spec in the same shape.
- Effort is recorded on every agent (run record, worker entry, reviewer and read-only agent records, ticket history), so that stats can group by model plus effort.
- The `find` tool records each decision call against `WICK_AGENT_ID`; the decision client requests usage and stores what comes back. The first ticket probes whether the endpoint returns usage or cost.
- The archive copier runs inside the session's own wick process (the `wick-mcp` attached to the harness), tailing the harness transcript with a byte offset; the same process samples memory every 5 s by walking its parent harness process tree in `/proc` (RSS per process: pid, name, rss). Settled sessions get a final copy.
- System prompt, tool definitions (as served by the MCP server for the role) and harness flags are hashed and stored once at session start; the session's archive references the hashes.
- No price table and no wick-side cost calculation in v1.
- `wick-pig stats prune` takes age or run filters; no automatic retention.
- Core module stays dependency-free; everything here is files and JSON.

## Testing Decisions

- Good tests drive public behaviour: spawn/finish through the existing fake herdr and assert the records on disk; feed parsers real transcript lines and assert normalized records.
- Usage parser: fixtures of real pig and claude transcript lines (including the claude cost-state record) → expected records.
- Run record and history: temporary state dir plus fake `Exec`, run implement → spawn → review → finish, assert history survives worker deletion with verdicts and usage.
- `find` attribution: fake decision provider returning with and without usage; assert log lines carry the agent id.
- Memory sampler: point it at a fake `/proc` tree with known pids, parents, names and RSS; assert per-process rows. Archive copier: append to a fake transcript in steps, assert the copy tracks it and resumes from its offset.
- Blob store: same prompt twice → one blob, two references.
- Prior art: the existing deliver, spawn, worker and review tests (statePath swap, fake Exec, fake herdr writing transcripts).

## Out of Scope

- Displaying any of this (TUI spec, stats spec).
- SQLite indexing and per-piece token attribution (stats spec).
- Wick-calculated prices.
- Web search tool: needs its own grill session.

## Further Notes

Spec 3 of 5. Blocks the TUI spec and the stats spec. The agy usage parser is delivered by the harnesses spec; this spec should not wait on it.
