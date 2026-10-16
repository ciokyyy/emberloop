# fast-agent (Python) — bench results

**Verdict: STRONGEST of the three — cancel works, second-prompt queueing works, set_config_option accepted, any `generic.<id>` passes through, no login gate. Weaknesses: slowest startup (~14–19 s, flaky past 30 s under load) and highest RAM (~170–195 MB/session). Plus an upstream packaging bug in the exact pinned artifact.**

- Pinned artifact: PyPI `fast-agent-acp==0.10.1` — **wheel is metadata-only (no `fast_agent_acp` module); its `fast-agent-acp` console script crashes with `ModuleNotFoundError`**. Real code is `fast-agent-mcp==0.10.1`; working entrypoint is `fast-agent acp` (wrapper for `serve --transport acp`). Upstream packaging bug, reported below.
- Install: `python3 -m venv` (CPython 3.12.14 standalone, sha256-verified) under `/tmp/acpbench/work/fast-agent/venv`; `pip install fast-agent-acp==0.10.1` (pulls fast-agent-mcp==0.10.1).
- Version: fast-agent v0.10.1. Size on disk: 306 MB total (venv 304 MB).
- sha256: `fast_agent_acp-0.10.1-py3-none-any.whl` = `61f970f5…8837b`; `fast_agent_mcp-0.10.1-py3-none-any.whl` = `0ca29103…4037f`.
- Start cmd (working): `fast-agent --home <dir> acp -x --model generic.fake-py-small` (`-x` = local shell/filesystem runtime; `--no-update-check` suppresses a PyPI version check).

## Provider → fakellm config (verified, model id seen at fake)

`--home <dir>/fast-agent.yaml` (relocatable; secrets optionally in `fast-agent.secrets.yaml`):

```yaml
generic:
  base_url: "http://127.0.0.1:18400/v1"
  api_key: "fake-key-for-bench"
  default_model: "fake-py-small"
```

(`GENERIC_BASE_URL` env also overrides base_url; generic api key falls back to `"ollama"` if unset — fakellm accepts any key.) fakellm log confirmed `POST /v1/chat/completions model=fake-py-small` and `model=fake-custom-1` with `acpbench-stub__echo/__slow` in tools.

## Scenario results (`acpbench --scenario all --mcp-stub`, extended timeouts)

| scenario | verdict | latency | notes |
|---|---|---|---|
| initialize | ok | 14–19 s | protocolVersion 1; caps: loadSession, list/resume, MCP http+sse, image, embeddedContext; authMethods=[fast-agent-ai-secrets]; NO steering |
| new_session | ok | ~19 s | stub tools namespaced `acpbench-stub__echo` at LLM; no `_x.ai/mcp_initialized` |
| prompt_text | wrong-result | ~21 s | `end_turn`, fakellm `OK`, no tool call (probe/fake limitation) |
| cancel | ok | 18 s | cancelled in `streaming` phase; session reusable (`reuseStop end_turn`); MANUAL SLOW:15000+cancel@5s → `cancelled` in 5.0 s |
| steer_followup | ok | 20 s | steering not advertised; 2nd concurrent prompt `queued`, then `end_turn` |
| load_resume | ok | 53 s | `session/resume` used; next prompt `end_turn` |
| set_model_config | ok* | 32 s | *first run protocol-error (init timeout under load — see Flakiness). `session/set_model` rejects unknown ids, **accepts valid** `generic.*` ids (manual proof); `session/set_config_option` **accepted** (even arbitrary key) |
| images | ok | 17 s | `accepted`, `end_turn` (no capability config needed) |
| permission | skipped | — | no ask on tool-less turn; MANUAL TOOL: turn → permission asked for `acpbench-stub/echo`, approved `allow_once`, full loop done (3 fakellm requests) |
| crash_recovery | skipped | — | `raced` |
| concurrency (N=3) | ok | 33 s | 3/3 multi + 3/3 single; marginal ≈157 MB |
| soak (5) | wrong-result | 8 s | 0 hangs; same tool-call expectation issue |

Soak-50: 50/50 turns, 0 hangs, 0 crashes, p50 ≈210 ms / p95 ≈393 ms per warm turn (all `wrong-result` for the probe/fake reason only).

## RAM (lock-held; heavy machine load — order-of-magnitude)

| phase | RSS |
|---|---|
| startup-init / idle-after-initialize | ~195 MB (1 proc) |
| idle-after-new-session / post-turn | ~200–212 MB (2 procs incl. stub) |
| concurrency N=1 peak | 213 MB, ok |
| concurrency N=5 peak | 1059 MB (10 procs), 10/10 turns ok → marginal ≈173 MB/session |
| concurrency N=10 peak | 2133 MB (20 procs), 20/20 turns ok → marginal ≈194 MB/session |

Startup (time-to-initialize): 13–19 s typical. Time-to-first-chunk: ~0 ms.

## Flakiness

- 3 slow-start flakes under machine load (load avg ~18, other workers' agents running): `set_model_config` once + `soak` twice failed at session start (~35 s ≈ init timeout), plus 1 bare-`initialize` timeout-hang and 1 init timeout on an unlocked manual run. All recovered with `ACPBENCH_INIT_TIMEOUT=120s`. Root cause is environmental (CPU starvation of a heavy Python import), but it shows fast-agent's ~15 s startup has the least headroom of the three against the 30 s probe budget.
- Apart from starts: 50-turn soak 100% completion, 0 hangs/crashes; 20/20 concurrency turns at N=10; cancel always clean.

## Catalog / providers

- Custom provider WITHOUT rebuild: yes — `fast-agent.yaml` `<provider>:` section (`base_url`, `api_key`, `default_model`, `default_headers`) or env (`GENERIC_BASE_URL`); model picked via `--model generic.<id>` / config default. No validation against any catalog.
- Custom model list: yes, trivially — **any `generic.<id>` is accepted and forwarded verbatim** (proven `fake-custom-1`). `session/new` carries NO model list; `session/set_model` accepts valid ids; `session/set_config_option` accepted.
- Remote catalog fetch: none on the generic path (local `model_database.py`; OpenRouter/HF lookups exist for those providers only). Only egress seen: PyPI update check (disabled with `--no-update-check`).
- Built-in providers: 22 in the `Provider` enum (anthropic, anthropic-vertex, deepseek, zai, moonshot, fast-agent, generic, googleoai, google, openai, openrouter, tensorzero, azure, aliyun, hf, xai, metaai, bedrock, groq, codexresponses, responses, openresponses).
- fakellm `/v1/models` custom ids: usable directly as `generic.<id>`.

## Subscription logins (source + `auth provider` CLI surface only, no login)

`fast-agent auth provider login` supports OAuth/token login for **xAI (SuperGrok) and Codex (ChatGPT subscription)**; HuggingFace hub token fallback; API keys via `fast-agent.secrets.yaml`/env; keyring storage (keyring backend was unavailable in this sandbox). Of the six: SuperGrok YES, ChatGPT sub YES, Antigravity NO, OpenCode Go/Zen NO, Claude sub NO, Copilot NO.

## Gotchas for a Go Daemon integration

1. Use `fast-agent acp`, NOT the `fast-agent-acp` script (broken in 0.10.1).
2. Always pass `--home <per-user dir>` (contains agent-cards/, sessions/, fast-agent.yaml) and `--no-update-check`; default home resolution otherwise touches user dirs.
3. `-x` enables the local shell/filesystem toolset; `--prefer-local-shell` keeps shell local instead of ACP-client terminal; `--no-permissions` disables permission prompts (daemon auto-approves anyway).
4. Slowest startup (~15–19 s) and fattest sessions (~170–195 MB marginal); budget timeouts/RAM accordingly. Session history persists under `--home/sessions` (288 KB for ~10 sessions — small, but grows; plan rotation).
5. `session/new` has no model list — the daemon must know model ids out-of-band; switching via `session/set_model` (valid ids) or `set_config_option`.
6. Second concurrent prompt is queued, not rejected — good for a daemon that may double-send.

## Raw commands used

```
$PY -m venv /tmp/acpbench/work/fast-agent/venv
.../venv/bin/pip install 'fast-agent-acp==0.10.1'
HOME=.../home /tmp/acpbench/bin/acpbench --cmd '.../venv/bin/fast-agent --no-update-check --home .../home acp -x --model generic.fake-py-small' --cwd .../cwd --mcp-stub --scenario all --report fa-all.json
ACPBENCH_INIT_TIMEOUT=120s ACPBENCH_CTL_TIMEOUT=60s ... --scenario soak --iterations {20,50} ... ; ... --scenario concurrency --sessions {1,5,10} ...
python3 -I /tmp/acpbench/scripts/pyworker/acp_turn.py --cmd '... acp -x --model generic.fake-custom-1' ... --prompt '...TOOL:acpbench-stub__echo:{...}' [--set-model X] [--cancel-after S]
```
