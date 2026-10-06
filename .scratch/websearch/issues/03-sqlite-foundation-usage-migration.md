# 03: SQLite database for usage, with search cache and quota tables

**What to build:** One SQLite database in the wick state directory (honouring the existing state-directory override), owned and migrated by wick-pig with a schema version table, pure-Go driver, WAL mode and a busy timeout. Usage records (request, find, cost) are written to it instead of per-agent JSONL, the usage tailer inserts idempotently, and existing JSONL history is imported once on first run. `prune` deletes rows as well as files. The schema also carries a search cache table and a search quota table, unused until the websearch ticket. The TUI can open the database read-only while others write, and the `wick-tui` module's read model is switched from tailing JSONL usage files to reading the database (incrementally by row id), with its tests adapted.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Database created on first use with a schema version; upgrades are tested
- [ ] New usage records land in the database, not in JSONL
- [ ] One-time import of existing JSONL logs, safe to re-run without double counting
- [ ] Tailer restart does not double count
- [ ] `prune` removes a run's usage rows
- [ ] Concurrent writers plus a read-only reader work without lock errors
- [ ] Search cache and search quota tables exist with a documented shape
- [ ] The chosen pure-Go driver supports FTS5 (a test creates an FTS5 virtual table with the porter tokenizer, so a driver without it fails loudly); ticket 09 builds on this
- [ ] `wick-tui`'s usage reading uses the database read-only; its tests pass against a fixture database
- [ ] Workers, runs and the archive stay in their JSON files; repo check passes
