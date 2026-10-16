# acpbench — ACP client probe/benchmark

A black-box ACP (agentclientprotocol.com, JSON-RPC over stdio,
newline-delimited) **client** that tests any agent command and emits one
JSON report. Standalone Go module, stdlib only. Not covered by the repo
check; run `go vet ./...` and `go test ./...` here.

## Build

```sh
cd scripts/acpbench
go build -o acpbench .
```

## Usage

```sh
./acpbench --cmd '<argv>' [--env K=V ...] [--cwd DIR] [--mcp-stub]
           [--mcp-transport stdio|http|sse] [--stub-log FILE]
           [--scenario all|<name>] [--iterations K] [--sessions N]
           [--report out.json] [--fake-llm-url URL] [--cooperate]
           [--echo-directive TEXT] [--auth-method ID|auto] [--meta-json FILE]
           [--pre-prompt-wait-mcp] [--init-timeout D] [--turn-timeout D]
           [--kill-group] [--cancel-mix N] [--fault-rate R] [--fault CLASS]
./acpbench compare a.json b.json ...   # markdown table agents x scenarios x RAM
```

- `--cmd` is shell-split (single/double quotes, backslashes) and spawned
  per scenario. Example: `--cmd 'npx -y @agentclientprotocol/claude-agent-acp'`.
- `--env K=V` passes extra environment through (repeatable). Values may
  contain `{{FAKE_LLM_URL}}`, replaced with `--fake-llm-url`. The report
  records env **key names only**, never values.
- `--cwd` sets the agent working directory (default: current directory).
- `--mcp-stub` offers one MCP server, `acpbench-stub`, with tools
  `echo(text)` and `slow(ms)`. `--mcp-transport` selects how it is offered:
  `stdio` (default: `{command, args}` entry) or `http`/`sse` (the probe
  serves `acpbench mcp-stub --http 127.0.0.1:0` once per run and passes its
  `{name, type, url}` entry) for agents that only accept http/sse servers.
  With no stub, `mcpServers` is omitted entirely (never `null`).
- `--stub-log FILE` is where the stub appends one JSON line per
  `tools/list` and `tools/call` (server-side proof the tool really ran).
  Default: a temp file. Prompt and permission verdicts use the stub log, not
  just the agent's `session/update` traffic.
- `--scenario` is `all` or one name (comma-separated also works):
  `initialize new_session prompt_text cancel steer_followup load_resume
  set_model_config images permission crash_recovery concurrency soak
  provider_fault`.
- `--iterations K` sets soak turns (default 5); `--sessions N` sets
  concurrency sessions (default 3).
- `--report` writes the JSON report (default: stdout). Exit 0 when every
  scenario is `ok` or `skipped`; exit 1 otherwise.
- `--fake-llm-url URL` + `--env BASE_URL={{FAKE_LLM_URL}}`: point the agent
  at a local fake LLM. `--cooperate` forces the same cooperative prompts
  without a fake URL. Cooperative scripted prompts carry both plain English
  and a fakellm `TOOL:echo:{...}` directive; the expected tool name is
  derived from the agent's announced `tool_call` names with server prefixes
  stripped (`acpbench-stub__echo` → `echo`). `--echo-directive TEXT`
  appends custom directive text to the plain (non-cooperative) echo prompt.
- `--auth-method <id>|auto`: send one `authenticate {methodId}` after
  `initialize` and retry `session/new` after `AUTH_REQUIRED`. `auto` picks
  the first advertised auth method needing no user interaction, preferring
  env-var/api-key types.
- `--meta-json FILE`: JSON object merged into every `session/new` `_meta`
  (replicates Daemon-specific options, e.g. `_meta.claudeCode.options`).
- `--pre-prompt-wait-mcp`: wait for the MCP-ready notification before each
  session's first prompt.
- `--init-timeout D` / `--turn-timeout D`: Go durations overriding the
  initialize and per-turn prompt timeouts (`ACPBENCH_*` env knobs still work).
- `--kill-group`: kill the whole agent process group on teardown (needed for
  agents like qwen-code that leave orphaned grandchildren).
- `--cancel-mix N`: soak cancels every Nth iteration mid-stream (SLOW
  directive) and records it as a `cancelled` iteration.
- `--fault-rate R`: recorded in the soak report and exported to the agent as
  `ACPBENCH_FAULT_RATE` (passthrough hint for a fakellm `--faults` config;
  probe behavior is unchanged — see docs below).
- `--fault CLASS`: `provider_fault` scenario input (see below).
- No real model APIs are ever called and no credentials are read: only
  `--env` pairs reach the agent. Point the agent at a local fake LLM via
  `--env` (e.g. `--env BASE_URL={{FAKE_LLM_URL}} --fake-llm-url
  http://127.0.0.1:PORT`).

Timeouts are hard per call (30s control, 90s prompt, 10s cancel/crash
detect, 15s steer) with the Daemon's idle-bound rule: any adapter message
restarts the wait, so a healthy long turn is never cut. For fast
self-tests, `ACPBENCH_PROMPT_TIMEOUT`, `ACPBENCH_CTL_TIMEOUT`,
`ACPBENCH_INIT_TIMEOUT`, `ACPBENCH_CANCEL_WAIT`, `ACPBENCH_STEER_WAIT`,
`ACPBENCH_CRASH_WAIT`, `ACPBENCH_MCP_WAIT`, `ACPBENCH_RACE_WAIT`,
`ACPBENCH_SETTLE_WAIT` accept Go durations. `ACPBENCH_FAULT_RATE`
documents the soak fault-rate passthrough; `ACPBENCH_FAULT_STALL_MS`
shortens the fake's `stall-5s` fault (self-tests only).

## Client behavior (mirrors the Daemon)

`initialize` sends `protocolVersion 1` with fs read/write caps and no
terminal. `session/new`/`session/load` carry the MCP list (omitted when
there is no stub, `[]` never `null`) and a `_meta` judgment
(`systemPrompt` + auto-approve marker, plus `--meta-json`; agents ignore
unknown keys). Permission requests auto-approve the allow-once option
(`--scenario permission` also runs a deny leg). Agent-initiated fs/terminal
requests answer `-32601 unsupported`. `session/cancel` is a notification.
Steering uses `_session/steering` with `promptRequired` idle behavior.

## Scenarios and verdicts

Verdicts: `ok timeout-hang protocol-error crash silent-empty-turn
wrong-result skipped`. `skipped` means the probe could not open the
situation it needs (a turn faster than the probe needs a slow one; an
agent that never asks permission; a turn that needs `--slow-ms`; no
discoverable model id; `provider_fault` without `--fault`).
Probes needing an open turn skip honestly instead of asserting on a
closed turn; rerun them against `fake-agent --slow-ms 15000 --slow-match
"slow tool"`.

- `initialize`: records `protocolVersion`, `agentCapabilities`,
  `authMethods`, `promptCapabilities`, `mcpCapabilities` (when present),
  steering advertisement, time-to-initialize. With `--auth-method` also
  records the selected method and the `authenticate` outcome.
- `new_session`: reports `notificationSeen` (the `_x.ai/mcp_initialized`
  wait) and `toolListSeen` (stub-log `tools/list`) as separate columns; a
  missing notification is reported, never a skip.
- `prompt_text`: scripted echo-tool turn (cooperative prompt when
  `--fake-llm-url`/`--cooperate`); requires `stopReason end_turn`, the echo
  tool derived from namespaced announcements, **and** stub-side proof (the
  stub log shows echo ran). Records time-to-first-chunk.
- `cancel`: cancels mid-turn (phase recorded: `slow-tool` vs `streaming`);
  requires `stopReason cancelled` within 10s and a reusable session after.
- `steer_followup`: steering advertised → `_session/steering`, records
  `injected`/other; else a second concurrent prompt, records
  `queued`/`rejected`/`concurrent`/`hang`.
- `load_resume`: three legs, each reported: `resume` (`used`/`unsupported`/
  `failed` for `session/resume`), `loadSameProcess` (`session/load` in the
  live process + verify turn), `loadCrossProcess` (kill, respawn,
  `session/load`, history verified — the fake proves history via
  `history:<n>`).
- `set_model_config`: discovers **real** ids from `session/new`
  (`models.availableModels`, `configOptions` with category `model` or id
  `model`/`thought_level`/`effort`) and `session/load` `configOptions`,
  then sets a real different id via `session/set_config_option
  {sessionId, configId, value}`, falling back to `session/set_model
  {sessionId, modelId}` (spec key `modelId`, never `model`). Reports
  `method-absent` / `id-rejected` / `applied-and-verified` (or
  `applied-unverified`) per method. Never probes a bogus id: with no
  discoverable id the scenario skips.
- `images`: one 1x1 PNG + text; records `accepted`/`dropped`/`error`.
- `permission`: approve leg (turn must complete) and deny leg (the **stub
  log** execution count must not grow — optimistic `tool_call` updates are
  not proof); skips when the agent never asks (needs `--ask-permission`).
- `crash_recovery`: SIGKILLs mid-turn; the pending prompt must fail fast
  (no infinite hang).
- `concurrency`: N sessions across N processes (parallel) plus N sessions
  in one process (sequential); reports both counts and per-session
  marginal RAM (needs the idle baseline, so run after `new_session`).
- `soak`: K `prompt_text` turns back to back. `successRate` counts
  `end_turn` as success. Records `p50`/`p95`, per-iteration
  `{latencyMs, stopReason, class}` records under `iters`, `--cancel-mix`
  cancelled iterations, `--fault-rate`, and RAM phases (`idle-after-
  new-session`, `prompt-turn`, `post-turn`) even on soak-only runs.
- `provider_fault`: needs `--fault CLASS`. Sends a prompt containing
  `FAULT:<class>` and classifies how the agent surfaces the provider
  failure: clean error within the turn timeout (good, `ok`), silent empty
  `end_turn` (bad, `silent-empty-turn`), hang past the timeout (bad),
  crash (bad), retries then success (good, `ok` with `retryCount` from
  transcript stats). Classes: `http429 http500 http529 midstream-error
  stall-5s stall-forever truncate reset empty`.

## fakellm2 fault contract

`provider_fault` is defined against the `FAULT:<class>` directive text so
it works without depending on fakellm2 being finished: any backend honoring
the directive (fakellm2, or the in-repo fake agent used by `go test`)
drives the classification above. Hand-run hint for fakellm2: point the
agent at fakellm2 (`--env BASE_URL={{FAKE_LLM_URL}} --fake-llm-url
<fakellm2-url>`) and pass `--fault <class>`; fakellm2 should fail the
`FAULT:<class>` prompt with the matching HTTP failure / stall /
truncation. fakellm2 additionally honors the `X-Acpbench-Fault: <class>`
header and `ACPBENCH_FAULT=<class>` env when the agent strips the prompt
directive. Soak `--fault-rate R` is exported as `ACPBENCH_FAULT_RATE` for
fakellm `--faults` configs and recorded in the report.

## RAM

VmRSS summed over each agent process tree (`/proc/<pid>/task/*/children`
walk, like `emberloop-web/el/memory.go`) every 250ms, plus a synchronous
tick at every phase boundary. Phases: `startup-init`,
`idle-after-initialize`, `idle-after-new-session`, `prompt-turn`,
`post-turn`, `concurrency-multi`, `soak-*` (soak records its own idle and
turn windows). `perSessionMarginalBytes` is
`(multi-process peak − idle baseline) / N`. Values are approximate (short
phases, runtime RSS wobble); treat them as order-of-magnitude.

## Machine-readable output

Reports carry `schemaVersion` (currently 2), per-scenario `latencyMs`, and
a `summary` object counting scenarios per verdict class. Compare reports:

```sh
./acpbench compare a.json b.json ...   # markdown table agents x scenarios x RAM
```

`compare` prints one markdown table (scenarios × agents with verdicts and
latencies, plus prompt-turn peak, concurrency peak, and per-session
marginal RAM rows) and refuses files with an unknown `schemaVersion`.

## Self-test double

`acpbench fake-agent` is the in-repo fake ACP agent used by `go test`:

```
--hang-prompt --crash-prompt --empty --wrong
--slow-ms N [--slow-match TEXT] --slow-cancel-ms N
--ask-permission --image-behavior accept|drop|error
--allow-concurrent --state-dir DIR --tool-prefix PFX
--require-auth --no-models --no-config-options
```

`--tool-prefix` namespaces announced tool names (tests prefix stripping);
`--require-auth` fails `session/new` with `AUTH_REQUIRED` until
`authenticate`; `--no-models`/`--no-config-options` hide discovery (the
probe must skip, never use a bogus id). `FAULT:<class>` prompts simulate
the provider-fault contract. When offered a stub the fake really invokes
it (initialize + tools/list at session start, tools/call per echo turn),
so stub-log verification is exercised end to end. `--state-dir` persists
sessions across respawns (load_resume needs it in `--cmd`). Fault flags
map 1:1 to verdicts (`--hang-prompt` → `timeout-hang`, `--crash-prompt` →
`crash`, `--empty` → `silent-empty-turn`, `--wrong` → `wrong-result`).
`go test ./...` runs the classifier unit tests plus end-to-end runs of
every scenario class.

## Against real agents

Only run against binaries already installed locally **and** pointed at the
fake LLM endpoint via `--env`; otherwise skip. Never run a live
claude/pig session from inside this work. Example (once a local fake LLM
exists):

```sh
./acpbench --cmd 'claude-agent-acp' --mcp-stub \
  --env ANTHROPIC_BASE_URL={{FAKE_LLM_URL}} --fake-llm-url http://127.0.0.1:PORT \
  --report report.json
```

Agents needing login: `--auth-method auto` (or an explicit advertised id).
Daemon option replication: `--meta-json opts.json` with e.g.
`{"claudeCode": {"options": {...}}}`. Group-killing teardown:
`--kill-group`. http/sse-only MCP clients: `--mcp-transport http`.

## Known gaps

- Namespaced tool derivation strips common separators; exotic agent
  namespacing may need a mapping update in `deriveBaseToolName`.
- `set_model_config` verification is transcript-based and only exact
  against the fake; real agents report method acceptance only
  (`applied-unverified`).
- The second-prompt steer probe assumes one turn per session; agents with
  queueing answer `queued` only when the second prompt outlives the first.
- `session/resume` params are a guess (`sessionId` + `cwd` + MCP + `_meta`);
  unknown shapes report `failed` and the `session/load` legs still run.
- RAM numbers are approximate; see above.
