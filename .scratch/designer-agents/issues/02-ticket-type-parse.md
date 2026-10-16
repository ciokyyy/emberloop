# 02: Ticket type line

**What to build:** A Ticket file may carry `**Type:** code` or `**Type:** design` beside its `**Blocked by:**` line. The Run preview and the Run snapshot report each Ticket's type; a missing line means `code`; any other value fails only that Ticket's preview row and, like a bad blocker, fails the Run's check-world pass before any Worker starts. No routing yet: this ticket only reads, validates and reports the type. Re-read `emberloop-web/el/ticketview.go` (`driverBlockedValue`, `PreviewTickets`), `el/driver.go` (`DriverTicket`, check-world pass) and `runs_exec.go` (`staticTicket`, Run detail snapshot).

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `**Type:**` parses with the same header reader as `**Blocked by:**`, case-insensitive on the value, tolerant of the bold markers like the blocker line
- [ ] `DriverTicket` and `staticTicket` carry the type; absent line yields `code`
- [ ] `GET /api/runs/<id>` and the preview payload include each Ticket's `type`; `GET /api/runs` summaries do not change
- [ ] An unknown value fails that preview row with a message naming the allowed values, and fails the driver's check-world pass
- [ ] Existing spec fixtures without a Type line behave exactly as before (no snapshot diff beyond the new field)
- [ ] `emberloop-web/el/AGENTS.md` and `emberloop-web/AGENTS.md` state the Type line contract; `.emberloop/check` passes
