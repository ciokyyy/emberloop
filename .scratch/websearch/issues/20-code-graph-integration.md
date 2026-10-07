# 20: Code graph (codebase-memory-mcp) with automatic per-worktree indexing

**What to build:** Per the "Code graph integration" section of the spec. Register codebase-memory-mcp as the `graph` MCP server through tickets 10 and 15, found in the emberloop bin directory, on `PATH`, or at `graph.binary`; `graph.enabled: false` or a missing binary exposes nothing and breaks nothing. Exposed tools: `search_graph`, `trace_path`, `get_code_snippet`, `get_file_outline`, `get_architecture`. Excluded from every agent: `detect_changes` and all index management tools. Placement is `main` for `review` and `scout`, `execute` for other roles. Lifecycle, driven by emberloop and invisible to agents: an asynchronous index of the worktree when emberloop creates it; an incremental re-index just before each reviewer or scout spawn; `delete_project` for the worktree's project in `finish` and on any path that drops a worktree (the project name is the path slug of the worktree). Indexing failures are logged and never block spawn or finish. The first graph query waits on the index status for a short bounded time instead of answering from a missing graph. `graph.cache_dir`, when set, is passed to the binary as `CBM_CACHE_DIR`. The binary's `install` command is never run. Tests use the fake MCP server from ticket 15; a live probe against the real binary goes with the other live probes, outside the standard check.

**Blocked by:** 10, 15, 17, 19

**Status:** done

- [x] The exposed tool set per role matches the spec; management tools and `detect_changes` are never exposed
- [x] A worktree is indexed asynchronously on creation, re-indexed before a reviewer or scout spawn, and its project deleted on `finish`
- [x] Indexing and deletion failures are logged and never block spawn or finish
- [x] A missing binary or `graph.enabled: false` exposes no graph tools and raises no error
- [x] Two worktrees of one repo map to different projects
- [x] Repo check passes
