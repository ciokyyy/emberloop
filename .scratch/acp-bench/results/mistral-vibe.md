# mistral-vibe (Python) — bench results

**Verdict: VIABLE and leanest (fastest init ~4–5 s, ~100–130 MB/session), but with a critical harness caveat: only `--legacy-harness` forwards client `mcpServers`; the default unified harness does NOT. Full MCP tool loops additionally hit two fakellm strictness gaps (reported, not agent bugs). Model switching via `set_config_option(model=alias)` works.**

- Pinned artifact: PyPI `mistral-vibe==2.26.1` (binary wheel `mistral_vibe-2.26.1-cp312-abi3-manylinux_2_28_x86_64.whl`).
- Install: `python3 -m venv` (CPython 3.12.14 standalone, sha256-verified) under `/tmp/acpbench/work/mistral-vibe/venv`; `pip install mistral-vibe==2.26.1`.
- Version: 2.26.1 (`vibe-acp --version`). Size on disk: 457 MB total (venv 390 MB).
- sha256: wheel = `1ceb43e3…f58704133`.
- Start cmd: `vibe-acp [--legacy-harness]` with `VIBE_HOME` isolated and `enable_telemetry=false` (telemetry default true, incl. Sentry).

## Provider → fakellm config (verified, model id seen at fake)

`$VIBE_HOME/config.toml` (default `~/.vibe/config.toml`; relocatable via `VIBE_HOME`; layers merge `[[providers]]` by `name`, `[[models]]` by `alias`):

```toml
enable_telemetry = false
active_model = "fake-alias"
[[providers]]
name = "fake"
api_base = "http://127.0.0.1:18400/v1"
api_key_env_var = "FAKE_API_KEY"   # env var NAME holding the key
api_style = "openai"               # or anthropic / openai-responses / reasoning / vertex-anthropic
[[models]]
name = "fake-py-small"             # id sent to the provider; any custom id works
provider = "fake"
alias = "fake-alias"               # id used by active_model + set_config_option
supports_images = true
max_context_length = 200000
```

API key via env var named in `api_key_env_var` (here `FAKE_API_KEY=fake-key-for-bench`). fakellm log confirmed `POST /v1/chat/completions model=fake-py-small` and (after switch) `model=fake-custom-1`. NOTE: `api_style="anthropic"` appends `/v1/messages` to `api_base`, so that style needs `api_base` WITHOUT the `/v1` suffix.

## Scenario results (`vibe-acp --legacy-harness`, api_style openai, `--mcp-stub`)

| scenario | verdict | latency | notes |
|---|---|---|---|
| initialize | ok | 4.3–10 s | protocolVersion 1; caps: loadSession, close/fork/list sessions, image, embeddedContext; authMethods=[]; NO steering; NO MCP caps section |
| new_session | ok | ~10 s | stub forwarded by LEGACY harness (tools `acpbench-stub_echo/_slow` at LLM); no `_x.ai/mcp_initialized` |
| prompt_text | wrong-result | 4.2 s | `end_turn`, fakellm `OK`, no tool call (probe/fake limitation) |
| cancel | skipped | — | `raced`; MANUAL SLOW:15000+cancel@4s → `cancelled` in 4.0 s |
| steer_followup | skipped | — | `raced`; steering not advertised |
| load_resume | ok | 7.4 s | `session/resume` used; next prompt `end_turn` |
| set_model_config | ok | 3.5 s | NO `session/set_model` (`Method not found`); `session/set_config_option` allowlisted to `mode`/`model`/ `thinking`/`max_turns`/`max_tokens`. **Model switch proven**: `set_config_option(model=fake-custom)` → next turn called fakellm with `model=fake-custom-1` |
| images | ok | 4.6 s | `accepted`, `end_turn` (config had `supports_images=true`) |
| permission | skipped | — | no ask on tool-less turns (approval behavior on real tool calls untested — see fakellm gaps) |
| crash_recovery | skipped | — | `raced` |
| concurrency (N=3) | ok | 21.6 s | 3/3 multi + 3/3 single; marginal ≈98 MB |
| soak (5) | wrong-result | 6.2 s | 0 hangs; same tool-call expectation issue |

Soak-50: 50/50 turns, 0 hangs, 0 crashes, p50 ≈56 ms / p95 ≈81 ms per warm turn.

## RAM (lock-held; heavy machine load — order-of-magnitude)

| phase | RSS |
|---|---|
| startup-init / idle-after-initialize | ~96 MB (1 proc) |
| idle-after-new-session / post-turn | ~129–136 MB (1 proc) |
| concurrency N=1 peak | 246 MB, ok |
| concurrency N=5 peak | 683 MB (6 procs), 10/10 turns ok → marginal ≈117 MB/session |
| concurrency N=10 peak | 1369 MB (10 procs), 20/20 turns ok → marginal ≈127 MB/session |

Startup: 4.3–5 s typical (fastest of the three). TTFC: ~0 ms (one `timeToFirstChunkMs: -6` clock artifact in prompt_text).

## Flakiness

50-turn soak 100% completion, 0 hangs/crashes; 20/20 concurrency turns at N=10; manual cancel clean. No init timeouts (init budget never stressed). Most stable starter of the three.

## Manual MCP/tool-loop findings (beyond the probe)

- Default (unified) harness does NOT forward `session/new` `mcpServers`: its LLM request listed only built-in tools (`search_tool_functions`, `run_typescript`, …, `unified_harness_scratchpad`, `cron`; UA `python-httpx`). Legacy harness DOES (`acpbench-stub_echo`, `acpbench-stub_slow`; UA `Mistral-Vibe`). **A daemon MUST pass `--legacy-harness`** (temporary escape hatch per `--help` — confirm it survives the "legacy runtime cutover").
- Legacy + `api_style=openai` + directed `TOOL:acpbench-stub_echo:` turn: fakellm emitted the tool call, vibe hard-failed with `Tool call chunk missing index` — fakellm's streamed (and non-streamed) `tool_calls` entries omit the per-call `index` that real OpenAI sends. PROBE/FAKE BUG (report below), not an agent bug.
- Legacy + `api_style=anthropic` + directed turn: vibe emitted ACP `tool_call` + `tool_call_update` (tool delegation PROVEN), then failed the turn on `Usage data missing in final chunk of streamed completion` — fakellm's `streamAnthTools` final `message_delta` carries no `usage` (real Anthropic does). PROBE/FAKE BUG (report below).
- Net: vibe is the strictest client of the three (kimi/fast-agent tolerate both gaps); with the two one-line fakellm fixes, full tool loops should complete.

## Catalog / providers

- Custom provider WITHOUT rebuild: yes — append `[[providers]]` (keys: `name`, `api_base`, `api_key_env_var`, `api_style`, `backend`, `reasoning_field_name`, `emits_finish_reason`, `extra_headers`, …) + `[[models]]` (keys: `name`, `provider`, `alias`, `temperature`, `thinking`, `supports_images`, `max_context_length`, prices, …) to `$VIBE_HOME/config.toml`, pin with `active_model = "<alias>"`. Union-merge by name/alias; no rebuild.
- Custom model list: yes — arbitrary `name` ids accepted and forwarded verbatim (proven `fake-custom-1`); switchable live via `set_config_option(model=<alias>)`. `session/new` carries NO model list; there is no `session/set_model`.
- Remote catalog fetch: no fetch of the model list at runtime (local `model_catalog/` + `model_catalog_cache.json`); CAVEAT from source: a GrowthBook config layer can inject `routed_default_model` at runtime when `active_model` is unpinned — pin `active_model` to keep the host in control. Only other egress: telemetry/Sentry (disabled) and console sign-in.
- Built-in providers: 2 (`mistral` with `MISTRAL_API_KEY`/browser sign-in, `llamacpp` at `127.0.0.1:8080`).
- fakellm `/v1/models` custom ids: usable directly as `[[models]] name`.

## Subscription logins (source/docs only, no login attempted)

`MISTRAL_API_KEY` env (or per-provider `api_key_env_var`) and Mistral browser sign-in (console.mistral.ai; token cached under `VIBE_HOME`); MCP-server OAuth is separate (`vibe/core/auth/mcp_oauth.py`). Of the six: SuperGrok NO, ChatGPT sub NO, Antigravity NO, OpenCode Go/Zen NO, Claude sub NO, Copilot NO.

## Gotchas for a Go Daemon integration

1. `--legacy-harness` is REQUIRED for `mcpServers` forwarding — and it is documented as a "temporary escape hatch". Re-verify every upgrade; if removed, vibe becomes unusable for the daemon's per-session MCP design.
2. No `session/set_model`, no steering, no authMethods, no MCP caps advertisement; model control is `set_config_option(model=<alias>)` only.
3. Strict response parsing: budget for provider responses that omit OpenAI/Anthropic optional-but-customary fields (index, usage) — fine against real APIs, brittle against minimal fakes.
4. Leanest resource profile (~96 MB init, ~120 MB/session, 4–5 s start) — best fit for many-session daemons.
5. Config layers (user/project/admin/GrowthBook) merge — a project `.vibe/config.toml` or GrowthBook routing could surprise the daemon; pin `active_model` and prefer explicit `$VIBE_HOME`.
6. `vibe-acp` writes logs/sessions under `VIBE_HOME` (logs/, plans/, model_catalog_cache.json, whoami_cache.json) — per-user isolation falls out of `VIBE_HOME`, but plan rotation.

## Raw commands used

```
$PY -m venv /tmp/acpbench/work/mistral-vibe/venv
.../venv/bin/pip install 'mistral-vibe==2.26.1'
HOME=.../home VIBE_HOME=.../home FAKE_API_KEY=fake-key-for-bench /tmp/acpbench/bin/acpbench --cmd '.../venv/bin/vibe-acp --legacy-harness' --cwd .../cwd --mcp-stub --scenario all --report mv-all.json
ACPBENCH_INIT_TIMEOUT=120s ... --scenario soak --iterations 50 ... ; ... --scenario concurrency --sessions {1,5,10} ...
python3 -I /tmp/acpbench/scripts/pyworker/acp_turn.py --cmd '.../vibe-acp --legacy-harness' ... --prompt '...TOOL:acpbench-stub_echo:{...}' [--set-config model=X] [--cancel-after S]
```
