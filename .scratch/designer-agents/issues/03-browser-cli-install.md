# 03: `el deps` installs the browser CLI

**What to build:** `el deps` installs a pinned `agent-browser` release binary into the el bin dir as an optional dependency, the way it installs `codebase-memory-mcp`: download the platform asset from the vercel-labs GitHub release, verify its sha256 against the release digest, prove it by running `--version`, print one line with version and status, change nothing on a second run, and only warn (never fail) when it cannot install. A `browser.enabled` setting (default true) skips it. Pin the version the research measured (0.39.1) and expose the override seams the graph install has (release base, platform, bin dir, settings) for tests. Re-read `emberloop-web/el/deps.go` (`DepsConfig`, graph install) and `deps_test.go` first.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `el deps` downloads the right asset for linux amd64/arm64 and darwin arm64/amd64, verifies sha256, and writes an executable into the el bin dir; the binary resolves through the shared bin-dir-then-PATH resolver
- [ ] A second run changes nothing and says so; a bad checksum or failed download warns and leaves the exit status zero
- [ ] `browser.enabled: false` skips the install and the deps line says so
- [ ] Tests use a fake release server and fake binary; no test downloads for real
- [ ] `el/AGENTS.md` and the `el deps` header comment name the browser CLI; `.emberloop/check` passes
