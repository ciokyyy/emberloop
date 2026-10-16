# crow-cli 0.1.33 — final bench result (worker 4)

Verdict: **USABLE (best of five)** — the only agent with a working end-to-end MCP tool
loop (bare tool names), full session ops, real cancel, and crash recovery (with
`--kill-group`). Slow turns (6–20 s) and slow bounded retries (429/500 ~15–20 s);
two sharp edges: provider hangs are waited out forever (150 s+, no agent timeout) and
empty responses trigger ~7.5 min retry storms ending in SILENT empty turns.

## Install / start

- Artifact: GH `crow-cli-linux-x86_64.tar.gz` (Python PyInstaller bundle, 67 MB;
  `crow-cli-0.1.33.dist-info` inside). **NOT the venv** (has no crow package).
- `crow-cli acp --config-dir <home>/.agents/crow`, cwd=home, env `HOME FAKE_KEY`.
- `~/.agents/crow/config.yaml`: `providers.fakellm {api_key, base_url …/18766/v1}`,
  `models: {fake-small (default), fake-large}`. `crow-cli models` works offline.
- acpbench2 flags: `--mcp-stub --cooperate --fake-llm-url … --auth-method auto`
  (advertises a `uvx` terminal-auth method; `auto` needs no interaction).
- **`--kill-group` REQUIRED**: without it, SIGKILLed crow leaves grandchildren holding
  stdio and the probe hangs (same class as qwen). Bundle prints a benign
  `RequestsDependencyWarning` (missing chardet) and fastmcp stdio-teardown
  `ExceptionGroup`s in stub logs (stub-side noise).

## Scenario results (grouped runs; `--scenario all` exceeds crow's per-run budget)

| group | scenario | verdict |
|---|---|---|
| g1 | initialize | ok 7.5–10 s |
| g1 | new_session | ok 11.6–15 s (mcpReady no-notification, stub `tools/list` seen) |
| g1 | prompt_text | **ok 5.8–6.9 s, stub-verified echo** (only agent to pass: announces BARE `echo`/`slow`) |
| g2 | cancel | **ok 6–11 s** (real slow-turn cancel via `slow` tool) |
| g2 | steer_followup | ok (queued; no steering advertised) |
| g2 | load_resume | ok 12.5 s |
| g3 | set_model_config | ok 5.5 s (`applied:model=fakellm:fake-bench-small`) |
| g3 | images | ok 6.5 s |
| g3 | permission | skipped (never asks on echo turns) |
| gap3 | crash_recovery | **ok 7 s** (detection 18 ms) with `--kill-group`; WITHOUT it the probe hangs |
| gap3/driver | concurrency | ok N=1 (190 MB) and N=3/N=5 |

## Soak / cancel-mix / fault soak

- soak 25+25 (split: 50×~8 s exceeds one <10 min hold): **50/50 ok, 0 hangs**,
  p50 157–189 ms, p95 230–255 ms (warm turns; first-turn tool loop ~6 s).
- cancel-mix 30: 24 ok + 6 cancelled (all slow turns cancelled mid-`slow`-tool; ONLY agent where cancel-mix cancels), 0 hangs, p50 198 ms, p95 14 s.
- fault soak 40 @0.1/seed7 (swap-validated): 38 ok + 2 protocol-error, **0 hangs** (81 reqs ≈ 2/turn, 7 faults fired).

## provider_fault matrix (3 reps)

| class | result |
|---|---|
| http429 | clean-error ~14–19 s ×3 (bounded retry w/ backoff, then surfaces) |
| http500 | clean-error ~16–21 s ×3 |
| http529 | clean-error ~6.5–7.5 s ×3 (fewer retries) |
| reset | clean-error ~5.6–7 s ×3 |
| empty | **silent-empty-turn after 447/492/449 s ×3** (~7.5 min retry storm, then silent empty end_turn — worst empty behavior of the five) |
| stall-5s / stall-forever (stock) | DEGRADED (500 ms): retried-success ~6–8 s |
| truncate (stock) | VOID: `truncated-unflagged` artifact |

TRUE: hang → **pure hang, no surface within 150 s in 6/6 reps** (exactly 1 hung req per rep in fake log, no retry, no timeout; all reps killed by bound, zero reports — the probe's 90 s turn timeout never fires, implying crow sends keepalive traffic while waiting); truncated body → silent recovery (retries, completes `OK`, never flags) ~5–12 s ×3; 5 s stall → tolerated, end_turn OK ~10–18 s ×3.

## RAM

- idle ~162–168 MB; prompt-turn ~186 MB; N=1: 181–190 MB; N=3: ~340 MB (r2-all N=3 era);
  N=5: 911 MB → **marginal ~149 MB/session**. N=10 skipped. Init 7.5–10 s (imports).

## Catalog

- Never GETs a remote catalog (0 fetches). `PUT /__catalog` custom id did NOT surface
  (`discoveredModels` null; `set_config_option` applies `model=fakellm:<name>` from
  config.yaml). Host override: `config.yaml` models (no rebuild).

## Go-daemon verdict

Env: `HOME` + `FAKE_KEY` (real key in production); `--config-dir` flag makes per-user
dirs trivial. Spawn `crow-cli acp --config-dir …`; **always kill the process GROUP**.
Gotchas: (1) turns are slow (5–20 s) — size pools accordingly; (2) no agent-side timeout
on provider stalls or empty storms — daemon MUST enforce its own turn timeout (90 s is
too generous for empties: crow burns 7.5 min); treat silent empty turns as failures;
(3) MCP is the reference implementation (bare names, real calls, stub-verified) — the
only agent where client tools actually execute; (4) permission never triggers on
tool turns (yolo-ish default) — don't rely on ACP permission flow; (5) model switch via
`set_config_option model=fakellm:<name>` applies (transcript verification pending).
