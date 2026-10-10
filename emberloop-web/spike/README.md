# Spike probes

Live-only probes for ticket 01. Each runs against the real installed
harness and prints every Event it sees. Never part of an automated
check; never import this code from product code.

## Pig (Go SDK)

```sh
cd pig && go run . 2>&1 | tee pig-run.log
```

Needs `pig auth` credentials and a reachable model
(`deepseek/deepseek-flash` default, override with `PI_SPIKE_MODEL`).

## Claude (ACP adapter)

```sh
cd claude && npm install && node probe.mjs 2>&1 | tee claude-run.log
```

Needs `claude` login credentials. The probe starts the pinned
`@agentclientprotocol/claude-agent-acp` adapter as a subprocess and
drives it over ACP on stdio.

## Claude Backend live probe (ticket 04)

```sh
cd .. && EL_LIVE=claude go test -tags live -run TestLiveClaudeBackend . 2>&1 | tee spike/claude-backend-run.log
```

Drives `ClaudeBackend` against the real pinned adapter with the echo
server as its MCP server: streams normalized Events, asserts no native
tools, asserts allow-once auto-approve, then loads the session again
(repeating the stripping set) and prompts it. Needs `claude` login
credentials, node and network. Never runs in the main check.

## Claude stream-timing probes (assistant-chunk bursts)

```sh
cd .. && EL_LIVE=claude go test -tags live -run 'TestLiveStreamTiming' . 2>&1 | tee spike/stream-timing-run.log
```

Drives `ClaudeBackend` against the real pinned adapter with the adapter's
stdio routed through `claude/acp-tee.mjs` (a byte-preserving proxy that
logs one timestamped line per ACP message: monoNs, tee pid, direction,
kind, detail, bytes). Each phase timestamps every `assistant_chunk` at
OnUpdate dispatch; the in-test report compares adapter emission pacing
against dispatch pacing, per tee pid so concurrent adapter processes never
mix clocks. `TestLiveStreamTiming` covers fresh/tool/resumed short replies,
`TestLiveStreamTimingBig` burst-scale replies with a ~40K-token pasted
context, `TestLiveStreamTimingModel` the model-vs-effort bisection
(sonnet/medium, sonnet plain, haiku/medium). To split adapter from CLI,
point `CLAUDE_CODE_EXECUTABLE` at a shell wrapper that execs
`claude/acp-tee.mjs --log cli.log -- <sdk-bundled-claude> "$@"`: the tee
sniffs CLI stream-json `type`/`event` per line. Needs `claude` login
credentials, node and network. Never runs in the main check.
