# Plan: MCP Streamable-HTTP + OAuth client for emberloop

Goal: teach emberloop's MCP client what scintilla already consumes —
`didww` (HTTP + OAuth) and `livekit-docs` (HTTP, no auth) — while keeping
today's stdio path untouched. Claude builds it; human does the browser
approvals.

## Background (read first)

- Root: `AGENTS.md` (DOX contract + child index).
- Local contracts: `emberloop/AGENTS.md` (Exec seam, check), `emberloop/tools/AGENTS.md`
  (Tool interface, execute/resolution, mcpclient/settings ownership),
  `emberloop/mcp/AGENTS.md` (transport only, no capability semantics).
- Code: `emberloop/tools/mcpclient.go` (`Client`/`Registry`, stdio child,
  lazy start, cached `tools/list`, `Close` kills children), `emberloop/tools/settings.go`
  (`mcp_servers` map + merge), `emberloop/tools/mcpclients.go` (snapshot),
  `emberloop/tools/execute.go` (serves as `tools.<server>.<name>`),
  `emberloop/tools/toolresolve.go` + `header_tool.go` (`toolStyles`),
  `emberloop/tools/mcpfake/` (fake pattern to copy).
- Reference consumer: scintilla `.omp/mcp.json` (`didww`, `livekit-docs`,
  `telnyx` with `!cat` secret indirection) and `doc.didww.com/mcp/connection-details.html`
  (no API key, no client secret, browser approve, 30-day re-auth).

## Scope

1. Settings schema: extend the `mcp_servers` entry beside `command`/`args`/`env`/`cwd`
   with `url` / `headers` / `oauth` (callback port, client name, metadata override).
   Project file overrides user file per name. Secrets never committed.
2. HTTP transport in `tools/`: new client next to stdio `Client` —
   Streamable-HTTP initialize + `tools/list` + `tools/call`, session id handling,
   same timeouts as stdio (`DefaultMCPInitTimeout`, `DefaultMCPCallTimeout`).
3. OAuth: dynamic registration (DIDWW sends no id/secret), PKCE browser flow,
   per-user token store, one retry on 401, surfaced `reauth-needed` on revoke/expiry.
4. Registry: one entry per server name regardless of transport; lazy start;
   shared list cache; `Close` closes child or HTTP session.
5. Wiring: snapshot, execute names, placement, `toolStyles` entry per new tool.
6. Tests + docs.

## Pseudocode (structure only, translate to Go)

```
for each entry in mcp_servers:
    if entry has command -> keep stdio Client path as-is
    if entry has url -> open HTTP session:
        POST initialize, then tools/list, cache answer
        on call -> POST tools/call with session id
        on 401 naming an auth challenge AND oauth configured:
            run register -> browser approve -> store token
            retry original call exactly once
        on revoked / expired token -> return reauth-needed error, never fatal

Registry holds either kind behind one interface:
    list tools (cached) / call tool / close

execute serves union of stdio + HTTP tools:
    native and MCP alike minus itself and orchestration tools
    renames respected, Daemon proxies excluded by mark, Guards kept
```

## Build order

1. HTTP no-auth (`livekit-docs` URL proves the round trip).
2. OAuth (`didww`: User Panel creds + 2FA + approve; revoke under
   Account Settings -> MCP Clients; 30-day idle re-auth).
3. Meta-style stdio stays as-is (command + env secrets, user-level file).

## Acceptance

- `livekit-docs` lists + calls with no credentials configured.
- `didww` first call triggers browser approve; after approval calls succeed;
  revoked token yields actionable re-auth message, not a hang.
- Stdio servers (e.g. `chrome-devtools`) unchanged.
- New tools appear as `tools.<server>.<name>` in execute, with `toolStyles` row.
- Tests: `httptest` HTTP fake + OAuth-retry fake mirroring `mcpfake/`;
  `Serve` through in-memory pipes; no live network, no real home.

## Verification

- From `emberloop/`: `go build ./...`, `go vet ./...`, `go test -skip TestLive ./...`
- Full: `.emberloop/check` end to end. Never run live tests, never start `el`/`pig`/`claude` from inside the work.
- Behavioural change needs a test through the tool/command, not internals.

## DOX pass (same change)

- Update `emberloop/tools/AGENTS.md` (new transport + oauth ownership) and
  `emberloop/mcp/AGENTS.md` if the serving side changes at all.
- Refresh child indexes; delete stale text; report checks run.
