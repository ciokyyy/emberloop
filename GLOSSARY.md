# Enderbloom Web

A browser UI and local daemon that run coding-agent sessions and show them live. It is a separate harness from the herdr-based emberloop loop.

## Language

**Daemon**:
The local Go process that owns every Session, talks to Backends, and serves the UI over HTTP and server-sent events.
_Avoid_: Server, backend service

**Session**:
One conversation between a user and one agent, with a harness, a model and a working directory. Persisted as an event log.
_Avoid_: Chat, thread, run

**Backend**:
The adapter that drives one harness behind the Daemon's `Agent` interface: claude through ACP, pig through its Go SDK.
_Avoid_: Driver, provider, adapter

**Subagent**:
A Session spawned by another Session through the `spawn` MCP tool. It records its parent Session.
_Avoid_: Child agent, worker, sub-session

**Group**:
The Subagents created by one `spawn` call. The parent receives one grouped result when the last member finishes.
_Avoid_: Batch, swarm

**Steer**:
A prompt delivered to a running Session at its next tool boundary.
_Avoid_: Interrupt, nudge

**Follow-up**:
A prompt delivered to a Session after its current turn ends.
_Avoid_: Queue message, next prompt

**Event**:
One normalized record of something a Session did or received: message chunk, tool call, prompt, status change.
_Avoid_: Update, message

**Event log**:
The append-only file of a Session's Events. The UI renders a Session by replaying it.
_Avoid_: Transcript file, history
