# 11: Minimum interval on manual refresh

**What to build:** The per-card refresh button and the refresh-all button always fetch really fresh data — the manual path still skips the age guard — but repeated clicks can never hammer an upstream or endanger the account. Each card shows a busy state during refresh and keeps its values.

**Blocked by:** 05-concurrent-fetch-isolation (both change the refresh path)

**Status:** done

- [x] Manual refresh skips the age guard but enforces a minimum interval per provider (default 10s): rapid repeat clicks cause no further upstream calls (HTTP-seam test asserting stub call counts)
- [x] Per-key and refresh-all refreshes each report a busy state and keep showing last values until the fetch lands
- [x] Go tests at the refresh HTTP seam with temp state dir and local stubs; repo check passes
