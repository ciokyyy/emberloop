# 07: A el MCP server with no role refuses to start

**What to build:** Started with no role, a el MCP server exits with a message naming the roles; the full tool set is still available when asked for explicitly, by role flag or environment value. The live scripts and the debugging path are updated to ask for it. The pig and claude harnesses always set a role, so launches are unchanged. Nothing outside a harness can reach spawn, finish or tell by starting a server by hand.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] No role: non-zero exit and a message naming the roles
- [x] Explicit full role serves every tool
- [x] Every launch path and the live scripts still work (their tests pass)
- [x] Spec stories 7, 8, 9

Spec: `.scratch/archive/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
