---
status: accepted
---

# Enderbloom Web drives claude through ACP and pig through its Go SDK

The Daemon is Go. Claude has no Go SDK, so the Daemon talks ACP to `claude-agent-acp`, the adapter around the Claude Agent SDK. Pig is itself Go, so the Daemon embeds its SDK in-process: steering (`QueueSteer`, `QueueFollowUp`) is native and there is no extra process or Node runtime per pig Session. Both sit behind one `Agent` interface that emits normalized Events.

## Considered Options

- ACP for both: needs a pig adapter; the community `pi-acp` targets the Node Pi, not PiG, and adds a translation layer for no gain.
- Pig over `--mode rpc` as a child process: isolates crashes and reuses CLI flags, but costs a process per Session. Revisit if SDK instability bites.

## Consequences

- The PiG SDK is unstable before its first public release: pin an exact version.
- A pig crash or leak shares the Daemon's process.
- ACP applies to claude only; the UI never sees ACP types.
