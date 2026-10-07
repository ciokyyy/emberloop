# 15: Stdio MCP client and lazy server registry

**What to build:** A hand-written stdio JSON-RPC MCP client in Go, in the same style as the existing hand-written server (no third-party MCP library). Given an `mcp_servers` entry from settings (ticket 10) it starts the server on first use, performs `initialize` and `notifications/initialized`, lists tools (cached for the session), calls tools, and kills the child process group when emberloop-mcp exits. Initialize and call timeouts exist; a server that fails to start, never answers, exits mid-call or returns a protocol error produces a clear tool error naming the server and never a hang. A registry holds one client per configured server per emberloop-mcp process, applies the server's `include`, `exclude` and `rename` from the resolved settings, and exposes each tool as a `tools.Tool`, so it plugs into the same resolution as native tools. Servers are never shared across sessions. A small fake MCP server (test helper over stdio) is added for this and later tickets.

**Blocked by:** 10

**Status:** done

- [x] Start-on-first-use: a configured server is not started until one of its tools is called or listed
- [x] Initialize, list, call and cleanup on exit work against the fake server
- [x] A server that never answers, exits, or fails to start gives a bounded, named error
- [x] `include`, `exclude` and `rename` shape the exposed tool set
- [x] Children are killed when the emberloop-mcp process ends, including on error paths
- [x] The fake server helper is reusable by tickets 17 and 20; repo check passes
