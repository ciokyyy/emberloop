# qwen-code 0.25.1 — bench result (longtail worker, 2026-10-14/15)

Verdict: **USABLE with caveats** — full ACP turn loop works against a custom
OpenAI-compatible endpoint; heaviest RAM of the long tail (~400 MB idle,
~220–410 MB per extra session); client stdio MCP servers are rejected at
`session/new` time only for the probe's shape (see gotcha); 1 transient
prompt `Internal error` in ~80 turns; SIGKILLed agent leaves stdio open
>10 s (kill the process GROUP, not the PID).

## Install

- Artifact: `npm @qwen-code/qwen-code@0.25.1`
  tarball `https://registry.npmjs.org/@qwen-code/qwen-code/-/qwen-code-0.25.1.tgz`
  integrity `sha512-6cEbtfx4Ugl1d3FdFLa8fcsG6knHuUUHqvDseLxGY06Xan8p7Fcd7+g5KlX694zX4mo/+Vh56AHxT/4R15zpYA==`
- Size on disk: 156 MB (`pkg/` with node_modules). No build step.
- Start: `node …/cli-entry.js --acp` (also `qwen serve` HTTP/SSE daemon, not benched).
- Isolated HOME: `/tmp/acpbench/work/qwen-code/home`, fakellm on 127.0.0.1:18600.

## Provider / model config (no rebuild)

`$HOME/.qwen/settings.json` (verified working):

```json
{
  "modelProviders": {
    "openai": [
      {"id": "fake-tools", "wireApi": "chat-completions",
       "envKey": "OPENAI_API_KEY", "baseUrl": "http://127.0.0.1:18600/v1"}
    ]},
  "security": {"auth": {"selectedType": "openai"}},
  "model": {"name": "fake-tools"}
}
```

- NOTE: `wireApi` must be `"chat-completions"` (or `"responses"`); `"chat"`
  is rejected with `Invalid wireApi "chat"`.
- fakellm log confirms `model=fake-tools`, `UA=QwenCode/0.25.1`, tools = qwen
  native set. Auth types in binary: openai/anthropic/gemini/qwen-oauth/vertex-ai.
- `session/new` returns `models: {currentModelId: "fake-tools(openai)",
  availableModels: [fake-tools, fake-small]}` — custom ids ARE exposed over ACP.
- `session/set_model` + `session/set_config_option`: both rejected
  (`Internal error` / `Unsupported configId`) — model switching is
  settings.json + restart only.
- Remote catalog: binary embeds a models.dev snapshot
  (`fetchedAt 2026-10-03`, projection v3, `~/.qwen/model-registry.json`
  cache) AND fetches `https://models.dev/api.json` at runtime (observed real
  fetch during bench). Override: `QWEN_CODE_MODELS_DEV_URL=http://…/api.json`
  (verified: fakellm saw the GET), disable via `QWEN_CODE_MODELS_DEV`.
  Host-overridable without rebuild. **Caveat:** with the override pointed at
  fakellm's minimal `/api.json`, one `session/prompt` failed with
  `Internal error` (shape mismatch suspected) — override shape must be full
  models.dev shape.

## Scenario results (acpbench, --mcp-stub)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok | 4.8–10.8 s (node startup); caps: loadSession, image/audio/embeddedContext, list+resume; mcp http/sse only |
| new_session | ok | 10–17 s; mcpReady `not-advertised` (stdio stub) |
| prompt_text | wrong-result | turn completes (`end_turn`, TTFC ~1 s) but no tool call — probe's NL echoPrompt vs fakellm's literal `TOOL:` directive (probe-side, see report) |
| cancel | ok (slow turn) | cancelled mid-streaming, session reusable; fast turns race (honest skip) |
| steer_followup | ok | no steering advertisement; 2nd prompt `queued` |
| load_resume | ok | `session/resume` USED, next prompt works |
| set_model_config | ok* | both methods rejected; transcript unverifiable (*no-op ok) |
| images | ok | accepted |
| permission | skipped | never asks on ECHO turns (honest skip) |
| crash_recovery | **timeout-hang** | client hung >10 s after SIGKILL — node children keep stdio open; kill process group |
| concurrency N=3 | ok | 3/3 both modes |
| soak 5 / 50 | wrong-result | 0 hangs in both; 50/50 turns complete (all "wrong" = no tool call, probe-side) |

## Flakiness / RAM (under flock)

- ECHO soak (own script): **50/50 ok, 0 hangs**, p50 605 ms, p95 1049 ms;
  cancel-mixed 20/20 ok, 0 hangs (cancels mostly post-completion; mid-stream
  cancel proven separately under latency).
- Probe prompt_text runs: 4 ok-pattern + 2 transient `Internal error`
  (1 with models.dev override, 1 shortly after) — rare, mechanism unclear,
  flag for re-probe. 72/72 ECHO prompts clean.
- RAM (probe phases, RSS process-tree sum): startup/init idle ~373 MB,
  idle-after-new ~397–408 MB; concurrency peak N=3 ~1051 MB
  (marginal ~218 MB/session), N=5 peak 2052 MB (~410 MB/session marginal at
  load), N=1 413 MB. **N=10 not attempted** (~4 GB projected, exceeds box).
- Startup (init): 3.5–10.8 s. TTFC: ~0.9–1.1 s.
- qwen logs own `event loop stall max=1–2 s` warnings under load.

## Subscription logins (docs/source, no login attempted)

Qwen OAuth (`qwen-oauth`, tokens `~/.qwen/oauth_creds.json`), DashScope key,
OpenAI/Anthropic/Gemini/Vertex keys. No SuperGrok / ChatGPT-sub /
Antigravity / OpenCode-Go / Copilot paths. ACP `authMethods` advertises
`openai` (terminal `--auth-type=openai`) only in this config.

## Gotchas for the Go daemon

1. Spawn cost ~400 MB + ~5–10 s per process; prefer few long-lived sessions.
2. On agent death, kill the whole process group (children hold stdio).
3. Client stdio MCP: advertised http/sse only. The probe's stub shape
   `{name,command,args,env:[]}` PASSES session/new, but a hand-rolled
   `{name,command,args}` (no `env`) FAILS with zod `invalid_union` — always
   send the full stdio shape.
4. `qwen` phones home (models.dev fetch, update check?) — sandbox/offline
   hosts should set `QWEN_CODE_MODELS_DEV_URL` or disable.
5. No `session/set_model` — model switches need settings rewrite + respan.

## Raw commands

```
npm install --prefix ./pkg --no-audit --no-fund @qwen-code/qwen-code@0.25.1
# settings.json as above; OPENAI_API_KEY=fake-test-key
acpbench --cmd 'node …/cli-entry.js --acp' --env OPENAI_API_KEY=fake-test-key \
  --cwd …/_proj --mcp-stub --fake-llm-url http://127.0.0.1:18600 --scenario all
python3 -I echo_soak.py --cmd 'node …/cli-entry.js --acp' --env OPENAI_API_KEY=fake-test-key --iterations 50
```
