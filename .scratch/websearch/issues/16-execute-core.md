# 16: `execute` tool: Python with native tools as functions, sandboxed for read-only roles

**What to build:** The `execute` tool per the "Execute tool" section of the spec. Arguments `code`, `timeout` (default 120, max 600), `intent`. Runs system `python3 -I` (stdlib only) as a fresh process per call. Wick creates a private temp directory holding a unix socket and a generated `tools` module; `tools.<name>(**kwargs)` sends a JSON line over the socket, wick runs the real tool through the role's resolved toolset (ticket 10) and existing `Guard` wrappers, and returns the string or raises `ToolError`. The script can call exactly the tools the role has after settings, excluding `execute` itself and orchestration tools; tools whose placement is `execute` (grep, git, check by default) live only here. `tools.search(query)` and `tools.help(name)` return names with signatures. The description lists the tools behind `execute` by name and carries a short `tools.grep` example. Output is stdout then stderr through the bash sanitiser, with a traceback tail and `[exit status N]` on failure, passed through the overflow helper (ticket 09); with `intent`, over-threshold output also returns the matching chunks inline. For `work`, `plan` and `full` the Python runs directly; for `review` and `scout` it runs inside the ticket 14 sandbox, with the bridge directory as the only writable bind, and refuses with the sandbox's error (naming `wick-pig deps`) when the sandbox is unavailable. Roles offered `execute`: `work`, `plan`, `full`, `review`, `scout`; not `implement`. On by default; disabled via settings. A missing `python3` gives an error naming `wick-pig deps`. The reviewer and scout toolsets in `roles.go` gain `execute` (still read-only through the sandbox).

**Blocked by:** 09, 10, 11, 12, 13, 14

**Status:** done

- [x] A script calls `tools.read`, `tools.grep`, `tools.git`, `tools.find`, and for allowed roles `tools.check` and `tools.bash`
- [x] A refused or failing tool call raises a catchable `ToolError`; an uncaught exception returns the traceback tail
- [x] A disabled tool is not callable; renamed tools appear under the new name; `Guard` limits still apply for guarded roles
- [x] Timeout kills the process group; the temp directory and socket are removed after every call
- [x] Over-threshold output is stored and summarised; `intent` returns matching chunks inline
- [x] For `review` and `scout`, the script cannot write files or reach the network, yet `tools.check` can still write its own caches; with the sandbox broken, `execute` refuses and names `wick-pig deps`
- [x] `tools.search` and `tools.help` work; the description lists the tools behind `execute`
- [x] `implement` does not get `execute`; real-`python3` tests skip clearly when absent; repo check passes
