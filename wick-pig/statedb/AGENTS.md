# AGENTS.md — wick-pig/statedb

## Purpose

Wick's SQLite state database: one file (`wick.db`) in the wick state
directory holding usage rows today and the search cache, quota and overflow
stores follow-up tickets build on. Files: `statedb.go` (paths, WAL/busy-timeout
opens, versioned migrations), `usage.go` (usage rows, idempotent inserts,
one-time JSONL import, per-agent delete), `flock.go` (migration file lock).

## Ownership

Owns this directory. It is a leaf: it imports nothing from the harness root
or the tools, so both can import it without a cycle.

## Local Contracts

- Pure-Go driver (modernc.org/sqlite, no cgo), WAL mode and a 5s busy timeout
  on every open, so readers never block writers.
- Schema versions migrate forward with each version recorded; concurrent first
  opens serialize on a file lock, so migration runs exactly once.
- Usage inserts are idempotent by dedupe key (unique index, `INSERT OR
  IGNORE`); a re-tailed line or a re-run import never double counts.
- `Open` migrates and imports legacy JSONL once (only while the usage table
  is empty; old files stay until prune); `OpenReadOnly` neither migrates nor
  writes, for the TUI's polls. `OpenReadOnly` refuses a missing file instead
  of creating one.
- FTS5 with the porter tokenizer is proved by test, for the overflow store.

## Work Guidance

- Behavioural change needs a test through `Open`/`OpenReadOnly` on a temp
  dir, never the real state dir; expected values come from the spec.
- New tables document their shape in the migration comment for the ticket
  that fills them.

## Verification

`.wick/check`.

## Child DOX Index

No child docs.
