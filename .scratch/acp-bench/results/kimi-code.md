# kimi-code (Python) — bench results

**Verdict: USABLE with caveats — best ACP model-list support of the three, but ACP sessions are hard-gated on Kimi OAuth token presence, startup is slow (~6-9 s), and per-session RAM is ~130-150 MB.**

- Pinned artifact: PyPI `kimi-code==1.50.0` (alias shim, depends on `kimi-cli==1.50.0`, the real agent). NOTE: PyPI `kimi-code>=1.51.0` is a tombstone ("[Archived] Legacy PyPI package"); 1.50.0 is the last real Python release. The new TS Kimi Code CLI (MoonshotAI/kimi-code, npm `@moonshot-ai/kimi-code@2.x`) is a different artifact and was NOT tested.
- Install: `python3 -m venv` (CPython 3.12.14 standalone, astral-sh/python-build-standalone tag 20260924, sha256 verified `269b2c99…0b41`) under `/tmp/acpbench/work/kimi-code/venv`; `pip install kimi-code==1.50.0`.
- Version: `kimi --version` → 1.50.0. Size on disk: 343 MB total (venv 335 MB).
- sha256: `kimi_code-1.50.0-py3-none-any.whl` = `2511edde…aa30957`; `kimi_cli-1.50.0-py3-none-any.whl` = `0341d283…131b0b7e7`.
- Start cmd: `kimi acp` (also deprecated `kimi --acp`). Needs `HOME`+`KIMI_SHARE_DIR` isolated; `telemetry=false` in config (default true phones home).

## Provider → fakellm config (verified, model id seen at fake)

`$KIMI_SHARE_DIR/config.toml` (default `~/.kimi/config.toml`; relocatable via `KIMI_SHARE_DIR`, or `--config-file`):

```toml
telemetry = false
default_model = "fake-test"
[providers.fake]
type = "openai_legacy"   # or openai_responses / anthropic / gemini / vertexai / kimi
base_url = "http://127.0.0.1:18400/v1"
api_key = "fake-key-for-bench"
[models.fake-test]
provider = "fake"
model = "fake-py-small"  # any id; forwarded verbatim (proven fake-custom-1 too)
max_context_size = 200000
capabilities = ["image_in"]
```

Env overrides also work for openai types: `OPENAI_BASE_URL`/`OPENAI_API_KEY` (and `KIMI_BASE_URL`/`KIMI_API_KEY`/`KIMI_MODEL_NAME` for type `kimi`). fakellm log confirmed `POST /v1/chat/completions model=fake-py-small stream=true` with stub tools `echo,slow` in the tool list.

## Scenario results (`acpbench --scenario all --mcp-stub`, plus reruns)

| scenario | verdict | latency | notes |
|---|---|---|---|
| initialize | ok | 5.6–8.8 s | protocolVersion 1; caps: loadSession, list/resume sessions, MCP http (no sse), image, embeddedContext; authMethods=[login terminal-auth]; NO steering |
| new_session | ok | 11.1 s | MCP stub forwarded (tools visible at LLM); no `_x.ai/mcp_initialized` (`not-advertised`) |
| prompt_text | wrong-result | 11.7 s | turn completes `end_turn` but fakellm answers natural-language prompt with `OK`, no tool call (probe/fake limitation, not agent bug) |
| cancel | skipped | — | turn faster than cancel window (`raced`); MANUAL SLOW:15000+cancel@4s → `cancelled`, returned in 4.0 s |
| steer_followup | skipped | — | `raced`; steering not advertised |
| load_resume | ok | 24.8 s | `session/resume` used; next prompt `end_turn` |
| set_model_config | ok | 6.8 s | `session/set_model` REJECTS unknown ids (`Invalid params`); **accepts config-defined ids** (manual: set `fake-second` → next turn used `fake-custom-1`); `session/set_config_option` unsupported (`Method not found`) |
| images | ok* | 5.9 s | *only after adding `capabilities=["image_in"]`; without it → protocol-error (LLMNotSupported). `accepted`, `end_turn` |
| permission | skipped | — | no permission asked on tool-less turn; MANUAL TOOL: turn → permission asked for `echo`, approved, `tool_call`+`tool_call_update`, `end_turn` (full MCP loop proven, 3 fakellm requests: call→result→DONE) |
| crash_recovery | skipped | — | `raced` (turn finished before SIGKILL) |
| concurrency (N=3) | ok | 47 s | 3/3 multi + 3/3 single; marginal ≈156 MB |
| soak (5) | wrong-result | 11 s | same tool-call expectation issue; 0 hangs |

Soak-50 (`--iterations 50`): 50/50 turns completed, 0 hangs, 0 crashes, p50 ≈90 ms / p95 ≈158 ms per warm turn (all `wrong-result` only for the no-tool-call reason).

## RAM (RSS summed per process tree; lock-held; machine load was high — order-of-magnitude)

| phase | RSS |
|---|---|
| startup-init / idle-after-initialize | ~100 MB (1 proc) |
| idle-after-new-session / post-turn | ~150–153 MB (2 procs incl. stub) |
| concurrency N=1 peak | 151 MB, ok |
| concurrency N=5 peak | 761 MB (12 procs), 10/10 turns ok → marginal ≈130 MB/session |
| concurrency N=10 peak | 1398 MB (20 procs), 20/20 turns ok → marginal ≈130 MB/session |

Startup (time-to-initialize): 5.6–8.8 s. Time-to-first-chunk: ~0 ms (streams immediately).

## Flakiness

50-turn soak: 100% turn completion, 0 hangs, 0 crashes. No init timeouts seen (6–9 s starts, well under the 30 s probe budget). Cancel: clean `cancelled`, no hang.

## Catalog / providers

- Custom provider WITHOUT rebuild: yes — add `[providers.<name>]` (keys: `type`, `base_url`, `api_key`, optional `env`, `custom_headers`, `reasoning_key`, `oauth`) + `[models.<alias>]` (keys: `provider`, `model`, `max_context_size`, `capabilities`, `display_name`) to `config.toml`; or env overrides. No restart of anything else needed.
- Custom model list: yes — `session/new` returns `availableModels` from local config (proven: `fake-test`+`fake-second` listed with display names); `session/set_model` switches between them live (and persists `default_model` to config file).
- Remote catalog fetch: none for the model list (purely local file). `display_name` may be enriched from the provider's own `/models` (not verified). Telemetry (default ON) and OAuth refresh are the only other egress.
- Built-in providers: 6 `ProviderType` values (`kimi`, `openai_legacy`, `openai_responses`, `anthropic`, `gemini`/`google_genai`, `vertexai`); 3 managed login platforms (Kimi Code OAuth, Moonshot moonshot.cn, Moonshot moonshot.ai). Unlimited custom entries of the non-`kimi` types.
- fakellm `/v1/models` custom ids: accepted and forwarded verbatim (proven `fake-py-small`, `fake-custom-1`).

## Subscription logins (source/docs only, no login attempted)

Kimi Code OAuth (`kimi login`, browser flow; token file `$KIMI_SHARE_DIR/credentials/kimi-code.json`) and Moonshot Open Platform API keys (moonshot.cn / moonshot.ai). Of the six: SuperGrok NO, ChatGPT sub NO, Antigravity NO, OpenCode Go/Zen NO, Claude sub NO, Copilot NO.

## Gotchas for a Go Daemon integration

1. **OAuth gate**: `session/new|load|resume` raise `AUTH_REQUIRED` unless a Kimi OAuth token file exists and is unexpired (`_check_token_usable`), EVEN when the active model uses an API-key provider. Bench used a dummy local token (fake strings, far-future expiry, isolated dir) — a real deployment needs `kimi login`. This is the single biggest integration blocker.
2. **Auto-compaction footgun**: with `max_context_size` (32k) ≤ `reserved_context_size` (default 50k), auto-compaction fires on turn 1 and silently drops the just-appended user message (system-only LLM call, vacuous `OK` turn, or `APIEmptyResponseError` retries). Always configure `max_context_size` well above 50k (200k used here). Arguably an agent wart: compaction should never nuke the current user message.
3. `capabilities` must declare `image_in` or image turns hard-fail; `thinking`/`always_thinking` alter reasoning behavior.
4. `set_model` persists `default_model` to the config file (side effect across sessions).
5. No `session/set_config_option`, no steering, no MCP-sse, no `_x.ai/mcp_initialized` advertisement (harmless — tools still flow).
6. Slow startup (6–9 s Python); ~130–150 MB per session process; fine for few sessions, heavy for dozens.

## Raw commands used

```
PY=/tmp/acpbench/work/_py312/python/bin/python3; $PY -m venv /tmp/acpbench/work/kimi-code/venv
/tmp/acpbench/work/kimi-code/venv/bin/pip install kimi-code==1.50.0
HOME=.../home KIMI_SHARE_DIR=.../home/.kimi /tmp/acpbench/bin/acpbench --cmd '.../venv/bin/kimi acp' --cwd .../cwd --mcp-stub --scenario all --report kimi-all.json
... --scenario soak --iterations 50 ... ; ... --scenario concurrency --sessions {1,5,10} ...
... --scenario images ... (after capabilities fix)
python3 -I /tmp/acpbench/scripts/pyworker/acp_turn.py --cmd '.../kimi acp' --cwd .../cwd --prompt '...TOOL:echo:{...}' [--set-model X] [--cancel-after S]
/tmp/acpbench/bin/fakellm --addr 127.0.0.1:18400 --log /tmp/acpbench/fakellm-py.jsonl --models .../models.json
```
