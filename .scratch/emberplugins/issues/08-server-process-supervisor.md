# 08: Server process supervisor

**What to build:** An Emberplugin may declare a server process (any language; agents use Go). The Daemon starts it on the first request, gives it a loopback port, the token, the Daemon URL and the data dir through env vars, and proxies `/plugins/<name>/api/*` to it. Output goes to a log file shown in the UI. A crash restarts with backoff and three strikes disable the plugin. An idle server stops after 10 minutes unless the manifest sets `keepAlive`. The plugin list shows RAM per process.

**Blocked by:** 03 (Token, permissions and LLM call)

**Status:** ready-for-agent

- [ ] Start on first request, stop after the idle timeout, `keepAlive` opt-in
- [ ] Env-var contract (port, token, Daemon URL, data dir) documented in the code and tested
- [ ] The proxy forwards `/plugins/<name>/api/*` and refuses when the plugin is disabled or unapproved
- [ ] Stdout and stderr go to a per-plugin log file, readable through the API and the UI
- [ ] Crash restart with backoff; three strikes disable the plugin and show why
- [ ] Live process count is capped by `max_plugins`; over-cap start is refused with a clear error
- [ ] The plugin list shows RAM per process
- [ ] The Daemon's in-place restart treats a running plugin process like a live agent process (idle check, clean close)
- [ ] Tests use a fake server binary fixture and a fake clock for idle and backoff
