# AGENTS.md — emberloop-web/memory

## Purpose

The memory engine package `memory`: one SQLite bank per User (observations, Rules, queue, Profile versions, ledger) with FTS5 recall, append-only Rules, human purge and `VACUUM INTO` snapshots. In process, Go only, `modernc.org/sqlite`, no cgo. Design: `.scratch/web-v1/spec.md` (Data model, Retrieval and injection, Nightly job) and `docs/adr/0003-in-process-memory-engine-with-grounded-rules.md`.

## Ownership

Owns `emberloop-web/memory/`. The Daemon reaches it only through the `Memory` interface. `NewFake` is the test double for Daemon API tests. Nothing else opens bank files.

## Local Contracts

- Files: `Open(root)` returns an `Engine`; `Bank(userID)` opens `root/users/<userID>.db` on first use. Snapshots go to `root/snapshots/<userID>/<UTC stamp>.db`. A user ID must match `^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`, else `ErrInvalid`.
- Writes: each bank has one writer goroutine and one writer connection. Every write goes through `Bank.write`. Reads use a `query_only` pool. Pragmas: WAL, `busy_timeout(5000)`, `synchronous(NORMAL)`. Writer transactions begin `IMMEDIATE` (`_txlock=immediate`).
- Schema: `migrations` in `schema.go` is append-only; `PRAGMA user_version` counts them. Never edit a shipped migration; add one.
- IDs: every table uses `id INTEGER PRIMARY KEY AUTOINCREMENT`. The FTS rowid is that `id`. VACUUM renumbers rowids of tables without it, which would desync FTS.
- Never delete: `BEFORE DELETE` triggers abort every DELETE with `use purge`. `rules_guard` aborts an UPDATE that changes anything but `status`. An edit is a new version: `ReviseRule` marks the old version `superseded` in the same transaction.
- Purge is the one delete path. In one transaction it drops the abort triggers, deletes the selection, clears the purged text from `queue.payload`, `ledger.detail` and `profile_versions` (JSON fields are matched on decoded strings and keys, so the document stays valid), rebuilds both FTS indexes and recreates the triggers. Then it runs `VACUUM`, `wal_checkpoint(TRUNCATE)` and the same purge on each snapshot file, with `secure_delete` on. A failed step after the commit does not skip the rest; the errors join.
- Text: `split_text` is `SplitText(text)`: camelCase words expand to their parts, so `TestUploadHandler` also matches `upload handler`. `Query(text)` quotes each word (letters, digits and combining marks), adds camelCase parts, dedupes, caps input at 512 runes, each term at 40 and the count at 24, and joins with `OR`. No term gives an empty query, not an error. Punctuation and operators never reach FTS5.
- Recall: live rows only, through an `EXISTS` filter (status not `superseded` or `dormant`), at most 20 (`MaxRecall`). `parked` Rules are live here; injection decides whether to use them.
- Snapshots: `Snapshot` runs `VACUUM INTO` unless the newest snapshot is under 6 hours old (`SnapshotInterval`). It keeps the newest 10 (`SnapshotKeep`). File names carry the time, so order comes from names.
- Fake: `fake.go` keeps the same rules (validation, live filter, purge scrub, snapshot policy) and matches by term count, not FTS5 rank. Every `Memory` method needs a case in `contract_test.go`, which runs against the bank and the fake.
- Notice: `schema.go` adapts mnemopi (MIT). `NOTICE.md` holds its notice. Keep both when the schema changes.

## Verification

In `emberloop-web/`: `go vet ./memory/ && go test ./memory/`. The repo check runs `go test ./...` there. Trigger, FTS integrity (`integrity-check`), purge file-bytes and concurrency tests live in `bank_test.go`, `purge_test.go` and `snapshot_test.go`.
