# 03: SQLite database for usage, with search cache and quota tables

**What to build:** One SQLite database in the el state directory (honouring the existing state-directory override), owned and migrated by el with a schema version table, pure-Go driver, WAL mode and a busy timeout. Usage records (request, find, cost) are written to it instead of per-agent JSONL, the usage tailer inserts idempotently, and existing JSONL history is imported once on first run. `prune` deletes rows as well as files. The schema also carries a search cache table and a search quota table, unused until the websearch ticket. The TUI can open the database read-only while others write, and the `el-tui` module's read model is switched from tailing JSONL usage files to reading the database (incrementally by row id), with its tests adapted.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Database created on first use with a schema version; upgrades are tested
- [x] New usage records land in the database, not in JSONL
- [x] One-time import of existing JSONL logs, safe to re-run without double counting
- [x] Tailer restart does not double count
- [x] `prune` removes a run's usage rows
- [x] Concurrent writers plus a read-only reader work without lock errors
- [x] Search cache and search quota tables exist with a documented shape
- [x] The chosen pure-Go driver supports FTS5 (a test creates an FTS5 virtual table with the porter tokenizer, so a driver without it fails loudly); ticket 09 builds on this
- [x] `el-tui`'s usage reading uses the database read-only; its tests pass against a fixture database
- [x] Workers, runs and the archive stay in their JSON files; repo check passes

**Prior attempt:** an earlier worker finished this ticket and it passed both reviewers at an older base, but the run was stopped before it merged. Its commit is preserved on branch `archive/websearch-03-sqlite` (one commit on top of the integration branch). Start by cherry-picking it onto your branch, run the repo check, and fix what fails or is stale against the current integration tip, instead of reimplementing. Reviews run fresh on the resulting tip as for any ticket.
