# 03: Probe agy's real behaviour

**What to build:** A written record, in this ticket, of how the installed `agy` behaves, with captured output saved as test fixtures. The grill found these in the binary and its changelog but could not run them (the service returned 503 on eligibility). Answer each:

1. Does `--agent` accept a custom agent definition at a path outside the workspace (e.g. under `~/.wick-pig/agy/`)? If not, does `.agents/agents/<name>/` in the worktree work?
2. With `excludeDefaultComponents: true`, are agy's built-in tools and default prompt gone? Are `~/.gemini/GEMINI.md`, `AGENTS.md` and rules still injected, and can they be turned off?
3. Can the agent definition give it only one MCP server (wick-mcp with env)? How is the system prompt set?
4. Where does the per-tool `background_mode` setting for MCP tools go, and does `off` keep a long call in the foreground?
5. Which hooks exist (`PreInvocation`, `PostInvocation`, `PreToolUse`, `PostToolUse`), what do they receive on stdin, and can any of them block the agent from stopping or inject a message?
6. Where is the conversation transcript written, in what format, and does it contain prompts, tool calls, results and per-request usage?
7. What does `stream-json` output contain for usage?
8. Does `--effort` change behaviour for effort-suffixed Gemini ids (`gemini-3.8-flash-high`)?
9. How does text typed into an interactive agy pane during a turn behave in default queue mode?

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Every question answered with evidence (command, output) in this ticket
- [ ] Fixtures saved: `agy models` output, a transcript sample, a stream-json result, hook stdin payloads
- [ ] Each answer that changes the harnesses spec's fixed fallbacks noted explicitly
