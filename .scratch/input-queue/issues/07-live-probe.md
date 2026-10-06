# 07: Live probe proves the row, raw mode and colors with real agents

**What to build:** A probe script in the scripts area, outside the repo check, runs claude and pig under the wrapper and confirms the wick row, steer and queue delivery, the stale clear, raw mode with its dirty flag, the OSC pane colors and the claude and pig themes. It also covers a startup dialog that swallows typed text. It extends the header-strip spike script and records its findings in this ticket. It uses isolated terminals and never touches running agents.

**Blocked by:** 02-wick-input-row, 04-raw-mode, 06-shared-theme

**Status:** ready-for-agent

- [ ] Probe runs claude and pig and prints pass or fail per behavior
- [ ] Startup dialog case is covered and recorded
- [ ] The script never submits a prompt to a model unless a flag says so
- [ ] Findings are recorded in this ticket
- [ ] Spec Testing Decisions, live probe paragraph

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
