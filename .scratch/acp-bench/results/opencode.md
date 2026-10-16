# opencode @ 1.19.0 — benchmark result (2026-10-14, worker: node)

**Verdict: conditional fit.** Best custom-provider story (file-based, custom
models surface in ACP `configOptions`), resume + cancel + images all work, and
steady-state turns are fast (p50 ~0.4 s). But: heaviest RAM measured
(**445 MB/session marginal**), slowest cold start (6–26 s/initialize), no
steering advertisement, `set_mode`/MCP-stdio limits. Upstream moved
`sst/opencode` → `anomalyco/opencode`; installed npm artifact is unchanged.

Version: `opencode-ai@1.19.0`
(npm integrity `sha512-lddJhFwZI3GLlw8VyOo4yNwBVGeVN/mxH6ORh60h69zAXL7QhjSWY9orj10rwO2kxVnxH9dh45uDCWNuiqPb8A==`).
Install: `npm install --prefix /tmp/acpbench/work/opencode/pkg
opencode-ai@1.19.0`, isolated `HOME=/tmp/acpbench/work/opencode/home`. Size:
**355 MB** (Bun binary ~178 MB ×2: baseline + regular in `node_modules`).
Start: `opencode acp` (stdio; UA `opencode/1.19.0 ... runtime/bun/1.3.14`).

## Provider = fakellm (verified)

`~/.config/opencode/opencode.json`:
`{provider:{fakellm:{npm:"@ai-sdk/openai-compatible",name:"Fake LLM",
options:{baseURL:"http://127.0.0.1:18201/v1",apiKey:"fake-test-key"},
models:{"fake-small":{name}, "fake-tools":{name}}}}, model:"fakellm/fake-small"}`.
fakellm JSONL confirms `POST /v1/chat/completions stream model=fake-small`
with 12 tools incl. `acpbench-stub_echo`, `acpbench-stub_slow` — the
command-stdio stub IS forwarded despite caps advertising only http+sse.

## Scenario results (`acpbench --mcp-stub`, report `oc-func.json`)

| Scenario | Verdict | Latency | Notes |
|---|---|---|---|
| initialize | ok | 6–14 s | caps: loadSession, close/fork/list/resume, image+embeddedContext, MCP http+sse; NO steering; authMethods=[opencode-login] (no auth needed for key providers) |
| new_session | ok | 15–26 s | mcpReady not-advertised (5 s wait baked in); dominated by cold start |
| prompt_text | wrong-result | ~20 s | PROBE+FAKE ARTIFACT (no tool call; text `OK`); TTFC 16–30 ms |
| cancel | ok 4/5 | ~22–39 s | streaming → `cancelled`, reusable; 1 transient protocol-error under box load |
| steer_followup | ok | 18 s | no steering → 2nd prompt `queued`, both end_turn |
| load_resume | ok | 19 s | `session/resume` **used** (`ses_*` ids) |
| set_model_config | ok* | 17 s | *probe shape wrong; manual `modelId: fakellm/fake-tools` → ACCEPTED |
| images | ok | 15 s | accepted |
| permission | skipped | — | never asks on fake turns |
| crash_recovery | ok | det. 134 ms | fast fail, no hang |
| concurrency N=1 | ok | 27 s | 1/1 + 1/1 |
| concurrency N=5 | ok | 100 s | 5/5 + 5/5 |

`opencode models fakellm` CLI lists both custom models (config-only merge).

## RAM (VmRSS summed over process tree)

| Phase | RSS |
|---|---|
| idle-after-initialize | 357–360 MB |
| idle-after-new-session | 374–411 MB |
| prompt-turn peak | 524 MB |
| post-turn | 485 MB |
| 5 parallel peak | 2638 MB (10 procs) |
| **marginal/session (N=5)** | **445 MB** (467,086,540 B) |

N=1 peak 514 MB — inside the claimed 500–800 MB band. N=10 not run
(~4.5 GB needed; box had <3 GB free).

## Flakiness / latency

soak 2×25 (one session each): 50/50 completed, **0 hangs, 0 crashes**,
p50 0.36–0.52 s / p95 0.76–0.82 s. Key insight: per-PROCESS cold start
(14–26 s: bun init + provider/catalog load) dominates; steady-state turns are
~0.5 s. Cancel 4/5 (one transient protocol-error, machine loaded at the time).

## Catalog / providers

- Remote catalog: **models.dev** (`opencode models --refresh` refreshes the
  cache); cached at `~/.cache/opencode/models.json` — observed 5.6 MB,
  **229 providers / 8818 models**; custom `fakellm` entry is config-merged, not
  cached. Host override of update = `--refresh` flag / `opencode models` CLI.
- Custom provider WITHOUT rebuild: `opencode.json` `provider.<id>` =
  `{npm:@ai-sdk/openai-compatible, name, options:{baseURL, apiKey},
  models:{<id>:{name}}}` — verified live, incl. appearance in ACP
  `session/set_config_option` model select (11 options: 9 `opencode/*` Zen
  models + 2 fakellm) and `opencode models fakellm`.
- `session/set_model {modelId: provider/model}` ACCEPTED (incl. custom).
  `session/set_config_option` works for real keys (`model` → returns full
  `configOptions`; `mode`); bogus keys → `unknown config option` (correct).
  `session/set_mode` rejected (`mode not found: yolo`) — modes live behind
  the `mode` config option (`build` +1).
- Built-in providers: 229 in cache snapshot; binary references 183
  `*_API_KEY` envs + 19 `@ai-sdk/*` packages.

## Subscriptions (docs + CLI, no login attempted)

Zen subscription via `opencode providers login` (interactive picker, not run);
per-provider API keys via env/config; credentials file
`~/.local/share/opencode/auth.json` (observed: `0 credentials`).
ChatGPT sub / Claude sub / Copilot / SuperGrok / Antigravity / OpenCode Go:
none (Zen is its own thing; `opencode/*` model ids are Zen-gated).

## Gotchas for the Go Daemon

1. Budget ~15–25 s cold start per process; reuse sessions/processes.
2. Advertised MCP caps say http+sse only, but command-stdio servers from
   `session/new` ARE spawned and forwarded (verified); still no
   `_x.ai/mcp_initialized` → every session pays the probe's 5 s mcp wait.
3. No steering advertisement; second prompt queues (fine, but no follow-up
   injection primitive).
4. Writes models cache (MBs, network fetch of models.dev on first/refresh
   run), sqlite db, logs under HOME/XDG — isolate per user.
5. Session ids are `ses_*`; resume works cross-process.
6. 5 parallel sessions ≈ 2.6 GB — heaviest of the three by ~1.7×.

## Raw commands

```sh
FAKE=http://127.0.0.1:18201
/tmp/acpbench/bin/fakellm --addr 127.0.0.1:18201 --log /tmp/acpbench/work/fakellm-18201.jsonl &
npm install --prefix /tmp/acpbench/work/opencode/pkg opencode-ai@1.19.0
export HOME=/tmp/acpbench/work/opencode/home XDG_CONFIG_HOME=$HOME/.config \
  XDG_DATA_HOME=$HOME/.local/share XDG_CACHE_HOME=$HOME/.cache
# opencode.json as above under $HOME/.config/opencode/
BIN=/tmp/acpbench/work/opencode/pkg/node_modules/.bin/opencode
/tmp/acpbench/bin/acpbench --cmd "$BIN acp" --cwd /tmp/acpbench/work/_proj \
  --mcp-stub --scenario all --fake-llm-url $FAKE
```

Reports: `/tmp/acpbench/work/oc-{func,soak25a,soak25b,conc1,conc5,cancel4,cancel5}.json`.
