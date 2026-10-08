# 03: Pig Backend through the Go SDK

**What to build:** Choosing harness `pig` in the new-Session form starts a real pig session through the embedded SDK, stripped to MCP-only tools, with the exact SDK version pinned. Message chunks, tool calls and plans stream into the transcript as normalized Events. Model choices come from pig's model listing.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A pig Session answers a prompt and streams Events in the normalized schema
- [ ] The Session offers no native tools, only the emberloop MCP tools
- [ ] Model choices for pig match the pig model listing
- [ ] Ending the Session closes the SDK session and runtime without leaks
- [ ] A live probe (not in the main check) proves the above with a real pig
