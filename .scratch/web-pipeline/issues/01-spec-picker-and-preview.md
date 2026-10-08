# 01: Runs view with spec picker and Ticket preview

**What to build:** A Runs view beside chat and settings on the existing view switch. The user enters a repo directory (default: the working directory of the selected Session). The Daemon lists the specs it finds there, to a small fixed depth. The user picks one and sees a Ticket preview: number, title, blockers, checkbox progress. An unparsable Blocked by line shows as a clear error and blocks nothing else in the list. The Daemon reuses core spec resolution and ticket reading; if core hides a piece, export the minimum. Plain markup, no design work. See the spec sections on discovery and preview.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The Daemon API lists specs under a repo directory and returns title and Ticket count for each
- [ ] The Daemon API returns the Ticket preview for one spec, with blockers and checkbox progress
- [ ] A directory with no spec returns an empty list, and the view shows a clear message
- [ ] An unparsable Blocked by line surfaces as an error on that Ticket
- [ ] The Runs view is reachable from the sidebar and lets the user pick a repo, a spec, and see the preview
- [ ] The API is tested with a fake Backend, and the view is tested against a mocked API
- [ ] The Daemon imports nothing from herdr
