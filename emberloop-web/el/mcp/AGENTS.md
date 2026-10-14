# AGENTS.md — emberloop-web/el/mcp

## Purpose

The MCP transport: serves `tools.Tool` values over stdio JSON-RPC so any
MCP-speaking harness gets the same tools. Files are `server.go` (protocol,
role gate) and its test; the binary entry point is
`emberloop-web/cmd/el-mcp/main.go`.

## Ownership

Owns this directory. Tool behaviour itself is owned by
`emberloop-web/el/tools/`.

## Local Contracts

- Protocol handling is the transport only; capability semantics stay in the
  tools package.

## Work Guidance

- Test through `Serve` with in-memory pipes and a fake tool, never a live
  harness.

## Verification

`.emberloop/check`.

## Child DOX Index

No child docs.
