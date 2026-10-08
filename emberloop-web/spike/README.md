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
