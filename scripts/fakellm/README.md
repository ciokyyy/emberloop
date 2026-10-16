# fakellm — deterministic fake LLM server for benchmarking coding agents

Stdlib-only Go module. Scripted replies, no real model calls. Accepts any
API key. Point a harness at it by overriding the provider base URL, e.g.
`OPENAI_BASE_URL=http://127.0.0.1:8080/v1`.

## Run

```sh
cd scripts/fakellm
go run . --addr 127.0.0.1:8080 --log /tmp/fakellm.jsonl
```

Flags: `--addr` (default `127.0.0.1:8080`), `--models` JSON catalog file
(see `testdata/models.json`; accepts a string array, an object array with
`id`/`name`/`context_length`/`pricing`, or `{"data":[...]}` /
`{"models":[...]}` wrappers), `--log` JSONL request log file,
`--latency-ms` base latency on every response, `--cooperate` (COOPERATE
fuzzy tool-calling for all requests; per-request via `X-Fake-Cooperate`
header or `FAKELLM_COOPERATE`/`FAKE_COOPERATE` env), `--faults` JSON or file
(by-counter fault rules, e.g. `'{"3":"http500"}'`), `--fault-rate p`
(seeded soak probability per request) with `--fault-seed` (default 42).

Tests: `go test ./...` (httptest, no network, no real APIs).

## Script directives

Put these in the last user message; the reply is deterministic:

| Directive | Behaviour |
|---|---|
| `ECHO:<text>` | reply with `<text>` |
| `TOOL:<name>:<json>` | emit a tool call of `<name>` (only when listed in the request tools); separate parallel calls with `;` or newlines; after the tool result arrives the reply is `DONE:<result>` |
| `SLOW:<ms>` | sleep `<ms>` between streamed chunks (cancel tests) |
| `HANG` | send one chunk, then hold the connection open |
| `CRASHCONN` | send partial bytes, then close the TCP connection (real RST via hijack) |
| `BIG:<kb>` | reply with ~`<kb>` KB of text |
| `FAULT:429` | HTTP 429 + `Retry-After: 1` |
| `FAULT:500` / `502` / `503` / `401` | HTTP status with a protocol-shaped error body |
| `FAULT:529` | HTTP 529 `overloaded_error` (Anthropic envelope) |
| `FAULT:SSE_ERROR` | stream dies mid-way: Anthropic `event: error` / OpenAI `data: {"error":...}` |
| `FAULT:STALL:N:D` | no bytes for `D` ms after `N` chunks, then resume |
| `FAULT:TTFB:MS` | slow first byte (`MS` ms, default 300) |
| `FAULT:TRUNCATED` | cut-off JSON body, connection closes |
| `FAULT:INVALID_SSE` | non-SSE bytes on a stream (non-JSON body off-stream) |
| `FAULT:RESET` | mid-stream TCP RST (same real close as `CRASHCONN`) |
| `FAULT:EMPTY` | zero content, finish `stop` |
| `FAULT:LENGTH` | `finish_reason` `length` (`max_tokens` / `MAX_TOKENS`) with a partial reply |
| `FAULT:DUPLICATE` | every streamed chunk sent twice |
| (none) | reply `OK` |

Tool-call ids (`call_<n>`, `toolu_<n>`, `fc_<n>`) are process-unique: they
never repeat across turns. OpenAI Chat tool calls carry a per-call `index`;
streamed tool calls emit a first chunk with `id`/`type`/function `name`
then argument deltas, with `usage` on the final chunk. Anthropic
`message_start.message` carries `usage` (`input_tokens`, `output_tokens: 0`),
the final `message_delta` carries `usage` (`output_tokens`), and `tool_use`
starts carry `input: {}`.

Reasoning/thinking blocks are emitted when the request enables them
(OpenAI `reasoning_effort`/`reasoning`, Responses `reasoning`, Anthropic
`thinking`, Gemini `generationConfig.thinkingConfig`).

## COOPERATE mode

`--cooperate`, per-request `X-Fake-Cooperate: 1` header, or
`FAKELLM_COOPERATE=1` / `FAKE_COOPERATE=1` env. For a prompt with no
directives that mentions an available tool, emit exactly one call to the
best-matching tool; after the tool result the reply is `DONE:<result>`.

Matching rules: case-insensitive; namespace-agnostic (split tool names on
any non-alphanumeric run, match the full name, the normalised name, or the
last segment, so `echo` matches `echo`, `acpbench-stub_echo`,
`mcp__acpbench_stub__echo`, `acpbench_stub__echo`, `acpbench-stub__echo`,
`functions.echo`); full-name hits outrank normalised hits, which outrank
base-segment hits, ties go to the longest name. A prompt containing `slow`
prefers the best slow-named tool. Fire-once: once any tool result is in
history no further cooperate call is emitted. Default args: string-ish
schema fields get `"hello"`, ms-ish numeric fields get `100`, other numbers
`1`, booleans `false` (no schema: `{"text":"hello"}`); the slow tool gets
`{"ms": N}` with `N` from the first integer in the prompt or `5000`.

## Failure injection

Directives (table above) fire per request. `--faults` fires by global LLM
request counter (1-based): `'{"2":"http500"}'`, `{"faults":{...}}`, or
`{"rules":[{"at":3,"fault":"http429"}]}` / array form; a file path is read.
`--fault-rate 0.05 --fault-seed 7` fails ~5% of requests with a random fault
class from a fixed pool. Every injected fault bumps `by_fault` in
`/__stats` and is logged as `"fault"` in the JSONL log.

## Endpoints

- `POST /v1/chat/completions` — OpenAI Chat (stream + non-stream)
- `POST /v1/responses` — OpenAI Responses (stream + non-stream)
- `POST /v1beta/models/{m}:generateContent`,
  `POST /v1beta/models/{m}:streamGenerateContent` (same under `/v1/models/`)
- `GET /v1/models` — OpenAI-compatible list
- `GET /api/v1/models` — OpenRouter-style (`context_length`, `pricing`)
- `GET /api.json` — models.dev-style catalog
- `GET /api/models/providers/<provider>?types=chat,image,classifier` —
  pi.dev-style catalog (same shape `emberloop-web/catalogsync.go` reads)
- `GET /__catalog` — read the live model list; `PUT /__catalog` replaces it
  (same shapes as `--models`); catalog GETs bump `catalog_fetches`
- `GET /__stats` — `{"total", "by_path", "by_model", "by_fault",
  "catalog_fetches", "last_request"}`

Every request is appended as JSONL (`path`, `model`, `tools`, `auth`,
`ua`, `stream`, `fault`) when `--log` is set.

## curl examples

```sh
B=http://127.0.0.1:8080

# Chat, default reply
curl -s $B/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model":"fake-small","messages":[{"role":"user","content":"hi"}]}'
# {"choices":[{"finish_reason":"stop","index":0,
#   "message":{"content":"OK","role":"assistant"}}],...,
#   "usage":{"completion_tokens":4,"prompt_tokens":9,"total_tokens":13}}

# Chat, echo + streaming
curl -sN $B/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model":"m","stream":true,"messages":[{"role":"user","content":"ECHO:stream ok"}]}'
# data: {"choices":[{"delta":{"role":"assistant"},...}],...}
# data: {"choices":[{"delta":{"content":"stream ok"},...}],...}
# data: {"choices":[{"delta":{},"finish_reason":"stop",...}],...}
# data: [DONE]

# Chat tool loop, turn 1: model emits the call
curl -s $B/v1/chat/completions -H 'Content-Type: application/json' -d '{
  "model":"fake-tools",
  "messages":[{"role":"user","content":"TOOL:read:{\"path\":\"a\"}"}],
  "tools":[{"type":"function","function":{"name":"read"}}]}'
# {"choices":[{"finish_reason":"tool_calls","index":0,"message":{
#   "role":"assistant","content":null,
#   "tool_calls":[{"id":"call_1","type":"function","index":0,
#     "function":{"name":"read","arguments":"{\"path\":\"a\"}"}}]}}],...}

# Chat tool loop, turn 2: feed the result back, model replies DONE:<result>
curl -s $B/v1/chat/completions -H 'Content-Type: application/json' -d '{
  "model":"fake-tools",
  "messages":[
    {"role":"user","content":"TOOL:read:{\"path\":\"a\"}"},
    {"role":"assistant","tool_calls":[{"id":"call_1","type":"function",
      "function":{"name":"read","arguments":"{}"}}]},
    {"role":"tool","tool_call_id":"call_1","content":"file-bytes"}],
  "tools":[{"type":"function","function":{"name":"read"}}]}'
# {"choices":[{..."message":{"content":"DONE:file-bytes",...}}],...}

# Parallel calls: "TOOL:a:{\"x\":1}; TOOL:b:{\"y\":2}" with both listed

# COOPERATE: natural language triggers one fuzzy tool call
curl -s $B/v1/chat/completions -H 'Content-Type: application/json' \
  -H 'X-Fake-Cooperate: 1' -d '{
  "model":"m","messages":[{"role":"user","content":"please use echo"}],
  "tools":[{"type":"function","function":{"name":"mcp__acpbench_stub__echo"}}]}'
# ... one tool_calls entry with {"text":"hello"} args ...

# Faults: HTTP + stream shapes
curl -s -i $B/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model":"m","messages":[{"role":"user","content":"FAULT:429"}]}'
# HTTP/1.1 429 ... Retry-After: 1
curl -sN $B/v1/messages -H 'Content-Type: application/json' \
  -d '{"model":"m","max_tokens":8,"stream":true,
       "messages":[{"role":"user","content":"FAULT:SSE_ERROR"}]}'
# ... event: error ... overloaded_error ...

# Mutable catalog + stats
curl -s -X PUT $B/__catalog -d '["bench-a","bench-b"]'
curl -s $B/__catalog
curl -s $B/v1/models
curl -s $B/__stats
# {"total":...,"by_path":{...},"by_model":{...},"by_fault":{...},
#  "catalog_fetches":...,"last_request":"..."}

# Responses
curl -s $B/v1/responses -H 'Content-Type: application/json' \
  -d '{"model":"m","input":"ECHO:resp ok"}'
# {"id":"resp_fake","object":"response","status":"completed",
#   "output":[{"type":"message","role":"assistant",
#     "content":[{"type":"output_text","text":"resp ok"}]}],...}

# Anthropic
curl -s $B/v1/messages -H 'Content-Type: application/json' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"ECHO:anth ok"}]}'
# {"id":"msg_fake","type":"message","role":"assistant",
#   "content":[{"type":"text","text":"anth ok"}],
#   "stop_reason":"end_turn",...}

# Gemini
curl -s "$B/v1beta/models/gemini-fake:generateContent" \
  -H 'Content-Type: application/json' \
  -d '{"contents":[{"role":"user","parts":[{"text":"ECHO:gem ok"}]}]}'
# {"candidates":[{"content":{"parts":[{"text":"gem ok"}],"role":"model"},
#   "finishReason":"STOP",...}],"usageMetadata":{...}}

# Discovery
curl -s $B/v1/models
curl -s $B/api/v1/models            # context_length + pricing
curl -s $B/api.json                 # models.dev-style
curl -s "$B/api/models/providers/openai?types=chat,image,classifier"
curl -s $B/__stats
```
