# 04: Claude Backend through ACP

**What to build:** Choosing harness `claude` starts a stripped Claude Session through the ACP adapter `@agentclientprotocol/claude-agent-acp` (pinned; the ticket-01 spike ruled out any fallback), with MCP connected before the first prompt is delivered. Message chunks, tool calls, plans and diffs stream as normalized Events; permission requests are auto-approved and logged.

**Blocked by:** 02

**Status:** done

- [x] A claude Session answers a prompt and streams Events in the normalized schema
- [x] The Session offers no native tools, only the el MCP tools
- [x] The first prompt is delivered only after MCP tools are ready
- [x] Permission requests are auto-approved and appear as logged Events
- [x] Every `session/load` repeats the full `_meta.claudeCode.options` stripping set (the spike showed a load without them re-asks permission)
- [x] Claude models offered are opus, sonnet and haiku
- [x] A live probe (not in the main check) proves the above with a real claude
