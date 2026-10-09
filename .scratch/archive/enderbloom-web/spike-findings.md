# Spike findings (ticket 01)

Probes: `el-web/spike/pig` (Go, PiG SDK v0.4.0, model
`deepseek/deepseek-flash`) and `el-web/spike/claude` (Node,
`@agentclientprotocol/claude-agent-acp` 0.88.0, ACP protocol version 1).
Raw logs are not committed; excerpts below are quoted verbatim.

## 1. The adapter accepts every stripping option; no fallback needed

`session/new` `_meta.claudeCode.options` is forwarded to the Claude
Agent SDK `Options`. `settingSources: []`, `strictMcpConfig: true`,
`allowedTools` and `disallowedTools` are all accepted (all present in
the installed SDK `sdk.d.ts`, and the adapter logged proof the
allow-list reached the SDK):

```
[CLAUDE_SDK_CAN_USE_TOOL_SHADOWED] Warning: canUseTool will not be invoked for: mcp__spike__spike_echo. Bare allowedTools entries auto-approve the whole tool before the callback is consulted.
```

With `disallowedTools: ["Task","Bash","Read","Write","Edit","Glob",
"Grep","WebFetch","WebSearch"]` and `allowedTools:
["mcp__spike__spike_echo"]`, the only tool calls across three runs were
the MCP tool and the adapter-internal `ToolSearch`:

```
TOOL-CALLS [{"id":"toolu_01...","title":"ToolSearch",...},{"id":"toolu_01...","title":"mcp__spike__spike_echo",...}]
```

`ToolSearch` is the adapter's own tool-discovery call (it searches for a
tool by name; it found no match for the MCP tool until the server was
correctly attached), not an offered native tool: the model can never
call `Bash`, `Read`, `Task` or any other native tool, because the SDK
`disallowedTools`/`allowedTools` pair enforced above removes them before
the model sees the tool list. No native tool was ever offered or called.

**Recommendation: the ACP adapter.** No sidecar, no `stream-json`
fallback.

## 2. MCP servers connect before the first prompt

`session/new` carries `mcpServers`; the adapter connects them during
session create (`sdk-initialize` ~1s in the adapter log) and the first
turn already called the MCP tool (`status=completed`). One trap: the
ACP schema requires an absolute `command` and an `env` array; with a
relative command the server was silently absent and the model reported
"no tool with that name is available here".

## 3. A mid-turn prompt is queued until the turn ends

The adapter advertises `_meta.claudeCode.promptQueueing: true`. A
second `session/prompt` sent while the first turn ran a tool
(`sawTool=true firstDone=false`) returned `end_turn` only after the
first turn's `end_turn`, and ran as its own turn:

```
PROBE-ACTION mid-turn session/prompt sent (sawTool=true firstDone=false)
FIRST-RESP {"ok":true,"stopReason":"end_turn"}
MIDTURN-RESP {"ok":true,"stopReason":"end_turn"}
```

Its reply ("STEERED, noted. I'll call the tool now." + a fresh
`mcp__spike__spike_echo` call) shows queue-until-turn-end, not
tool-boundary injection. **The Daemon must degrade a claude Steer to a
Follow-up and label it so**, as the spec already says. The
user-message priority setting does not apply (managed by ACP).

## 4. No subagent activity to link while Task is off

With `Task` disallowed, no subagent (Task) activity occurred in any
run. The adapter advertises a `subagents` session capability, but under
stripping there is nothing to parent-link, so the Daemon ignores the
channel entirely; Subagents come only from `spawn`.

## 5. `session/load` resumes a session after adapter restart

Killing the adapter and calling `session/load` with the same cwd and
`sessionId` returned the same id, and a prompt answered `end_turn`
with the MCP tool callable:

```
LOAD-RESP {"sessionId":"3ca9325d-...","modes":...}
FIRST-RESP {"ok":true,"stopReason":"end_turn"}
```

Caveat found by accident: the probe's load path omitted `_meta`
options, and the loaded session then asked permission for the MCP tool
(see 7). **Ticket 04 must repeat the full `_meta.claudeCode.options`
on every `session/load`.**

## 6. Pig SDK: chunks and tool calls stream, no plans, steer delivery proven, follow-up and tool execution unproven

Observed subscription events: `AgentStart`, `TurnStart`,
`MessageStart/Update/End` with `ai.ThinkingStart/Delta`,
`ai.TextStart/Delta/End`, `ai.ToolCallStart/Delta`, `TurnEnd`,
`AgentEnd(willRetry=false)`, `AgentSettled`, `QueueUpdate`. So message
chunks and tool calls are covered; `ToolExecutionStart/Update/End`
exist in the SDK but never fired live (no tool call completed, see
below). **There is no plan event type in SDK v0.4.0** (no `Plan` in
`agent/agent.go`); the Daemon cannot stream pig plans as Events.

`QueueSteer`/`QueueFollowUp` admit and report:

```
EV QueueUpdate steering=["Steered note: ..."] followUp=[]
EV QueueUpdate steering=["Steered note: ..."] followUp=["Follow-up: ..."]
```

Steer delivery is proven: a steer sent 1s into a no-tool turn was
consumed as a continuation turn (`TurnStart index=1`,
`QueueUpdate steering=[]`), and the session transcript parents the
steer user message after the finished assistant reply. The continuation
turn itself then stalled (see below), so the reply carries no STEERED.

Stripping works through SDK options: `SkipBuiltinTools: true` plus one
`ExtraTools` marker gave `TOOLS ["mcp__el__spike_echo"]` and the
wire logged `sending 1 tools`.

Limitation: the only authed provider (`deepseek/deepseek-flash`)
never completes a tool-call turn through the SDK (five attempts,
single and double calls, high and low thinking, up to 9 minutes each:
the stream yields thinking/tool-call deltas, then silence, and no
assistant message is ever persisted), and also stalls longer text
turns. So tool execution (`ToolExecution*`) and follow-up-after-turn
delivery are **not yet proven live**; ticket 03 must re-prove them
with a healthier provider or model.

## 7. Permission requests need auto-approval, and the shape is known

A stripped session still emits `session/request_permission` (seen when
the load path dropped the allow-list). Shape:

```json
{"toolCall":{"toolCallId":"toolu_01...","name":"mcp__spike__spike_echo","status":"pending","rawInput":{"text":"alpha"},"title":"mcp__spike__spike_echo","kind":"other","content":[]},
 "options":[{"optionId":"allow-once","name":"Yes","kind":"allow_once"},
            {"optionId":"allow-with-updates","name":"Yes, and don't ask again for Spike Echo commands","kind":"allow_always"},
            {"optionId":"reject","name":"No","kind":"reject_once"}],
 "sessionId":"3ca9325d-..."}
```

Answering `{"outcome":"selected","optionId":"allow-once"}` let the tool
run. With bare `allowedTools` entries the SDK auto-approves without
asking (see the warning in 1). **Auto-approval is needed**: the Daemon
must answer every `session/request_permission` with the allow option
(prefer `allow-once`) and log each as a permission auto-approved Event.
No prompt is ever cancelled.
