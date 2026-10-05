# AGENTS.md — wick-pig/mcp

## Purpose

The MCP transport: serves `tools.Tool` values over stdio JSON-RPC so any
MCP-speaking harness gets the same tools. Files are `server.go` (protocol,
role gate) and its test; the binary entry point is
`wick-pig/cmd/wick-mcp/main.go`.

## Ownership

Owns this directory and the `wick-pig/cmd/wick-mcp` entry point. Tool
behaviour itself is owned by `wick-pig/tools/`.

## Local Contracts

- Protocol handling is the transport only; capability semantics stay in the
  tools package.

## Work Guidance

- Test through `Serve` with in-memory pipes and a fake tool, never a live
  harness.

## Verification

`.wick/check`.

## Child DOX Index

No child docs.
