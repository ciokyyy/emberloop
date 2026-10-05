# Spec: Stats: where tokens and memory went

Status: ready-for-agent

## Problem Statement

I know roughly what a run cost, but not why. Was it the system prompt, the tool definitions sent on every request, huge tool results, re-reading cached context, a harness-injected brief, or one runaway tool call? Which tools cost the most? Which harness eats the most RAM? Providers only report tokens per request, and the raw data sits in archives no one can query.

## Solution

The TUI module indexes wick's archive (transcripts, usage logs, prompt and tool-definition blobs, memory samples) into a SQLite database at `~/.wick-pig/stats.db`, live as files grow, and rebuildable from the archive at any time. Per-request token totals are exact. Each request's new input is split across the pieces that turn added (system prompt, tool definitions, user prompts, harness prompts, tool call arguments, tool results, assistant text, reasoning) by character share and marked "≈". Cache reads are shown separately. A Stats tab and `wick-pig stats --json` show breakdowns at every scope, and every row opens down to the raw content.

## User Stories

1. As a user, I want a Stats tab scoped globally, per repo, spec, ticket or agent, so that I can zoom from everything down to one session.
2. As a user, I want tokens broken down by piece type (system, tool definitions, user, harness, tool arguments, tool results, assistant, reasoning, cache read), so that I see what kind of content is expensive.
3. As a user, I want per-tool rows (call count, argument tokens, result tokens, average and max), so that I know which tools cost the most.
4. As a user, I want the 50 most expensive single items, so that I can find runaway tool results or prompts.
5. As a user, I want to open any row and see the raw text: the full prompt, arguments or result, so that I can debug it.
6. As a user, I want breakdowns by role and agent type, by model, by model plus effort, and by harness, so that I can compare choices.
7. As a user, I want tokens per request over a session's life, so that I can spot context growth and compactions.
8. As a user, I want exact numbers and estimated numbers visibly distinguished, so that I never mistake an estimate for a measurement.
9. As a user, I want cache reads separate from new input, so that I see what re-sending context costs.
10. As a user, I want system prompt and tool-definition cost counted per request, so that a bloated tool description shows up.
11. As a user, I want `find`/jev calls in the same breakdowns, so that decision-model spend is included.
12. As a user, I want memory history grouped by harness, model and role (peak, average and p95 RSS per session), so that I can compare how heavy each harness is.
13. As a user, I want a session's memory over time and per process, so that I can tell harness growth from a heavy child process.
14. As a user, I want the worst sessions by memory listed and openable, so that I can investigate them.
15. As an agent debugging later, I want `wick-pig stats --json` with the same scopes and breakdowns, so that I can analyse without the TUI.
16. As a user, I want the index rebuilt from the archive on demand, so that a corrupt or outdated database is never a loss.
17. As a user, I want to run SQL myself against the database, so that I can ask questions the UI doesn't.
18. As a user, I want the index kept live while the TUI is open, so that stats include running agents.

## Implementation Decisions

- SQLite via `modernc.org/sqlite` (pure Go), owned by the TUI module. The archive is the source of truth; the database is a disposable index with a schema version and a full rebuild command.
- Indexer per harness transcript format (pig, claude, agy) turns lines into pieces `{agent, request, kind, tool?, chars, raw-ref}` and requests `{agent, at, model, input, output, cacheRead, cacheWrite, reasoning, reportedCost}`; reuses the run-records usage parsers for the request rows.
- Attribution: for consecutive requests in a session, new input = input(n) − input(n−1) (counting cache read plus cache write plus uncached input consistently per provider); that exact delta is split across pieces added since request n−1 by character share. The first request's input is split across system prompt and tool-definition blobs plus the first prompt. A negative delta marks a compaction or reset; attribution restarts from that request. All piece-level numbers carry an estimated flag.
- Raw content is not copied into SQLite; rows reference the archive (file and offset or blob hash) and are read on open.
- Memory samples indexed as `{agent, at, pid, name, rss}`; peak, average and p95 computed by query.
- Incremental indexing with per-file offsets, polled with the TUI's 1-second tick; a cold start indexes everything first.
- `wick-pig stats` subcommands: `--json` with scope flags, `rebuild`, plus `prune` from the run-records spec.

## Testing Decisions

- The indexer is the deep module. Tested by indexing fixture archives (one per harness, including a compaction and a `find` log) into a temporary database and asserting breakdown query results.
- Attribution test: a synthetic session with known input totals and piece sizes yields the expected split; piece sums equal the exact per-request delta.
- Rebuild check: indexing the same archive twice, cold and incremental, gives identical query results.
- Memory aggregate test over known samples.
- A few TUI tests: drilling a row opens the raw content view.
- Prior art: the fixture-transcript tests in the core and the TUI spec's Snapshot fixture approach.

## Out of Scope

- Wick-calculated prices (raw tokens are kept, so this can come later).
- Exact per-piece tokenization with model tokenizers.
- Subscription quota tracking.
- Web search tool: needs its own grill session.

## Further Notes

Spec 5 of 5. Blocked by the run-records spec (archive and samples) and the TUI spec (tabs and module).
