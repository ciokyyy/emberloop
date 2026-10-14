# 13: Quota second sidebar with Live status list

**What to build:** Quota navigates like the other areas, as in the mock `.scratch/settings-redesign/mock/index.html`. The Quota rail icon (gauge, area label "Quota and Providers") opens the second sidebar titled "Quota and Providers" with the Quota entry (a Providers entry arrives with ticket 22) and a "Live status" list: one row per provider with a tone dot, the name and the highest window percent in the tone color (`—` with a neutral dot when the provider has no percent). It is fed by the same data as the page, so quotas are glanceable from anywhere. The page no longer collapses the second sidebar to `w-0` on desktop, and no Project switcher shows here (Quota is global).

**Blocked by:** 03-suite-time-target-and-check-timeout (Part 7 goes first; builds to the spec response contract with a mocked live channel)

**Status:** done

- [x] Second sidebar shows the "Quota and Providers" title, the Quota entry and the Live status list (tone dot, name, top percent per provider, driven only by `pct`)
- [x] The list reads its provider data from one shared client store with a replaceable source; this ticket feeds it from the existing fetch and a mocked channel, and ticket 15 connects it to the live topic
- [x] The sidebar stays open on desktop while Quota is the active area
- [x] Vitest covers the status list and the sidebar behaviour; repo check and `npm test` pass
