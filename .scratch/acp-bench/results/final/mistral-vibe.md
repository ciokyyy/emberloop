# FINAL: mistral-vibe 2.26.1 (worker 3)

PyPI `mistral-vibe==2.26.1`, reused install `/tmp/acpbench/work/mistral-vibe/venv`
(CPython 3.12 standalone). My homes: `home3` (clean :18740), `home3f` (fault soak
:18750), `home3h`/`home3t` (hang/truncate :18744/:18745). fakellm2 + acpbench2.
Flags: `vibe-acp --legacy-harness`, `--mcp-stub --cooperate --fake-llm-url ...
--auth-method auto` (stdio stub fine; `--legacy-harness` REQUIRED — default
unified harness does not forward client mcpServers). All timing under
`flock /tmp/acpbench/bench.lock`; machine shared with 3 workers (load 2-9).

## 1. Full scenario

| scenario | verdict | ms | notes |
|---|---|---|---|
| initialize | ok | 3840 (tti 3237) | proto 1, no steering, authMethods=[] |
| new_session | ok | 10475 | legacy forwards stub (tools at LLM); no mcp_ready notif |
| prompt_text | wrong-result | 4015 | end_turn, no tool call (probe/fake limitation, known) |
| cancel | skipped | — | raced |
| steer_followup | skipped | — | not advertised + raced |
| load_resume | wrong-result ×2 | ~7.4s | `session/resume` used; **loadSameProcess fails `Session is already open`** (reproduced); cross-process load ok, history OK |
| set_model_config | applied-then-poison | 4036 | `set_config_option(model=mistral-medium-3.5)` APPLIED and **persisted to config.toml `active_model`** → every later scenario auth-failed (probe-poison artifact, see bugs) |
| images | ok | 5346 | accepted (rerun with clean config) |
| permission | skipped | — | no ask on tool-less turns |
| crash_recovery | skipped | — | raced |
| concurrency N=3 | ok | 24374 | 3/3 + 3/3 |

## 2. Soak

- soak50: **ok 50/50 end_turn**, 0 hang, p50 67ms / p95 119ms.
- cancel-mix (30, every 5th): 24 end_turn + 6 no-slow-turn, 0 cancelled (turns ~67ms,
  cancel never lands), wrong-result.

## 3. provider_fault matrix (3 reps each; time-to-surface = turn latency)

| class | result | ms | notes |
|---|---|---|---|
| http429 | 3× clean-error | 301/305/305s | **~300 retries at fixed 1s**, then `Rate limit exceeded` |
| http500 | 3× clean-error | 311/314/310s | **12 attempts, exp backoff 0.5/1/2/4/8/16/32/60(cap)s**, then backend error |
| http529 | 3× clean-error | 309/311/310s | same stamina as 500 |
| midstream-error | 3× clean-error | 18/9.3/3.8s | `Model stream ended without a finish reason` (fast) |
| stall-5s | 3× retried-success | 5.3/4.8/5.1s | **only 500ms stall actually injected** (faultRe bug); waited out |
| stall-forever | 3× retried-success | 1.3–11s | **VOID — only 500ms stall injected, never infinite** |
| truncate | 3× truncated-unflagged | ~3.6s | **VOID — `truncate` unknown to fakellm2, no fault injected** |
| reset | 3× clean-error | 3.4/3.5/7s | TCP RST surfaced as ReadError backend error |
| empty | 3× silent-empty-turn | 6.9/9.4/5.5s | **end_turn, zero text, no error** (swallowed) |

TRUE stall (by-counter `hang`: one chunk then hold forever, plain soak-1 turn):
**3/3 timeout-hang at 90.1/90.2/90.1s** — 1 request each, zero retries, agent never
surfaces its own timeout (answer to the 120s question: NO).
TRUE truncate (by-counter): **3/3 clean-error `Stream chunk contains malformed
JSON`** in 3.5/3.9/8.1s.

## 4. Fault soak (40 iters, --fault-rate 0.1 --fault-seed 7)

38 end_turn + 2 protocol-error, 0 hang/crash. Seed 7 drew only 2 faults/40 turns:
stream-no-finish-reason (189ms) and http401 Invalid API key (48ms, not retried).
Both surfaced, nothing hung.

## 5. RAM (sampler, lock-held)

startup-init ~101MB (1 proc), idle-new ~136MB, post-turn ~143MB. conc N=1 peak
143MB ok; N=3 peak 463MB; N=5 peak 953MB (last 716MB) → **marginal ~116MB/session**.
N=10 skipped (shared box, prior worker measured ~127MB). Startup 3.2–4.6s
(fastest of my four). TTFC ~0ms (clock artifact −8ms).

## 6. Catalog

Zero fetches, ever: `catalog_fetches=0`, only `/v1/chat/completions` over 1100+
requests. `session/new` carries NO model list. Arbitrary ids via `[[models]] name`
forwarded verbatim; live switch via `set_config_option(model=<alias>)` (proven,
but it **persists `active_model` to disk**). PUT bench-a/bench-b catalog caused no
refetch. Host override: pin `active_model` + append `[[providers]]`/`[[models]]`
to `$VIBE_HOME/config.toml` (note: GrowthBook layer can inject
`routed_default_model` when unpinned).

## 7. Integration verdict

VIABLE and leanest (~115MB/session, 3–5s start, 50/50 soaks clean, honest fast
failures on stream errors). Daemon MUST: pass `--legacy-harness`; isolate
`VIBE_HOME` per user; pin `active_model` (config layers merge; set_config_option
persists!); never run set_model-style probes mid-run (poisons the file);
budget 5-min 429/5xx retry storms (no backoff cap on 429!); treat image turns and
empty provider replies as silent-success hazards (empty → end_turn, no error);
enforce its own stream-idle timeout — vibe never times out a stalled stream
(90s+ hangs). Strictest parser of the four (needs real OpenAI `index`/Anthropic
`usage` fields).
