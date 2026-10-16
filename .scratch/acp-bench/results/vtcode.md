# vtcode — benchmark result (worker3, 2026-10-14)

VERDICT: CONDITIONAL FIT — best RAM (~64 MB idle) and the only candidate with working ACP runtime
model/provider switching (`set_config_option` with real ids), but the ACP bridge does NOT wire
client-supplied session MCP servers (model saw only 5 built-ins; stdio stub ignored), sessions are
in-memory only (no cross-process resume), and `session/resume` is advertised-but-broken. Usable only if
the Daemon does not depend on per-session MCP provisioning.

- Version: 0.177.0 (`vtcode --version` → `vtcode-core 0.177.0`), pinned GH release asset
  `vinhnx/VTCode` `0.177.0`.
- Prebuilt linux exists (the brief's fallback was not needed): `vtcode-0.177.0-x86_64-unknown-linux-gnu.tar.gz`.

## Install

- Asset (gnu): https://github.com/vinhnx/VTCode/releases/download/0.177.0/vtcode-0.177.0-x86_64-unknown-linux-gnu.tar.gz
  (31,392,801 bytes; sha256 `11518d876a58c7f47dff415fd1eac15b5696aaf67afff08aa27bfa2f2335d6dc` VERIFIED against
  published `.sha256`).
- The gnu build does NOT run on this box (Debian 12 glibc 2.36; binary needs GLIBC_2.38/2.39).
- Used instead: `vtcode-0.177.0-x86_64-unknown-linux-musl.tar.gz` (32,322,571 bytes; sha256
  `b349c90cc59bdd9f32568a211a8f03c7847a84eb8f5701d41ffe0c9d77bac615` VERIFIED against published `.sha256`),
  82 MB extracted (`vtcode` + `man/`). Same version, static musl — behavior representative for protocol/config
  purposes; glibc-vs-musl performance asterisk noted.
- Install: fresh dir `/tmp/acpbench/work/vtcode/`, tar into `app-musl/`. Isolated HOME
  `/tmp/acpbench/work/vtcode/home`; fake key only.
- ACP bridge is DISABLED by default: `Error: Agent Client Protocol integration is disabled. Enable it via [acp]
  in vtcode.toml or set VT_ACP_ENABLED=1.` Enabled via user config `~/.config/vtcode/vtcode.toml` (`[acp]
  enabled = true`).
- Provider config (no rebuild): `vtcode models config openai --base-url http://127.0.0.1:18300/v1 --api-key
  fake-key-for-bench --model fake-tools` writes `~/.config/vtcode/config.toml` `[providers.openai]`
  (api_key IN PLAINTEXT — keyring via separate `vtcode secret`/`login` flows). NOTE: the openai provider
  validates the model id against its catalog (`Model 'fake-tools' not found … Check docs/models.json`), so the
  live config used a `[[custom_providers]]` entry in `vtcode.toml` (name=fakebench, base_url=…/v1,
  api_key_env=FAKEBENCH_API_KEY, model=fake-custom-model-xyz, api_format=openai-chat) +
  `vtcode acp standard --provider fakebench --model fake-custom-model-xyz`. fakellm log confirms
  `model=fake-custom-model-xyz` on every turn.

## acpbench --scenario all (with --mcp-stub)

| scenario | verdict | detail |
|---|---|---|
| initialize | ok (~1 s; init itself ~383 ms) | protocolVersion 1; loadSession true; image+audio true; NO steering; mcpCaps http-only; sessionCaps close/delete/list/resume; authMethods oauth-openai, oauth-openrouter, terminal-login, env-api-keys |
| new_session | ok | mcpReady `not-advertised` (5 s wait expires) |
| prompt_text | wrong-result (probe artifact, same as goose) | end_turn, `OK` relay, no tool_call |
| cancel | skipped (probe artifact) | manual SLOW:20000 test → `cancelled`, session reusable |
| steer_followup | skipped | no steering; race too fast |
| load_resume | protocol-error (agent limitation, see §) | `session/resume` → Invalid params always; `session/load` fallback ALSO failed cross-process in the probe |
| set_model_config | ok (mislabeled — method EXISTS, see §) | probe sent bogus id `acpbench_probe` → Invalid params, reported "unsupported". Manual: `set_config_option{configId:model|provider|primary_agent}` → OK |
| images | ok | 1x1 PNG accepted, end_turn |
| permission | skipped | nothing asked (no tool attempt in `OK`-only turns) |
| crash_recovery | skipped | race too fast |
| concurrency | ok (19.9 s) | 3/3 multi-process AND 3/3 single-process |
| soak (5x) | wrong-result (probe artifact) | 0 hangs, p50 ~215 ms |

§ Manual load/resume investigation (w3load2/w3load3): session ids are per-process counters
(`vtcode-zed-session-0` — note `zed` even with TARGET `standard`). Same-process `session/load` WORKS (returns
`configOptions`); `session/list` WORKS; `session/resume` FAILS even same-process (`-32602 Invalid params`) despite
the advertised `resume:{}` capability → resume is unimplemented (agent bug). Cross-process `session/load` of a
previous process's id FAILS → sessions are in-memory only; a Daemon restart (or per-turn respawn) loses the
session. acpbench's `load_resume` protocol-error is therefore a true agent limitation, not a probe bug.

§ `set_config_option` DOES work (probe misclassification — REAL probe bug, see final message): with real ids from
the `session/load` response (`model`, `provider`, `primary_agent`) the call returns the updated `configOptions`.
`session/set_model` is genuinely absent (`-32601`). So vtcode is the only candidate of the three with ACP runtime
model/provider switching — but only via `set_config_option` with discovered ids.

## MCP gap (verified, verdict-relevant)

Manual TOOL-directive turn (`TOOL:acpbench-stub__echo:{…}`) with the stub offered in `session/new`: the model saw
ONLY 5 built-ins (`read_file, list_files, apply_patch, exec_command, write_stdin` — fakellm `tools=` log), the
stub was never called, fakellm answered `MISSING_TOOL:acpbench-stub__echo`, and vtcode relayed that string as the
user-visible reply. `handle_new_session` ignores `arguments.mcp_servers` (source-read in ui_acp/agent.rs); the
bridge forwards only workspace tools (`[acp.zed.tools] read_file/list_files`, gated on client fs caps) per
`docs/guides/zed-acp.md` ("Forward additional tools (for example MCP proxies)" is listed as a Next Step).
http-type MCP servers untested (probe only offers stdio); stdio — what the Daemon uses — is confirmed NOT wired.

## RAM (VmRSS process-tree sums)

| condition | RSS | notes |
|---|---|---|
| idle after initialize | ~68 MB | acpbench phase |
| idle after new-session | ~61 MB | acpbench phase |
| prompt-turn / post-turn | ~71 MB | acpbench phase |
| concurrency 3-proc peak | ~190 MB total | marginal ≈ 43 MB/session (acpbench, unlocked box) |
| locked N=1: idle / turn-peak | 63.5 / 66.1 MB | w3ram.py |
| startup | spawn ~45 ms; init ~0.5–1.6 s (box-loaded) | newSession 2–32 ms (fastest of three) |

N=5/N=10 locked RAM runs were abandoned: box load hit 16 with 131 MB free and the batch stalled; the acpbench
concurrency numbers above cover the marginal. Leanest candidate by ~2x.

## Flakiness (fakellm-backed, ECHO turns + manual legs)

- Soak: 19/19 `ECHO:soak-ok` fresh-session turns ok (4+5+10 across three locked batches; 50 not reached — lock
  contention with fellow workers plus box saturation; each batch 100% clean). Turn p50 198 ms (116–528),
  first-chunk p50 198 ms.
- Cancel: 1/1 `cancelled` + reusable (SLOW:20000, cancel @2 s).
- Hangs: 0. Crashes: 0. `MISSING_TOOL` text-relay noted above (cosmetic).
- acpbench soak/prompt legs are wrong-result by the same `expectTool` artifact as goose.

## Catalog / providers

- Custom provider WITHOUT rebuild: `[[custom_providers]]` in `vtcode.toml` (USER or SYSTEM layer only — repo/
  project-layer values are rejected; `--config path` selects a file explicitly). Schema: `name, display_name,
  base_url, api_key_env, model, [context_window], [api_format: auto|openai-chat|openai-responses|
  anthropic-messages], capability defaults (supports_tools/vision/…), per-model `profiles`. Verified live.
- Built-in model-list override WITHOUT rebuild: `[providers.<name>] models = [...]` allowlist additions
  (e.g. fine-tuned `ft:…` ids for openai); per-provider `base_url` / `api_key_env` overrides in the same table.
- Built-in provider count: 29 in the ACP `provider` configOption (anthropic, copilot, deepseek, openai,
  openrouter, ollama, ollama-cloud, lmstudio, llamacpp, zai, moonshot, minimax, mimo, stepfun, qwen, mistral,
  meta, xai, nvidia, huggingface, vercel, merge-gateway, opencode-go, opencode-zen, openresponses, poolside,
  evolink, gemini + custom entries appended, e.g. `fakebench` appeared live).
- Remote catalog: curated per-provider lists compiled in (`models list` shows them; `docs/models.json`
  referenced in the not-found warning). Per-provider `*_BASE_URL` overrides (e.g. `OPENCODE_ZEN_BASE_URL`)
  redirect endpoints; no single host-overridable catalog URL found. Custom model id DOES appear in the ACP
  `model` configOption (observed `fake-custom-model-xyz` as currentValue+sole option) and is switchable via
  `set_config_option` — best ACP catalog story of the three.

## Subscription logins (docs + `login`/`auth` CLI, never logged in)

- ChatGPT sub: YES — `vtcode login openai` (ChatGPT-subscription OAuth), or `vtcode login codex [--device-code
  | --from-codex]` (imports Codex `auth.json`). Tokens in OS keychain (`secret` manages keyring/encrypted-file).
- Claude sub: NO (Anthropic = `ANTHROPIC_API_KEY` only).
- Copilot: YES — managed auth through the installed `copilot` CLI (`vtcode login copilot`; `VTCODE_COPILOT_COMMAND`
  override; `docs/providers/copilot.md`).
- SuperGrok: NO (xAI = `XAI_API_KEY` only; curated grok-4.x list).
- Antigravity: NO (no mention anywhere).
- OpenCode Go: YES (subscription key `OPENCODE_GO_API_KEY`, `provider="opencode-go"`); OpenCode Zen: key-based
  pay-as-you-go gateway (`OPENCODE_ZEN_API_KEY`), not a subscription login.

## Gotchas for a Go Daemon integration

1. Per-session MCP is the blocker: the Daemon's stdio MCP servers will be silently ignored; only read_file/
   list_files-style workspace bridging exists. (http-type MCP untested — possible but unproven escape hatch.)
2. No cross-process resume: in-memory counter ids; Daemon must keep one process per session for its lifetime.
   `loadSession:true` + `resume:{}` over-promise; `session/load` is same-process-only, `session/resume` is dead.
3. Bright spot: `session/load` returns `configOptions` (primary_agent/provider/model) and `set_config_option`
   with those ids works at runtime — the Daemon can switch provider/model mid-session (verify the follow-up turn
   hits the new model; not explicitly re-verified here).
4. ACP bridge off by default (`[acp] enabled` or `VT_ACP_ENABLED=1`); TARGET `standard` still mints
   `vtcode-zed-session-*` ids (cosmetic).
5. Plaintext `api_key` lands in `config.toml` via `models config` — prefer `api_key_env` + env (or keyring login)
   in Daemon-managed homes.
6. musl build works on old glibc; gnu build needs ≥2.38. Discount binary: 82 MB, ~64 MB RSS — cheapest to run.
7. Untested: permission flow (never triggered), `standard` vs `zed` TARGET differences, audio blocks.

## Raw commands used

```sh
export HOME=/tmp/acpbench/work/vtcode/home XDG_CONFIG_HOME=$HOME/.config XDG_DATA_HOME=$HOME/.local/share
export FAKEBENCH_API_KEY=fake-key-for-bench
# vtcode.toml: [acp] enabled=true + [[custom_providers]] fakebench -> http://127.0.0.1:18300/v1
/tmp/acpbench/bin/acpbench --cmd '/tmp/acpbench/work/vtcode/app-musl/vtcode acp standard --provider fakebench --model fake-custom-model-xyz --workspace /tmp/acpbench/work/vtcode/home' \
  --mcp-stub --fake-llm-url http://127.0.0.1:18300 --scenario all --report vt-all.json
# manual legs: w3turn.py (TOOL:/ECHO:), w3cancel.py, w3soak.sh, w3ram.py, w3load2/3.py, w3opts.py, w3setcfg.py
```
