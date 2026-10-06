# AGENTS.md — wick-pig/tools

## Purpose

The agent tools both hosts serve: pig through the extension adapter, claude
through the MCP server. Files: `tool.go` (the `Tool` interface: name,
schema, execute over a workspace root), `all.go` (the `All()` registry),
`files.go` (read, edit, write), `readurl.go` (the tiered URL reader `read`
serves: native fetch, then Jina, then an uninstalled browser tier),
`bash.go`, `background.go`, `find.go`
(plus the `cascade.go` ranker), `hashline.go` (tag-anchored edits),
`listing.go`, `guard.go`,
`credentials.go`, `settings.go`, `usage.go`, and `systemone/` (the
decisions-model client the find cascade asks).

## Ownership

Owns this directory and `systemone/`. The interface (`tool.go`) is owned
here; the transports that serve it (`mcp/`, `cmd/`) live with their docs.

## Local Contracts

- A tool imports nothing from the harness root (`example.com/wick` would
  cycle); the one shared dependency is the `statedb` leaf, which owns the
  state database the tools and the harness both write, so a plain `go
  build` of `cmd/wick-mcp` keeps working.
- `edit` answers with the changed region plus context and flags a patch
  whose deletions greatly exceed its additions; tag semantics do not change.
- `read` refuses loopback, private and link-local URL targets on every
  redirect hop, caps the body near 10 MB at 20 s, and never sends
  credentialed or token-like URLs to Jina.

## Work Guidance

- New tools implement `Tool` and register beside the existing ones; wire
  both hosts (extension adapter and MCP path) or say why one is excluded.

## Verification

`.wick/check`.

## Child DOX Index

No child docs.
