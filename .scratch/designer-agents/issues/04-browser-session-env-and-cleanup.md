# 04: Per-Session browser environment and cleanup

**What to build:** Every harness Session gets an isolated, localhost-only browser setup through its environment, and its browser dies with it. The Daemon adds `AGENT_BROWSER_SESSION` (Session ID), `AGENT_BROWSER_NAMESPACE` (per Daemon), a per-Session `AGENT_BROWSER_SCREENSHOT_DIR`, `AGENT_BROWSER_ALLOWED_DOMAINS=localhost,127.0.0.1` (admin setting `browser.allowed_domains` widens it), `AGENT_BROWSER_MAX_OUTPUT`, `AGENT_BROWSER_CONTENT_BOUNDARIES`, `AGENT_BROWSER_IDLE_TIMEOUT_MS` of about five minutes, `AGENT_BROWSER_EXECUTABLE_PATH` when a Chromium or Chrome is found, and the el bin dir on `PATH`, to the environment of the el server each Backend launches. `bash` and `execute` inherit it. It applies to the claude Backend (`mcpServersFor`) and to the pig Backend on both the ACP (`mergedMCP`) and SDK paths. On Session end or worktree removal the Daemon runs `agent-browser close` for the Session's name; Daemon shutdown runs `close --all` in its namespace; screenshot directories fall under the disk janitor. Only when `browser.enabled` and the binary exists. Re-read `claude.go`, `pigacp.go`, `pig.go`, `disk.go`, `endMergedWorker` in the Run driver and `Shutdown`.

**Blocked by:** 01-remove-grok, 03-browser-cli-install

**Status:** done

- [x] Wire tests on each Backend's el server entry show the full environment, with the Session ID in the session and screenshot-dir values, and no variables when `browser.enabled` is false or the binary is absent
- [x] A test runs the `bash` tool and an `execute` script against a stub `agent-browser` and sees the variables and the bin dir on `PATH`
- [x] Session end, merged-Worker end and worktree removal each invoke `close` for that Session's name (stub binary records calls); Daemon shutdown invokes `close --all` once per namespace
- [x] `browser.allowed_domains` is an admin-only host-global setting served through `/api/settings` and validated as a domain list
- [x] Per-Session screenshot directories are cleaned by the disk janitor with the existing cap logic
- [x] `emberloop-web/AGENTS.md` records the environment and cleanup contract; `.emberloop/check` passes
