# 04: Claude Backend through ACP

**What to build:** Choosing harness `claude` starts a stripped Claude Session through the ACP adapter (or the fallback chosen in ticket 01), with MCP connected before the first prompt is delivered. Message chunks, tool calls, plans and diffs stream as normalized Events; permission requests are auto-approved and logged.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A claude Session answers a prompt and streams Events in the normalized schema
- [ ] The Session offers no native tools, only the emberloop MCP tools
- [ ] The first prompt is delivered only after MCP tools are ready
- [ ] Permission requests are auto-approved and appear as logged Events
- [ ] Claude models offered are opus, sonnet and haiku
- [ ] A live probe (not in the main check) proves the above with a real claude
