# 08: Sidebar Active/All control and time groups

**Type:** code

**What to build:** In the sidebar data module: an Active/All mode (Active default). Active keeps running, idle and needs-you items and the User's own Sessions unless Everyone is picked; Project sections stay. All groups by Today, Yesterday, Previous 7 days and older. Finished and old Runs appear only in All. The mode persists per device. Logic and tests only; look comes in the design Ticket.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A mixed set of Runs and Sessions produces the Active rows and the All groups the spec lists
- [ ] Search works in both modes and finds finished items in All
- [ ] Subagent and Run Worker nesting is preserved
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
