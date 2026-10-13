# 12: Quota second sidebar with Live status list

**What to build:** Quota navigates like the other areas. The Quota rail icon opens a second sidebar holding the Quota entry and a Live status list — one row per provider with a tone dot and percent — fed by the same data as the page, so quotas are glanceable from anywhere on the page. The page no longer collapses the second sidebar on desktop.

**Blocked by:** None (can start immediately; builds to the spec response contract with a mocked live channel)

**Status:** ready-for-agent

- [ ] Second sidebar lists the Quota entry and the Live status list (tone dot plus percent per provider, driven only by `pct`)
- [ ] The list reads its provider data from one shared client store with a replaceable source; this ticket feeds it from the existing fetch and a mocked channel, and ticket 14 connects it to the live topic
- [ ] The sidebar stays open on desktop while Quota is the active area
- [ ] Vitest covers the status list and the sidebar behaviour; repo check passes
