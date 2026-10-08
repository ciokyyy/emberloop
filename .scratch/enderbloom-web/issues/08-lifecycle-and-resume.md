# 08: Concurrency cap, idle release and resume

**What to build:** The Daemon caps running Sessions using the global `max_workers` setting, releases idle Sessions' agent processes, and resumes them from the log on the next prompt. After a Daemon restart, Sessions resume where the harness supports it (claude ACP session load, pig saved session) and otherwise show read-only history with a clear label.

**Blocked by:** 03, 04

**Status:** ready-for-agent

- [ ] Starting a Session over the cap waits or is refused with a clear message
- [ ] An idle Session's agent process is released and the next prompt resumes it
- [ ] After a Daemon restart, a resumable Session accepts a new prompt
- [ ] A non-resumable Session shows read-only history and says why
- [ ] Seam tests with the fake Backend cover the cap, release and resume paths
