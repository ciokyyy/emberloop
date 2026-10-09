# 06: Subagents through the spawn tool

**What to build:** Agents call a `spawn` MCP tool that creates Subagent Sessions with a parent and a shared Group. The session tree shows Subagents nested under their parent; each opens as its own transcript. When the last member of a Group finishes, the parent receives one grouped result as a Steer. Scouts are read-only Subagents. This ticket also decides whether `spawn` is served by the existing el MCP server calling the Daemon or by a Daemon-hosted server.

**Blocked by:** 05

**Status:** done

- [x] A `spawn` call creates one Subagent Session per requested agent, each recording its parent and Group
- [x] The session tree nests Subagents under their parent and each opens its own transcript
- [x] The parent receives exactly one grouped result when the last Group member finishes
- [x] A scout Subagent cannot write
- [x] Seam tests with the fake Backend cover spawn, nesting, grouped delivery and parent notification
