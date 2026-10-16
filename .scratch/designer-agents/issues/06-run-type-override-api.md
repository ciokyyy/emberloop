# 06: Run-file type override

**What to build:** A User can override a Ticket's type for one Run before its Worker starts. The override is stored in the Run file per Ticket, survives a Daemon restart, wins over the Ticket file's `**Type:**` line in the preview and in routing, and never edits the Ticket file. An API route sets or clears it and refuses once the Ticket's Worker has started. The Run detail snapshot reports both the file type and the effective type. Re-read `runs_exec.go` (`savedRow`, `runState`), the Run routes in `runs.go` and the restart-recovery path (`runs_restart_resume.go`).

**Blocked by:** 02-ticket-type-parse

**Status:** ready-for-agent

- [ ] `PUT /api/runs/<id>/tickets/<ticket>/type` accepts `code`, `design` or an empty value to clear; other values are 400; owner and admin rules match the other Run actions
- [ ] The override persists in the Run file, round-trips through restart, and is cleared on empty
- [ ] Refused with 409 once the Ticket has a Worker, with a message naming that
- [ ] Run detail shows file type, override and effective type; summaries are unchanged
- [ ] Run driver tests: override wins over the file line; clear returns to the file line; restart keeps it
- [ ] `emberloop-web/AGENTS.md` states the override contract; `.emberloop/check` passes
