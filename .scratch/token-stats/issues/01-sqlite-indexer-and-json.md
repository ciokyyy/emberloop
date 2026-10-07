# 01: SQLite indexer for requests, rebuild, stats --json

**What to build:** The TUI module indexes the archive's usage logs into `~/.emberloop/stats.db` (pure-Go SQLite), one row per request and per find call, with agent, run, ticket, role, harness, model, effort. Indexing is incremental with per-file offsets; `emberloop stats rebuild` recreates the database from the archive; `emberloop stats --json` reports totals by model, model plus effort, harness and role at global, repo, spec, ticket or agent scope.

**Blocked by:** emberloop-tui 01, run-records 03, run-records 05

**Status:** ready-for-agent

- [ ] Schema versioned; a version mismatch triggers rebuild
- [ ] Rebuild test: cold and incremental indexing of the same fixture archive give identical query results
- [ ] `stats --json` scopes and groupings tested against fixtures
