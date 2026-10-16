# bench-pi-acp-node — pi-acp 0.0.34 + node pi 1.2.0 (reference child)

Verdict: **clean but heavier/livelier than pig.** Same adapter verdicts as pig
(100/100 soak, all concurrency N/N), ~165–180 MB/session marginal (≈1.6× pig),
prompt turns ~40ms hot but ~2.5–4s cold (MCP connect + node warmup). Useful as
the upstream reference for pi-acp behavior; the Daemon should prefer the pig
child (leaner, same protocol surface).

- pi-acp: same `pi-acp@0.0.34` install as pig run (shared
  `/tmp/acpbench/work/pi-acp-node/node_modules/`; tarball sha256
  `4b3bf6bc71bed2b7f43dd7fcfa1906e464828e471740da593107e4f1608f2650`).
- pi: `@earendil-works/pi-coding-agent@1.2.0`, tarball
  `https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-1.2.0.tgz`,
  sha256 `1dfed308f46a69aec06bf9dc20700ddc6fe2370b1898f01334b7898397541e66`,
  integrity `sha512-h/4WaVWv1VlCaY8x8qEN5U+7jbF/e7+Ck6moPeEOFn5juMTWwNAXjwgYM7gRgbBYh8Zppwt6qWvJDzNTf4GNEQ==`.
  Install: `npm install --prefix <dir> @earendil-works/pi-coding-agent`
  (112 pkgs). Work dir total 328 MB (123 MB node_modules + sessions/logs).
- Bench harness: `node pi-acp/dist/index.js` with
  `PI_ACP_PI_COMMAND=<nodebin>/pi` (1.2.0, verified via session preambles
  "operating inside pi"), `PI_CODING_AGENT_DIR=<agent>` carrying
  `settings.json` (quietStartup, compaction off, `defaultModel`
  `fakellm/fake-small` — node pi ACCEPTS the qualified form, unlike pig),
  `models.json` (same custom `fakellm` provider → 18103), `mcp.json`
  (server `acpbench-stub` → stub, exposure `direct`). Model switch post-new
  via `set_config_option` (verified: `fakellm/fake-big` + `thought_level=low`).
  IMPORTANT HYGIENE NOTE: an early run series resolved `pi` to a foreign
  `el-web pig-shim` through inherited PATH; all numbers below are from pinned
  reruns (explicit PATH + `PI_ACP_PI_COMMAND`), verified per-session via
  preamble forensics.

## Scenario results (acpbench --mcp-stub; MCP via mcp.json, session list ignored)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok (0.9–1.8s) | TT-init ~1.2s; image in, steering out, terminal-login auth |
| new_session | ok | mcpReady not-advertised; MCP ready ~1–4s after new |
| prompt_text | ok (2.8–3.8s cold) | `mcp__acpbench_stub__echo` (node normalizes `-`→`_`); TTFC ~8–10ms hot |
| cancel | ok (4–6s) | slow-tool phase, `cancelled`, reusable |
| steer_followup | ok | `queued` (no steering; one-at-a-time queue), second ends end_turn |
| load_resume | ok (3.7–6.7s) | resume unsupported → load fallback |
| set_model_config | ok* | same probe artifact as pig; real ids verified manually (model switch reflected in fakellm log) |
| images | ok | accepted |
| permission | skipped | never asks (pi has no per-tool approval) |
| crash_recovery | ok | ~11ms detect |
| concurrency (1/3/5/10) | ok | all N/N both legs |

## Soak / flakiness

- soak --iterations 100, single session: **100/100 ok**, p50 41–45ms,
  p95 86–89ms. Plus 4×25 (100 more turns) 100/100. Zero failure classes.
- Hot turns ~25–45ms (vs pig ~50ms, vs claude ~150–300ms); cold first turn
  ~3s (node + MCP connect).

## RAM (locked)

| phase | MB |
|---|---|
| idle-after-initialize | 79–81 |
| idle-after-new-session | 184–186 |
| prompt settled | ~185–195 |
| concurrency peak N=1 / 3 / 5 / 10 | 290 / 681 / 1053 / 1950 |

Marginal ≈ **165–180 MB/session**. 10 node sessions ≈ 2 GB.

## Catalog / providers (e)

- Custom provider + model list: same `models.json` mechanism as pig (verified
  live: `fakellm/{fake-small,fake-big}` in session/new `configOptions`).
- Remote catalog: bundled + pi.dev overlay, cached offline;
  `pi update --models` forces refresh; background refresh at startup/`/model`
  open/after `/login`; `PI_OFFLINE=1` disables all network activity.
- Built-in providers: **~35+** (33 single-key env vars in the providers table
  + Azure/Bedrock/Vertex/Cloudflare-Gateway/Workers-AI/Radius specials).
- Wire APIs per provider: openai-completions/responses, anthropic-messages,
  google-generative-ai, bedrock-converse, mistral-conversations, pi-messages…

## Subscription logins (f, docs-only)

`/login [provider]` (browser/device OAuth → `auth.json`) or env API keys.
`OPENCODE_API_KEY` covers Zen + Go; `COPILOT_GITHUB_TOKEN` covers Copilot;
Anthropic OAuth token supported; no SuperGrok/ChatGPT-sub/Antigravity mapping
beyond what pig documents (node pi predates the `openai-codex` provider —
verify against latest pi before relying on ChatGPT-sub here).

## Gotchas

1. Same as pig (pins, mcp.json staging, no steering/permission/resume,
   post-new model select).
2. Tool-name normalization `-`→`_` (`acpbench-stub`→`acpbench_stub`): use
   dashboard-safe server names or match loosely.
3. `defaultModel` accepts the qualified `provider/id` form (divergence from
   pig — do NOT rely on it; select explicitly post-new).
4. Node warmup dominates cold-start latency (~3s first turn); steady-state is
   the fastest of the three baselines.

## Raw commands

- fake: `fakellm-coop --addr 127.0.0.1:18103 …`
- runner `scripts/benchworker/run-pinode.sh` (pinned PATH + PI_ACP_PI_COMMAND)
- model probe: `acp-pimodel.py <adapter> fakellm/fake-big low`
