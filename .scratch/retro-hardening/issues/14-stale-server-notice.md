# 14: A wick process says when its binary was replaced

**What to build:** A wick MCP server records its own binary's identity at start and, on a call, adds a line to its reply when the installed binary has changed since. It does not restart itself, because a restart would drop state; the install gate prevents the situation and this makes a stale orchestrator visible when it happens anyway.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A server started before its binary changed adds the stale line to its next reply (test)
- [x] An unchanged server's replies are unchanged
- [x] Spec story 43

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
