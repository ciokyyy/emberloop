# codex-acp @ 2.2.2 — FINAL uniform bench (worker 2, 2026-10-15)

Tools: `/tmp/acpbench/bin/acpbench2`, `/tmp/acpbench/bin/fakellm2` (port 18720).
Install reused: `/tmp/acpbench/work/codex-acp/pkg` (`@agentclientprotocol/codex-acp@2.2.2`),
isolated `HOME=/tmp/acpbench/work/codex-acp/home`. No re-download.

Flags: `--mcp-stub` (stdio transport, default), `--cooperate`,
`--fake-llm-url http://127.0.0.1:18720`, `--auth-method auto`.
No `--meta-json`, no `--kill-group`, no `--mcp-transport http` needed.
Env: `CODEX_API_KEY=fake-test-key`,
`CODEX_CONFIG={"model_provider":"custom-gateway","model":"gpt-5-fake","model_providers":{"custom-gateway":{"name":"fakellm","base_url":"{{FAKE_LLM_URL}}/v1","wire_api":"responses"}}}`,
`MODEL_PROVIDER=custom-gateway`, `NO_BROWSER=1`, `DEFAULT_AUTH_REQUEST={"methodId":"api-key"}`,
`HOME=<isolated>`. fakellm2 always `--cooperate`.

## (1) Full `all` (`cx-final-all.json`): 10 ok / 1 wrong-result / 2 skipped

| Scenario | Verdict | Latency | Notes |
|---|---|---|---|
| initialize | ok | 1381 ms (init itself 726 ms) | steering advertised; authMethods=[api-key], authenticate ok |
| new_session | ok | 6880 ms | mcpReady no-notification (5 s wait); toolListSeen=true (stub tools reach model) |
| prompt_text | wrong-result | 8045 ms | end_turn, 11 updates, 213 B text, TTFC 16 ms; model replies text, never calls echo (still true WITH --cooperate) |
| cancel | ok | 9064 ms | streaming-phase `cancelled`, session reusable |
| steer_followup | ok | 1907 ms | `injected` via `_session/steering` |
| load_resume | ok | 20646 ms | resume **used**, load same+cross-process ok |
| set_model_config | ok | 7310 ms | discovered 40+ bundled ids (`gpt-6.1-sol/-astra…[low..ultra]`); configOptions=[mode, collaboration_mode, model]; currentModel `gpt-5-fake[medium]` |
| images | ok | 7394 ms | 1x1 PNG accepted |
| permission | skipped | 8856 ms | never asks on fake turns |
| crash_recovery | ok | 2085 ms | SIGKILL detect 9 ms |
| concurrency N=3 | ok | 35270 ms | 3/3 multi + 3/3 single; marginal 183 MB (N=3) |
| soak (5) | ok | 34259 ms | 5/5 end_turn, p50 6456 ms p95 6655 ms |
| provider_fault | skipped | — | no --fault (covered in §3) |

## (2) Soaks

- soak50 (`cx-final-soak50.json`): 50/50 end_turn, 0 hangs, 0 crashes, p50 6576 ms / p95 6881 ms (range 6038–7142 ms).
- cancel-mix 5 x30 (`cx-final-cancelmix.json`): 24 ok + 6 cancelled, 0 hangs; p50 6540 ms / p95 6858 ms.

## (3) provider_fault matrix (3 runs per class)

| Class | 3-run outcome | Time-to-surface | Notes |
|---|---|---|---|
| http429 | 3x clean-error (probe: surfaced-error, ok) | ~1.1–1.4 s | single attempt, text `exceeded retry limit, last status: 429 Too Many Requests` |
| http500 | 3x clean-error slow (probe: retried-success r=11, ok) | ~26.3–26.5 s | ~30 provider hits/turn, ends end_turn with composed error text (`We're currently experiencing high demand…`) |
| http529 | 3x clean-error slow (probe: surfaced-error, ok) | ~24–27 s | text `unexpected status 529 … Overloaded` |
| midstream-error | 3x clean-error (probe: retried-success r=11, ok) | ~6.8–6.9 s | error text surfaced, end_turn |
| truncate | 3x **surfaced-as-success** (probe: truncated-unflagged, wrong-result) | ~7.6–8.2 s | text DOES say `stream disconnected before completion: stream closed before response.completed` but stopReason=end_turn — a Daemon keying on stopReason sees success |
| reset | 3x recovered (probe: retried-success r=11, ok) | ~7.7–7.8 s | |
| empty | 3x silent-empty-turn | ~7.3–7.9 s | turn text is only the `Model metadata for gpt-5-fake not found…` boilerplate, no content |
| stall-5s | 3x ok (probe: retried-success r=11) | ~10.4–10.9 s | |
| stall-forever | 3x clean-error, **never hangs** (probe: retried-success, ok) | ~10.3–11.0 s | codex's own stream idle timeout fires ~10 s: `stream disconnected before completion`; NO run reached the 120–130 s probe timeout |

Tallies: clean-error 15 (429x3, 500x3, 529x3, midstreamx3, stall-foreverx3), recovered-ok 6 (resetx3, stall-5sx3),
surfaced-as-success 3 (truncatex3), silent-empty-turn 3 (emptyx3), hang 0, crash 0.

## (4) Fault soak (`cx-final-faultsoak.json`, fakellm2 `--fault-rate 0.1 --fault-seed 7`)

40/40 end_turn, 0 hangs, 0 crashes, p50 6486 ms / p95 6926 ms.
Fakellm log confirms 22 injected faults over 244 requests (~9%):
http529x3, ttfbx3, sse_errorx2, http401x2, truncatedx2, resetx2, stallx1, emptyx1,
http429x1, http503x1, http502x1, invalid_ssex1, duplicatex1, http500x1 — all absorbed.

## (5) RAM (acpbench sampler, VmRSS over process tree)

| Phase | RSS |
|---|---|
| startup-init | 319 MB |
| idle-after-initialize | 305–330 MB |
| idle-after-new-session | 276–350 MB |
| prompt-turn peak | 381 MB |
| post-turn | 306–322 MB |
| concurrency-multi N=1 peak | 393 MB |
| concurrency-multi N=3 peak | 900 MB (marginal 183 MB) |
| concurrency-multi N=5 peak | 1631 MB (marginal ~266 MB) |
| concurrency-multi N=10 peak | 3140 MB, 10/10 ok (marginal ~285 MB) |

Startup (init) 0.7–1.4 s cold; TTFC 16 ms. Marginal/session N=5–10: **~265–285 MB**.

## (6) Catalog

- `GET /__stats catalog_fetches` = 0 before AND after `PUT /__catalog ["bench-cat-a","bench-cat-b","gpt-5-fake"]` + a fresh `set_model_config` run: codex issues **zero catalog/model-list HTTP calls**; model list is bundled in the binary.
- Post-PUT discoveredModels identical (bundled `gpt-6.1-sol…`); `bench-cat-*` never appear in ACP model options.
- Host override path: no remote catalog to override. Per-session routing via `providers/set` (client-driven, zero-config) or `CODEX_CONFIG` env (`model_providers.<id>={name,base_url,wire_api:"responses"}` + `model` any string); `session/set_model` accepts listed ids only.

## (7) Integration verdict (Go Daemon)

Spawn `node …/codex-acp/dist/index.js` over stdio with an isolated HOME (per-user CODEX_HOME accumulates sqlite/rollout logs — isolate and rotate).
Required: `CODEX_API_KEY` (fake accepted) + `CODEX_CONFIG` custom-gateway block pointing at the provider base URL with `wire_api:"responses"`, `MODEL_PROVIDER=custom-gateway`, `NO_BROWSER=1`, and either an `authenticate {api-key}` call after initialize or `DEFAULT_AUTH_REQUEST`. No `--kill-group` needed; stdio MCP servers ARE forwarded to the model (fakellm log proof) despite caps advertising http+sse only and no `mcp_initialized` notification (do not gate on it). Cancel/steer/resume/images all solid; expect ~7 s/turn overhead even against an instant model; budget ~285 MB/session. Never hangs on provider stalls (own ~10 s stream timeout; 5xx tails ~26 s) — but a Daemon MUST scan turn text for `stream disconnected…`, `exceeded retry limit`, `unexpected status` because stopReason stays end_turn (truncate/empty faults look like success otherwise). `session/set_config_option` unimplemented; modes via `session/set_mode`.

## ADDENDUM A — wire-validity audit (2026-10-15 ~04:00 UTC, curl-verified against fakellm2)

Direct curl probes of every `FAULT:<class>` on all four paths (chat±stream, responses±stream)
plus `/__stats by_fault` deltas and per-run timestamp correlation against
`fakellm-18720.jsonl` show:

- REAL on the wire: `http429/500/529` (HTTP errors), `midstream-error`→`sse_error`
  (mid-stream `event: error`), `reset`→`crashconn` (mid-stream TCP RST), `empty`
  (zero-content stop). Codex retried http500/529 ~30×/turn (~1.2 req/s for ~25 s),
  midstream/reset/empty ~6×/turn; http429 exactly 1×/turn (no retry on 429).
- VOID (recognized but no-op; clean `OK` on the wire): `stall-5s`, `stall-forever`
  (`fault=stall` is logged but nothing ever holds — 0.5 s clean OK by curl on every
  path) and `truncate` on streaming paths (alias `truncate` unmapped; non-streaming
  chat `truncate` RSTs instead of truncating — so it never tests truncation either).
- Consequence: §3 `stall-5s`, `stall-forever` and `truncate` rows measured CLEAN TURNS,
  not faults. Codex's `truncate` error text (`stream disconnected…`, end_turn) was
  generated on a clean wire — itself a minor codex oddity (error text with no wire fault).
  The `retried-success … retryCount: 11` labels are a probe artifact (`parseRetryCount`
  falls back to the update count). The 120 s-hang question is answered by Addendum B, not §3.

## ADDENDUM B — REAL stall: HANG directive, 130 s budget (`w2-hang.py` raw ACP driver)

One `HANG` prompt (fakellm sends 1 chunk then holds the connection open; curl-verified
12 s hold on both APIs). Result: **`HANG-no-response-in-130s`** — 1 wire request
(`fault=hang`, no retries), 3 agent updates in the first 0.01 s, then silence; driver
killed the session. **Codex never surfaces a timeout on a stalled provider stream:
a single hung stream hangs the turn (and session) indefinitely.** The Daemon must enforce
its own turn timeout and `session/cancel`. (This overturns any stall-proof reading of §3.)
