# AGENTS.md — emberloop-web/el/statedb

## Purpose

Emberloop's SQLite state database: one file (`emberloop.db`) in the emberloop state
directory holding usage rows, the search cache and quota the websearch tool
fills, and the overflow store. Files:
`statedb.go` (paths, WAL/busy-timeout opens, versioned migrations),
`search.go` (search cache reads and writes, per-call quota rows, latest
reported quota), `usage.go` (usage rows, idempotent inserts,
one-time JSONL import, per-agent delete), `ctx.go` (overflow store:
`ctx_blobs` plus the FTS5 porter chunk index, heading-aware chunking, BM25
search, 24-hour purge, per-session delete), `flock.go` (migration file lock).

## Ownership

Owns this directory. It is a leaf: it imports nothing from the harness root
or the tools, so both can import it without a cycle.

## Local Contracts

- Pure-Go driver (modernc.org/sqlite, no cgo), WAL mode and a 5s busy timeout
  on every open, so readers never block writers.
- Schema versions migrate forward with each version recorded; concurrent
  opens serialize tune, migrate, import and purge on a file lock, so setup
  runs exactly once and never collides on the write lock.
- Usage inserts are idempotent by dedupe key (unique index, `INSERT OR
  IGNORE`); a re-tailed line or a re-run import never double counts.
- `Open` migrates, imports legacy JSONL once (only while the usage table
  is empty; old files stay until prune), and purges overflow rows older
  than 24 hours; `OpenReadOnly` neither migrates nor writes, for the TUI's
  polls. `OpenReadOnly` refuses a missing file instead of creating one.
- FTS5 with the porter tokenizer is proved by test and carries the overflow
  chunk index (headings weigh more in BM25 ranking).

## Work Guidance

- Behavioural change needs a test through `Open`/`OpenReadOnly` on a temp
  dir, never the real state dir; expected values come from the spec.
- New tables document their shape in the migration comment for the ticket
  that fills them.

## Verification

`.emberloop/check`.

## Child DOX Index

No child docs.
