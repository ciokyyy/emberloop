# glm-acp-agent 1.15.0 — bench result (longtail worker, 2026-10-15)

Verdict: **BEST long-tail fit so far** — tiny, fast, honest ACP surface:
custom endpoint + custom model roster via env (no rebuild), client stdio
MCP forwarded to the model, full tool loop verified, `session/set_model`
VERIFIED to switch models mid-session, clean cancel + 10 ms crash
detection, ~100 MB idle / ~100 MB per session, 50/50 soak clean at
p50 21 ms.

## Install

- Artifact: `npm glm-acp-agent@1.15.0`
  tarball `https://registry.npmjs.org/glm-acp-agent/-/glm-acp-agent-1.15.0.tgz`
  integrity `sha512-WL/ZueEc5MY3ZtvPtMTveb1hekerqzG+pg6C/sYBtmVUaF6VJ1wUd8eD1sywe+xoi7C/ul6g+ISe4g6XULsCkg==`
- Size on disk: 30 MB. No build. Start: `node …/dist/index.js` (stdio ACP).
- Purpose-built for the Z.AI GLM Coding Plan
  (`https://api.z.ai/api/coding/paas/v4`); "not a general-purpose client" —
  but the endpoint IS overridable (below).

## Provider / model config (no rebuild, all env)

| var | use |
|---|---|
| `Z_AI_API_KEY` | key (fake accepted against fakellm) |
| `ACP_GLM_BASE_URL` | e.g. `http://127.0.0.1:18600/v1` — redirects all model calls |
| `ACP_GLM_MODEL` | default model id (any string; `fake-tools` verified at fakellm) |
| `ACP_GLM_AVAILABLE_MODELS` | comma list REPLACING the roster (verified: `fake-small,fake-tools` → `session/new.models.availableModels` exactly those two) |
| `ACP_GLM_SESSION_DIR` | session persistence (default `~/.local/state/glm-acp-agent/sessions`) |
| others | `ACP_GLM_MAX_TURNS` (default 100), `ACP_GLM_MAX_TOKENS`, `ACP_GLM_COMMAND_TIMEOUT_MS`, `ACP_GLM_TOOL_RESULT_LIMIT_BYTES`, `ACP_GLM_THINKING/STREAM_THINKING`, `ACP_GLM_DEBUG`, `ACP_GLM_STREAM_IDLE_TIMEOUT_MS` |

- No remote catalog fetch observed (no models.dev/api.json URLs in bundle).
- `session/new` advertises `models` + `modes`; default roster = 4 GLM
  models (5.3 / 5.3-flash / 5-turbo / 4.7 with descriptions) + current.
- `session/set_model {sessionId, modelId}` → `{}` ACCEPTED and VERIFIED:
  next prompt hit fakellm with `model=fake-small`. (`session/set_config_option`
  exists but rejects unknown ids — correct behavior.)
- Client MCP: stdio stub CONNECTED despite only `mcpCapabilities {http:true}`
  advertised — fakellm saw `echo,slow` in the tool list. SSE explicitly
  rejected (documented). Full tool loop verified manually:
  `TOOL:echo:{…}` → `tool_call` update → completed `echo:hello` → `end_turn`.
- Sessions persist (`ACP_GLM_SESSION_DIR`); `session/resume` + load both work.

## Scenario results (acpbench, --mcp-stub)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok | 1.6 s; loadSession, close/fork/list/resume, image+embeddedContext |
| new_session | ok | 6.9 s; mcpReady not-advertised (probe waits for a notification glm never sends — yet tools ARE forwarded) |
| prompt_text | wrong-result | completes, TTFC ~0–1 ms(!), no tool call — probe-bug signature (NL prompt vs fakellm `TOOL:` directive) |
| cancel | ok (slow turn) | streaming, reusable |
| steer_followup | skipped | turn too fast to steer (honest skip) |
| load_resume | ok | resume USED |
| set_model_config | ok | set_model Invalid-params on probe's shape, set_config rejected unknown id; real-id set_model verified manually |
| images | ok | accepted (17.8 s — agent tried to `npx` @z_ai/mcp-server for vision: REAL npm fetch, see gotcha) |
| permission | skipped | never asks on ECHO turns |
| crash_recovery | ok | 10 ms detection, clean |
| concurrency N=3/10 | ok | 3/3 and 10/10, both modes |
| soak 5 | wrong-result | 0 hangs (probe-bug signature) |

## Flakiness / RAM (under flock)

- ECHO soak: **50/50 ok, 0 hangs**, p50 21 ms, p95 300 ms (cancel-every-5 mixed in; cancels landed post-completion, session stayed healthy).
- RAM: idle-init ~95 MB, idle-new ~103 MB, post-turn ~107 MB; N=3 peak
  304 MB (marginal ~67 MB); N=10 peak 1021 MB (~100 MB/session). Lightest
  long-tail agent per session after stakpak.
- Startup: ~1.6 s init. TTFC: ~0–20 ms against fakellm.

## Subscription logins (docs/source)

Z.AI API key only (`Z_AI_API_KEY` env or `--setup` stored on disk).
No SuperGrok / ChatGPT-sub / Antigravity / OpenCode-Go / Claude-sub /
Copilot. ACP `authMethods`: `z-ai-api-key` + `z_ai_api_key` (env_var).

## Gotchas for the Go daemon

1. Images trigger `npx @z_ai/mcp-server` download on first use (network +
   HOME pollution + 17 s latency) — pre-seed or block vision MCP.
2. Single-vendor model roster by default, but fully overridable via
   `ACP_GLM_AVAILABLE_MODELS` — good enough for a generic backend if the
   endpoint speaks OpenAI chat-completions.
3. `mcpReady` probe signal never fires though tools work — daemon should
   not gate on `_x.ai/mcp_initialized` for this agent.
4. No sandboxing (shell/fs run as the agent user); permission prompts are
   the only guard — daemon must auto-approve/deny explicitly.

## Raw commands

```
npm install --prefix ./pkg --no-audit --no-fund glm-acp-agent@1.15.0
acpbench --cmd 'node …/dist/index.js' --env Z_AI_API_KEY=fake-test-key \
  --env ACP_GLM_BASE_URL=http://127.0.0.1:18600/v1 --env ACP_GLM_MODEL=fake-tools \
  --cwd …/_proj --mcp-stub --fake-llm-url http://127.0.0.1:18600 --scenario all
```
