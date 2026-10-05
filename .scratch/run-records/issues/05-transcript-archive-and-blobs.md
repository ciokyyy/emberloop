# 05: Transcript archive and prompt/tool-definition blobs

**What to build:** Each agent's harness transcript is copied into wick's archive under its run, kept current while the agent runs and finalised when it settles, so it outlives harness cleanup. At session start, the system prompt, the tool definitions served for its role, and the harness flags are stored once by content hash in a blob store; the session's archive references the hashes.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Archive copy tracks a growing transcript and resumes from its offset after restart
- [ ] Same prompt or tool set across sessions → one blob, many references
- [ ] Archive layout documented in this ticket for the stats indexer
- [ ] Tests: fake transcript appended in steps; blob dedup
