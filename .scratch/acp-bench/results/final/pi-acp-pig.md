# FINAL pi-acp 0.0.34 + PiG 0.4.1 (Daemon pig path) — worker 1

Tools: acpbench2 `4939ebd1…`, fakellm2 `1661afA9…` (copies at /tmp/w1bin, hashes pinned;
repo binaries were being rebuilt by another worker mid-run).
fakellm2 on 127.0.0.1:18701 `--cooperate` (server-side). Reused installs:
adapter `/tmp/acpbench/work/pi-acp-node/node_modules/pi-acp/dist/index.js`,
pig `/home/cioky/.local/bin/pig` 0.4.1, shim `/tmp/acpbench/work/pi-acp-pig/bin/pi`
(`[ "$1" = "--version" ] && exit 0; exec pig --no-builtin-tools --no-skills --no-prompt-templates "$@"`),
agent dir copied to /tmp/w1/pig-agent (models.json baseUrl→18701, mcp.json server `el`
→ `acpbench2 mcp-stub --stub-log /tmp/w1/pig-stub.log` shared with the probe for stub-side proof).
Env: `PI_ACP_PI_COMMAND=<bin/pi>` + `PATH=<bin>:/usr/bin:/bin`, `PIG_CODING_AGENT_DIR`,
`PIG_HOME` (reused bench pighome), `PIG_OFFLINE=1`, `PI_SKIP_VERSION_CHECK=1`, isolated HOME.
Flags: `--mcp-stub --auth-method auto --pre-prompt-wait-mcp` (pi-acp never sends MCP-ready;
the flag acts as a 5 s settle for out-of-band MCP connect). **No `--cooperate/--fake-llm-url`**:
the probe force-appends `TOOL:<bare>:` directives when either is set and fakellm2 matches
explicit TOOL: directives exactly, so namespaced `mcp__el__echo` never matches → every turn
returns `MISSING_TOOL:echo` with no LLM call (verified by curl; NOT pig intercepting).
Plain English + server-side COOPERATE fuzzy match is the working combo; one spec'd-flags run
below documents the artifact. Per-turn latencies exclude the 5 s settle unless noted.

## (1) full `all`: plain 11 ok / 2 skipped; spec'd 7 ok / 5 skipped / 1 wrong-result

| scenario | plain verdict | latency* | note |
|---|---|---|---|
| initialize | ok | 926ms | proto 1, no steering, terminal-login auth |
| new_session | ok | 11.4s | mcpReady no-notification (never advertised); stub tools/list seen via shared log |
| prompt_text | ok | 5.7s | `mcp__el__echo`, end_turn, stubVerified, TTFC artifact −5000ms (measured across settle) |
| cancel | ok | 6.6s | slow-tool phase, `cancelled`, session reusable |
| steer_followup | ok | 6.7s | no steering → second prompt `queued`, ends end_turn |
| load_resume | ok | 6.0s | `session/resume` Method-not-found → `session/load` fallback works |
| set_model_config | ok | 485ms | bogus probe id rejected; real ids verified (see catalog) |
| images | ok | 5.5s | accepted |
| permission | skipped | — | pi never asks (no per-tool approval; matches earlier baseline) |
| crash_recovery | ok | 6.6s | SIGKILL detected fast |
| concurrency | ok | 16.9s | N=3: 3/3 multi-proc + 3/3 single-proc |
| soak (5) | ok | 6.3s | successRate 1 |
| provider_fault | skipped | — | needs --fault (matrix below) |

Spec'd-flags twin (`--cooperate --fake-llm-url`, same env): identical except
prompt_text wrong-result (`MISSING_TOOL:echo`, stub never called), cancel/steer/crash skipped
(fast cooperative turns). Soak there is vacuous (see below).

## (2) soak: 50× same-session ok; TRUE tool soak 50/50; cancel-mix + 6/6 cancels

- `soak --iterations 50` (plain): **50/50 end_turn, 0 hangs**, p50 8ms p95 17ms per turn.
  Caveat: COOPERATE fire-once → tool ran on iter 0 only; iters 1–49 are no-tool OK turns
  (same-session LLM-loop stability, not tool stability).
- `soak --iterations 50 --echo-directive 'TOOL:mcp__el__echo:{"text":"hello"}'`: **50/50,
  tool announced in all 50 iters**, stub echo calls verified. True 50-turn tool soak, 0 failures.
- `soak --iterations 30 --cancel-mix 5` (plain): 24 ok + **6 no-slow-turn, 0 cancelled**
  (same fire-once cause: slow tool never requested after iter-0 tool result). Verdict wrong-result.
- 6× fresh-session `--scenario cancel`: **6/6 `cancelled`** (~6.6s each incl. settle), plus the
  full-run cancel leg reusable. Cancel path is solid; cancel-mix needs fresh sessions or an
  explicit slow directive (probe gap, see bugs).

## (3) provider_fault matrix (3 runs/class; stall-forever via TCP proxy, truncate via by-counter)

| class | 3-run classification | time-to-surface | note |
|---|---|---|---|
| http429 | 3× quiet-end_turn w/ retry chatter | ~14.0s | initial + 3 backoff retries (2/4/8s), all 429 (4 reqs/run), then end_turn, **no error surfaced** |
| http500 | 3× same | ~14.0s | identical to 429 |
| http529 | 3× same | ~14.0s | identical to 429 |
| midstream-error | 3× clean end_turn `par` | 14–21ms | **no retry**; partial chunk returned as a complete answer, no error flag |
| stall-5s* | 3× ride-through OK | ~0.52s | *fakellm2 actually stalls 0.5s (hyphen args ignored); a true 5s stall untested |
| stall-forever* | 3× **hang** (probe timeout 125s) | never (client timeout only) | *via midstream-hold proxy (fakellm2 native never hangs); **prompt never resolves, no retry, 1 upstream conn**; pig session shows user msg with zero reply (wedged in Send) |
| truncate* | 3× quiet-end_turn w/ retry chatter | ~14.0s | *via by-counter `truncated` (`FAULT:truncate` inert); 4 truncated reqs/run then silent end |
| reset | 3× quiet-end_turn `par`+chatter | ~14.0s | partial `par` per attempt + backoff retries, then silent end |
| empty | 3× **silent-empty-turn** | 20–25ms | empty reply → empty end_turn, no error |

Pattern: HTTP-like failures are retried 3× (2/4/8s) then **swallowed into a normal-looking
end_turn** (retry chatter or partial text, never an error); stream errors/empties are not
retried at all. A 429 storm or RST streak looks like a completed junk turn to the ACP client.
No crash in 27 fault turns. **Mid-stream stall leaves the ACP prompt permanently unresolved**
(Daemon watchPigStall covers the SDK path at 90s+continue+rebuild; the ACP path has no such
watch — client turn timeout is the only surface).

## (4) fault soak: 40 iters, `--fault-rate 0.1 --fault-seed 7`: 40/40 ok, 0 hang, 0 crash

41 LLM reqs, 2 faults injected (http401×1, sse_error×1, per /__stats), both absorbed into
normal end_turns. (Phase script's own faultsoak ran clean by accident — fakellm restart failed
to bind; this is the manual redo with confirmed by_fault counts. Same-session caveat as (2).)

## (5) RAM (probe sampler, RSS over process tree): marginal ~90–106 MB/session; N=10 skipped

| phase | MB |
|---|---|
| startup-init / idle-after-initialize (adapter only) | 80 / 81 |
| idle-after-new-session (adapter+pig) | 133 |
| prompt-turn peak / post-turn | 140 / 140 |
| concurrency-multi peak N=1 / N=5 | 137 / 663 |
| perSessionMarginal | 90 (N=3 run) – 106 (N=5: (663−133)/5) |

N=10 skipped: box shared with 10+ waiters, swap active, free ~1.7GB (avail 4.5–4.8GB but
contended); earlier baseline already measured N=10 peak 1316MB. Matches baseline 105–115MB.

## (6) catalog: file-based, no remote fetch; new ids visible per fresh process, no rebuild

- `/__stats catalog_fetches` = 0 across ~400 reqs; `by_path` = only `/v1/chat/completions`
  (pig never GETs any catalog endpoint; `PIG_OFFLINE=1`).
- `PUT /__catalog ["…","fake-sneaky-remote"]`: invisible to the agent (nothing refetches).
- models.json hot-add `fake-sneaky` → fresh `set_model_config` run discovers
  `fakellm/fake-small, fake-big, fake-sneaky` in `models.availableModels`, and
  `set_config_option model=fakellm/fake-big` applies (accepted).
- Host override (Daemon): per-session `models.json` (provider baseUrl/api/apiKey/models)
  selects endpoint+ids — this run's mechanism; plus `emberloop-web/catalogsync.go` syncs the
  pi.dev per-provider catalog into pig's `models-store.json` for the SDK path (ACP path reads
  the binary's own stores; remote overlay `pi-in-go.dev` 404s, hence the Daemon sync job).

## (7) integration verdict (Go Daemon, pig-over-ACP)

Spawn `node <pi-acp-0.0.34>/dist/index.js` with `PI_ACP_PI_COMMAND=<session>/bin/pi` (absolute;
bypasses PATH for new+load) AND `PATH=<session>/bin:/usr/bin:/bin`, `PIG_CODING_AGENT_DIR`
(+`settings.json` quietStartup/compaction-off, `models.json` provider, `mcp.json` server
`el`→stub exposure direct timeout 600), `PIG_HOME`, `PIG_OFFLINE=1`, isolated HOME; select
model post-new via `session/set_config_option` (`model`, `thought_level`) — no `set_model`,
no `session/resume`, no steering, no permission round-trips. Gate the first prompt on
out-of-band MCP readiness (pi-acp never sends MCP-ready; ~1–4s, probe flag waits 5s).
Gotchas: (a) never put bare `TOOL:`/ cooperatively-worded directives in prompts when tools
are namespaced — with this fake they MISSING_TOOL the turn (fake-side exact match, but a
Daemon sending literal `TOOL:` text would hit the same wall); (b) persistent provider
failures surface as **normal end_turns** (retry chatter/partial text), so the Daemon cannot
rely on ACP errors to detect them — watch turn text; (c) **mid-stream stalls wedge the ACP
prompt forever** (no adapter timeout/retry in 3×125s trials) — the Daemon needs an ACP-side
stall watch with cancel+rebuild analogous to `pig.go` watchPigStall, or prompts hang until
the client timeout; (d) cancel works (7/7) and sessions stay reusable; (e) budget ~100MB RAM
per session.
