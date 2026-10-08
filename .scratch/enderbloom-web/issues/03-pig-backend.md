# 03: Pig Backend through the Go SDK

**What to build:** Choosing harness `pig` in the new-Session form starts a real pig session through the embedded SDK, stripped to MCP-only tools, with the exact SDK version pinned. Message chunks and tool calls stream into the transcript as normalized Events; plans are not streamed (the ticket-01 spike found no plan event type in the pig SDK). Model choices come from pig's model listing. The spike proved steer admission and delivery as a continuation turn, but tool execution and follow-up-after-turn delivery are unproven (the spike provider stalls tool-call turns), so the live probe must prove those with a healthier provider or model.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A pig Session answers a prompt and streams Events in the normalized schema
- [ ] The Session offers no native tools, only the emberloop MCP tools
- [ ] Model choices for pig match the pig model listing
- [ ] Ending the Session closes the SDK session and runtime without leaks
- [ ] A live probe (not in the main check) proves the above with a real pig
