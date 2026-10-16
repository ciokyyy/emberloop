# bench-claude-agent-acp — @agentclientprotocol/claude-agent-acp 0.88.0 (pinned) + 0.90.0 (latest)

Verdict: **viable but heavy and CPU-sensitive.** Protocol-complete (resume,
steering, images, cancel both phases), model switch verified, 50/50 soak.
Costs: ~190 MB idle / ~450 MB peak per session (4.7 GB for 10), ~2s TT-init,
first-prompt MCP race that aborts tool calls, and full stalls under high CPU
load. Pick only if Claude-subscription auth outweighs 4× RAM.

- 0.88.0 (Daemon pin, `ClaudeAdapterPackage`): tarball
  `https://registry.npmjs.org/@agentclientprotocol/claude-agent-acp/-/claude-agent-acp-0.88.0.tgz`,
  sha256 `5655e1ed94efbc2f05ee0d34a26d98697245946250b334f7e009debe14e5fba6`,
  integrity `sha512-o2bfN6S47wHTV7US/0SrDguCQVs7RhgSwA5CzC37oeDvcXTfHJFTlAquglEybIMQXwSk1zYda5Wxa0F17pOlnA==`.
  `npm install --prefix <dir>` (108 pkgs, 312 MB — 268 MB is the bundled
  `@anthropic-ai/claude-agent-sdk` + claude-cli).
- 0.90.0 (latest): same URL pattern, sha256
  `fb5538b0b433f7ec1b8dfde7c78e9197f8e9784f2407e3ff4f627436e386f66a`,
  integrity `sha512-XgbpMB0CIVDAxZKESwUPUiuXxdzlh8Ga1gT6pBufwkD38Zclo8/GP5khCTKssvLGad2T/8iASZusWcsDzlqpQQ==`
  (317 MB).
- Bench harness: `node …/dist/index.js` with isolated HOME/XDG dirs,
  `ANTHROPIC_BASE_URL=http://127.0.0.1:18101` (fakellm-coop) +
  `ANTHROPIC_API_KEY=fake-key-test` + the Daemon's disabling env
  (`ENABLE_TOOL_SEARCH=false`, `CLAUDE_CODE_DISABLE_*`, `DISABLE_TELEMETRY=1`).
  Fake auth is accepted: requests arrive as `claude-opus-5-5` (default),
  UA `claude-cli/2.1.293 (external, sdk-ts, agent-sdk/0.3.293)`.
  Daemon `_meta` replication: acpbench cannot send `_meta` (probe gap, see
  below); a manual client verified the Daemon's exact
  `_meta.claudeCode.options` shape is honored (tools list collapses 24→2).

## Scenario results (0.88.0; acpbench --mcp-stub)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok | proto 1, TT-init ~1.9s; steering advertised; authMethods []; subagents/resume/fork caps |
| new_session | ok | mcpReady `not-advertised`; MCP tools arrive ~1–3s after new (RACE, see gotchas) |
| prompt_text | ok (2.2–7.3s cold) | `mcp__acpbench-stub__echo`, 16–17 updates, end_turn, TTFC 1–130ms |
| cancel | ok ×6 runs | slow-tool AND streaming phases both `cancelled`, session reusable every time |
| steer_followup | ok | steering advertised → `_session/steering` `injected` |
| load_resume | ok (12s) | `session/resume` **used** (native resume works) |
| set_model_config | ok* | *probe artifact*: `session/set_model` does not exist; probe's `acpbench_probe` id is unknown. Real ids verified manually: `set_config_option model=sonnet` accepted, next turn served as `claude-sonnet-5-5` |
| images | ok | accepted |
| permission | approve ok / deny ARTIFACT | approve leg (allow-once) fine. Deny leg (`outcome: cancelled`) flagged "denied tool still ran" — disproven with a logging MCP proxy: the stub's `tools/call` was NEVER invoked; the adapter emits `tool_call` updates optimistically pre-permission, which the probe miscounts as execution. Transcript acknowledges the denial |
| crash_recovery | ok | SIGKILL detected ~14ms |
| concurrency (1/3/5/10) | ok | 1/1, 3/3, 5/5, 10/10 both legs |

## 0.90.0 status (rerun 2026-10-14 late, locked)

- Group A (init/new/prompt/setmc/images): identical to 0.88.0, all ok.
- Soak-20: 20/20 ok (p50 300ms). Model/effort/config surface unchanged
  (`model`/`mode`/`effort`/`fast` config ids; aliases incl. `opus/sonnet/haiku`).
- B/C via stock acpbench (no `_meta`, 15s MCP wait + 6s settle, load 1.7–4):
  cancel/steer/crash STILL skipped (fast abort, see below), load_resume hung
  90s a third time. The abort reproduces at low load, so it is NOT
  load-caused: without the Daemon `_meta`, the first MCP-tool turn is aborted
  adapter-side (`tool_result: "Tool permission request failed: Error: Tool use
  aborted"`, confirmed in the fakellm-coop request log — the fake DID emit
  the slow tool call). acpbench cannot send `_meta` (probe gap), so stock
  acpbench cannot exercise claude B/C meaningfully.
- B/C via manual client WITH the Daemon's exact `_meta` (daemon-meta.json,
  6s settle, locked): **cancel ok** (slow turn `stop=cancelled`, tool was
  executing at cancel time), **steer ok** (`_session/steering` outcome
  `injected`, turn ended `end_turn`), **crash ok** (SIGKILL detected in
  ~50ms, pending prompt never resolves = fail fast), **resume/load legs ok**
  (`session/resume` answered natively in 51ms, cross-process `session/load`
  in ~1s). So 0.90.0 matches 0.88.0 on every B/C leg in the Daemon's
  configuration.
- Caveat — tool-less turns flake (BOTH versions): plain-text prompts with no
  tool call ("Remember turn one.", "history-check please") intermittently
  wedge into an SDK re-query storm (fake sees the same request retried with
  1,1,2,5,9,…19s backoff, turn never completes in 75–90s). Observed 5× on
  0.90.0 (with and without `_meta`) and 1× on 0.88.0 (21:25 `load0880.json`
  hang; the same leg passed 12s at 19:28 in `gC0880.json`), so it is a
  load-sensitive flake of tool-less turns, NOT a 0.90.0 regression.
  Tool-bearing turns in the same load windows completed every time. Daemon
  impact is limited (real prompts always carry the MCP tool surface), but
  bare tool-less follow-ups should be treated as suspect past ~30s.

## Soak / flakiness (0.88.0, Daemon-meta manual client, locked)

- soak 50 same-session echo turns: **50/50 ok, 0 hangs**, p50 150ms,
  p95 200ms, 6.7s wall. RSS idle 303 MB, peak 528 MB.
- cancel-mix 20: **20/20 ok, 4/4 cancelled properly**, reusable after.
- acpbench soak was unusable stock (multi-turn tool loop against the fake —
  fake-side gaps, see probe notes); manual-client numbers above are the valid
  ones and match acpbench single-turn verdicts.

## RAM (locked)

| phase | MB |
|---|---|
| startup / idle-after-initialize | ~100 / 190–215 |
| idle-after-new-session | ~300–313 |
| prompt-turn peak | 430–590 (0.90.0 sample highest; treat as range) |
| post-turn settled | ~330–345 |
| concurrency peak N=1 / 3 / 5 / 10 | 565 / 1421 / 2435 / 4785 |

Marginal ≈ **370–450 MB/session**. 10 sessions peak 4.8 GB — does not fit an
8 GB box alongside other work (measured with an OOM guard, no OOM fired but
no headroom left).

## Catalog / providers (e)

- Model list comes from the Claude Agent SDK (Anthropic API), surfaced as the
  `model` configOption (`default` + concrete ids; family aliases
  opus/sonnet/haiku/fable resolved fuzzily, incl. version targeting).
- Restrict WITHOUT rebuild: Claude settings `availableModels` allowlist +
  `deniedModels` (managed) — honored by the adapter's picker.
- Custom provider WITHOUT rebuild: `ANTHROPIC_BASE_URL` redirect works
  (proven), but model IDs remain Claude's — there is NO models.json-style
  custom endpoint+model catalog. A fake/local model must impersonate the
  Anthropic API AND the exact SSE shape (see probe notes).
- No host-overridable remote catalog (SDK talks to the API directly).
- Token/window: usage reports input/output tokens; window 1M (claudeWindow).

## Subscription logins (f, docs-only + env observation, never logged in)

- Claude subscription: via the Agent SDK auth (OAuth token file under the
  isolated HOME / `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_OAUTH_TOKEN`; the
  adapter also offers terminal `claude /login` and reports auth status).
  The Daemon's `CLAUDE_CODE_OAUTH_TOKEN` + setup-token flow maps here.
- API key (`ANTHROPIC_API_KEY`) works (bench used a fake one).
- SuperGrok / ChatGPT-sub / Antigravity / OpenCode-Go / Copilot: none (this
  adapter is Claude-only by construction).

1. **First-prompt MCP race (both versions, worse on 0.90.0):** tools are
   listed before the MCP servers connect; a prompt sent immediately after
   session/new that uses an MCP tool gets
   `Tool permission request failed: Error: Tool use aborted` fed back as
   model context (and the failure prefix accumulates across turns). Waiting
   does NOT fix it (15s MCP wait + 6s settle still aborts); the Daemon's
   `_meta` bypass-permissions posture DOES (manual B/C all green with
   `_meta`). The Daemon does NOT wait for MCP-ready on claude today (unlike
   pig's `waitMCPReady`) — keep the `_meta` bypass and consider an
   MCP-ready gate or aborted-turn retry as belt-and-braces.
2. There is no `session/set_model` — model select is
   `session/set_config_option` with `configId: "model"` (Daemon's `SetModel`
   already does this; verified).
3. `outcome: cancelled` on permission = deny: tool does NOT execute, but a
   `tool_call` update was already emitted (activity trackers see intent).
4. CPU-hungry: node adapter + bundled CLI stall (multi-minute hangs, SDK
   backoff storms) when the 4-core box exceeds ~load 8; pig/node-pi sailed
   through the same load. Budget CPU or isolate.
5. TT-init ~2s and ~190 MB idle per session before any work happens.
6. acpbench cannot send `_meta` (no flags): Daemon `_meta` posture
   (stripping options, bypass mode) is unverifiable with the stock probe.
7. Tool-less turns wedge: a prompt that calls no tool can spin the SDK into
   a re-query storm (never completes in 75–90s) on both versions, more
   often under load; tool-bearing turns in the same windows always
   completed. Real Daemon prompts carry the MCP tool surface, but treat a
   bare tool-less follow-up with no completion past ~30s as suspect.

## Raw commands

- fake: `fakellm-coop --addr 127.0.0.1:18101 …` (bench-local patches, see probe notes)
- runner `scripts/benchworker/run-claude.sh <v0880|v0900> <report> --scenario <csv>`
- manual: `acp-manual.py` (deny proof + setmodel), `acp-soak.py` (soak/mix +
  RSS), `acp-soak2.py/acp-t2.py` (multi-turn repro)
- 0.90.0 B/C rerun: `ACPBENCH_MCP_WAIT=15s ACPBENCH_SETTLE_WAIT=6s flock
  /tmp/acpbench/bench.lock run-claude.sh v0900 gBC0900-rerun.json --scenario
  cancel,steer_followup,crash_recovery,load_resume` (abort reproduces at low
  load); `acp-cancelN.py <adapter> 3` (cancel with `_meta`),
  `acp-bc-manual.py <adapter> <steer|crash|resume>` (B/C with `_meta`),
  `acp-plain1.py <adapter> "<text>"` (tool-less turn repro; `ACP_META`
  set/empty selects with/without `_meta`, `ACP_STDERR` captures adapter log)
- installs: `npm install --prefix <dir> @agentclientprotocol/claude-agent-acp@{0.88.0,0.90.0}` (+ `npm pack` for sha256)
