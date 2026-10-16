# 07: Type badge and flip in the Run preview

**What to build:** The Run preview and Run detail show each Ticket's effective type as a badge (`code` default, `design` distinct), and before a Ticket's Worker starts the User can flip it from the badge. A flipped Ticket shows that it differs from its file. After the Worker starts the badge is read-only. Re-read `web/src/lib/RunsView.svelte`, `RunsSidebar.svelte`, `runsStore` and their tests, and the `/p/<id>/runs` preview flow.

**Blocked by:** 06-run-type-override-api

**Status:** ready-for-agent

- [ ] Badge renders for every Ticket in preview and detail on desktop and phone layouts
- [ ] Clicking a pre-start badge calls the override API and updates the row; failure shows the Daemon's message
- [ ] A flipped Ticket is marked "overridden" with a way to clear
- [ ] Read-only once the Worker has started, with the reason in a title or note
- [ ] `RunsView.test.js` covers badge, flip, clear and read-only; `npm test` and `npm run build` pass in `emberloop-web/web/`; `web/README.md` maps the new tests
