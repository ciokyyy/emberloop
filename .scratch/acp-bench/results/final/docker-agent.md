# docker-agent v1.150.0 — final bench result (worker 4)

Verdict: **USABLE** — fastest native startup (1.1 s), full session ops (resume + both
load legs + history), all valid fault classes surface cleanly in 0.5–5 s, zero hangs
anywhere, MCP forwarded (but tools renamed to per-session hashes). Gateway mode
(`--models-gateway`) opens sessions but prompting fails with Internal error.

## Install / start

- Artifact: GH `docker-agent-<os>-<arch>` standalone binary (Go).
- `docker-agent serve acp agent.yaml --config-dir …/home/config --data-dir …/home/data
  --session-db …/home/session.db`, env `HOME=<iso> FAKE_KEY=fake-test-key`.
- `agent.yaml`: `providers.fakellm {base_url: …/18762/v1, token_key: FAKE_KEY}`,
  `models.fake_small {provider: fakellm, model: fake-bench-small}`, root agent on it.
- acpbench2 flags: `--mcp-stub --cooperate --fake-llm-url … --auth-method auto`
  (stdio MCP accepted though caps advertise only http+sse; no `--kill-group` needed).

## Scenario results (`--scenario all`)

| scenario | verdict | detail |
|---|---|---|
| initialize | ok 1.1–1.5 s | loadSession, close/delete/list/resume/additionalDirectories, audio/image/embedded |
| new_session | ok 6.2 s | mcpReady no-notification, stub `tools/list` seen |
| prompt_text | wrong-result | stub never invoked: provider sees tools as `acp_<32hex>` per-session hashes (TOOL:echo + fuzzy both unmatchable) |
| cancel / steer | skipped | raced (fast ~5 ms turns) |
| load_resume | ok | resume USED; loadSameProcess ok; loadCrossProcess ok, history verified |
| set_model_config | ok | `applied:model=default` (applied-unverified; `discoveredModels` null) |
| images | ok | accepted |
| permission | skipped | never asks on echo turns |
| crash_recovery | skipped | raced |
| concurrency N=3 | ok | 3/3 both modes |
| soak 5 | ok | 5/5, p50 5 ms |

## Soak / cancel-mix / fault soak

- soak 50: **50/50 ok, 0 hangs**, p50 9 ms, p95 17 ms.
- cancel-mix 30: 24 ok + 6 no-slow-turn, 0 cancelled, 0 hangs (slow tool unreachable — hashed).
- fault soak 40 @0.1/seed7 (swap-validated): 39 ok + 1 protocol-error, **0 hangs** (41 reqs, faults http401+sse_error fired).

## provider_fault matrix (3 reps)

| class | result |
|---|---|
| http429 | clean-error ~2.2 s ×3 (`model failed: error receiving from stream: HTTP 429`, no retry storm) |
| http500 | clean-error ~4.9 s ×3 (limited retries w/ backoff, then surfaces) |
| http529 | clean-error ~4.8 s ×3 |
| midstream-error (=sse_error) | clean-error ~0.8–1.1 s ×3 (partial `parparpar` + streamed error) |
| reset | clean-error ~0.9 s ×3 |
| empty | surfaced-error ~0.4–0.8 s ×3 (`Warning: Model … returned an empty response`) |
| stall-5s / stall-forever (stock) | DEGRADED (500 ms stall): retried-success ~660–750 ms |
| truncate (stock) | VOID (fault never fires): `truncated-unflagged` artifact |

TRUE stalls: hang → **timeout-hang @90 s ×3** (`par` partial; agent never gives up itself);
truncated body → end_turn WITH warning `Model … returned an empty response (stop r…)` 150–405 ms ×3
(flagged, completes); 5 s stall → end_turn OK @5.2 s ×3 (tolerated).

## RAM

- startup-init 82 MB; idle ~95 MB; prompt-turn ~105 MB; N=1: 93 MB; N=5: 518 MB →
  **marginal ~85 MB/session**. N=10 skipped (avail 3.8 GB). Init ~1.1 s (fastest native).

## Catalog / gateway

- Never GETs a remote catalog (only my curl in logs). ACP models from `agent.yaml` only;
  `PUT /__catalog` custom id did NOT surface. Host override: `agent.yaml`
  providers/models (no rebuild).
- Gateway mode (`agent-gw.yaml` model `openai/fake-bench-small` +
  `--models-gateway http://127.0.0.1:18762/v1`): `session/new` ok, `session/prompt`
  → protocol-error `acp: Internal error` (82 B, 6 updates, no tool call). Gateway path
  half-works; needs real-gateway docs investigation, not benchable against fakellm2 as-is.

## Go-daemon verdict

Env: `FAKE_KEY` (or real token key) + `--config-dir/--data-dir/--session-db` flags (all
overridable → per-user dirs easy). Spawn `serve acp <agent.yaml>`. PID kill suffices.
Gotchas: (1) client MCP tools are forwarded but renamed to per-session `acp_<hash>` —
correlating tool calls across sessions is impossible; pass-through works for real models
that read descriptions; (2) no stdout MCP in caps but stdio stub works; (3) cancel/load/
resume/set_config all verified; model switching unverified transcript-side; (4) fault
behavior is the best of the five (bounded retries, fast clean surfaces, warnings on
empty/truncated); (5) gateway mode is not a drop-in fake endpoint — verify against a real
gateway before using `--models-gateway`.
