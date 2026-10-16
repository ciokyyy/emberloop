# opencode @ 1.19.0 — FINAL uniform bench (worker 2, 2026-10-15)

Tools: `/tmp/acpbench/bin/acpbench2`, `/tmp/acpbench/bin/fakellm2` (port 18721).
Install reused: `/tmp/acpbench/work/opencode/pkg` (`opencode-ai@1.19.0`),
isolated `HOME=/tmp/acpbench/work/opencode/home` (config `opencode.json` repointed
from 18201 to `http://127.0.0.1:18721/v1`, models `fake-small`/`fake-tools`). No re-download.

Flags: `--cmd '<pkg>/node_modules/.bin/opencode acp' --mcp-stub` (stdio; caps say
http+sse only but the command-stdio stub IS spawned and forwarded — fakellm log shows
`acpbench-stub_echo/slow` in request tools), `--cooperate`,
`--fake-llm-url http://127.0.0.1:18721`, `--auth-method auto` (authenticate
`opencode-login` succeeds headless for key providers). No `--meta-json`, no `--kill-group`.

## (1) Full `all` (`oc-final-all.json`): 9 ok / 2 wrong-result / 2 skipped

| Scenario | Verdict | Latency | Notes |
|---|---|---|---|
| initialize | ok | 3376 ms (init 2745 ms) | NO steering; authMethods=[opencode-login]; mcpCaps http+sse |
| new_session | ok | 9914 ms | mcpReady no-notification (5 s wait baked in, never sent) |
| prompt_text | wrong-result | 6070 ms | end_turn 17 B, 2 updates, TTFC 10 ms; no tool call despite --cooperate (same artifact as codex) |
| cancel | wrong-result | 6827 ms | `end_turn` — 0.5 s turn finished before the cancel race landed (updatesBeforeCancel=1) |
| steer_followup | ok | 7643 ms | no steering → 2nd prompt `queued`, both end_turn |
| load_resume | ok | 12959 ms | `session/resume` **used** (`ses_*` ids) |
| set_model_config | ok | 7051 ms | `set_config_option model=opencode/big-pickle` applied-AND-verified (next turn names it); discoveredModels=null, currentModel="" |
| images | ok | 6689 ms | accepted |
| permission | skipped | 7861 ms | never asks on fake turns |
| crash_recovery | ok | 5842 ms | fast fail |
| concurrency N=3 | ok | 23793 ms | 3/3 + 3/3; marginal 456 MB |
| soak (5) | ok | 7875 ms | 5/5 |
| provider_fault | skipped | — | no --fault (see §3) |

## (2) Soaks + cold vs steady-state (the opencode extra)

- soak50: 50/50 end_turn, 0 hangs/crashes, **p50 548 ms / p95 1319 ms**.
- cancel-mix 5 x30: 24 ok + **0 cancelled** + 6 `no-slow-turn`, p50 641 ms / p95 1008 ms.
  The SLOW-directive cancel windows never materialized (turns ~0.6 s < cancel race), so
  **cancel is UNEXERCISED this round** (neither proven nor broken; earlier round: 4/5 ok).
- Cold start (unloaded box, 02:00): init 2.7 s, first-session setup ~10 s, first turn ~6 s.
  Loaded-box history: 6–26 s. Steady-state turn: ~0.5 s (p50 548 ms).
  Ratio cold-first-turn : steady ≈ 10–40×. Daemon MUST reuse processes/sessions.

## (3) provider_fault matrix (3 runs/class; each run = fresh process, ~3–10 s cold start baked into latencies)

| Class | Wire | 3-run outcome | Scenario latency | Notes |
|---|---|---|---|---|
| http429 | REAL | 3x clean-error | ~11.5–12 s | `acp: Internal error: rate limit exceeded` — provider message preserved |
| http500 | REAL | 3x clean-error, SLOW tail | ~72–81 s | ~65 s of retries before surfacing `internal server error` |
| http529 | REAL | 3x clean-error, SLOW tail | ~77–93 s | `Overloaded`, same ~70 s retry tail |
| midstream-error | REAL (SSE error) | 3x clean-error, SLOW tail | ~69–70 s | partial text `parpar…` + error |
| reset | REAL (TCP RST) | 3x clean-error, SLOW tail | ~67–71 s | NOT recovered (contrast codex); error after ~60 s retries |
| empty | REAL | 3x silent-empty-turn | ~5–13 s | (variance = cold start, not the fault) |
| stall-5s | **VOID (no-op)** | 3x ok ~6–8 s | — | fakellm2 `stall` never holds on chat-streaming (verified: 0.5 s clean OK by curl); measures a clean turn |
| stall-forever | **VOID (no-op)** | 3x ok ~5.3–5.6 s, text `OK` | — | same; the 120 s-hang question is NOT answered by this class (see HANG test §3b) |
| truncate | **VOID (no-op)** | 3x wrong-result `truncated-unflagged`, text `OK` | — | streaming `FAULT:truncate` unrecognized → clean OK; opencode behaved CORRECTLY, probe label is wrong |

Tallies on REAL classes: clean-error 15 (429/500/529/midstream/reset x3), silent-empty-turn 3,
hang 0, crash 0. Retry-tail rule: 4xx fast (~1 s turn), 5xx/midstream/reset slow (~60–70 s).

### (3b) REAL stall test: see ADDENDUM (HANG: no response in 130 s)

## (4) Fault soak (`oc-final-faultsoak.json`, fakellm2 `--fault-rate 0.1 --fault-seed 7`)

39/40 end_turn + 1 protocol-error (iter 20), 0 hangs, p50 233 ms / p95 331 ms.
Thin coverage: only 2 of 42 requests actually faulted (`sse_error`x1, `http401`x1, seed luck).
The single protocol-error turn coincides with a faulted request — error surfaced, no hang.

## (5) RAM (acpbench sampler)

| Phase | RSS |
|---|---|
| startup-init | ~400 MB |
| idle-after-initialize | ~404 MB |
| idle-after-new-session | ~538 MB |
| prompt-turn peak | ~626 MB |
| post-turn | ~617 MB |
| concurrency-multi N=1 peak | 567 MB |
| concurrency-multi N=3 peak | 1781 MB (marginal 456 MB) |
| concurrency-multi N=5 peak | 2932 MB (marginal ~505 MB) |
| concurrency-multi N=10 peak | 4018 MB — **8/10 multi** (2x `initialize timed out after 30 s` under RAM pressure), 10/10 single |

Marginal/session: **~455–505 MB** (heaviest of the three agents). N=10 not viable on an 8 GB box
alongside other tenants; N=5 needs ~3 GB.

## (6) Catalog

- `catalog_fetches` = 0 before AND after `PUT /__catalog ["bench-cat-a","bench-cat-b","fake-small"]`:
  opencode NEVER queries the provider endpoint for a model list. Its remote catalog is
  **models.dev** (cached `~/.cache/opencode/models.json`), refreshed only via `opencode models --refresh`.
- Custom `fakellm` provider/models come from `opencode.json` (config-merge) and DO appear in ACP
  `set_config_option` model select; `bench-cat-*` (fakellm-side only) never appear.
- Host override: `opencode.json` `provider.<id>` file block, or `--refresh`/models CLI for the
  global cache. No per-session catalog override exists.

## (7) Integration verdict (Go Daemon)

Run `<pkg>/node_modules/.bin/opencode acp` with isolated HOME/XDG (writes models cache MBs,
sqlite, logs). Config: `opencode.json` `{provider:{<id>:{npm:@ai-sdk/openai-compatible,
options:{baseURL,apiKey},models:{…}}}, model:"<id>/<model>"}` — no rebuild for custom providers,
custom ids accepted by `session/set_model` and `set_config_option`. No login needed for key
providers (`--auth-method auto` succeeds). Reuse processes: cold start 3–25 s vs 0.5 s steady-state.
Do not gate on `mcp_initialized` (never sent; stdio MCP still works). No steering primitive
(second prompt queues). No `session/set_mode` (modes via `mode` config option). Cancel path
exists but needs a slow turn to land — unverified this round. Never hangs on REAL faults, but
5xx/midstream/RST tails run ~60–70 s before a clean `acp: Internal error: …` surfaces (budget
turn timeouts accordingly). ~500 MB/session — heaviest of the three. Truncate/stall classes in
the standard matrix are wire-void (see §8 bugs); the real stall verdict comes from the HANG test.

## ADDENDUM — REAL stall: HANG directive, 130 s budget (`w2-hang.py` raw ACP driver)

One `HANG` prompt (1 chunk then held connection; curl-verified). Result:
**`HANG-no-response-in-130s`** — 2 wire requests (`fault=hang`: initial + 1 retry ~1 s
later), 2 agent updates, then silence; driver killed the session. **Opencode never
surfaces a timeout on a stalled provider stream.** Replaces the void `stall-forever`
row in §3: the 120 s-hang answer is HANG, not clean-`OK`. Daemon must enforce its own
turn timeout + `session/cancel` (cancel path itself needs a slow turn to land — §2).
