# 03: Per-agent token usage log for pig and claude

**What to build:** Every request a pig or claude agent makes ends up as a line in that agent's usage log: `{agent, at, harness, provider, model, input, output, cacheRead, cacheWrite, reasoning, reportedCost?}`, raw, with the harness-reported cost exactly as reported (pig's per-message `usage.cost`, claude's per-message usage and its `cost-state` totals). The session's own el process tails the harness transcript and appends as requests happen. A usage-parser interface per harness turns transcript lines into records.

**Blocked by:** 01

**Status:** done

- [x] Parser interface with pig and claude implementations
- [x] Fixture tests from real pig and claude transcript lines, including claude's cost-state record
- [x] Tailing resumes from a stored offset, never double-counts
- [x] A final pass when the session settles
- [x] No el-side price calculation

## Transcript shapes (read off live local transcripts, Oct 2026)

- pig assistant message: `usage{input, output, cacheRead, cacheWrite,
  reasoning?, totalTokens, cost{input, output, cacheRead, cacheWrite,
  total}}` plus `provider` and `model` per message; one message is one
  request (message id is the dedupe key). Errored turns log their zeros.
- claude assistant block: `message.usage{input_tokens,
  cache_creation_input_tokens, cache_read_input_tokens, output_tokens,
  output_tokens_details.thinking_tokens?}`; one request streams as several
  blocks sharing `message.id` with identical usage (message id, else
  requestId, else uuid is the key). Per-message usage carries no cost.
- claude `cost-state`: no timestamp, only cumulative `modelUsage{model:
  {inputTokens, outputTokens, thinkingTokens, cacheReadInputTokens,
  cacheCreationInputTokens, costUSD}}`; empty usage logs nothing.

## Log layout (for the stats indexer)

One line per request in `<state>/usage/<agent-id>.jsonl`, sharing the file
with ticket 04's find lines: `{agent, at, harness, provider, model, input,
output, cacheRead, cacheWrite, reasoning, reportedCost?, kind}` with
`kind:"request"` per request and `kind:"cost"` per claude cost-state
snapshot (cumulative totals, not per-request — sum only `request` rows for
tokens). pig rows report `usage.cost.total`; claude request rows report no
cost; claude cost rows report the model's `costUSD`. No el-side prices.
`provider` is the transcript's for pig, `"anthropic"` for claude (its
transcript names none). Tail state lives beside the log in
`<agent-id>.tail` (`{offset, seen}`); `EL_STATE_DIR` redirects both.
