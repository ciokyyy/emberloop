# FINAL: glm-acp-agent 1.15.0 (worker 3)

npm `glm-acp-agent@1.15.0` (`node .../dist/index.js`, 30MB, no build), reused
install. Env-driven: `Z_AI_API_KEY`, `ACP_GLM_BASE_URL`, `ACP_GLM_MODEL`,
`ACP_GLM_AVAILABLE_MODELS`, `ACP_GLM_SESSION_DIR`, isolated `HOME3`.
My ports: clean :18743, fault :18753, hang :18754, trunc :18755. Flags:
`--mcp-stub --cooperate --fake-llm-url ... --auth-method auto` (stdio stub works
despite only `http` MCP cap advertised). Lock-held; shared box.

## 1. Full scenario — near sweep, exit 0

| scenario | verdict | ms | notes |
|---|---|---|---|
| initialize | ok | 1574 | proto 1, load/resume/list/fork/close, image+embedded |
| new_session | ok | 6510 | tools forwarded; no ready notif (don't gate on it) |
| prompt_text | ok | 1187 | **stubVerified echo, TTFC 44ms** (only clean tool loop of my four) |
| cancel | ok | 1729 | slow-tool phase, `cancelled`, session reusable |
| steer_followup | ok | 1840 | 2nd prompt `queued`, then end_turn (not advertised, queue works) |
| load_resume | ok | 1502 | resume used; same (61ms?/18ms) + cross ok |
| set_model_config | ok | 1032 | thought_level applied; model switch verified manually |
| images | ok | 8746 | accepted — **but first use shells `npx @z_ai/mcp-server` (REAL fetch)** |
| permission | skipped | — | never asks on echo turns |
| crash_recovery | ok | 1665 | **8ms detection**, clean |
| concurrency N=3 | ok | 3414 | 3/3 + 3/3 |
| soak 5 | ok | 1672 | 0 hangs |

## 2. Soak

- soak50: **ok 50/50**, p50 39ms / p95 58ms (min 16, max 271) — fastest turns.
- cancel-mix (30, every 5th): **ok, 24 end_turn + 6 real `cancelled`** — slow tool
  turns let cancels land; session stayed healthy.

## 3. provider_fault matrix (3 reps) — fastest honest failures

| class | result | ms | notes |
|---|---|---|---|
| http429 | 3× clean-error | 2.8/3.4/2.6s | 429 text surfaced fast |
| http500 | 3× clean-error | 1.9/2.7/1.8s | Internal error |
| http529 | 3× clean-error | 1.9/2.3/2.3s | Internal error |
| midstream-error | 3× clean-error | 0.9/0.6/0.6s | `stream interrupted` detail |
| reset | 3× cancelled | 0.9/0.7/0.8s | **TCP RST → stopReason `cancelled`** + `Connection lost` text (no error) |
| stall-5s | 3× retried-success | 1.1/1.0/1.0s | only 500ms stall injected |
| stall-forever | 3× retried-success | 1.0/1.0/1.8s | **VOID — 500ms stall only** |
| truncate | 3× truncated-unflagged | ~1s | **VOID — no fault injected** |
| empty | 3× silent-empty-turn | 0.6/0.7/0.5s | fast silent (still silent!) |

TRUE stall (by-counter hang, soak-1): **3/3 timeout-hang at 90.1/90.1/90.1s**,
single request, zero retries — never surfaces own timeout within 120s.
TRUE truncate: **3/3 protocol-error `Internal error`** in 60–84ms (fast fail).

## 4. Fault soak (40 iters, rate 0.1 seed 7)

38 end_turn + 2 fast protocol-errors (13–14ms Internal error), 0 hang/crash.
Seed 7 drew 7 faults/80 reqs (2 reqs/turn: call + DONE — full tool loop every
turn). Retries through random faults and completes. Best fault tolerance of my
four.

## 5. RAM — lightest

idle-init ~95MB, idle-new ~109MB, post-turn ~119MB. conc N=1 peak 115MB;
N=3 peak 344MB (marginal ~79MB); N=5 peak 582MB/10procs → **marginal
~96MB/session**. N=10 skipped. Startup 1.6s (fastest). TTFC 0–44ms.

## 6. Catalog + GENERIC-backend verdict

No fetch ever (`catalog_fetches=0`, 260 reqs all chat/completions). Roster =
`ACP_GLM_AVAILABLE_MODELS`: proven `session/new.availableModels` exactly
`[bench-a, bench-b]` with `currentModelId=bench-a` after env-only change — no
restart. `session/set_model {modelId}` → `{}` ACCEPTED **and VERIFIED** (next
turn hit fakellm with `model=fake-small`).
**CONFIRMED as GENERIC backend** (earlier ranking upheld): single-file-ish node
service using the standard `openai` npm package `chat.completions.create`
against overridable `ACP_GLM_BASE_URL` (default
`https://api.z.ai/api/coding/paas/v4`); arbitrary model ids forwarded verbatim;
arbitrary roster via env; no id validation (`glm-5.x` prefix checks are
display-only). Caveats: GLM-isms in the wire path (`thinking.type: enabled`
only — sending `disabled` errors; `reasoning_content` mapping); default
endpoint is the subscription coding-plan URL; auth is ONLY `Z_AI_API_KEY` (or
`--setup` file) — **no other providers/subscriptions**: no SuperGrok, ChatGPT
sub, Antigravity, OpenCode Zen, Claude sub, Copilot. Images trigger a real
`npx @z_ai/mcp-server` download (network + HOME pollution + 8s) — pre-seed or
block vision MCP. No sandboxing (shell/fs as agent user); permission prompts are
the only guard.

## 7. Integration verdict

BEST small-footprint generic OpenAI-compatible backend of my four: 1.6s start,
~100MB/session, full tool loop verified, live model switch, queued follow-ups,
8ms crash detection, survives 10% fault soup at 38/40. Daemon needs: env trio
(`Z_AI_API_KEY` fake-accepted, `ACP_GLM_BASE_URL`, `ACP_GLM_AVAILABLE_MODELS`),
per-user `ACP_GLM_SESSION_DIR` (+ session rotation), explicit permission policy,
blocked/pre-seeded vision MCP, and — like all four — its own stream-idle
timeout (true stalls hang past 90s with no agent-side timeout).
