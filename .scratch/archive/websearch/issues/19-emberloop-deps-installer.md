# 19: `el deps`: install and verify every external dependency

**What to build:** A `el deps` command, also run at the end of `el install`, per the "Dependency installer" section of the spec. It checks `python3` (3.9 or newer), `rg`, `git`, `bwrap` and, unless `graph.enabled` is false, the code graph binary (codebase-memory-mcp). Missing system packages are installed with the detected package manager (`pacman`, `apt`, `dnf`, `zypper`, `brew`) from one package-name table: the exact command is printed and run through `sudo` only when attached to a terminal; otherwise it is printed and the command exits non-zero. The graph binary is downloaded from its GitHub release for the platform, verified against the release's published SHA-256, and placed in `~/.el/bin` (`EL_BIN_DIR` overrides); its own `install` command is never run and no agent client config is touched. El resolves these binaries from the el bin directory first, then `PATH` (a shared helper used by `grep`, the sandbox runner, `execute` and the graph). Each dependency is proved after install (python runs, `rg --version`, `git --version`, a bubblewrap self-test, the graph binary answers an MCP `initialize`). Output is one line per dependency with version and status; exit non-zero if a required one fails; the graph binary only warns. Idempotent: a second run changes nothing and says so. `el install`'s refusal while agents are live is unchanged.

**Blocked by:** None (can start immediately). If ticket 14's sandbox self-test exists, reuse it; otherwise implement the equivalent check here in one function that ticket 14 adopts.

**Status:** done

- [x] Detects each missing dependency and prints the right install command per package manager (table-driven test)
- [x] Without a terminal it prints the command and exits non-zero instead of calling sudo
- [x] Graph binary download is checksum-verified; a wrong checksum is refused; the binary lands in the bin directory with the execute bit
- [x] Each dependency gets a proof line with its version; a failing required dependency fails the command; the graph binary only warns
- [x] A second run changes nothing; `el install` runs the same step
- [x] The shared binary resolver (bin directory, then `PATH`) is used by the other tools
- [x] Fake package manager and fake release server cover the tests; no real installs in the standard check; repo check passes
