# fx 0.0.13 — final bench result (worker 4)

Verdict: **USABLE with caveats** — leanest agent benched (13 MB/session, 38 ms turns);
fast clean surfaces for 529/midstream/truncation; but HTTP 429/500 trigger unbounded
retry storms (no backoff cap surfaced in 18 min), empty faults go SILENT, and provider
stalls surface only via the 90 s client timeout. MCP tools are forwarded namespaced
(`mcp_<server>_<tool>`), which defeats the v2 cooperative prompt (MISSING_TOOL).

## Install / start

- Artifact: GH release `fx-linux-x86_64.tar.gz` (Zig, ~6 MB), `bin/fx`, `fx acp`.
- Isolated HOME `/tmp/acpbench/work/fx/home`; no env keys needed.
- `~/.fx/settings.json`: provider `fakellm` protocol `openai-chat-completions`,
  `base_url http://127.0.0.1:18760/v1`, `model_metadata` for `fake-bench-small`
  (ctx 32768, tools+vision true) / `fake-bench-large`.
- acpbench2 flags: `--mcp-stub --cooperate --fake-llm-url … --auth-method auto
  --env HOME=…` (no `--kill-group` needed; stdio MCP accepted, caps acp+http+sse).

## Scenario results (`--scenario all`)

| scenario | verdict | detail |
|---|---|---|
| initialize | ok 646 ms | proto 1, steering NOT advertised, loadSession, image+embeddedContext |
| new_session | ok 5.9 s | mcpReady no-notification, but stub `tools/list` seen |
| prompt_text | wrong-result | `MISSING_TOOL:echo` (17 B), 1 LLM req, 0 `tools/call` |
| cancel | skipped | raced (fast turns; see slow-fake leg) |
| steer_followup | skipped | raced |
| load_resume | ok | `session/resume` USED |
| set_model_config | wrong-result | tried provider→gateway: needs Vercel AI Gateway (`AI_GATEWAY_API_KEY`); `discoveredModels` null |
| images | ok | accepted (vision metadata true) |
| permission | skipped | skipped (never asks on echo turns) |
| crash_recovery | skipped | raced |
| concurrency N=3 | ok | 3/3 both modes, marginal ~11 MB |
| soak 5 | ok | 5/5, p50 30 ms |

## Soak / cancel-mix / fault soak

- soak 50: **50/50 ok, 0 hangs**, p50 38 ms, p95 63 ms.
- cancel-mix 30 (every 5th): 24 ok + 6 no-slow-turn (TOOL:slow→MISSING_TOOL, slow turns race), 0 cancelled, 0 hangs.
- cancel vs slow fake (`--latency-ms 15000`): **ok** 16.3 s, cancelled mid-streaming, session reusable. Cancel works; protocol skip is a probe artifact.
- fault soak 40 @0.1/seed7 (swap-validated onto fault fake): 38 ok + 2 wrong (1 protocol-error, 1 refused), **0 hangs** (40 reqs, 2 faults fired: http401, sse_error).

## provider_fault matrix (3 reps; FAULT: classes)

| class | result (per rep) |
|---|---|
| http429 | **HANG (retry storm)**: 1155+ retries over ~18 min (Retry-After:1 honored, no cap); 3 bounded reps killed at 170 s (~165 reqs each). Never surfaces. |
| http500 | **HANG (slow-backoff storm)**: 33 reqs over 3×170 s bounded reps (all killed, no report); ~15 s spacing; never surfaces. |
| http529 | clean-error ~50 ms ×3 (`HTTP 502 … overloaded_error`, immediate, no retry) |
| midstream-error (=sse_error) | clean-error ~50–120 ms ×3 |
| reset | retried-success ~380 ms ×3 — surfaces as TOOL error text (`Dynamic MCP tool not found…`), turn completes |
| empty | **silent-empty-turn** ~50–90 ms ×3 (fault swallowed, clean-looking end_turn) |
| stall-5s / stall-forever (stock probe) | DEGRADED readings (fakellm2 maps both to a 500 ms stall): retried-success ~540–640 ms. See TRUE below. |
| truncate (stock probe) | VOID (fault never fires — fakellm2 has no `truncate` alias): `truncated-unflagged` artifact, agent behaved fine. |

TRUE stalls (by-counter fakes + void-inline class, per-rep fresh fake):

| class | result |
|---|---|
| hang (infinite hold) | **timeout-hang @90 s ×3** (`session/prompt timed out after 1m30s`; agent never gives up itself) |
| truncated body | clean-error `acp: IncompleteStream` ~50 ms ×3 |
| 5 s TTFB stall | tolerated: end_turn OK @5.07 s ×3 |

## RAM (RSS process-tree sums, 250 ms sampler)

- startup-init 3 MB; idle 12–17 MB; prompt-turn peak ~21 MB; N=1: 20 MB; N=5: 84 MB → **marginal ~13 MB/session**. N=10 skipped (box avail 3.8 GB < 4 GB).
- Startup (init) ~0.6 s. TTFC ~0 ms on text turns.

## Catalog

- Never GETs any remote catalog (`catalog_fetches` 0 across 1300+ reqs). Models come from `settings.json` `model_metadata` only; `PUT /__catalog` with `bench-custom-xyz` did NOT surface in ACP (`discoveredModels` null).
- Host override: `~/.fx/settings.json` providers/model_metadata (no rebuild).

## Go-daemon verdict

Env: none (settings file only). Spawn `fx acp` with `HOME` pointed at a per-user config dir
containing `settings.json` (provider base_url + model_metadata). Kill PID is enough (no
grandchildren seen). Gotchas: (1) unbounded 429/500 retry storms — a hung provider looks
like a hung session; enforce a daemon-side turn timeout well under 90 s and treat
`IncompleteStream`/timeout as retryable; (2) empty responses arrive as silent empty turns —
validate turn text server-side; (3) client MCP tools arrive namespaced (`mcp_<server>_<tool>`)
and the v2 cooperative prompt cannot address them (probe-side MISSING_TOOL); (4) model
switching via `session/set_config_option` demands Vercel AI Gateway auth — pin model in
settings instead; (5) `session/resume` works, steering is not advertised.
