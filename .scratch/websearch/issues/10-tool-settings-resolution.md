# 10: Tool settings: enable, disable, rename, place, per role, plus MCP server entries

**What to build:** Settings (global `~/.emberloop/settings.json` and project `.emberloop/settings.json`, project winning field by field, maps merged by key, unknown keys preserved as today) gain a `tools` map and an `mcp_servers` map, per the "Tool settings" section of the spec. A tool entry has `enabled` (default true), `placement` (`main` or `execute`), `rename`, and `roles.<role>` overrides of `enabled` and `placement`. An MCP server entry has `command`, `args`, `env`, `placement`, `include` and `exclude` glob lists over its tool names, `rename` (original to new name), and `roles`. One resolution function takes a role and returns the final tool list: which tools are in the main list, which are behind `execute`, under which names. Both the MCP server's `tools/list` and the future `execute` stub generator must call it. Settings can only narrow a role's allowlist, never widen it past what `roles.go` defines. `submit` cannot be disabled or moved. Unknown tool names are logged once and ignored. The settings command (`emberloop settings`) can set and show these keys. The existing MCP server wiring uses the resolution so `enabled: false` and `rename` already take effect for main-list tools in this ticket; `placement: execute` hides a tool from the main list (the `execute` tool itself arrives in ticket 16).

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Global and project files merge as specified; unknown keys survive a settings write
- [x] Per-role overrides, rename, include and exclude globs resolve correctly (table tests)
- [x] A disabled tool is absent from the served tool list for that role (and globally when disabled globally)
- [x] Settings never widen a role's allowlist; `submit` is undisableable
- [x] Unknown tool names are reported once and ignored
- [x] `emberloop settings` can show and set the new keys
- [x] Repo check passes
