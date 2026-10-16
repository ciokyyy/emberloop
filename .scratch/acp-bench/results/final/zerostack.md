# zerostack v1.8.5 — final bench result (worker 4)

Verdict: **MARGINAL** — thinnest ACP surface of the five (no load/resume, no
discoverable models, full client-MCP drop, hang-on-cancel confirmed), but what exists
is fast, lean (11 MB/session) and surfaces errors crisply (429/500/refusals in ≤2 s,
precise truncation message). Kept short per brief: hang + MCP drop confirmed with the
fixed probe, no extended debugging.

## Install / start

- Artifact: GH `zerostack-x86_64-unknown-linux-musl.tar.gz` (Rust). GPL-3.0-only.
- `zerostack --acp` (stdio), env `HOME=<iso> FAKE_KEY=fake-test-key`.
- `~/.local/share/zerostack/config.yaml`: `custom_providers.fakellm {provider_type:
  openai, base_url …/18763/v1, api_key_env: FAKE_KEY}`, `default_permission_mode: yolo`.
- acpbench2 flags: `--mcp-stub --cooperate --fake-llm-url … --auth-method auto`
  (no auth methods advertised; stdio MCP offered but ignored).

## Scenario results (`--scenario all`)

| scenario | verdict | detail |
|---|---|---|
| initialize | ok ~1 s | NO loadSession; mcp http:false sse:false; NO image/audio/embedded; NO session caps |
| new_session | ok 6.7 s | mcpReady no-notification; `toolListSeen` FALSE (never lists stub) |
| prompt_text | wrong-result | MISSING_TOOL path; provider tools = native only |
| cancel / steer | skipped | raced |
| load_resume | protocol-error | no `session/resume` AND no `session/load` (`Method not found`) |
| set_model_config | skipped | no discoverable model id |
| images | ok | accepted (despite caps) |
| permission | skipped | never asks |
| crash_recovery | skipped | raced |
| concurrency N=3 | ok | 3/3 both modes |
| soak 5 | ok | p50 ~50 ms |

## Hang on cancel — CONFIRMED

- cancel vs slow fake (15 s base latency): **timeout-hang** — cancel sent mid-streaming
  (`cancelPhase: streaming`), turn never returned `cancelled` within 10 s. Production
  cancel of a streaming turn hangs the session. (Single confirmation, kept short.)

## MCP drop — CONFIRMED (double)

- Discovery: stub `tools/list` NEVER called (`toolListSeen: false` across all runs).
- Forwarding: provider tool list = native only
  (`read write edit bash grep find_files list_dir todo_write task memory_*`), zero stub names.
- Client `mcpServers` are silently ignored end to end.

## Soak / fault soak

- soak 50: **50/50 ok, 0 hangs**, p50 30 ms, p95 52 ms.
- cancel-mix 30: 24 ok + 6 no-slow-turn, 0 cancelled, 0 hangs.
- fault soak 40 @0.1/seed7 (swap-validated): 38 ok + 2 wrong (both `refusal`), **0 hangs**.

## provider_fault matrix (3 reps)

| class | result |
|---|---|
| http429 | clean-error (`refusal`) ~1.8 s ×3 (`ProviderResponseError: status 429…`) |
| http500 | clean-error (`refusal`) ~1.8 s ×3 |
| http529 | clean-error ~50–220 ms ×3 (`<unknown status code>` — SDK doesn't know 529, still surfaces) |
| midstream-error (=sse_error) | clean-error ~100–180 ms ×3 |
| reset | `refusal` 28–172 ms ×3 (`HttpError: … error decoding response body`) — probe verdict `unexpected-stop` (classifier doesn't match `refusal`; reclassified clean) |
| empty | `refusal` ~100 ms ×3 (`empty response too many times, aborting` — retries internally first) — same probe gap |
| stall-5s / stall-forever (stock) | DEGRADED (500 ms): retried-success ~560 ms |
| truncate (stock) | VOID: `truncated-unflagged` artifact |

TRUE: hang → **timeout-hang @90 s ×3** (`par` partial; never gives up); truncated body →
`refusal` @~85 ms ×3 (`ResponseError: the reply ended before the provider ended it` —
precise); 5 s stall → end_turn OK @5.06 s ×3 (tolerated).
Agent stderr shows one retry (`retryable error on first…`) then a surfaced ERROR — retries once, then fails crisply.

## RAM

- startup-init 10 MB; idle 13–21 MB; N=1: 28 MB; N=5: 73 MB → **marginal ~11 MB/session**.
  N=10 skipped. Init ~1 s. Leanest alongside fx.

## Catalog

- Never fetches (0 across all runs); no discoverable model ids anywhere. Host override:
  `config.yaml` provider/model (no rebuild).

## Go-daemon verdict

Env: `FAKE_KEY` (or provider key env) + `HOME` for the config dir. Spawn `zerostack --acp`.
Gotchas: (1) cancel of a live turn hangs — daemon must kill + respawn, never reuse a
cancelled session; (2) NO resume/load — every session is fresh, no history restore;
(3) client MCP servers are dropped — do not offer any (wasted bytes, zero effect);
(4) model choice is config-file + restart only; (5) error surfaces are good
(refusal + precise text for 429/500/truncation/empty-after-retries) but arrive as
`refusal`, not `end_turn` — the v2 classifier mislabels these as `unexpected-stop`;
(6) GPL-3.0-only license.
