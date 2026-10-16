# FINAL: fast-agent 0.10.1 (worker 3)

PyPI `fast-agent-mcp==0.10.1` (the `fast-agent-acp` wheel is metadata-only —
use `fast-agent --home <dir> acp -x --model generic.<id>`), reused venv.
My homes: `home3` (clean :18741), `home3f` (fault :18751), `home3h`/`home3t`
(:18746/:18747). Flags: `--mcp-stub --cooperate --fake-llm-url ...
--auth-method auto --kill-group` + `ACPBENCH_INIT_TIMEOUT=120s`. Lock-held;
shared box, load 2–9.

GOTCHA THAT COST TWO 560s RUNS: `GENERIC_BASE_URL` env is **overridden by
`generic.base_url` in fast-agent.yaml** (`llm_generic.py`: env read first, then
replaced when config present). Config must point at the fake URL; I used
separate homes per port.

## 1. Full scenario (split in 4 groups; single `all` exceeds 560s under load)

| scenario | verdict | ms | notes |
|---|---|---|---|
| initialize | ok | 9087 | proto 1, MCP http+sse, authMethods=[fast-agent-ai-secrets] |
| new_session | ok | 12656 | stub namespaced `acpbench-stub__echo` at LLM; no ready notif |
| prompt_text | wrong-result | 7527 | end_turn, no tool call (known probe/fake limitation) |
| cancel | skipped | — | raced (cooperate turns too fast; earlier worker got ok/streaming) |
| steer_followup | skipped | — | not advertised; 2nd prompt would queue |
| load_resume | ok | 26486 | resume used; same-process + cross-process ok |
| set_model_config | skipped | 8215 | no advertised ids; never probes bogus (honest skip) |
| images | ok | 9117 | accepted, no capability config |
| permission | skipped | — | no ask on tool-less turn (earlier worker proved full loop manually) |
| crash_recovery | skipped | — | raced |
| concurrency N=3 | ok | 41884 | 3/3 + 3/3 |

## 2. Soak

- soak50: **ok 50/50 end_turn**, p50 63ms / p95 129ms (fast when unloaded).
- cancel-mix (30, every 5th): 24 end_turn + 6 no-slow-turn, 0 landed.

## 3. provider_fault matrix (3 reps)

| class | result | ms | notes |
|---|---|---|---|
| http429 | 3× clean-error | 43.7/43.8/48.8s | 10/20s backoff retries, then `Internal error` naming the 429 |
| http500 | 3× clean-error | 46/46.4/41.8s | same shape |
| http529 | 3× clean-error | 43.4/41.6/41.3s | same shape |
| midstream-error | 3× unexpected-stop | ~37s | **stopReason `refusal`, text `parparpar`** (3 partials); no clean error |
| stall-5s | 3× retried-success | 13.6/11.3/28.8s | only 500ms stall injected |
| stall-forever | 3× retried-success | ~19s | **VOID — 500ms stall only** |
| truncate | 3× truncated-unflagged | 8–10s | **VOID — no fault injected** |
| reset | 3× clean-error | 40.6/37.2/38.7s | TCP RST → retries → Internal error |
| empty | 3× clean-error | 39.4/37.2/39.1s | **empty surfaced as Internal error (NOT silent)** |

TRUE stall (by-counter hang, soak-1): **3/3 timeout-hang at 90.5/90.9/90.5s**,
single hang request each, zero retries — never surfaces own timeout within 120s.
TRUE truncate: **3/3 end_turn recovered** in 0.5–0.8s.

## 4. Fault soak (40 iters, rate 0.1 seed 7)

**40/40 end_turn ok**, 0 hang/crash. Seed 7 drew only 2 faults/42 reqs (http401,
sse_error); both recovered. (1 req/turn — fa runs no tool loop on these turns.)

## 5. RAM

startup-init ~204MB, idle-new ~215MB, post-turn ~230MB. conc N=1 peak 228MB;
N=3 690MB; N=5 1155MB/10procs → **marginal ~188MB/session** (fattest of my four).
N=10 skipped (prior worker: ~194MB; shared-box OOM risk). Startup 9–19s typical,
flaky past 30s under load — daemon needs ≥60–120s init budget. TTFC ~0ms.

## 6. Catalog

No fetch ever (`catalog_fetches=0`, 233 reqs all chat/completions, OpenRouter/HF
lookups untouched on generic path). `session/new` carries NO models and NO
configOptions. Any `generic.<id>` forwarded verbatim (235/235 `fake-py-small`).
PUT bench-a/bench-b: no refetch. Host override: `fast-agent.yaml` `generic:`
section or `--model generic.<id>` (config file BEATS env).

## 7. Integration verdict + CORRECTION

STRONGEST interop (cancel works, queueing works, permission flow proven) but
heaviest (~190MB/session, ~15s start). **CORRECTION to earlier notes:
`session/set_model` is a NO-OP.** It returns `{}` for ANY id (including `bogus`)
yet changes nothing — no `set_session_model` handler exists in fast_agent; the
`{}` is schema-validation-only. Verified: 235/235 fakellm requests used the
startup model after accepted set_model calls. Same likely applies to
`set_config_option` (no handler; earlier "accepted" was also validation-only).
**Model is fixed at process start — the daemon must respawn (or rewrite config)
to switch models.** Other gotchas: `--home` per user (sessions persist, rotate);
`--no-update-check` (PyPI egress); `-x` for local shell/fs runtime. Second
concurrent prompt queues — safe for double-sends. Midstream failure → `refusal`
stop with partial text (not an error) — daemon must treat `refusal`+partial as
provider failure. Stalled streams hang forever — daemon-side idle timeout
mandatory.
