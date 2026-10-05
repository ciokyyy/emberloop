# 02: Split tokens into pieces

**What to build:** Transcripts (pig, claude, agy) and blobs are indexed into pieces: system prompt, tool definitions, user prompt, harness prompt, tool call arguments (with tool name), tool result (with tool name), assistant text, reasoning, each referencing its raw content in the archive. Each request's new input (input minus the previous request's input) is split across the pieces added since by character share and marked estimated; the first request's input covers system prompt, tool definitions and the first prompt; a drop in input marks a compaction and restarts attribution. Cache reads stay a separate category. `stats --json` gains breakdowns by piece type and by tool (count, argument tokens, result tokens, average, max).

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Synthetic session test: piece estimates sum to the exact per-request delta
- [ ] Compaction case handled
- [ ] Harness prompts (briefs, notices) separated from user prompts
- [ ] Every piece number flagged as estimated; request totals exact
