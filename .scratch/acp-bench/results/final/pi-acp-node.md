# FINAL pi-acp 0.0.34 + node pi 1.2.0 (upstream reference) — worker 1

Tools: acpbench2 `4939ebd1…`, fakellm2 `1661afA9…` (/tmp/w1bin). fakellm2 on 127.0.0.1:18702
`--cooperate`. Reused installs: adapter (shared
`/tmp/acpbench/work/pi-acp-node/node_modules/pi-acp/dist/index.js`), pi
`@earendil-works/pi-coding-agent@1.2.0` via `node_modules/.bin/pi`. Agent dir /tmp/w1/node-agent
(models.json fakellm→18702, mcp.json server `acpbench-stub`→`acpbench2 mcp-stub --stub-log`
shared with probe). Env: `PI_ACP_PI_COMMAND=<.bin/pi>` + `PATH=<.bin>:/usr/bin:/bin`,
`PI_CODING_AGENT_DIR`, `PI_OFFLINE=1`, `PI_SKIP_VERSION_CHECK=1`, isolated HOME.
Flags: `--mcp-stub --auth-method auto --pre-prompt-wait-mcp`, plain English prompts
(no `--cooperate/--fake-llm-url`; spec'd twin run documents the MISSING_TOOL artifact, same
mechanism as pig: fakellm2 explicit `TOOL:` needs exact name). Tool name normalized by node:
`mcp__acpbench_stub__echo` (`-`→`_`). Node lists built-in read/bash/edit/write alongside MCP
tools in LLM requests. Latencies include the 5 s MCP settle unless noted.

## (1) full `all`: plain 11 ok / 2 skipped; spec'd 7 ok / 5 skipped / 1 wrong-result

| scenario | plain | latency | note |
|---|---|---|---|
| initialize | ok | 865ms | proto 1, no steering, terminal-login auth |
| new_session | ok | 13.3s | mcpReady never advertised; MCP out-of-band |
| prompt_text | ok | 7.6s | stubVerified, end_turn |
| cancel | ok | 8.2s | slow-tool, `cancelled`, reusable |
| steer_followup | ok | 8.2s | `queued`, ends end_turn |
| load_resume | ok | 9.0s | resume Method-not-found → load fallback |
| set_model_config | ok | 2.2s | real ids via set_config_option |
| images | ok | 7.2s | accepted |
| permission | skipped | — | never asks |
| crash_recovery | ok | 8.1s | fast detect |
| concurrency | ok | 27.1s | 3/3 both legs |
| soak (5) | ok | 8.3s | successRate 1 |

Spec'd twin: prompt_text wrong-result (MISSING_TOOL), cancel/steer/crash skipped. Same artifact as pig.

## (2) soak: 50/50; true tool soak 50/50; cancel-mix + 6/6 cancels

- `soak 50`: **50/50 end_turn, 0 hangs**, per-turn min/med/max 7/9/111ms
  (same-session; COOPERATE fire-once → tool effectively on early iters only).
- `soak 50 --echo-directive 'TOOL:mcp__acpbench_stub__echo:{…}'`: **50/50, tool in all 50**.
- `soak 30 --cancel-mix 5`: 24 ok + 6 no-slow-turn (same fire-once cause), 0 cancelled.
- 6× fresh `--scenario cancel`: **6/6 `cancelled`** (~8.2s each incl. settle).

## (3) provider_fault matrix (3/class; stall-forever via proxy, truncate via by-counter)

| class | 3-run result | time-to-surface |
|---|---|---|
| http429 / http500 / http529 | 3× quiet-end_turn w/ retry chatter each (initial + 3 backoff retries, all faulted) | ~14.1s, no error surfaced |
| midstream-error | 3× clean end_turn `par`, no retry | ~0.1s |
| stall-5s* | 3× ride-through OK | ~0.59s (*0.5s actual, see pig notes) |
| stall-forever* | 3× **hang** (125s client timeout) | never (*proxy; prompt never resolves, no retry) |
| truncate* | 3× **silent-empty-turn**, 1 req each, no retry | ~0.1s (*by-counter; **DIVERGES from pig**, which retries noisily) |
| reset | 3× quiet-end_turn `par`+chatter (retried) | ~14.1s |
| empty | 3× silent-empty-turn | ~0.1s |

No crash in 27 turns. Truncate divergence is the one behavioral delta vs pig 0.4.1 found.

## (4) fault soak 40 @0.1 seed7 (redo, confirmed faults): 40/40 ok

41 reqs, injected http401×1 + sse_error×1 absorbed. (Phase script's own faultsoak ran clean
by accident — fakellm restart failed to bind; this redo has confirmed by_fault counts.)

## (5) RAM: marginal ~169–171 MB/session; N=10 skipped

startup-init 85 / idle-init 85 / idle-session 201 / turn ~215 / conc1 315 / conc5 1058 MB;
marginal (1058−201)/5 ≈ 171 (all-run N=3: 169). ~1.6× pig. N=10 skipped (shared box, swap
active; baseline measured 1950MB peak).

## (6) catalog: identical file mechanism to pig

models.json hot-add `fake-sneaky` → discovered with no rebuild; `set_config_option`
model applies; `catalog_fetches` = 0 over ~300 reqs (never fetched; `PI_OFFLINE=1`);
`PUT /__catalog` invisible. Remote overlay: bundled + pi.dev (`pi update --models`; offline disables).

## (7) integration verdict

Same Daemon contract as pig (pins, mcp.json staging + 5 s MCP settle, post-new model select,
no steering/permission/resume). Prefer the pig child (same protocol surface, 1.6× leaner,
truncate fails noisily rather than silently). Node-only notes: tool `-`→`_` normalization
(use dashboard-safe MCP server names); `defaultModel` accepts qualified `provider/id`
(diverges from pig — select explicitly post-new anyway); cold first turn slower (node warmup
+ MCP), hot turns fastest (~40ms); built-in tools are offered to the model unless stripped.
Same two Daemon-side requirements as pig: ACP-side stall watch (prompt wedges forever) and
no reliance on ACP errors for provider failures (swallowed into normal end_turns).
