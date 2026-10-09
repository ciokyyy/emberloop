# 04: find / jev calls recorded against the calling agent

**What to build:** Each decision-model call made by `find` is logged in the calling agent's usage log with the decision model, a find-call id, and any usage or cost the endpoint returns. First, probe the decision endpoint: does it return usage or cost when asked (e.g. OpenRouter's usage option)? Request it if supported; otherwise log call counts only.

**Blocked by:** 01

**Status:** done

- [x] Probe result recorded in this ticket
- [x] Decision client requests usage when supported and returns it with the answers
- [x] Log lines carry `EL_AGENT_ID`, kind `find`, model, call count, usage or cost if any
- [x] Tests with a fake decision provider, with and without usage

## Probe result (2026-10-05, live endpoint, `typesafe/jev-1.13`)

POST `https://openrouter.ai/api/v1/systemone` with no usage parameter returns
usage **by default**, alongside the resolved model, a generation id and the
serving provider:

{"model":"typesafe/jev-1.13-20260917","answers":{"q1":{"type":"noul","noul":0.65}},"usage":{"input_tokens":278,"output_tokens":21,"cost":0.000011676},"id":"gen-dec-1791237092-NwAW4V0hD1GqPMkgL5J0","provider":"TypeSafe"}

A second probe with `"usage":{"include":true}` returns the identical shape, so
the flag is accepted and unnecessary. The client therefore sets
`usage.include=true` anyway (cheap insurance if the default ever changes) and
parses `usage`, `id`, `provider`, `model` off every response; a response
without `usage` parses to nil and the log keeps the call-count line.

## Log layout (for ticket 03 and the stats indexer)

One line per decision call in `<state>/usage/<agent-id>.jsonl`:
`{agent, at, kind:"find", model, find_call_id, questions, generation?,
provider?, usage?}` with `usage:{input_tokens, output_tokens, cost?}` verbatim
from the endpoint. Each line is one call, so call counts come from counting
lines grouped by `find_call_id`; `questions` is how many decisions that call
carried. No `EL_AGENT_ID` means no line (bare runs have nobody to bill).
`EL_STATE_DIR` redirects the state dir in tests.
