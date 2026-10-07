# 14: Bubblewrap sandbox runner for read-only roles

**What to build:** A runner that starts a command under bubblewrap for the `review` and `scout` roles: root filesystem bind-mounted read-only, the worktree read-only, `--unshare-net`, a PID namespace, `--die-with-parent`, a private tmpfs `/tmp`, an extra read-write bind only for a caller-supplied bridge directory (where ticket 16 puts its socket and generated module), a minimal environment, and process-group kill on timeout. The runner resolves `bwrap` from the emberloop bin directory then `PATH`. A one-time self-test (cached per process) runs a script that tries to write a file inside and outside the worktree and to open a network socket; the runner reports the sandbox healthy only when all three fail as expected. When `bwrap` is missing or the self-test fails, the runner returns a typed error whose message names `emberloop deps`; it never falls back to running unsandboxed. The runner is a library with a small test helper; it does not know about Python.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A sandboxed command cannot write inside the worktree, outside it, or to the home directory
- [x] A sandboxed command cannot open a network connection
- [x] A command can use the bridge directory (read-write) and read the worktree and system files
- [x] Timeout kills the whole process group, including children
- [x] Missing `bwrap` and a forced self-test failure both return the typed error naming `emberloop deps`
- [x] Tests skip with a clear message when `bwrap` is absent; repo check passes
