# codex-acp @ 2.2.2 — benchmark result (2026-10-14, worker: node)

**Verdict: strong fit (best of the three).** Zero-config client-driven routing
(`providers/set` with no auth, no files), full cancel/steer/resume/set_mode
support, ChatGPT-sub + custom gateway, 262 MB/session marginal. Only caveats:
needs an `authenticate` call the probe never sends (env bypass exists), and
`session/set_model` accepts only listed Codex ids.

Version: `@agentclientprotocol/codex-acp@2.2.2`
(npm integrity `sha512-vNdeNV3jxbqaH7MYvVWzyB6cMOz+YGIV9W1xyBDsQfoii92DulZLM+ahCL2T2G6uohlaB1Yma347p2kLfAab2w==`,
matches survey). Install: `npm install --prefix /tmp/acpbench/work/codex-acp/pkg
@agentclientprotocol/codex-acp@2.2.2` into isolated dir, isolated
`HOME=/tmp/acpbench/work/codex-acp/home`. Size on disk: **445 MB**
(427 MB is `@openai/codex` SDK + bundled codex binary). Start:
`node .../codex-acp/dist/index.js` (stdio).

## Provider = fakellm (verified)

`CODEX_API_KEY=fake-test-key`,
`CODEX_CONFIG={"model_provider":"custom-gateway","model":"gpt-5-fake","model_providers":{"custom-gateway":{"name":"fakellm","base_url":"http://127.0.0.1:18200/v1","wire_api":"responses"}}}`,
`MODEL_PROVIDER=custom-gateway`, `NO_BROWSER=1`,
`DEFAULT_AUTH_REQUEST={"methodId":"api-key"}`.
fakellm JSONL confirms `POST /v1/responses stream model=gpt-5-fake` with tools
including `mcp__acpbench_stub` (client MCP servers ARE forwarded to the model).
Second routing also verified: **no env/config at all** —
`initialize → providers/set {providerId:openai, apiType:openai,
baseUrl:fakellm/v1} → session/new → prompt** hits fakellm (model gpt-6.1-sol).

## Scenario results (`acpbench --mcp-stub`, report `codex-func.json`)

| Scenario | Verdict | Latency | Notes |
|---|---|---|---|
| initialize | ok | 1.8–2.5 s | caps: loadSession, subagents, close/fork/list/resume/delete, modes, image+embeddedContext, MCP http+sse+acp-stdio; steering advertised; authMethods=[api-key] |
| new_session | ok | 6.5–8 s | mcpReady not-advertised (5 s wait); MCP tools still reach model |
| prompt_text | wrong-result | ~9 s | PROBE+FAKE ARTIFACT: model replies OK, no tool call (see §4); end_turn, 11 updates, TTFC 24–31 ms |
| cancel | ok 6/6 | ~9 s | streaming phase → `cancelled`, session reusable (5 repeat runs 5/5) |
| steer_followup | ok | 3.3 s | `injected` via `_session/steering` |
| load_resume | ok | 17 s | `session/resume` **used** cross-process |
| set_model_config | ok* | 8 s | *probe shape wrong (`model` vs spec `modelId`); manual `modelId: gpt-5.5[low]` → ACCEPTED |
| images | ok | 9 s | accepted (1×1 PNG) |
| permission | skipped | — | agent never asks on fake turns (honest skip) |
| crash_recovery | ok | det. 11 ms | SIGKILL detected fast |
| concurrency N=1 | ok | 19 s | 1/1 multi + 1/1 single |
| concurrency N=5 | ok | 62 s | 5/5 multi + 5/5 single |

## RAM (VmRSS summed over process tree)

| Phase | RSS |
|---|---|
| startup-init | 229 MB (11 procs) |
| idle-after-initialize | 236 MB |
| idle-after-new-session | 240–263 MB |
| prompt-turn peak | 287 MB |
| post-turn | 217 MB |
| 5 parallel peak | 1573 MB (30 procs) |
| **marginal/session (N=5)** | **262 MB** (274,694,144 B) |

N=1 peak 296 MB. N=10 not run (box only had ~2 GB free; OOM risk to others).

## Flakiness

soak50 (`--iterations 50`, one session): 50/50 turns completed, **0 hangs,
0 crashes**, p50 6.6 s / p95 7.0 s per turn (verdict wrong-result ×50 is the
tool-call artifact, not instability). Cancel 6/6. No flakes observed all day.

## Catalog / providers

- `session/new` lists **40+ models** (`gpt-6.1-sol/-astra/-sol/-luna,
  gpt-5.6-sol/-terra/-luna, gpt-5.5` × `[low…ultra]`); ids are
  `modelId[effort]`. `currentModelId` follows `CODEX_CONFIG.model`
  (`gpt-5-fake[medium]`).
- Model list is **bundled** (strings present in codex binary; served with fake
  key, no OpenAI fetch) → updates ride codex releases, host cannot override
  except via `set_model`/config `model`.
- Custom provider WITHOUT rebuild: `CODEX_CONFIG` JSON env
  (`model_providers.<id> = {name, base_url, http_headers?, wire_api:
  "responses"}` + `model_provider`/`model`) — or `~/.codex/config.toml`, or
  zero-config `providers/set` at runtime (per providerId slot; `usage: null`
  on gateway path).
- Custom model ids: via config `model` (any string passed through to base_url)
  YES; via `session/set_model` only listed ids (arbitrary → `Unsupported
  format of modelId`).
- Built-in providers: `openai` slot (default `https://api.openai.com/v1`) +
  ChatGPT OAuth + unbounded custom gateways. No remote catalog fetch.
- `session/set_config_option`: not implemented (`Invalid params`); modes via
  `session/set_mode` (read-only accepted); `providers/disable` exists.

## Subscriptions (docs + observed, no login attempted)

ChatGPT subscription YES (browser OAuth, `NO_BROWSER=1` hides method);
API key YES (`CODEX_API_KEY`/`OPENAI_API_KEY`, persisted to
`$CODEX_HOME/auth.json` as `{"auth_mode":"apikey","OPENAI_API_KEY":...}` —
key at rest in JSON). Custom gateway YES. SuperGrok / Antigravity /
OpenCode Go / Claude sub / Copilot: NO.

## Gotchas for the Go Daemon

1. Client MUST send `authenticate` (or set `DEFAULT_AUTH_REQUEST`); otherwise
   `session/new` → `Authentication required`.
2. `session/set_model` params are `{sessionId, modelId}` with
   `modelId[effort]` format — the probe sends `model` (probe bug §4).
3. Codex binary warns `Refusing to create helper binaries under temporary dir
   "/tmp"` when CODEX_HOME is under /tmp (benign).
4. Per-session sqlite/rollout logs accumulate in CODEX_HOME (disk growth).
5. `providers/set` routing is per-connection client state; `adoptRoutingFrom`
   copies it across app-server reconnects.
6. Turns take ~7–9 s even against an instant model (app-server overhead).

## Raw commands

```sh
FAKE=http://127.0.0.1:18200
/tmp/acpbench/bin/fakellm --addr 127.0.0.1:18200 --log /tmp/acpbench/work/fakellm-18200.jsonl &
npm install --prefix /tmp/acpbench/work/codex-acp/pkg @agentclientprotocol/codex-acp@2.2.2
export HOME=/tmp/acpbench/work/codex-acp/home XDG_CONFIG_HOME=$HOME/.config \
  XDG_DATA_HOME=$HOME/.local/share XDG_CACHE_HOME=$HOME/.cache
CCFG='{"model_provider":"custom-gateway","model":"gpt-5-fake","model_providers":{"custom-gateway":{"name":"fakellm","base_url":"'$FAKE'/v1","wire_api":"responses"}}}'
/tmp/acpbench/bin/acpbench --cmd 'node /tmp/acpbench/work/codex-acp/pkg/node_modules/@agentclientprotocol/codex-acp/dist/index.js' \
  --cwd /tmp/acpbench/work/_proj --mcp-stub --scenario all \
  --env CODEX_API_KEY=fake-test-key --env "CODEX_CONFIG=$CCFG" --env MODEL_PROVIDER=custom-gateway \
  --env NO_BROWSER=1 --env 'DEFAULT_AUTH_REQUEST={"methodId":"api-key"}' --fake-llm-url $FAKE
# manual checks: /tmp/acpbench/scripts/nodeworker/acp_raw.py (set_model/set_mode),
# acp_provset.py (providers/set zero-config routing)
```

Reports: `/tmp/acpbench/work/codex-{func,soak50,conc1,conc5}.json`.
