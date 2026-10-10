# 06: Handoff compress on claude

**What to build:** Compress by Handoff restart on the claude Backend (ACP). The new ACP session is ready, with the MCP tools, before the Handoff prompt is sent.

**Blocked by:** 01

**Status:** ready-for-agent

- [x] A claude Session compresses by Handoff restart and reports compressible
- [x] The new ACP session has MCP ready before the Handoff prompt
- [x] Model, effort and the voice reminder hook survive the new session
- [x] The resume token rotates and a Daemon restart then resumes the new session
- [x] Tests at the Daemon seam cover the case against the existing claude test double
