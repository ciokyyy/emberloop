# 10: Runs and Ticket pages

**Type:** design

**What to build:** Runs preview, Run detail, Ticket page, graph. Status and next action lead. Type, blockers, warnings, owner become chips. Phone: Ticket page tabs (Status, Transcript, Facts); graph is a list by default with an opt-in full-screen graph that never traps page scroll; chain table collapses to a summary row that opens a sheet. Re-read `RunsView`, `RunsGraph`, `TicketPage`, `RunSwitchPicker`, `GitPanel`.

**Blocked by:** 01-design-foundation

**Status:** open

- [ ] Preview of a 10-Ticket spec fits in two screens at 390 px without paragraph text
- [ ] Graph never captures touch scroll on the page
- [ ] Ticket page shows the next action without scrolling on both sizes
- [ ] GitPanel and diff usable at 390 px (view-only is acceptable)
- [ ] Console and a11y clean; `npm test`, `npm run build`, `.emberloop/check` pass
