# Spec: agy harness, and MCP tool calls that stay in the foreground

Status: ready-for-agent

## Problem Statement

I have an Antigravity subscription and the `agy` CLI installed, but wick can only drive pig and claude, so that quota sits unused while my Claude quota runs down. When I used agy in herdr before, most of its tool calls went to the background, and herdr's screen detection decided the agent had finished while it was still working. Claude does the same to MCP calls: after 120 seconds, or as soon as text is typed into its pane, it moves a running MCP call to the background and tells the model to carry on. That breaks wick, because `spawn` and other long calls are MCP calls, and wick's notice delivery types into the pane.

## Solution

agy becomes a third harness. wick generates a stripped agy custom agent per role under `~/.wick-pig/agy/`: no default prompt sections, no built-in tools, only wick's MCP server, with wick's system prompt. wick's MCP tools are marked never to background. agy hooks report the real working/idle state to herdr, the way the herdr-report extension does for pig, and a hook acts as the stop gate that refuses to stop before `submit`. Steering stays in agy's default queue mode. For claude, the harness turns background tasks off, so MCP calls always stay in the foreground.

## User Stories

1. As a user, I want to pick `agy` as the harness for any role, so that work runs on my Antigravity subscription.
2. As a user, I want an agy agent to see only wick's tools, so that it behaves like a pig or claude agent and cannot bypass wick's role policy.
3. As a user, I want an agy agent to get wick's system prompt and not agy's defaults, my GEMINI.md, or my global rules, so that every harness gets the same instructions.
4. As a user, I want agy's agent definitions kept under `~/.wick-pig/agy/`, so that my personal agy config is untouched.
5. As a user, I want wick's MCP tools never backgrounded in agy, so that a long `spawn` blocks like it does in pig.
6. As a user, I want herdr to show an agy pane as working while it works, so that herdr and wick don't treat it as finished while it runs.
7. As an orchestrator, I want an agy worker unable to stop before it submits, so that its result always arrives.
8. As an orchestrator running on agy, I want worker notices queued until my turn ends, so that a notice never interrupts a tool call halfway.
9. As a user, I want agy's models listed from `agy models` with the effort suffix stripped (`gemini-3.8-flash-high/-medium/-low` → `gemini-3.8-flash` with efforts low, medium, high), so that I choose model and effort separately like on every other harness.
10. As a user, I want agy sessions to have a transcript wick can read, so that last reply, turn state and token usage work like for other harnesses.
11. As a user, I want agy's reported token usage captured, so that the run records and stats cover agy panes.
12. As a user of the claude harness, I want MCP calls never moved to the background after 120 seconds, so that a long `spawn` returns its result in the same turn.
13. As a user of the claude harness, I want a notice typed into the pane not to push the running MCP call to the background, so that delivery never detaches a call.
14. As a user, I want a probe to verify agy's real behaviour before building on it, so that the harness doesn't rest on guesses about agy's interface.
15. As a user, I want every agent to receive its brief or prompt directly as its first message, instead of "Read <file> and follow it exactly", so that no agent spends a tool call and a round trip reading its own instructions.

## Implementation Decisions

- First ticket is a live probe of agy (installed CLI, currently 503 on eligibility at grill time), answering: does `--agent` accept a path outside the workspace; does `excludeDefaultComponents: true` drop GEMINI.md/AGENTS.md/rules or only defaults; where the per-tool `background_mode` setting lives for an MCP server and whether `off` holds; whether a `PostInvocation` (or other) hook can block a stop or inject a message; where agy writes its conversation transcript and in what format; what `stream-json` usage contains. The findings are recorded in the ticket and the remaining tickets adapt to them.
- Fallbacks fixed in advance: if `--agent` cannot take an outside path, the agent is written to the worktree's `.agents/agents/wick-<role>/` and excluded via the repo's local git exclude file, never into the user's global agy config. If no hook can block a stop, the gate falls back to the nudge-and-give-up path the stop gate already has for weaker harnesses.
- The agy harness implements the existing harness interface (command, session root, last reply, turn state, trust, late prompt) and registers beside pig and claude; it declares its stop-gate capability in the existing capability table.
- The generated agent definition is content-addressed by role and tool set so concurrent sessions don't race on writes.
- Hooks call `herdr pane report-agent` / `release-agent` with working/idle/done, overriding herdr's screen detection for that pane.
- Queue mode is agy's default and is not changed; wick never edits the user's global agy settings.
- Claude harness environment adds `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1` and `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0` (found in Claude Code 2.1.289: auto-background defaults to 120 s behind a remote flag; both variables return a zero threshold). It has `--tools ""`, so no Bash backgrounding is lost.
- Token usage parsing for agy is delivered as part of this spec in the shape the run-records spec consumes (per request: model, input, output, cache read, cache write, reasoning, reported cost if any).
- agy `Models()` (the interface from the global settings spec): parse `agy models`, group ids that differ only by an effort suffix (`-low`, `-medium`, `-high`, `-max`) into one base model whose efforts are the suffixes present; models without a suffix get agy's `--effort` levels (low, medium, high, max). At launch, a grouped model is sent as `<base>-<effort>`; an ungrouped one as the model plus `--effort <level>`. The probe checks whether `--effort` also affects suffixed Gemini ids; if it does, Gemini models also accept agy's full `--effort` levels.
- Agents are launched with their full prompt as the first message. Today the prompt is written to a file and the agent is told to read it, because `herdr pane run` types one shell line (newlines and quoting break, and the pane's shell may be fish). Instead, wick writes the complete launch (binary, args including the full prompt, env) to a launch file in its state dir and types one shell-agnostic line, `wick-pig exec <launch-file>`, which reads the file and execs the harness with the prompt as a real argument. The brief and prompt files are still written, for the archive and debugging, but the agent is not told to read them. Applies to workers, read-only agents (scouts, reviewers) and every harness.

## Testing Decisions

- Good tests assert the built command, the generated files and observed behaviour through the harness interface, not the steps inside the harness.
- `BuildCommand("agy", …)` joins the existing every-harness tests (one shell line, role env first, prompt last) and gets a stripped-agy test mirroring the stripped-claude test: agent flag, MCP config, no default components, hooks present.
- agy joins the stop-gate capability table test.
- Last reply and turn state tested against transcript lines captured by the probe, saved as fixtures, like the existing pig and claude transcript tests.
- agy `Models()` grouping tested against captured `agy models` output; launch tests assert `<base>-<effort>` for grouped models and `--effort` for ungrouped ones.
- Claude: the stripped-claude test asserts both new environment variables.
- Launch: `BuildCommand` tests assert the typed line is the single `wick-pig exec` line and the launch file holds the full multi-line prompt verbatim; an `exec` test with a fake binary asserts argv and env arrive intact, including newlines and quotes.
- Live tests, opt-in like the existing live test: an agy session completes a tool call and submits; a claude session with a slow MCP call receives a delivered notice mid-call and the call result still arrives in the foreground.

## Out of Scope

- Making pig itself use the Antigravity subscription (omp-style Cloud Code Assist client impersonation). Later spec, if ever; terms-of-service risk.
- Switching agy to send-immediately steering.
- Subscription quota display.
- Web search tool: needs its own grill session.

## Further Notes

Spec 2 of 5. Can run in parallel with the global settings spec; the settings spec's harness validation will pick up `agy` automatically through the registry.
