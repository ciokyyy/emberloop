# 04: Route table and 404 page

**What to build:** A Go route table replaces `isSPARoute` in `ui.go`. It covers every `router.js` shape, including `/p/<id>`, `/p/none`, `/p/<id>/runs[/<runId>[/<ticketId>]]`, `/p/<id>/settings`, `/s/<id>`, `/settings/*`, `/quota`, `/providers`, `/memory`. Unknown paths return a real 404 page.

**Blocked by:** 03

**Status:** open

- [ ] Refresh on every route returns 200 with the shell
- [ ] A table test pins parity with `router.js` route shapes
- [ ] An unknown path returns the 404 page; missing assets and `/api/*` still 404
- [ ] `emberloop-web/AGENTS.md` and `web/README.md` Routes text updated
