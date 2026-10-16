# FINAL: kimi-code 1.50.0 (worker 3)

PyPI `kimi-code==1.50.0` (shim over `kimi-cli`; ≥1.51 is a tombstone; TS 2.x not
tested), reused venv. My homes: `home3` (:18742), `home3f` (fault :18752),
`home3h`/`home3t` (:18748/:18749). Flags: `kimi acp`, `--mcp-stub --cooperate
--fake-llm-url ... --auth-method auto`, `HOME`+`KIMI_SHARE_DIR` isolated.
Lock-held; shared box.

## 1. Full scenario

| scenario | verdict | ms | notes |
|---|---|---|---|
| initialize | ok | 2929 | proto 1, MCP http (no sse), authMethods=[login terminal-auth] |
| new_session | ok | 12221 | stub forwarded; no ready notif |
| prompt_text | wrong-result | 7159 | announces echo tool call but stub never runs (name/path mismatch) |
| cancel | skipped | — | raced (manual SLOW+cancel earlier proved clean `cancelled`) |
| steer_followup | skipped | — | not advertised + raced |
| load_resume | ok | 8976 | resume used; same + cross ok |
| set_model_config | ok | 4132 | `set_model` to config id applied; `set_config_option` unsupported |
| images | ok | 4322 | needs `capabilities=["image_in"]` else LLMNotSupported |
| permission | ok | 11580 | **approve + deny legs; deny stub-verified** (best permission result of my four) |
| crash_recovery | skipped | — | raced |
| concurrency N=3 | ok | 34712 | 3/3 + 3/3 |
| soak 5 | ok | 5955 | 0 hangs |

## 2. Soak

- soak50: **ok 50/50**, p50 165ms / p95 251ms (min 63, max 1288).
- cancel-mix (30, every 5th): 24 end_turn + 6 no-slow-turn.

## 3. provider_fault matrix (3 reps) — THE SWALLOWER

| class | result | ms | notes |
|---|---|---|---|
| http429 | 3× silent-empty-turn | 13.4/20.2/12.3s | **end_turn, empty, NO error** |
| http500 | 3× silent-empty-turn | 10.7/15.1/9.3s | same |
| http529 | 3× silent-empty-turn | 6.3/5.9/5.9s | same |
| midstream-error | 3× retried-success | 6.3/3.9/5.4s | recovered with partial `par…` text |
| reset | 3× retried-success | 5.3/4.4/6.2s | recovered with `parpar` |
| stall-5s | 3× retried-success | 5.1/4.6/4.6s | only 500ms stall injected |
| stall-forever | 3× retried-success | 4.5/4.6/8.2s | **VOID — 500ms stall only** |
| truncate | (void) | — | **no fault injected** (`truncate` unknown to fakellm2) |
| empty | 3× silent-empty-turn | 7.5/6.5/8.5s | end_turn, empty, no error |

So: HTTP status errors AND empty replies are **all swallowed into silent empty
end_turns**; only stream-shape faults (midstream/reset) surface as recovered
partial text. A daemon cannot distinguish "model said nothing" from "provider
500'd" — budget accordingly.
TRUE stall (by-counter hang, soak-1): **3/3 timeout-hang at 90.3/90.9/90.3s**,
single request, zero retries — never surfaces own timeout within 120s.
TRUE truncate: **3/3 end_turn recovered** in 0.8/1.0/0.8s.

## 4. Fault soak (40 iters, rate 0.1 seed 7)

**40/40 end_turn ok** (redo vs fault URL — first attempt hit clean URL because
config `base_url` wins over env, same precedence gotcha as fast-agent). Seed 7
drew 2 faults/40 reqs; both survived. 0 hang/crash.

## 5. RAM

init ~105MB, idle-new ~156MB, post-turn ~170MB. conc N=1 peak 176MB; N=5 peak
850MB/11procs → **marginal ~140MB/session**. N=10 skipped. Startup 2.9–8.8s.
TTFC ~0ms.

## 6. Catalog

No fetch ever (`catalog_fetches=0`). Roster is purely local:
`availableModels=[fake-test(fake-py-small), fake-second(fake-custom-1)]`,
current `fake-test`. PUT bench-a/bench-b: no refetch, roster unchanged. Host
override: `config.toml` `[providers.<n>]` + `[models.<alias>]` (keys: type,
base_url, api_key / provider, model, max_context_size, capabilities).

## 7. OAuth gate (headless seeding: YES, with a caveat)

- **Seeded headlessly**: writing `credentials/kimi-code.json`
  `{access_token, refresh_token, far-future expires_at}` into a FRESH isolated
  `KIMI_SHARE_DIR` bypasses the gate — fresh-dir `session/new` ok in 14.5s. No
  browser, no login, fully scriptable. (Values are never validated against a
  real endpoint in fake-backend use.)
- **Unseeded**: `session/new` does NOT fail fast — it **hangs 70s**
  (timeout-hang), no usable AUTH_REQUIRED. Daemon must pre-seed the token file
  or pre-flight with a short timeout.
- Side effects: `set_model` **persists `default_model` to the config file**
  (observed `fake-second` → `fake-test` live); auto-compaction footgun when
  `max_context_size` ≤ reserved 50k (configure 200k); needs `image_in`
  capability or image turns hard-fail.

## 8. Integration verdict

USABLE with the strictest precondition: per-user `KIMI_SHARE_DIR` with a seeded
dummy token (rotate/expiry policy is yours — expiry semantics untested against
real auth). Best ACP model-list support of my four (real roster, working switch)
and the only clean permission deny. Worst failure-mode honesty: 4xx/5xx/empty
all collapse to silent empty turns. Mid-range resources (~140MB/session).
Stream stalls hang forever — daemon idle-timeout mandatory.
