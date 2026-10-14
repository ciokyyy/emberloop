# 15: Live patching of open Quota pages

**What to build:** An open Quota page, its second-sidebar Live status list and the phone drawer list move on their own: they share one subscription to the live `quota` topic. The page subscribes to the live `quota` topic on mount and patches its cards in place when values change, with no timers, no polling and no manual reload.

**Blocked by:** 10-live-topic-refresher, 12-quota-card-redesign, 13-quota-sidebar

**Status:** done

- [x] Page subscribes on mount, patches cards from topic payloads, and unsubscribes on unmount; no timers or polling remain
- [x] A window moving on the Daemon side appears on the open page without reload (covered at the HTTP-plus-live seam and in vitest with a mocked live channel)
- [x] The sidebar Live status list and the phone drawer list update from the same subscription without reload
- [x] Refresh buttons keep working alongside live updates; repo check passes
