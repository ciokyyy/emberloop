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
