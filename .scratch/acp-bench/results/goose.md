# goose — benchmark result (worker3, 2026-10-14)

VERDICT: STRONG FIT for a generic ACP backend, with two caveats: (1) no ACP-side model/config switching
(`session/set_model`, `session/set_config_option` both unimplemented — model is fixed at spawn via env/config),
(2) RAM is the heaviest of the three (~128 MB idle per session process, ~100 MB marginal).

- Version: 1.54.0 (`goose --version` → `1.54.0`), pinned GH release asset
  `aaif-goose/goose` v1.54.0 `goose-x86_64-unknown-linux-gnu.tar.bz2`.
- Note: upstream moved `block/goose` → `aaif-goose/goose` (Agentic AI Foundation); asset names unchanged.

## Install

- Artifact: https://github.com/aaif-goose/goose/releases/download/v1.54.0/goose-x86_64-unknown-linux-gnu.tar.bz2
- Size: 80,909,320 bytes on the wire; 294 MB extracted (single `goose` binary, runs on Debian 12 glibc 2.36).
- sha256 (observed, no published checksum file in the release): `bf01cb6359f7a2b1acb54ca4827e1d059b18aabb3d2b75202e21066cfd3a3396`
- Install: fresh dir `/tmp/acpbench/work/goose/`, `tar -xjf goose.tar.bz2 -C app`. No sudo, no deps.
- Isolated HOME `/tmp/acpbench/work/goose/home` (+ XDG_* under it); fake key only; no real credentials touched.
- Config used: env `GOOSE_PROVIDER=openai GOOSE_MODEL=fake-tools OPENAI_HOST=http://127.0.0.1:18300
  OPENAI_API_KEY=fake-key-for-bench GOOSE_MODE=auto` against fakellm on :18300.
  fakellm JSONL confirms `model=fake-tools` on `POST /v1/chat/completions` (227 hits) plus 89 `GET /v1/models`
  (goose queries the OpenAI models endpoint for discovery).
- Second config (custom provider, no rebuild): `~/.config/goose/custom_providers/fakebench.json`
  (`engine=openai`, `base_url=http://127.0.0.1:18300/v1/chat/completions`, `models=[{fake-custom-model-xyz}]`,
  `api_key_env=FAKEBENCH_API_KEY`) + `GOOSE_PROVIDER=fakebench GOOSE_MODEL=fake-custom-model-xyz` →
  fakellm log shows `model=fake-custom-model-xyz`. Custom model id accepted end-to-end.

## acpbench --scenario all (with --mcp-stub)

| scenario | verdict | detail |
|---|---|---|
| initialize | ok (0.8–1.9 s; init itself ~0.1–1.3 s) | protocolVersion 1; loadSession true; image true, audio false; NO steering; mcpCaps http-only/sse-no; authMethods=[goose-provider] |
| new_session | ok (0.6–7.5 s) | mcpReady `not-advertised` (5 s wait expires — goose never sends `_x.ai/mcp_initialized`, yet MCP tools DO work, see below) |
| prompt_text | wrong-result (probe artifact, see §) | end_turn, chunks streamed, but no tool_call: fakellm replies `OK` to the natural-language prompt so the agent honestly calls nothing |
| cancel | skipped (probe artifact) | turn finished before the 1.2 s race window; manual SLOW:20000 test → `cancelled`, session reusable (see §Flakiness) |
| steer_followup | skipped | no steering advertised; second-prompt race also too fast — nothing to steer into |
| load_resume | ok | resume unsupported → session/load fallback works, next prompt works |
| set_model_config | ok (both methods unsupported) | `session/set_model` → Method not found; `session/set_config_option` → Invalid params. Model fixed at spawn |
| images | ok | 1x1 PNG accepted, end_turn |
| permission | skipped | agent asks nothing (needs a tool attempt first). In `GOOSE_MODE=approve` a TOOL-directive turn still completes (22 updates vs 11 — permission round-trip auto-approved by the probe driver) |
| crash_recovery | skipped | turn finished before SIGKILL window — no slow turn available to the probe |
| concurrency | ok | 3/3 multi-process AND 3/3 single-process; marginal ≈ 100 MB/session (acpbench, unlocked box) |
| soak (5x echo) | wrong-result (same probe artifact) | 0 hangs, p50 ~21 ms (fast `OK` relays), success-by-probe-definition 0 |

§ Probe artifact (NOT a goose bug): `echoPrompt` ("Call the echo tool…") contains no fakellm `TOOL:` directive,
so any honest LLM-backed agent completes without a tool call and the probe's `expectTool:true` classifier marks
`wrong-result`. Manual ACP driver sending `TOOL:acpbench-stub__echo:{"text":"hello"}`: goose emitted the tool call
(`acpbench-stub: echo`), executed it via the stub, fed the result back, final text `DONE:echo:hello`, end_turn —
end-to-end MCP tool use PROVEN (goose namespaces the tool `server__tool`, which is also why the probe's exact-name
expectation could never match). Same artifact explains permission/cancel/crash skips (all need a slow/tool turn the
probe cannot script through fakellm).

## RAM (VmRSS summed over process tree; locked runs except acpbench row)

| condition | RSS | notes |
|---|---|---|
| idle after initialize | ~110–116 MB | acpbench phase sample |
| idle after new-session | ~136 MB | acpbench phase sample |
| prompt-turn peak | ~146 MB | acpbench phase sample |
| locked N=1: idle / turn-peak | 206 / 214 MB (cold) | w3ram.py; first-spawn pays cold mappings |
| locked N=5: per-proc idle | ~128 MB | steady state |
| locked N=10: per-proc idle | ~128 MB | steady state, peak ~143 MB/proc |
| marginal per session | ~100–128 MB | (multi-peak − idle)/N |
| startup (spawn→first reply) | ~26–130 ms spawn; init p50 ~431 ms warm (1.9 s cold) | |

## Flakiness (manual TOOL-directive turns, fakellm-backed)

- Soak: 50/50 fresh-session tool turns ok (49-run batch + 1 rerun; one batch was cut by the harness 10-min tool
  budget, not by an agent hang — rerun clean). Turn p50 335 ms (min 254, max 1269), first-chunk p50 ~331 ms.
- Cancel (SLOW:20000, cancel @2 s): `stopReason=cancelled`, follow-up `ECHO:reuse-ok` → end_turn. 1/1.
- Hangs observed: 0. Crashes: 0. Success rate on honest turns: 100% (50/50 tool turns).
- Measured under `load average 9-16` (user's own builds + fellow workers); latencies are
  order-of-magnitude, not lab numbers.

## Catalog / providers

- Custom provider WITHOUT rebuild: JSON file in `~/.config/goose/custom_providers/<name>.json`
  (schema: `name, engine: openai|anthropic|ollama, display_name, description, api_key_env|auth{command},
  base_url` (full URL incl. path, e.g. `…/v1/chat/completions`), `models:[{name, context_limit}]`,
  `headers{}, supports_streaming, requires_auth`). Verified live (fakebench above). CLI/Desktop wizards write the
  same files; `goose configure` is optional.
- Built-in provider count: ~50 (45-row Available-providers table — OpenAI w/ OPENAI_HOST override, Anthropic,
  Ollama, LiteLLM, OpenRouter, xAI, etc. — plus `cursor-agent` CLI provider, `claude-acp` + `codex-acp` ACP-sidecar
  providers, Z.AI Coding Plan). Source of truth: `crates/goose/src/providers/` (~`init.rs` registry).
- Remote catalog: the OpenAI-compatible providers query the endpoint's `GET /v1/models` (observed 89 calls);
  custom providers use their static `models` list. No host-overridable global catalog URL; per-provider `*_HOST`
  (e.g. `OPENAI_HOST` = root, `OPENAI_BASE_PATH` default `v1/chat/completions`) and `*_BASE_PATH` envs redirect
  individual providers. Host CAN override everything per-session via env (`GOOSE_PROVIDER`/`GOOSE_MODEL`/`<P>_HOST`),
  but NOT at ACP runtime (no set_model/set_config_option).
- Custom model id in ACP model list: N/A — goose exposes no `configOptions`/model list over ACP and no runtime
  switch; selection is spawn-time only. The custom id IS honored on the wire (fakellm log proof).

## Subscription logins (docs only, never logged in)

- ChatGPT sub: YES — `ChatGPT Codex` provider, browser-based OAuth, no API key (Plus/Pro). Tokens: OS keychain,
  else `~/.config/goose/secrets.yaml` (file-based fallback; API keys never go in config.yaml).
- Claude sub: NO direct (Anthropic provider is API-key only). Indirect: `claude-acp` sidecar provider reuses an
  active Claude Code subscription (needs `npm install -g @agentclientprotocol/claude-agent-acp`).
- Copilot: YES — GitHub Copilot provider via device-flow auth (needs Copilot access).
- SuperGrok: NO (xAI provider is `XAI_API_KEY` only).
- Antigravity: NO (no provider, no mention).
- OpenCode Go/Zen: NO (no provider; Tetrate Agent Router has its own PKCE flow, unrelated).

## Gotchas for a Go Daemon integration

1. Spawn-time model binding only — the Daemon must respawn (or new-session) to change provider/model; no
   `session/set_*` surface at all. `GOOSE_PROVIDER`/`GOOSE_MODEL`/`<P>_HOST` envs are the control plane.
2. MCP works but is silent: stdio stub tools were offered AND executed although goose advertises only
   http/sse mcpCapabilities and never emits `_x.ai/mcp_initialized` (the 5 s `mcpReady` wait always expires —
   Daemon should not gate on it for goose). Tool names arrive namespaced (`acpbench-stub: echo`).
3. No steering advertisement; second-prompt concurrency behavior untested (turns too fast to overlap — on a
   saturated box this needs a SLOW-model retest; protocol path exists via normal session/prompt).
4. `session/load` resume across processes works (fallback leg ok); `session/resume` unsupported.
5. Heaviest RAM of the three candidates (~128 MB idle vs ~64 vtcode); fine for one backend, heavy for fleets.
6. `GOOSE_MODE=auto` required for unattended tool execution (approve/chat modes gate/deny tools).
7. API keys via env are first-class (never in config.yaml); keychain vs secrets.yaml only matters for interactive
   `goose configure` flows.

## Raw commands used

```sh
export HOME=/tmp/acpbench/work/goose/home XDG_CONFIG_HOME=$HOME/.config \
  XDG_DATA_HOME=$HOME/.local/share XDG_STATE_HOME=$HOME/.local/state
export GOOSE_PROVIDER=openai GOOSE_MODEL=fake-tools OPENAI_HOST=http://127.0.0.1:18300 \
  OPENAI_API_KEY=fake-key-for-bench GOOSE_MODE=auto
/tmp/acpbench/bin/acpbench --cmd '/tmp/acpbench/work/goose/app/goose acp' \
  --env GOOSE_PROVIDER=openai --env GOOSE_MODEL=fake-tools \
  --env OPENAI_HOST={{FAKE_LLM_URL}} --env OPENAI_API_KEY=fake-key-for-bench --env GOOSE_MODE=auto \
  --mcp-stub --fake-llm-url http://127.0.0.1:18300 --scenario all --report goose-all.json
# manual TOOL-directive turns / soak / cancel / RAM:
/tmp/acpbench/scripts/worker3/w3turn.py / w3cancel.py / w3soak.sh / w3ram.py  (W3_ARGV/W3_PROMPT env driver)
```
