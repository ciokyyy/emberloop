# 04: Jump to an agent's herdr pane

**What to build:** Enter or double-click on an agent (in ticket detail, or the orchestrator on a spec row) focuses that agent's herdr tab and pane. The TUI keeps running in its own pane, untouched, so the user can come back to it. An agent whose pane is gone shows a short message instead.

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] Uses `herdr tab focus` and pane focus through an `Exec` seam
- [ ] Never closes, moves or splits panes
- [ ] Test: Enter on an agent sends the right focus calls to a fake `Exec`; a dead pane shows the message
