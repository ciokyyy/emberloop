# Enderbloom Web

## Problem Statement

Emberloop runs agents in herdr panes and shows them in a terminal dashboard. That works inside one terminal, but it cannot be opened from anywhere, it looks like a terminal, and it only reaches agents through typed keystrokes and `agent_status` guesses. The user wants a good-looking interface that opens in any browser, shows what each agent is doing as it happens, lets them talk to and steer agents, and shows subagents under the agent that spawned them. It must work with claude and pig, keep the same stripped-tools principle as emberloop (only MCP tools), and not depend on herdr.

## Solution

A new module, `emberloop-web`: a local Daemon (Go) plus an embedded Svelte single-page app. The user opens a browser tab, creates a Session by choosing a harness, a model and a working directory, and chats with the agent. Agent output streams live. While an agent works, the user can Steer it (delivered at its next tool boundary) or queue a Follow-up (delivered after the turn). Agents spawn Subagents through a `spawn` MCP tool; Subagents appear nested under their parent and report back as one grouped result. Every agent runs stripped: no native tools, only MCP tools. Sessions persist as Event logs on disk and survive Daemon restarts as history, with best-effort resume.

The UI is a static client of the Daemon's JSON and server-sent-event API, with no server rendering, so it can be wrapped in Tauri later without change. This version is web only.

## User Stories

1. As a user, I want to open the Daemon's address in a browser, so that I can use agents from any machine or window without a terminal multiplexer.
2. As a user, I want to create a Session by picking a harness (claude or pig), a model and a working directory, so that I control what runs and where.
3. As a user, I want the model list to match the harness, so that I never pick a model the harness cannot run.
4. As a user, I want a default working directory pre-filled, so that starting a Session takes few steps.
5. As a user, I want to send a prompt and see the answer stream in as it is produced, so that I do not wait for a finished reply.
6. As a user, I want tool calls shown as cards with name, arguments and result, so that I can see what the agent did.
7. As a user, I want long tool output collapsed by default and expandable, so that the transcript stays readable.
8. As a user, I want file edits shown as diffs, so that I can review changes at a glance.
9. As a user, I want a visible status per Session (idle, working, failed, ended), so that I know where attention is needed.
10. As a user, I want a list of all Sessions, newest activity first, so that I can switch between them.
11. As a user, I want Subagents nested under the Session that spawned them, so that I understand who did what.
12. As a user, I want to open a Subagent's own transcript, so that I can inspect its work.
13. As a user, I want a Group's results delivered to the parent as one message when the last member finishes, so that the parent does not get a stream of partial notices.
14. As a user, I want to Steer a working Session, so that I can correct its course without waiting.
15. As a user, I want to queue a Follow-up while a Session works, so that my next request runs when the turn ends.
16. As a user, I want queued Steers and Follow-ups listed and clearable, so that I can change my mind before they are delivered.
17. As a user, I want the composer to show which mode a message will use, so that I never send the wrong kind by accident.
18. As a user, I want no interrupt button, so that a running tool call is never killed half-way by a mistaken click.
19. As a user, I want to stop a Session explicitly, so that I can end it deliberately.
20. As a user, I want agents to run with native tools disabled and only MCP tools available, so that behaviour matches emberloop's stripped harnesses.
21. As a user, I want the `emberloop` MCP tools available in every Session, so that agents can spawn Subagents and use emberloop's tools.
22. As a user, I want permission requests from agents answered automatically and logged, so that sessions never stall on a prompt nobody sees.
23. As a user, I want to reload the page and see the Session exactly as it was, so that a refresh never loses my place.
24. As a user, I want the UI to reconnect after a dropped connection and catch up on missed Events, so that nothing is lost.
25. As a user, I want Sessions to remain readable after the Daemon restarts, so that history is never lost.
26. As a user, I want a Session to resume after a restart where the harness supports it, and to show read-only history where not, so that I know what is possible.
27. As a user, I want a cap on concurrent running Sessions, so that my machine's memory is not exhausted.
28. As a user, I want idle Sessions to release their agent process and resume from the log, so that memory stays low.
29. As a user, I want the UI to follow my system's light or dark theme, so that it fits my desktop.
30. As a user, I want a clean, calm layout with a single transcript column and a floating composer, so that the interface feels like a modern chat app.
31. As a user, I want the Daemon reachable only from my own machine, so that nobody else can drive my agents.
32. As a user, I want requests from other websites rejected, so that a page in my browser cannot type into my agents.
33. As a developer, I want a fake Backend, so that the Daemon and UI can be tested with no real agent.
34. As a developer, I want each Backend behind one `Agent` interface emitting normalized Events, so that adding a harness means writing one adapter.
35. As a developer, I want the UI to depend only on the API, so that Tauri can wrap the same build later.

## Implementation Decisions

- **Module.** A new Go module `emberloop-web` in the emberloop repo, beside `emberloop-tui`. One binary embeds the built Svelte app. It may import emberloop's core packages; the core never imports it. It does not use herdr and does not read emberloop's ticket state.
- **Daemon.** Owns all Sessions, spawns Backends, writes Event logs, serves the API. It binds the loopback interface only and rejects requests whose `Host` or `Origin` is not loopback. No authentication in this version.
- **Backends behind one `Agent` interface.** The interface creates a Session, sends a prompt, queues a Steer, queues a Follow-up, ends the Session, and emits normalized Events. Two real Backends and one fake.
  - **claude** over ACP, through the `claude-agent-acp` adapter (a Node subprocess wrapping the Claude Agent SDK). If the adapter cannot strip tools, fall back to a small Node sidecar that uses the SDK directly, or to the CLI's `stream-json` input and output.
  - **pig** through the embedded Go SDK (`github.com/MichaelKinsy/PiG`), pinned to an exact version. Streaming comes from the session subscription; Steer and Follow-up map to the SDK's native queue calls. See the ADR.
  - **fake** replays scripted Events for tests.
- **Stripping.** Every Backend runs with native tools off and only `emberloop` MCP tools allowed, no project settings, no memory files, strict MCP config. Claude: the same stripping wick uses (no built-in tools, empty setting sources, strict MCP config, allow-list of the MCP server, slash commands off, the same disabling environment variables). Pig: no built-in tools, MCP config, system prompt, as emberloop's pig harness does. Claude's MCP connects after launch, so the first prompt must wait until tools are ready.
- **Event schema.** One normalized, versioned schema is the contract between Backends, the log, and the UI. ACP and pig types never cross it. Kinds: user prompt (steer, follow-up or first), assistant message chunk, thinking chunk, tool call started, tool call updated, diff, permission auto-approved, status change, Subagent spawned, Subagent finished, error. Each Event has a per-Session sequence number.
- **Event log.** One append-only JSON-lines file per Session plus a small metadata file (harness, model, working directory, parent, group, created, status). State lives under the pig state directory in a new `enderbloom` subtree. No SQLite for state. The existing usage database stays a read-only stats source for later.
- **API.** JSON over HTTP for commands (create, list, get, prompt, steer, follow-up, clear queue, stop) and server-sent events for live Events, with replay from a last-seen sequence number so a reconnecting client catches up. The first connect replays the whole log. This is the Remult-style "UI is a function of the event stream" model.
- **Steering.** Two modes only, matching emberloop: Steer at the next tool boundary, Follow-up after the turn. No interrupt. Pig does both natively. For claude the Daemon keeps its own queue (as emberloop's input queue does) and delivers a Steer at a tool boundary if the adapter supports injecting mid-turn, otherwise degrades it to a Follow-up and says so in the UI. The spike decides.
- **Permissions.** Agent permission requests are auto-approved by selecting the allow option, and each is logged as an Event. No permission UI in this version.
- **Subagents.** The Daemon serves a `spawn` MCP tool (through the existing emberloop MCP server calling the Daemon, or a Daemon-hosted server; decided in ticket 06). A call creates one Session per requested agent, each with a parent and a shared Group. The parent receives a single grouped result when the last member finishes, delivered as a Steer. Read-only scouts are Subagents with a restricted tool set.
- **Lifecycle.** One agent process or SDK session per running Session. A concurrency cap read from the existing global `max_workers` setting. Idle Sessions release their agent and resume from the log: claude through ACP `session/load` where supported, pig through its saved session file; otherwise the Session shows read-only history.
- **Working directory.** Chosen at creation; no git worktrees in this version.
- **Models.** Pig models come from the pig binary's model listing. Claude models are a fixed alias list (opus, sonnet, haiku). Effort and system prompt are not user-editable yet.
- **UI.** Svelte 5 with Vite, built to static files, embedded in the binary, usable with no server rendering. shadcn-svelte on Tailwind, installing components as needed (sidebar, scroll area, textarea, button, badge, collapsible, select, dialog, command, tooltip, resizable). Layout: session tree on the left, a single transcript column of cards in the centre with a floating composer, a collapsible details pane on the right. Visual direction is a mix of ChatGPT's chat layout and Apple's design language (translucent surfaces, large radii, system type), following the system theme.
- **Tauri.** Out of this version, noted for the next. Constraint kept now: the UI is a static build talking to a configurable API base URL; no server rendering, no reliance on Daemon-rendered HTML.

## Testing Decisions

- A good test drives external behaviour through the highest seam and asserts what a user or client would observe, never internal state.
- **One seam: the Daemon's HTTP and SSE API, with the fake Backend.** Tests start the Daemon on an ephemeral loopback port with a temporary state directory and the fake Backend, then act as a client: create a Session, send prompts, read the SSE stream, Steer, Follow-up, spawn Subagents, restart the Daemon and read history. This covers the Event schema, log, replay, queueing, grouping and persistence.
- **Loopback guard tests** at the same seam: a request with a foreign `Origin` or `Host` is rejected.
- **Real Backends are not in the main check.** Each gets a live probe, run by hand with real agents (like the existing live probes), asserting only that a prompt streams Events in the normalized schema and that no native tools are offered. The pig Backend's unit coverage uses the SDK's own test helpers if they exist.
- **UI:** a smoke build check (the static build succeeds and the Daemon serves it) plus a small number of component tests for the composer mode and the Event-to-card mapping. Visual quality is reviewed by hand.
- Prior art: the existing driver and input-queue tests use fake runners at a single entry seam; the dashboard uses fake backends.

## Out of Scope

- Tauri packaging (next version).
- Authentication, remote access, TLS.
- A permission-approval UI.
- Interrupting a running turn.
- Git worktrees and emberloop ticket integration, including reading emberloop's state files.
- herdr integration.
- ACP for pig.
- Usage and cost dashboards from the stats database.
- Plan checklists (ACP plan updates). Pig's SDK has no plan event and the UI shows none for either harness.
- Effort, thinking level, and system prompt controls.
- Mobile layout polish.

## Spike Findings

Written by ticket 01 to `.scratch/enderbloom-web/spike-findings.md`. Open questions it must answer:

1. Does `claude-agent-acp` accept options to disable native tools, empty the setting sources, force strict MCP config and set an allowed-tool list? If not, which fallback works?
2. Does the adapter connect session-level MCP servers before the first prompt, or must the first prompt wait?
3. What happens to a prompt sent to the adapter mid-turn: queued until the turn ends, or injected at a tool boundary (does the user-message priority setting help)?
4. Does the adapter report subagent (Task tool) activity with a parent link, and can the Daemon ignore it because Subagents come from `spawn`?
5. Does `session/load` resume a claude Session after the adapter restarts?
6. For pig: which events does the SDK subscription emit, and do they cover message chunks and tool calls? Do `QueueSteer` and `QueueFollowUp` behave as documented? Can a pig Session be stripped to MCP-only through SDK options?
7. What does an ACP permission request look like when native tools are off, and is auto-approval ever needed?

## Further Notes

- Reference for the stripping flags: the wick claude harness. Reference for steering semantics and grouped delivery: emberloop's input queue and spawn groups.
- The glossary terms in the root `GLOSSARY.md` apply to all tickets and code.
