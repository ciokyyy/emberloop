# 15: Memory engine: schema, triggers, FTS, banks, purge, snapshots

**What to build:** New in-process Go package for the engine, `modernc.org/sqlite` (no cgo). Per-User bank files keyed by user ID; tables `observations`, `rules`, `queue`, `profile_versions`, `ledger`; external-content FTS5 with insert, delete and update triggers and an identifier-split text copy; `BEFORE DELETE` abort triggers and the rules update guard; one writer goroutine per bank; WAL, `busy_timeout`, `PRAGMA user_version` migrations; `VACUUM INTO` snapshots; human purge. The query builder quotes every token, caps length and joins with `OR`. A Go interface `Memory` sits in front, with a fake for API tests. Borrowed from mnemopi (MIT); keep the notice. See the spec, sections Data model and Nightly job.

**Blocked by:** 02

**Status:** done

- [x] Any `DELETE` outside purge fails with `use purge`; a `rules` UPDATE that changes more than `status` fails
- [x] FTS stays in sync across insert, update and delete, including the update trigger mnemopi lacks
- [x] A query with quotes, dashes, colons, `AND`/`OR` and parentheses never errors and never injects syntax
- [x] An identifier such as `TestUploadHandler` also matches `upload handler`
- [x] Search returns only live rows (not superseded, not dormant) using the `EXISTS` filter
- [x] Purge removes chosen records, rebuilds FTS, runs `VACUUM` and clears the text from snapshots, queue and ledger
- [x] Snapshots use `VACUUM INTO`, keep 10, and skip when the last one is under 6 hours old
- [x] A contract test suite runs against the real engine and the fake `Memory`
- [x] Concurrent writers (nightly job, miner, tool call) serialize without `SQLITE_BUSY`
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
