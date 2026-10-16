# goose @ 1.54.0 — FINAL uniform bench (worker 2, 2026-10-15)

Tools: `/tmp/acpbench/bin/acpbench2`, `/tmp/acpbench/bin/fakellm2` (port 18722).
Install reused: `/tmp/acpbench/work/goose/app/goose` (pinned GH release asset),
isolated `HOME=/tmp/acpbench/work/goose/home`. No re-download.

Flags: `--cmd '/tmp/acpbench/work/goose/app/goose acp' --mcp-stub` (stdio),
`--cooperate`, `--fake-llm-url http://127.0.0.1:18722`, `--auth-method auto`.
Env: `GOOSE_PROVIDER=openai GOOSE_MODEL=fake-tools OPENAI_HOST={{FAKE_LLM_URL}}
OPENAI_API_KEY=fake-key-for-bench GOOSE_MODE=auto` (+ isolated XDG_*). fakellm2 `--cooperate`.

## (1) Full `all` (`gs-final-all.json`): 7 ok / 5 skipped / 1 wrong-result

| Scenario | Verdict | Latency | Notes |
|---|---|---|---|
| initialize | ok | 1873 ms | NO steering; mcpCaps http-only; authMethods=[goose-provider] |
| new_session | ok | 7123 ms | mcpReady no-notification (5 s wait expires; tools still work, see §6) |
| prompt_text | wrong-result | 1342 ms | end_turn, no tool call — honest `OK` relay (probe artifact, not a bug) |
| cancel | skipped | 1046 ms | 26 ms turn finished before the race window |
| steer_followup | skipped | 772 ms | no steering; turns too fast to overlap |
| load_resume | ok | 1264 ms | `session/resume` unsupported → `session/load` fallback works cross-process |
| set_model_config | ok | 839 ms | `session/set_model` Method-not-found; `set_config_option model=` ACCEPTED — and wire-effective (see §6) |
| images | ok | 727 ms | accepted |
| permission | skipped | 825 ms | never asks on fake turns (`GOOSE_MODE=auto` required for unattended tools) |
| crash_recovery | skipped | 821 ms | turn too fast for the SIGKILL window |
| concurrency N=3 | ok | 2715 ms | 3/3 + 3/3; marginal 106 MB |
| soak (5) | ok | 2049 ms | 5/5 |
| provider_fault | skipped | — | no --fault (see §3) |

## (2) Soaks

- soak50: 50/50 end_turn, 0 hangs/crashes, **p50 26 ms / p95 48 ms** (fastest of the three by ~20×).
- cancel-mix 5 x30: 24 ok + 6 `no-slow-turn`, **0 cancelled** — probe cancel unexercised
  (turns ~26 ms). Manual prior art (SLOW:20000, cancel @2 s): `stopReason=cancelled`,
  session reusable — the cancel path EXISTS, the probe just can't catch a 26 ms turn.

## (3) provider_fault matrix (3 runs/class; fresh process per run, ~1 s cold start)

| Class | Wire | 3-run outcome | Latency | Notes |
|---|---|---|---|---|
| http429 | REAL | 3x clean-error | ~4.1–4.4 s | `Rate limit exceeded… Please retry…` — no retry tail, fastest 429 of the three |
| http500 | REAL | 3x clean-error | ~7.5–8.1 s | `Server error (500)…`, no 60 s tail (contrast opencode) |
| http529 | REAL | 3x clean-error | ~7.7–8.6 s | `Server error (529)…` |
| midstream-error | REAL | 3x error-text <1 s (probe: retried-success) | ~0.8–0.9 s | `par` + `stream interrupted…`; probe keyword list misses it |
| reset | REAL (RST) | 3x error-text <1.1 s (probe: retried-success) | ~0.8–1.1 s | `Network error: Stream decode error…`; instant surface |
| empty | REAL | 3x explicit-empty-text (probe: silent-empty-turn) | ~0.8–1.0 s | `The model returned an empty response. Please resend…` — BEST empty UX of the three; probe has no `empty` keyword |
| stall-5s | **VOID** | 3x ok ~1.2–1.4 s | — | no-op on wire (clean OK) |
| stall-forever | **VOID** | 3x ok ~1.5–1.6 s, text `OK` | — | no-op on wire; 120 s question answered by HANG test §3b |
| truncate | **VOID** | 3x wrong-result, text `OK` | — | no-op on wire; goose behaved correctly, probe label wrong |

Tallies on REAL classes: clean/explicit error text 15/15 (all <9 s, most <1 s),
hang 0, crash 0. Goose never retries 5xx into minute-long tails — fail-fast profile.

### (3b) REAL stall test: see ADDENDUM (HANG: no response in 130 s)

## (4) Fault soak (`gs-final-faultsoak.json`, fakellm2 `--fault-rate 0.1 --fault-seed 7`)

39/40 end_turn + 1 protocol-error, 0 hangs, p50 50 ms / p95 91 ms.
Thin coverage again: 2 of 44 requests faulted (`sse_error`x1, `http401`x1).
Note: fault-rate is per-REQUEST, so chatty agents (codex ~6 req/turn → 22 faults/244)
soak-test harder than quiet ones (goose/opencode ~1 req/turn → 2/42–44).

## (5) RAM (lightest of the three)

| Phase | RSS |
|---|---|
| startup-init | ~116 MB |
| idle-after-initialize | ~170 MB |
| idle-after-new-session | ~151 MB |
| prompt-turn peak | ~160 MB |
| post-turn | ~160 MB |
| concurrency-multi N=1 peak | 243 MB |
| concurrency-multi N=5 peak | 788 MB (marginal ~128 MB) |
| concurrency-multi N=10 peak | 1568 MB, 10/10 ok (marginal ~140 MB) |

Spawn→init ~0.4–1.9 s; marginal/session **~105–140 MB**.

## (6) Catalog + MCP + model binding (the goose extras)

- MCP forwarding VERIFIED on the wire: fakellm log shows `acpbench-stub__echo` and
  `acpbench-stub__slow` in the `tools` array of 22 requests (alongside goose's 17 native
  tools). Goose namespaces them (`acpbench-stub: echo`) and executes end-to-end (prior art).
  It advertises http-only mcpCaps and never sends `mcp_initialized` — do not gate on it.
- Catalog refetch: YES — goose GETs provider `/v1/models` at session setup
  (`catalog_fetches` 33→36 across one `set_model_config` run; per-session GETs observed).
  After `PUT /__catalog ["bench-cat-a","bench-cat-b","fake-tools"]`, no restart needed:
- Model binding, wire-verified (raw ACP driver, `w2-gsmodel.py`):
  (a) `session/new` with extra `"model":"bogus-session-model-xyz"` → IGNORED, wire still
  `fake-tools`. The session/new model param is NOT a binding.
  (b) `session/set_config_option {model: bench-cat-a}` → next turn goes out as
  **`bench-cat-a`** (2 wire hits). Runtime switch works, incl. PUT-injected ids, no restart.
  (Correction to the earlier note: `set_config_option` model is applied; only
  `session/set_model` proper is Method-not-found. acpbench2 reports `applied-unverified`
  solely because goose never echoes the model name in turn text.)
- Host override: per-provider `<P>_HOST`/` *_BASE_PATH` envs redirect both chat and catalog;
  custom providers via `~/.config/goose/custom_providers/*.json`. No global catalog URL.

## (7) Integration verdict (Go Daemon)

Spawn `goose acp` with isolated HOME/XDG and env control plane:
`GOOSE_PROVIDER`/`GOOSE_MODEL`/`OPENAI_HOST` (+`GOOSE_MODE=auto` for unattended tools;
API keys via env, never config.yaml). No login, no `--kill-group`, no meta needed.
Per-session cost is tiny (~130 MB, ~1 s setup with the 5 s mcp-wait always expiring —
do not gate on it). Cancel works (manual SLOW proof) but only catchable on slow turns.
Fail-fast fault profile: 4xx/5xx/RST/midstream all surface readable text in <9 s, no tails —
the most stall-proof-shaped agent here (HANG test pending). Runtime model switch via
`session/set_config_option model=` (wire-verified, even for fresh catalog ids);
`session/new` model params ignored, so respawn-or-set_config for model changes.
`session/resume` unsupported (use `session/load`), no steering advertisement.

## ADDENDUM — REAL stall: HANG directive, 130 s budget (`w2-hang.py` raw ACP driver)

One `HANG` prompt (1 chunk then held connection; curl-verified). Result:
**`HANG-no-response-in-130s`** — 2 wire requests (`fault=hang`: initial + immediate
retry), 4 agent updates, then silence; driver killed the session. **Goose never surfaces
a timeout on a stalled provider stream** despite its fail-fast profile on every other
fault class. Replaces the void `stall-forever` row in §3. Daemon must enforce its own
turn timeout + `session/cancel`.
