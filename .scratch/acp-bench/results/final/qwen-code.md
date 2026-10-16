# qwen-code 0.25.1 — final bench result (worker 4)

Verdict: **USABLE with caveats** — full ACP turn loop works against a custom
OpenAI-compatible endpoint; heaviest RAM of the five (~314 MB/session); slow bounded
retries (500/529 surface after ~100 s; 429 storms past 3.5 min); provider hangs surface
only via client timeout; client MCP is discovered but never forwarded (and offering http
MCP entries breaks `session/new`). The 2 transient Internal errors from the earlier
run did NOT reproduce under fault load (0 in 120+ faulted turns).

## Install / start

- Artifact: npm `@qwen-code/qwen-code@0.25.1` (156 MB); start
  `node …/qwen-code/cli-entry.js --acp` (**package root**, not `dist/`).
- Isolated HOME; env `OPENAI_API_KEY=fake-test-key`; **`--kill-group` REQUIRED**
  (node grandchildren hold stdio after SIGKILL; without it crash recovery hangs).
- `~/.qwen/settings.json`: `modelProviders.openai [{id: fake-tools|fake-small,
  wireApi: "chat-completions" ("chat" is rejected), envKey: OPENAI_API_KEY,
  baseUrl: …/18764/v1}]`, `security.auth.selectedType: openai`, `model.name: fake-tools`.
- acpbench2 flags: `--mcp-stub --cooperate --fake-llm-url … --auth-method auto --kill-group`
  (stdio MCP; `--mcp-transport http` breaks `session/new` — see below).

## Scenario results (`--scenario all`)

| scenario | verdict | detail |
|---|---|---|
| initialize | ok 6–8 s | loadSession, mcp http/sse ONLY, image/audio/embedded, list+resume; auth openai |
| new_session | ok 13 s | mcpReady no-notification, stub `tools/list` seen |
| prompt_text | wrong-result | no tool call: MISSING_TOOL:echo text + provider never receives stub tools |
| cancel | ok | cancelled mid-streaming, session reusable |
| steer_followup | ok | no steering adv; 2nd prompt `queued` |
| load_resume | ok | `session/resume` USED; both load legs ok, history verified |
| set_model_config | ok | discovers real ids `[fake-tools(openai), fake-small(openai)]`; `set_config_option model=fake-small(openai)` applied (model switch = settings rewrite + restart; no live-switch proof) |
| images | ok | accepted |
| permission | skipped | never asks on echo turns |
| crash_recovery | ok | fail-fast 57 ms WITH `--kill-group` |
| concurrency N=3 | ok | 3/3 both modes, marginal ~289 MB |
| soak 5 | ok | p50 ~1.2 s |

## Soak / cancel-mix / fault soak

- soak 50: **50/50 ok, 0 hangs**, p50 873 ms, p95 1373 ms.
- cancel-mix 30: 24 ok + 6 no-slow-turn, 0 cancelled, 0 hangs.
- fault soak 40 @0.1/seed7 (swap-validated): **40/40 ok, 0 hangs** (124 reqs ≈ 3.1/turn;
  11 faults fired: empty/401/429/503/529/stall/truncated/ttfb — all absorbed, ZERO
  `Internal error`s; earlier 2 transients not reproduced, rate <1/120 faulted turns).

## provider_fault matrix (3 reps)

| class | result |
|---|---|
| http429 | **HANG (retry storm)**: 168 reqs over 3×170 s bounded reps (all killed); long rep: **timeout-hang @209 s** (224 reqs total). Never surfaces by itself. |
| http500 | clean-error ~97–100 s ×3 (long bounded retry, then surfaces) |
| http529 | clean-error ~97–102 s ×3 (same) |
| midstream-error (=sse_error) | clean-error ~5–8 s ×3 |
| reset | retried-success ~5.4 s ×3 |
| empty | clean-error ~25–35 s ×3 (retries empties, then surfaces — vs crow's silent storm) |
| stall-5s / stall-forever (stock) | DEGRADED (500 ms): retried-success ~5.2 s (= normal turn time) |
| truncate (stock) | VOID: `truncated-unflagged` artifact |

TRUE: hang → 1 infra-flake (init timeout under contention) + **timeout-hang @94–96 s ×2**
(`par` partial; never gives up); truncated body → silent recovery (retries truncated req,
completes ToolSearch loop, never flags) ~6.5 s ×3; 5 s stall → tolerated, end_turn ~17–22 s ×3.

## RAM

- startup-init 353 MB; idle ~380 MB; prompt-turn ~440–485 MB; N=1: 397 MB; N=5: 1952 MB →
  **marginal ~314 MB/session**. N=10 skipped (avail 3.8 GB). Init 6–8 s. Node
  `event loop stall` warnings under load. Heaviest of the five.

## Catalog / MCP-transport extras

- qwen fetches `models.dev/api.json` at runtime (registry `fetchedAt` updated mid-bench;
  2× GET /api.json UA `node` during override test). `QWEN_CODE_MODELS_DEV_URL` override
  honored (fakellm shape accepted, session ok — no repeat of the old shape-mismatch
  Internal error).
- ACP-visible models come from `settings.json` ONLY: `PUT /__catalog` custom id did NOT
  surface (`discoveredModels` = local ids). Remote fetch feeds the built-in roster, not
  ACP options. Host override: `settings.json` (+ optional MODELS_DEV_URL/disable for
  sandboxed/offline hosts).
- `--mcp-transport http`: `session/new` → **protocol-error `acp: Internal error`**
  (no stub log created). Offering http MCP entries breaks session creation; stdio entries
  are accepted-but-ignored (listed, never forwarded). Daemon: send the FULL stdio shape
  (`{name,command,args,env}` — bare `{command,args}` fails zod), expect no tool use.

## Go-daemon verdict

Env: `OPENAI_API_KEY` (or provider key) + `HOME`; `--kill-group` on teardown (non-negotiable).
Spawn `node …/cli-entry.js --acp`. Prefer few long-lived sessions (~400 MB + ~7 s each).
Gotchas: (1) no agent-side stall/empty timeout — daemon-side turn timeout required
(500/529 cost ~100 s; 429 never returns); (2) empty responses ARE surfaced (25–35 s,
clean) but truncation is silently absorbed; validate turns; (3) client MCP is
list-but-drop — tools never reach the model; (4) no live model switch; (5) phones home
(models.dev, update check?) — override/disable in sandbox.
