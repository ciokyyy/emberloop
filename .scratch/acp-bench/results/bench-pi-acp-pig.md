# bench-pi-acp-pig — pi-acp 0.0.34 + PiG Go binary (the Daemon's pig path)

Verdict: **strongest baseline — use this.** 100/100 soak turns, 20/20 cancel-mix,
~105–115 MB marginal RAM per session, full Daemon model/effort path verified,
41 built-in providers + file-based custom providers, ChatGPT-sub + Copilot + Claude-sub logins.

- pi-acp: `pi-acp@0.0.34` (npm, pinned — same as `PigACPAdapterPackage` in
  `emberloop-web/pigacp.go`). Tarball
  `https://registry.npmjs.org/pi-acp/-/pi-acp-0.0.34.tgz`,
  sha256 `4b3bf6bc71bed2b7f43dd7fcfa1906e464828e471740da593107e4f1608f2650`,
  integrity `sha512-MrCp37UbpUzkTkzTIMJQf1RYrT+kr4fgGDGFoLmBN26UupuCZp0XxVIY+7XYsMo3yDnzDeZUJ0jPyM3Si6Bkkg==`.
  Install: `npm install --prefix <dir> pi-acp@0.0.34` (9 deps). Adapter-only
  footprint is small; the bulk below is the pi package (only needed for docs —
  the adapter does NOT need node-pi installed when driving pig).
- pig: `/home/cioky/.local/bin/pig` 0.4.1 (upstream pi 1.0.3), 63,690,960 bytes,
  sha256 `110ab2ece63f0ac5a70b6bd8901ff0c740ab7172ecbeef33e94aa95e213c7179`.
  System binary used as-is (like the Daemon's `exec.LookPath("pig")`); all pig
  state isolated via `PIG_HOME`, `PIG_CODING_AGENT_DIR`, isolated `HOME`.
- Bench harness: pi-acp spawned as
  `node <pi-acp-0.0.34>/dist/index.js` with
  `PI_ACP_PI_COMMAND=<work>/bin/pi` (pi-acp honors this env for session/new and
  session/load, bypassing PATH lookup) and a `bin/pi` shim byte-identical in
  form to the Daemon's (`[ "$1" = "--version" ] && exit 0` then
  `exec pig --no-builtin-tools --no-skills --no-prompt-templates "$@"`).
  Agent dir mirrors `prepareAgentDir`: `settings.json` (quietStartup,
  compaction off), `models.json` (custom `fakellm` provider → fakellm-coop on
  127.0.0.1:18104, `openai-completions`, dummy key, models `fake-small`,
  `fake-big`), `mcp.json` (server `el` → `acpbench mcp-stub`, exposure
  `direct`, timeout 600, as in `mergedMCP` minus `EL_READY_FILE`); env
  `PIG_CODING_AGENT_DIR`, `PIG_HOME`, `PIG_OFFLINE=1`, `PI_SKIP_VERSION_CHECK=1`.
  Model chosen post-new via `session/set_config_option` (`model`,
  `thought_level`) exactly like the Daemon's `afterStart`/`applyModel`/
  `applyPigEffort`. Fake LLM: bench-local `fakellm-coop` (see probe notes).

## Scenario results (acpbench, all with --mcp-stub; session mcpServers ignored by pi-acp, MCP flows through mcp.json)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok (0.9s) | proto 1, image in, steering NOT advertised, terminal-login auth; TT-init ~0.9–1.2s |
| new_session | ok | mcpReady `not-advertised` (pi-acp never sends `_x.ai/mcp_initialized`); MCP connects out-of-band in ~1–4s |
| prompt_text | ok (0.7–0.9s) | tool `mcp__el__echo` via mcp.json, end_turn, TTFC ~0–10ms |
| cancel | ok (1.7s) | slow-tool phase, `cancelled`, session reusable |
| steer_followup | ok | no steering → second prompt `queued`, ends end_turn (pi one-at-a-time queue) |
| load_resume | ok (1.2–1.4s) | `session/resume` unsupported → `session/load` fallback works |
| set_model_config | ok* | *probe artifact*: `session/set_model` is not a method (Method-not-found) and the probe's `acpbench_probe` id is bogus. Real ids verified manually: `model=fakellm/fake-big` accepted + reflected in `configOptions.currentValue`, next turn hit fake as `fake-big`; `thought_level=low` accepted |
| images | ok | accepted (1x1 PNG) |
| permission | skipped | pi never asks: pi executes MCP tools with no ACP permission round-trip (matches pi security docs: no per-tool approval). Daemon sudo-gating must live elsewhere (it does: el-server-side + shim) |
| crash_recovery | ok | SIGKILL mid-turn detected in ~7–11ms |
| concurrency (1/3/5/10) | ok | 1/1, 3/3, 5/5, 10/10 multi-proc and single-proc |

## Soak / flakiness (the Daemon's flaky baseline — no flakes found here)

- soak --iterations 100, single session: **100/100 ok, 0 hangs, 0 wrong**,
  p50 46–53ms, p95 76–89ms, 5–9s wall. Repeated runs identical.
- cancel-mix 20 (every 5th turn slow+cancel@0.8s): **20/20 ok, 4/4 cancelled**,
  session reusable after every cancel. Cancel-ack latency ~14s when the cancel
  lands mid-execution of the 15s stub tool (adapter waits out the tool; the
  stub ignores MCP cancellation — stub artifact, real el-server cancellation
  should be faster), fast (<1s) when it lands pre-execution/streaming.
- Zero failure classes across ~250 measured turns. Harness mechanics
  (adapter↔pig RPC, MCP round trip, cancel, resume) are solid against a fast
  deterministic model; the Daemon's production flakes must come from
  real-model variance, EL_READY_FILE races, or steer-socket paths — not the
  ACP adapter layer.

## RAM (locked runs, RSS summed over process tree)

| phase | MB |
|---|---|
| startup-init / idle-after-initialize (adapter only) | 80 |
| idle-after-new-session (adapter + pig) | 124 |
| prompt-turn peak / post-turn settled | ~128 |
| concurrency peak N=1 / 3 / 5 / 10 | 128 / 385 / 648 / 1316 |

Marginal ≈ **105–115 MB per session** ((1316−124)/10). 10 pig sessions fit in
~1.3 GB. 4 procs/session at N=1 (adapter, pig, mcp-stub…), 30 at N=10.

## Catalog / providers (e)

- Custom provider WITHOUT rebuild: `<agent>/models.json`
  (`$schema https://pi.dev/schemas/models.schema.json`): provider entry with
  `baseUrl`, `api: openai-completions|openai-responses|…`, `apiKey`
  (literal, `$ENV` interpolation, or `!command`), `models: [{id,…}]`.
  Verified live: custom `fakellm` provider + 2 models appeared in pi-acp's
  session/new `configOptions` and served traffic. Reload: `/model` (or
  process restart for ACP).
- Custom model list: same file (`models` array adds/replaces per provider;
  `modelOverrides` tweaks built-ins). Verified: `fake-big` added post-hoc and
  selected via `set_config_option`.
- Remote catalog: pig docs describe the same pi.dev overlay model as node pi
  (`pi update` refreshes; `PIG_OFFLINE=1`/`--offline` disables catalog refresh
  and update checks — used for the whole bench, no real network touched).
  The Daemon additionally syncs pi.dev per-provider data into the host pig
  `models-store.json` (`catalogsync.go`) for the SDK path; the ACP path reads
  the binary's own stores.
- Built-in providers: **41** (provider-key table in pig docs `providers.md`:
  amazon-bedrock … zai-coding-cn), wire APIs per provider
  (openai-completions/responses, anthropic-messages, google, bedrock…).
- Model ids are `provider/model`; `set_config_option model` takes the
  qualified spec; the wire model sent upstream strips the prefix
  (`fakellm/fake-big` → `fake-big`).

## Subscription logins (f, docs-only, never logged in)

Tokens persist to `<agent>/auth.json` (0600-style, keep private). Built-in
OAuth targets: anthropic, github-copilot, kimi-coding, meta, openai-codex,
openrouter, radius, xai (`pig login <provider>`; device or localhost-callback
flows; refresh supported).

| login | support |
|---|---|
| SuperGrok | unknown (xAI has API key + OAuth, no SuperGrok-sub mapping found) |
| ChatGPT sub | YES — `openai-codex` provider ("OpenAI Codex (ChatGPT subscription)"), OAuth via `pig login openai-codex` |
| Antigravity | NO — no provider |
| OpenCode Go | YES — `opencode-go` (and `opencode` Zen) via `OPENCODE_API_KEY` |
| Claude sub | PARTIAL — `anthropic` via OAuth/`ANTHROPIC_OAUTH_TOKEN`; `sk-ant-oat` subscription tokens sent with Claude Code identity (warning at session start) |
| Copilot | YES — `github-copilot` via `pig login github-copilot` (device flow) or `COPILOT_GITHUB_TOKEN` |

## Gotchas for a Go Daemon integration

1. `PI_ACP_PI_COMMAND` (absolute path to the `pi` shim/binary) is the robust
   spawn pin: it bypasses PATH lookup for session/new AND session/load.
   Always ALSO set `PATH=<session>/bin:/usr/bin:/bin` (Daemon form) — a
   polluted inherited PATH nearly invalidated this bench (adapter resolved a
   foreign `el-web pig-shim` from another session's bin dir).
2. pi-acp ignores `session/new` `mcpServers`: MCP must be staged through the
   per-session `mcp.json` (+ `EL_READY_FILE` + `waitMCPReady`, since pi-acp
   never sends MCP-ready). First prompts otherwise race MCP connect (~1–4s).
3. Model select is post-new `set_config_option` (`model`, `thought_level`);
   there is no `session/set_model`. pig's `settings.json defaultModel` must be
   UNqualified (`defaultProvider` + bare `defaultModel`); a qualified value
   is silently ignored and pig falls back to `anthropic/claude-opus-4-8`
   (empty fast replies, no errors — silent misbench trap; caught via session
   file forensics).
4. No ACP steering, no permission round-trips, no `session/resume`: Steer and
   sudo-gating stay on the Daemon's shim-socket + el-server paths (as today).
5. pig thinking ladder: none/low/medium/high/xhigh (Daemon accepts
   off/minimal/low/medium/high/xhigh/max; `low` verified through the adapter).
6. Session files accumulate under `<agent>/sessions/<cwd-slug>/` (~3–18 KB
   per turn with tool traffic); the Daemon's per-session agent dirs already
   isolate this, but janitor accordingly.

## Raw commands (all under /tmp/acpbench; repo untouched)

- fake: `/tmp/acpbench/bin/fakellm-coop --addr 127.0.0.1:18104 --log …/fakellm.jsonl --models scripts/fakellm/testdata/models.json`
- adapter: `/usr/bin/node …/pi-acp@0.0.34/dist/index.js`, env above;
  runner `scripts/benchworker/run-pipig.sh <report> --scenario <csv> [--sessions N]`
- soak-mix: `acp-soak.py` with `ACP_MODEL=fakellm/fake-small ACP_PI_CMD=<shim> ACP_CWD=<cwd> CANCELMIX=1`
- model switch: `acp-pigmodel.py <adapter> fakellm/fake-big low`
- install: `npm install --prefix <dir> pi-acp@0.0.34` (sha256 above); pig is the
  system binary (sha256 above), never reinstalled.
