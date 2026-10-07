# 01: Claude keeps MCP calls in the foreground

**What to build:** A claude session never moves a running emberloop MCP call to the background, neither after 120 seconds nor when a notice is typed into its pane mid-call. The claude harness sets `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1` and `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0`.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Both variables in the claude command environment (stripped-claude test)
- [x] Opt-in live test: a claude session calls a emberloop tool that runs longer than 120 s; the result arrives in the same turn, not as a background task
- [x] Opt-in live test: a notice delivered while a tool call runs does not detach the call; the notice is seen after the call returns
- [x] If either live test shows backgrounding still happens, record what Claude Code version and setting is involved in this ticket
