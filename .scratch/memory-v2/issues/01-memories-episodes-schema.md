# 01: Memories and Episodes schema

**What to build:** Add append-only migrations for `memories` and `episodes` as the spec Data model defines them: FTS5 external-content tables with insert, delete and update triggers and an identifier-split copy, `BEFORE DELETE` abort triggers, and an update guard on `memories` that allows only `status`, `superseded_by`, `pinned`, `importance`, `mention_count`, `last_seen_at`, `embedding`, `embedding_model`. Extend the `Memory` interface with Memory and Episode methods (insert, get, list with kind/status filter, set status, pin, bump, set embedding). Extend purge and snapshots to cover both tables and their FTS.

**Blocked by:** none

**Status:** todo

- [ ] Migrations apply on an existing bank; no shipped migration is edited
- [ ] An UPDATE of text, quote, author or provenance is refused by the guard
- [ ] A DELETE is refused outside purge; purge scrubs both tables, their FTS and snapshots
- [ ] `forgotten` Memories are excluded from every list used by recall, injection and dedup
- [ ] The fake and the bank both satisfy `contract_test.go`
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
