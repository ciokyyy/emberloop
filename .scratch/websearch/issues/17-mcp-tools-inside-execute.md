# 17: MCP tools inside `execute`

**What to build:** Connect the ticket 15 registry to ticket 16. MCP tools whose resolved placement is `execute` appear in the generated stub under their configured names and as `tools.<server>.<name>`; MCP tools placed in `main` appear in the served tool list. The `execute` description gains one line per MCP server (name, short summary, tool count) and nothing more; `tools.search(query)` and `tools.help(name)` cover MCP tools too, searching names and descriptions. Servers start lazily on the first call or help lookup for their tools. Per-role placement and `enabled` are honoured, and the `tools/list` output and the generated stub are compared in a test for the same settings so they cannot disagree.

**Blocked by:** 15, 16

**Status:** ready-for-agent

- [ ] A script calls an MCP tool from a fake server; a failing server call raises `ToolError` naming the server
- [ ] The description lists servers by one line each; no full schemas appear in the tool list
- [ ] `search` and `help` find MCP tools; the server starts only when needed
- [ ] `include`, `exclude`, `rename`, `enabled` and per-role placement all take effect inside `execute`
- [ ] MCP tools in the sandboxed read-only roles work through the socket (the server runs outside the sandbox)
- [ ] Repo check passes
